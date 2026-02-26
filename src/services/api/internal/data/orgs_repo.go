package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Org struct {
	ID           uuid.UUID
	Slug         string
	Name         string
	Type         string // "personal" | "workspace"
	OwnerUserID  *uuid.UUID
	Status       string
	Country      *string
	Timezone     *string
	LogoURL      *string
	SettingsJSON json.RawMessage
	DeletedAt    *time.Time
	CreatedAt    time.Time
}

type OrgRepository struct {
	db Querier
}

func NewOrgRepository(db Querier) (*OrgRepository, error) {
	if db == nil {
		return nil, errors.New("db must not be nil")
	}
	return &OrgRepository{db: db}, nil
}

func (r *OrgRepository) Create(ctx context.Context, slug string, name string, orgType string) (Org, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if slug == "" {
		return Org{}, fmt.Errorf("slug must not be empty")
	}
	if name == "" {
		return Org{}, fmt.Errorf("name must not be empty")
	}
	if orgType != "personal" && orgType != "workspace" {
		return Org{}, fmt.Errorf("org type must be personal or workspace")
	}

	var org Org
	err := r.db.QueryRow(
		ctx,
		`INSERT INTO orgs (slug, name, type)
		 VALUES ($1, $2, $3)
		 RETURNING id, slug, name, type, owner_user_id, status, country, timezone,
		           logo_url, settings_json, deleted_at, created_at`,
		slug,
		name,
		orgType,
	).Scan(
		&org.ID, &org.Slug, &org.Name, &org.Type,
		&org.OwnerUserID, &org.Status, &org.Country, &org.Timezone,
		&org.LogoURL, &org.SettingsJSON, &org.DeletedAt, &org.CreatedAt,
	)
	if err != nil {
		return Org{}, err
	}
	return org, nil
}

func (r *OrgRepository) GetByID(ctx context.Context, orgID uuid.UUID) (*Org, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if orgID == uuid.Nil {
		return nil, fmt.Errorf("org_id must not be empty")
	}

	var org Org
	err := r.db.QueryRow(
		ctx,
		`SELECT id, slug, name, type, owner_user_id, status, country, timezone,
		        logo_url, settings_json, deleted_at, created_at
		 FROM orgs
		 WHERE id = $1
		 LIMIT 1`,
		orgID,
	).Scan(
		&org.ID, &org.Slug, &org.Name, &org.Type,
		&org.OwnerUserID, &org.Status, &org.Country, &org.Timezone,
		&org.LogoURL, &org.SettingsJSON, &org.DeletedAt, &org.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &org, nil
}

// ListByUser 返回用户所属的所有 org（通过 org_memberships JOIN）。
func (r *OrgRepository) ListByUser(ctx context.Context, userID uuid.UUID) ([]Org, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if userID == uuid.Nil {
		return nil, fmt.Errorf("user_id must not be empty")
	}

	rows, err := r.db.Query(
		ctx,
		`SELECT o.id, o.slug, o.name, o.type, o.owner_user_id, o.status, o.country, o.timezone,
		        o.logo_url, o.settings_json, o.deleted_at, o.created_at
		 FROM orgs o
		 JOIN org_memberships m ON m.org_id = o.id
		 WHERE m.user_id = $1
		   AND o.deleted_at IS NULL
		 ORDER BY o.created_at ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("orgs.ListByUser: %w", err)
	}
	defer rows.Close()

	var orgs []Org
	for rows.Next() {
		var org Org
		if err := rows.Scan(
			&org.ID, &org.Slug, &org.Name, &org.Type,
			&org.OwnerUserID, &org.Status, &org.Country, &org.Timezone,
			&org.LogoURL, &org.SettingsJSON, &org.DeletedAt, &org.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("orgs.ListByUser scan: %w", err)
		}
		orgs = append(orgs, org)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orgs.ListByUser rows: %w", err)
	}
	return orgs, nil
}

func (r *OrgRepository) CountActive(ctx context.Context) (int64, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	var count int64
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM orgs WHERE deleted_at IS NULL`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("orgs.CountActive: %w", err)
	}
	return count, nil
}
