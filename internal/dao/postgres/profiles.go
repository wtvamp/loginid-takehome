package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"loginid-takehome/internal/dao"
	"loginid-takehome/internal/model"
)

type profileRepo struct{ r *repository }

const profileColumns = "id, name, phone, street_address, locality, region, postal_code, country, source, created_at, updated_at"

func scanProfile(scan func(dest ...any) error) (*model.UserProfile, error) {
	var p model.UserProfile
	err := scan(&p.ID, &p.Name, &p.Phone, &p.StreetAddress, &p.Locality, &p.Region, &p.PostalCode, &p.Country, &p.Source, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, dao.ErrNotFound
		}
		return nil, translateError(err)
	}
	return &p, nil
}

func (pr profileRepo) Get(ctx context.Context, id string) (*model.UserProfile, error) {
	if err := dao.ValidateID(id); err != nil {
		return nil, err
	}
	row := pr.r.db.QueryRowContext(ctx, "SELECT "+profileColumns+" FROM user_profile WHERE id = $1", id)
	return scanProfile(row.Scan)
}

func (pr profileRepo) Search(ctx context.Context, q dao.ProfileQuery) ([]model.UserProfile, int, error) {
	if err := dao.ValidateProfileQuery(q); err != nil {
		return nil, 0, err
	}

	var where []string
	var args []any
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if q.Name != nil {
		where = append(where, "LOWER(name) LIKE LOWER("+arg("%"+escapeLike(*q.Name)+"%")+") ESCAPE '\\'")
	}
	if q.Phone != nil {
		where = append(where, "phone = "+arg(*q.Phone))
	}
	if q.Region != nil {
		where = append(where, "LOWER(region) = LOWER("+arg(*q.Region)+")")
	}
	if q.Country != nil {
		where = append(where, "country = "+arg(strings.ToUpper(*q.Country)))
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	countQuery := "SELECT COUNT(*) FROM user_profile" + whereClause
	if err := pr.r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, translateError(err)
	}

	limit := dao.EffectiveLimit(q)
	limitArg := arg(limit)
	offsetArg := arg(q.Offset)
	selectQuery := "SELECT " + profileColumns + " FROM user_profile" + whereClause +
		` ORDER BY LOWER(name) COLLATE "C" ASC, id ASC LIMIT ` + limitArg + " OFFSET " + offsetArg

	rows, err := pr.r.db.QueryContext(ctx, selectQuery, args...)
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

	var created *model.UserProfile
	err := pr.r.withRetry(ctx, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
			INSERT INTO user_profile (name, phone, street_address, locality, region, postal_code, country, source)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING `+profileColumns,
			p.Name, p.Phone, p.StreetAddress, p.Locality, p.Region, p.PostalCode, p.Country, p.Source)
		got, err := scanProfile(row.Scan)
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

func (pr profileRepo) Update(ctx context.Context, p *model.UserProfile) (*model.UserProfile, error) {
	if err := dao.ValidateID(p.ID); err != nil {
		return nil, err
	}
	if err := dao.ValidateProfilePointers(p); err != nil {
		return nil, err
	}
	dao.NormalizeProfileForWrite(p)

	var updated *model.UserProfile
	err := pr.r.withRetry(ctx, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
			UPDATE user_profile
			SET name = $1, phone = $2, street_address = $3, locality = $4, region = $5,
			    postal_code = $6, country = $7, source = $8, updated_at = now()
			WHERE id = $9
			RETURNING `+profileColumns,
			p.Name, p.Phone, p.StreetAddress, p.Locality, p.Region, p.PostalCode, p.Country, p.Source, p.ID)
		got, err := scanProfile(row.Scan)
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

// Upsert's DO UPDATE SET list deliberately excludes created_at — it is set
// only on insert, per 05's Oren-finding-#3 ruling (§3a note 3). A
// re-hydration Upsert must never reset the original creation timestamp.
func (pr profileRepo) Upsert(ctx context.Context, p *model.UserProfile) (*model.UserProfile, error) {
	if err := dao.ValidateID(p.ID); err != nil {
		return nil, err
	}
	if err := dao.ValidateProfilePointers(p); err != nil {
		return nil, err
	}
	dao.NormalizeProfileForWrite(p)

	var result *model.UserProfile
	err := pr.r.withRetry(ctx, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
			INSERT INTO user_profile (id, name, phone, street_address, locality, region, postal_code, country, source)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (id) DO UPDATE SET
				name = EXCLUDED.name,
				phone = EXCLUDED.phone,
				street_address = EXCLUDED.street_address,
				locality = EXCLUDED.locality,
				region = EXCLUDED.region,
				postal_code = EXCLUDED.postal_code,
				country = EXCLUDED.country,
				source = EXCLUDED.source,
				updated_at = now()
			RETURNING `+profileColumns,
			p.ID, p.Name, p.Phone, p.StreetAddress, p.Locality, p.Region, p.PostalCode, p.Country, p.Source)
		got, err := scanProfile(row.Scan)
		if err != nil {
			return err
		}
		result = got
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (pr profileRepo) Delete(ctx context.Context, id string) error {
	if err := dao.ValidateID(id); err != nil {
		return err
	}
	return pr.r.withRetry(ctx, func(tx *sql.Tx) error {
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
		return nil
	})
}
