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

type profileRepo struct{ r *repository }

const profileColumns = "id, name, phone, street_address, locality, region, postal_code, country, source, created_at, updated_at"

const timeLayout = time.RFC3339

func scanProfile(scan func(dest ...any) error) (*model.UserProfile, error) {
	var p model.UserProfile
	var createdAt, updatedAt string
	err := scan(&p.ID, &p.Name, &p.Phone, &p.StreetAddress, &p.Locality, &p.Region, &p.PostalCode, &p.Country, &p.Source, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, dao.ErrNotFound
		}
		return nil, translateError(err)
	}
	p.CreatedAt, err = time.Parse(timeLayout, createdAt)
	if err != nil {
		return nil, fmt.Errorf("sqlite: parsing created_at: %w", err)
	}
	p.UpdatedAt, err = time.Parse(timeLayout, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("sqlite: parsing updated_at: %w", err)
	}
	return &p, nil
}

func (pr profileRepo) Get(ctx context.Context, id string) (*model.UserProfile, error) {
	if err := dao.ValidateID(id); err != nil {
		return nil, err
	}
	row := pr.r.db.QueryRowContext(ctx, "SELECT "+profileColumns+" FROM user_profile WHERE id = ?", id)
	return scanProfile(row.Scan)
}

// Search folds case EXPLICITLY (LOWER(col) LIKE LOWER(pattern)) rather
// than relying on SQLite's default ASCII case-insensitive LIKE, so the
// behavior is asserted by this code, not an accident of the engine
// default (per LT-37's refinement doc). Residual, accepted and named:
// Postgres's lower() is locale-aware, SQLite's is ASCII-only, so
// non-ASCII case folding still diverges between backends — SQLite is the
// local/dev/demo backend, not a production peer (multi-db-strategy.md,
// "What Search does on SQLite").
func (pr profileRepo) Search(ctx context.Context, q dao.ProfileQuery) ([]model.UserProfile, int, error) {
	if err := dao.ValidateProfileQuery(q); err != nil {
		return nil, 0, err
	}

	var where []string
	var args []any
	if q.Name != nil {
		where = append(where, "LOWER(name) LIKE LOWER(?) ESCAPE '\\'")
		args = append(args, "%"+escapeLike(*q.Name)+"%")
	}
	if q.Phone != nil {
		where = append(where, "phone = ?")
		args = append(args, *q.Phone)
	}
	if q.Region != nil {
		where = append(where, "LOWER(region) = LOWER(?)")
		args = append(args, *q.Region)
	}
	if q.Country != nil {
		where = append(where, "country = ?")
		args = append(args, strings.ToUpper(*q.Country))
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := pr.r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM user_profile"+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, translateError(err)
	}

	limit := dao.EffectiveLimit(q)
	selectArgs := append(append([]any{}, args...), limit, q.Offset)
	selectQuery := "SELECT " + profileColumns + " FROM user_profile" + whereClause +
		" ORDER BY LOWER(name) COLLATE BINARY ASC, id ASC LIMIT ? OFFSET ?"

	rows, err := pr.r.db.QueryContext(ctx, selectQuery, selectArgs...)
	if err != nil {
		return nil, 0, translateError(err)
	}
	defer func() { _ = rows.Close() }()

	results := []model.UserProfile{}
	for rows.Next() {
		p, err := scanProfile(rows.Scan)
		if err != nil {
			return nil, 0, err
		}
		results = append(results, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, translateError(err)
	}
	return results, total, nil
}

func (pr profileRepo) Create(ctx context.Context, p *model.UserProfile) (*model.UserProfile, error) {
	if err := dao.ValidateCreateID(p.ID); err != nil {
		return nil, err
	}
	if err := dao.ValidateProfilePointers(p); err != nil {
		return nil, err
	}
	dao.NormalizeProfileForWrite(p)

	// App-side UUID generation — SQLite has no server-side gen_random_uuid().
	id := uuid.NewString()
	now := time.Now().UTC().Format(timeLayout)

	_, err := pr.r.db.ExecContext(ctx, `
		INSERT INTO user_profile (id, name, phone, street_address, locality, region, postal_code, country, source, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, p.Name, p.Phone, p.StreetAddress, p.Locality, p.Region, p.PostalCode, p.Country, p.Source, now, now)
	if err != nil {
		return nil, translateError(err)
	}
	return pr.Get(ctx, id)
}

func (pr profileRepo) Update(ctx context.Context, p *model.UserProfile) (*model.UserProfile, error) {
	if err := dao.ValidateID(p.ID); err != nil {
		return nil, err
	}
	if err := dao.ValidateProfilePointers(p); err != nil {
		return nil, err
	}
	dao.NormalizeProfileForWrite(p)

	now := time.Now().UTC().Format(timeLayout)
	res, err := pr.r.db.ExecContext(ctx, `
		UPDATE user_profile
		SET name = ?, phone = ?, street_address = ?, locality = ?, region = ?,
		    postal_code = ?, country = ?, source = ?, updated_at = ?
		WHERE id = ?`,
		p.Name, p.Phone, p.StreetAddress, p.Locality, p.Region, p.PostalCode, p.Country, p.Source, now, p.ID)
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
	return pr.Get(ctx, p.ID)
}

// Upsert's DO UPDATE SET list deliberately excludes created_at — set only
// on insert, per 05's Oren-finding-#3 ruling.
func (pr profileRepo) Upsert(ctx context.Context, p *model.UserProfile) (*model.UserProfile, error) {
	if err := dao.ValidateID(p.ID); err != nil {
		return nil, err
	}
	if err := dao.ValidateProfilePointers(p); err != nil {
		return nil, err
	}
	dao.NormalizeProfileForWrite(p)

	now := time.Now().UTC().Format(timeLayout)
	_, err := pr.r.db.ExecContext(ctx, `
		INSERT INTO user_profile (id, name, phone, street_address, locality, region, postal_code, country, source, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			name = excluded.name,
			phone = excluded.phone,
			street_address = excluded.street_address,
			locality = excluded.locality,
			region = excluded.region,
			postal_code = excluded.postal_code,
			country = excluded.country,
			source = excluded.source,
			updated_at = excluded.updated_at`,
		p.ID, p.Name, p.Phone, p.StreetAddress, p.Locality, p.Region, p.PostalCode, p.Country, p.Source, now, now)
	if err != nil {
		return nil, translateError(err)
	}
	return pr.Get(ctx, p.ID)
}

func (pr profileRepo) Delete(ctx context.Context, id string) error {
	if err := dao.ValidateID(id); err != nil {
		return err
	}
	res, err := pr.r.db.ExecContext(ctx, "DELETE FROM user_profile WHERE id = ?", id)
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
