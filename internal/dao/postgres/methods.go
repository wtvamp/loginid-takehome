package postgres

import (
	"context"
	"database/sql"
	"errors"

	"loginid-takehome/internal/dao"
	"loginid-takehome/internal/model"
)

type authMethodRepo struct{ r *repository }

const authMethodColumns = "id, name, requires_secret, is_active, created_at"

func scanAuthMethod(scan func(dest ...any) error) (*model.AuthMethod, error) {
	var m model.AuthMethod
	err := scan(&m.ID, &m.Name, &m.RequiresSecret, &m.IsActive, &m.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, dao.ErrNotFound
		}
		return nil, translateError(err)
	}
	return &m, nil
}

func (ar authMethodRepo) Get(ctx context.Context, id string) (*model.AuthMethod, error) {
	if err := dao.ValidateID(id); err != nil {
		return nil, err
	}
	row := ar.r.db.QueryRowContext(ctx, "SELECT "+authMethodColumns+" FROM auth_method WHERE id = $1", id)
	return scanAuthMethod(row.Scan)
}

func (ar authMethodRepo) GetByName(ctx context.Context, name string) (*model.AuthMethod, error) {
	row := ar.r.db.QueryRowContext(ctx, "SELECT "+authMethodColumns+" FROM auth_method WHERE name = $1", name)
	return scanAuthMethod(row.Scan)
}

func (ar authMethodRepo) List(ctx context.Context, activeOnly bool) ([]model.AuthMethod, error) {
	query := "SELECT " + authMethodColumns + " FROM auth_method"
	if activeOnly {
		query += " WHERE is_active = true"
	}
	rows, err := ar.r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, translateError(err)
	}
	defer func() { _ = rows.Close() }()

	var results []model.AuthMethod
	for rows.Next() {
		m, err := scanAuthMethod(rows.Scan)
		if err != nil {
			return nil, err
		}
		results = append(results, *m)
	}
	return results, rows.Err()
}
