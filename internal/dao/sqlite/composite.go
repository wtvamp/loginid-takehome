package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"loginid-takehome/internal/dao"
	"loginid-takehome/internal/model"
)

// CreateProfileWithCredential writes both rows in one transaction (§3b).
// No retry wrapper needed — SQLite has no retryable-serialization error
// class — but atomicity still requires an explicit transaction.
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

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, translateError(err)
	}
	defer func() { _ = tx.Rollback() }()

	profileID := uuid.NewString()
	now := time.Now().UTC().Format(timeLayout)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO user_profile (id, name, phone, street_address, locality, region, postal_code, country, source, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		profileID, p.Name, p.Phone, p.StreetAddress, p.Locality, p.Region, p.PostalCode, p.Country, p.Source, now, now); err != nil {
		return nil, nil, translateError(err)
	}

	credentialID := uuid.NewString()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO user_credential (id, user_id, username, method_id, secret, secret_state, hash_algo, hash_cost, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		credentialID, profileID, c.Username, c.MethodID, c.Secret, c.SecretState, c.HashAlgo, c.HashCost, now, now); err != nil {
		return nil, nil, translateError(err)
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, translateError(err)
	}

	createdProfile, err := r.Profiles().Get(ctx, profileID)
	if err != nil {
		return nil, nil, err
	}
	createdCredential, err := r.Credentials().GetByUsername(ctx, c.Username)
	if err != nil {
		return nil, nil, err
	}
	return createdProfile, createdCredential, nil
}

// clockColumnFor mirrors postgres/composite.go's logic exactly, per
// pii-governance.md: "direct" and "idp_cache" (referenced by a credential)
// key on updated_at; "idp_cache_orphan" keys on created_at.
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

func (r *repository) DeleteExpired(ctx context.Context, class dao.RetentionClass, olderThan time.Time, maxRows int) (dao.SweepResult, error) {
	if maxRows < 1 || maxRows > 10_000 {
		return dao.SweepResult{}, dao.ErrInvalidArgument
	}
	clockColumn, extraWhere, ok := clockColumnFor(class)
	if !ok {
		return dao.SweepResult{}, dao.ErrInvalidArgument
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return dao.SweepResult{}, translateError(err)
	}
	defer func() { _ = tx.Rollback() }()

	cutoff := olderThan.UTC().Format(timeLayout)
	rows, err := tx.QueryContext(ctx,
		"SELECT id FROM user_profile WHERE "+extraWhere+" AND "+clockColumn+" < ? LIMIT ?",
		cutoff, maxRows)
	if err != nil {
		return dao.SweepResult{}, translateError(err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return dao.SweepResult{}, translateError(err)
		}
		ids = append(ids, id)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return dao.SweepResult{}, translateError(err)
	}

	examined := len(ids)
	deleted := 0
	for _, id := range ids {
		res, err := tx.ExecContext(ctx, "DELETE FROM user_profile WHERE id = ?", id)
		if err != nil {
			return dao.SweepResult{}, translateError(err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return dao.SweepResult{}, translateError(err)
		}
		if n == 0 {
			continue
		}
		deleted++
		deletionID := uuid.NewString()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO deletion_log (id, profile_id, source, reason, external_ref, job_run_id, deleted_at)
			VALUES (?, ?, ?, 'retention_sweep', NULL, NULL, ?)`,
			deletionID, id, string(class), time.Now().UTC().Format(timeLayout)); err != nil {
			return dao.SweepResult{}, translateError(err)
		}
	}

	var oldest sql.NullString
	if err := tx.QueryRowContext(ctx, "SELECT MIN("+clockColumn+") FROM user_profile WHERE "+extraWhere).Scan(&oldest); err != nil {
		return dao.SweepResult{}, translateError(err)
	}
	var oldestPtr *time.Time
	if oldest.Valid {
		t, err := time.Parse(timeLayout, oldest.String)
		if err != nil {
			return dao.SweepResult{}, translateError(err)
		}
		oldestPtr = &t
	}

	if err := tx.Commit(); err != nil {
		return dao.SweepResult{}, translateError(err)
	}

	return dao.SweepResult{
		RowsExamined:      examined,
		RowsDeleted:       deleted,
		OldestSurvivingAt: oldestPtr,
		Drained:           examined < maxRows,
	}, nil
}

func (r *repository) DeleteProfile(ctx context.Context, id string, externalRef *string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return translateError(err)
	}
	defer func() { _ = tx.Rollback() }()

	var source string
	if err := tx.QueryRowContext(ctx, "SELECT source FROM user_profile WHERE id = ?", id).Scan(&source); err != nil {
		if err == sql.ErrNoRows {
			return dao.ErrNotFound
		}
		return translateError(err)
	}

	res, err := tx.ExecContext(ctx, "DELETE FROM user_profile WHERE id = ?", id)
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

	deletionID := uuid.NewString()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO deletion_log (id, profile_id, source, reason, external_ref, job_run_id, deleted_at)
		VALUES (?, ?, ?, 'subject_request', ?, NULL, ?)`,
		deletionID, id, source, externalRef, time.Now().UTC().Format(timeLayout)); err != nil {
		return translateError(err)
	}

	return translateError(tx.Commit())
}
