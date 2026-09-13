package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"loginid-takehome/internal/dao"
	"loginid-takehome/internal/model"
)

type credentialRepo struct{ r *repository }

const credentialColumns = "id, user_id, username, method_id, secret, secret_state, hash_algo, hash_cost, created_at, updated_at"

func scanCredential(scan func(dest ...any) error) (*model.UserCredential, error) {
	var c model.UserCredential
	err := scan(&c.ID, &c.UserID, &c.Username, &c.MethodID, &c.Secret, &c.SecretState, &c.HashAlgo, &c.HashCost, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, dao.ErrNotFound
		}
		return nil, translateError(err)
	}
	return &c, nil
}

// GetByUsername lower-cases its input inside the DAO, matching the UNIQUE
// on lower(username) index — callers pass whatever the user typed.
func (cr credentialRepo) GetByUsername(ctx context.Context, username string) (*model.UserCredential, error) {
	row := cr.r.db.QueryRowContext(ctx, "SELECT "+credentialColumns+" FROM user_credential WHERE LOWER(username) = LOWER($1)", strings.ToLower(username))
	return scanCredential(row.Scan)
}

func (cr credentialRepo) ListByUserID(ctx context.Context, userID string) ([]model.UserCredential, error) {
	rows, err := cr.r.db.QueryContext(ctx, "SELECT "+credentialColumns+" FROM user_credential WHERE user_id = $1", userID)
	if err != nil {
		return nil, translateError(err)
	}
	defer func() { _ = rows.Close() }()

	var results []model.UserCredential
	for rows.Next() {
		c, err := scanCredential(rows.Scan)
		if err != nil {
			return nil, err
		}
		results = append(results, *c)
	}
	return results, rows.Err()
}

// validateMethod enforces two guarantees a CHECK constraint cannot,
// because CHECK cannot reference another table (multi-db-strategy.md §6
// items 10 and 11 — both named as the two weakest enforcement sites in
// the contract for exactly this reason):
//   - is_active: a deactivated method must not accept new credentials
//     (existing credentials against it are unaffected — this only gates
//     Create/method-reassignment, never invalidates what already exists).
//   - requires_secret ↔ secret_state: a credential whose method has
//     requires_secret = false must not carry secret_state = 'set'.
func validateMethod(ctx context.Context, tx *sql.Tx, methodID, secretState string) error {
	var requiresSecret, isActive bool
	err := tx.QueryRowContext(ctx, "SELECT requires_secret, is_active FROM auth_method WHERE id = $1", methodID).Scan(&requiresSecret, &isActive)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dao.ErrInvalidMethod
		}
		return translateError(err)
	}
	if !isActive {
		return dao.ErrInvalidMethod
	}
	if !requiresSecret && secretState == "set" {
		return dao.ErrInvalidCredential
	}
	return nil
}

func (cr credentialRepo) Create(ctx context.Context, c *model.UserCredential) (*model.UserCredential, error) {
	if err := dao.ValidateCreateID(c.ID); err != nil {
		return nil, err
	}
	if err := dao.ValidateCredentialPointers(c); err != nil {
		return nil, err
	}

	var created *model.UserCredential
	err := cr.r.withRetry(ctx, func(tx *sql.Tx) error {
		if err := validateMethod(ctx, tx, c.MethodID, c.SecretState); err != nil {
			return err
		}
		row := tx.QueryRowContext(ctx, `
			INSERT INTO user_credential (user_id, username, method_id, secret, secret_state, hash_algo, hash_cost)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING `+credentialColumns,
			c.UserID, c.Username, c.MethodID, c.Secret, c.SecretState, c.HashAlgo, c.HashCost)
		got, err := scanCredential(row.Scan)
		if err != nil {
			return err
		}
		created = got
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (cr credentialRepo) Update(ctx context.Context, c *model.UserCredential) (*model.UserCredential, error) {
	if err := dao.ValidateCredentialPointers(c); err != nil {
		return nil, err
	}

	var updated *model.UserCredential
	err := cr.r.withRetry(ctx, func(tx *sql.Tx) error {
		if err := validateMethod(ctx, tx, c.MethodID, c.SecretState); err != nil {
			return err
		}
		row := tx.QueryRowContext(ctx, `
			UPDATE user_credential
			SET username = $1, method_id = $2, secret = $3, secret_state = $4,
			    hash_algo = $5, hash_cost = $6, updated_at = now()
			WHERE id = $7
			RETURNING `+credentialColumns,
			c.Username, c.MethodID, c.Secret, c.SecretState, c.HashAlgo, c.HashCost, c.ID)
		got, err := scanCredential(row.Scan)
		if err != nil {
			return err
		}
		updated = got
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (cr credentialRepo) Delete(ctx context.Context, id string) error {
	return cr.r.withRetry(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "DELETE FROM user_credential WHERE id = $1", id)
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
		return nil
	})
}
