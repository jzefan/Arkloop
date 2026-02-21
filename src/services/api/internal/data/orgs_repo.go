package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Org struct {
	ID           uuid.UUID
	Slug         string
	Name         string
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

func (r *OrgRepository) Create(ctx context.Context, slug string, name string) (Org, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if slug == "" {
		return Org{}, fmt.Errorf("slug must not be empty")
	}
	if name == "" {
		return Org{}, fmt.Errorf("name must not be empty")
	}

	var org Org
	err := r.db.QueryRow(
		ctx,
		`INSERT INTO orgs (slug, name)
		 VALUES ($1, $2)
		 RETURNING id, slug, name, owner_user_id, status, country, timezone,
		           logo_url, settings_json, deleted_at, created_at`,
		slug,
		name,
	).Scan(
		&org.ID, &org.Slug, &org.Name,
		&org.OwnerUserID, &org.Status, &org.Country, &org.Timezone,
		&org.LogoURL, &org.SettingsJSON, &org.DeletedAt, &org.CreatedAt,
	)
	if err != nil {
		return Org{}, err
	}
	return org, nil
}
