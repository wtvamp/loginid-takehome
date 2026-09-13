package postgres

import (
	"context"
	"database/sql"
	"time"

	"loginid-takehome/internal/dao"
	"loginid-takehome/internal/model"
)

// CreateProfileWithCredential writes both rows in one transaction (§3b).
// c.UserID is ignored on input and set from the profile just created; on
// any failure neither row exists.
func (r *repository) CreateProfileWithCredential(ctx context.Context, p *model.UserProfile, c *model.UserCredential) (*model.UserProfile, *model.UserCredential, error) {
	if err := dao.ValidateCreateID(p.ID); err != nil {
		return nil, nil, err
	}
	if err := dao.ValidateProfilePointers(p); err != nil {
		return nil, nil, err
	}
	if err := dao.ValidateCreateID(c.ID); err != nil {
		return nil, nil, err
	}
	dao.PrepareCreateProfileWithCredential(c)
	if err := dao.ValidateCredentialPointers(c); err != nil {
		return nil, nil, err
	}

	var (
		createdProfile    *model.UserProfile
		createdCredential *model.UserCredential
	)
	err := r.withRetry(ctx, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
			INSERT INTO user_profile (name, phone, street_address, locality, region, postal_code, country, source)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING `+profileColumns,
			p.Name, p.Phone, p.StreetAddress, p.Locality, p.Region, p.PostalCode, p.Country, p.Source)
		gotProfile, err := scanProfile(row.Scan)
		if err != nil {
			return err
		}

		if err := validateMethod(ctx, tx, c.MethodID, c.SecretState); err != nil {
			return err
		}

		crow := tx.QueryRowContext(ctx, `
			INSERT INTO user_credential (user_id, username, method_id, secret, secret_state, hash_algo, hash_cost)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING `+credentialColumns,
			gotProfile.ID, c.Username, c.MethodID, c.Secret, c.SecretState, c.HashAlgo, c.HashCost)
		gotCredential, err := scanCredential(crow.Scan)
		if err != nil {
			return err
		}

		createdProfile = gotProfile
		createdCredential = gotCredential
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return createdProfile, createdCredential, nil
}

// clockColumnFor and the orphan class's extra predicate, per
// pii-governance.md: "direct" and "idp_cache" (referenced by a credential)
// key on updated_at; "idp_cache_orphan" (an idp_cache row nothing
// references) keys on created_at specifically so re-reads can't refresh
// its own expiry.
func clockColumnFor(class dao.RetentionClass) (column string, extraWhere string, ok bool) {
	switch class {
	case dao.RetentionDirect:
		return "updated_at", "source = 'direct'", true
	case dao.RetentionIDPCache:
		return "updated_at", "source = 'idp_cache' AND EXISTS (SELECT 1 FROM user_credential WHERE user_credential.user_id = user_profile.id)", true
	case dao.RetentionIDPCacheOrphan:
		return "created_at", "source = 'idp_cache' AND NOT EXISTS (SELECT 1 FROM user_credential WHERE user_credential.user_id = user_profile.id)", true
	default:
		return "", "", false
	}
}

// DeleteExpired batches one retention class per call (§3c). Bounded by
// maxRows, one transaction per batch — never one per sweep. Never call
// with a request-scoped context (enforced by the caller, per §3c).
func (r *repository) DeleteExpired(ctx context.Context, class dao.RetentionClass, olderThan time.Time, maxRows int) (dao.SweepResult, error) {
	if maxRows < 1 || maxRows > 10_000 {
		return dao.SweepResult{}, dao.ErrInvalidArgument
	}
	clockColumn, extraWhere, ok := clockColumnFor(class)
	if !ok {
		return dao.SweepResult{}, dao.ErrInvalidArgument
	}

	var result dao.SweepResult
	err := r.withRetry(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx,
			"SELECT id FROM user_profile WHERE "+extraWhere+" AND "+clockColumn+" < $1 LIMIT $2",
			olderThan, maxRows)
		if err != nil {
			return translateError(err)
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return translateError(err)
			}
			ids = append(ids, id)
		}
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			return translateError(err)
		}

		examined := len(ids)
		deleted := 0
		for _, id := range ids {
			res, err := tx.ExecContext(ctx, "DELETE FROM user_profile WHERE id = $1", id)
			if err != nil {
				return translateError(err)
			}
			n, err := res.RowsAffected()
			if err != nil {
				return translateError(err)
			}
			if n == 0 {
				continue
			}
			deleted++
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO deletion_log (profile_id, source, reason, external_ref, job_run_id, deleted_at)
				VALUES ($1, $2, 'retention_sweep', NULL, NULL, now())`,
				id, string(class)); err != nil {
				return translateError(err)
			}
		}

		// OldestSurvivingAt: the clock-column value of the oldest row
		// remaining in this class after this call — nil only when the
		// class has zero rows remaining, never for "nothing was old
		// enough" (§3c).
		var oldest sql.NullTime
		remainingQuery := "SELECT MIN(" + clockColumn + ") FROM user_profile WHERE " + extraWhere
		if err := tx.QueryRowContext(ctx, remainingQuery).Scan(&oldest); err != nil {
			return translateError(err)
		}
		var oldestPtr *time.Time
		if oldest.Valid {
			t := oldest.Time
			oldestPtr = &t
		}

		result = dao.SweepResult{
			RowsExamined:      examined,
			RowsDeleted:       deleted,
			OldestSurvivingAt: oldestPtr,
			Drained:           examined < maxRows,
		}
		return nil
	})
	if err != nil {
		return dao.SweepResult{}, err
	}
	return result, nil
}

// DeleteProfile is the subject-deletion path. reason is asserted
// internally as "subject_request", never caller-supplied (§3c).
func (r *repository) DeleteProfile(ctx context.Context, id string, externalRef *string) error {
	return r.withRetry(ctx, func(tx *sql.Tx) error {
		var source string
		if err := tx.QueryRowContext(ctx, "SELECT source FROM user_profile WHERE id = $1", id).Scan(&source); err != nil {
			if err == sql.ErrNoRows {
				return dao.ErrNotFound
			}
			return translateError(err)
		}

		res, err := tx.ExecContext(ctx, "DELETE FROM user_profile WHERE id = $1", id)
		if err != nil {
			return translateError(err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return translateError(err)
		}
		if n == 0 {
			return dao.ErrNotFound
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO deletion_log (profile_id, source, reason, external_ref, job_run_id, deleted_at)
			VALUES ($1, $2, 'subject_request', $3, NULL, now())`,
			id, source, externalRef); err != nil {
			return translateError(err)
		}
		return nil
	})
}
