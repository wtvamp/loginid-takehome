package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"loginid-takehome/internal/dao"
	"loginid-takehome/internal/model"
)

type credentialRepo struct{ r *repository }

const credentialColumns = "id, user_id, username, method_id, secret, secret_state, hash_algo, hash_cost, created_at, updated_at"

func scanCredential(scan func(dest ...any) error) (*model.UserCredential, error) {
	var c model.UserCredential
	var createdAt, updatedAt string
	err := scan(&c.ID, &c.UserID, &c.Username, &c.MethodID, &c.Secret, &c.SecretState, &c.HashAlgo, &c.HashCost, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, dao.ErrNotFound
		}
		return nil, translateError(err)
	}
	c.CreatedAt, err = time.Parse(timeLayout, createdAt)
	if err != nil {
		return nil, fmt.Errorf("sqlite: parsing created_at: %w", err)
	}
	c.UpdatedAt, err = time.Parse(timeLayout, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("sqlite: parsing updated_at: %w", err)
	}
	return &c, nil
}

func (cr credentialRepo) GetByUsername(ctx context.Context, username string) (*model.UserCredential, error) {
	row := cr.r.db.QueryRowContext(ctx, "SELECT "+credentialColumns+" FROM user_credential WHERE LOWER(username) = LOWER(?)", strings.ToLower(username))
	return scanCredential(row.Scan)
}

func (cr credentialRepo) ListByUserID(ctx context.Context, userID string) ([]model.UserCredential, error) {
	if err := dao.ValidateID(userID); err != nil {
		return nil, err
	}
	rows, err := cr.r.db.QueryContext(ctx, "SELECT "+credentialColumns+" FROM user_credential WHERE user_id = ?", userID)
	if err != nil {
		return nil, translateError(err)
	}
	defer func() { _ = rows.Close() }()

	results := []model.UserCredential{}
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
func validateMethod(ctx context.Context, db execQueryRower, methodID, secretState string) error {
	var requiresSecret, isActive bool
	err := db.QueryRowContext(ctx, "SELECT requires_secret, is_active FROM auth_method WHERE id = ?", methodID).Scan(&requiresSecret, &isActive)
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

// execQueryRower is satisfied by both *sql.DB and *sql.Tx — validateMethod
// runs identically whether called outside a transaction (Create, Update)
// or inside one (composite.go's CreateProfileWithCredential).
type execQueryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (cr credentialRepo) Create(ctx context.Context, c *model.UserCredential) (*model.UserCredential, error) {
	if err := dao.ValidateCreateID(c.ID); err != nil {
		return nil, err
	}
	if err := dao.ValidateCredentialPointers(c); err != nil {
		return nil, err
	}
	if err := validateMethod(ctx, cr.r.db, c.MethodID, c.SecretState); err != nil {
		return nil, err
	}

	id := uuid.NewString()
	now := time.Now().UTC().Format(timeLayout)
	_, err := cr.r.db.ExecContext(ctx, `
		INSERT INTO user_credential (id, user_id, username, method_id, secret, secret_state, hash_algo, hash_cost, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, c.UserID, c.Username, c.MethodID, c.Secret, c.SecretState, c.HashAlgo, c.HashCost, now, now)
	if err != nil {
		return nil, translateError(err)
	}
	row := cr.r.db.QueryRowContext(ctx, "SELECT "+credentialColumns+" FROM user_credential WHERE id = ?", id)
	return scanCredential(row.Scan)
}

func (cr credentialRepo) Update(ctx context.Context, c *model.UserCredential) (*model.UserCredential, error) {
	if err := dao.ValidateID(c.ID); err != nil {
		return nil, err
	}
	if err := dao.ValidateCredentialPointers(c); err != nil {
		return nil, err
	}
	if err := validateMethod(ctx, cr.r.db, c.MethodID, c.SecretState); err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(timeLayout)
	res, err := cr.r.db.ExecContext(ctx, `
		UPDATE user_credential
		SET username = ?, method_id = ?, secret = ?, secret_state = ?, hash_algo = ?, hash_cost = ?, updated_at = ?
		WHERE id = ?`,
		c.Username, c.MethodID, c.Secret, c.SecretState, c.HashAlgo, c.HashCost, now, c.ID)
	if err != nil {
		return nil, translateError(err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, translateError(err)
	}
	if n == 0 {
		return nil, dao.ErrNotFound
	}
	row := cr.r.db.QueryRowContext(ctx, "SELECT "+credentialColumns+" FROM user_credential WHERE id = ?", c.ID)
	return scanCredential(row.Scan)
}

func (cr credentialRepo) Delete(ctx context.Context, id string) error {
	if err := dao.ValidateID(id); err != nil {
		return err
	}
	res, err := cr.r.db.ExecContext(ctx, "DELETE FROM user_credential WHERE id = ?", id)
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
}
