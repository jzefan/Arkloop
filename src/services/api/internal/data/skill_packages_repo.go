package data

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"arkloop/services/shared/skillstore"
	"github.com/google/uuid"
"arkloop/services/shared/database"
)

type SkillPackage struct {
	OrgID           uuid.UUID
	SkillKey        string
	Version         string
	DisplayName     string
	Description     *string
	InstructionPath string
	ManifestKey     string
	BundleKey       string
	FilesPrefix     string
	Platforms       []string
	IsActive        bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type SkillPackageConflictError struct {
	SkillKey string
	Version  string
}

func (e SkillPackageConflictError) Error() string {
	return fmt.Sprintf("skill package %q@%q already exists", e.SkillKey, e.Version)
}

type SkillPackagesRepository struct {
	db Querier
}

func NewSkillPackagesRepository(db Querier) (*SkillPackagesRepository, error) {
	if db == nil {
		return nil, errors.New("db must not be nil")
	}
	return &SkillPackagesRepository{db: db}, nil
}

func (r *SkillPackagesRepository) Create(ctx context.Context, orgID uuid.UUID, manifest skillstore.PackageManifest) (SkillPackage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if orgID == uuid.Nil {
		return SkillPackage{}, fmt.Errorf("org_id must not be nil")
	}
	normalized, err := skillstore.ValidateManifest(manifest)
	if err != nil {
		return SkillPackage{}, err
	}
	var item SkillPackage
	err = r.db.QueryRow(
		ctx,
		`INSERT INTO skill_packages
		    (org_id, skill_key, version, display_name, description, instruction_path, manifest_key, bundle_key, files_prefix, platforms)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING org_id, skill_key, version, display_name, description, instruction_path, manifest_key, bundle_key, files_prefix, platforms, is_active, created_at, updated_at`,
		orgID,
		normalized.SkillKey,
		normalized.Version,
		normalized.DisplayName,
		normalized.Description,
		normalized.InstructionPath,
		normalized.ManifestKey,
		normalized.BundleKey,
		normalized.FilesPrefix,
		normalized.Platforms,
	).Scan(
		&item.OrgID,
		&item.SkillKey,
		&item.Version,
		&item.DisplayName,
		&item.Description,
		&item.InstructionPath,
		&item.ManifestKey,
		&item.BundleKey,
		&item.FilesPrefix,
		&item.Platforms,
		&item.IsActive,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return SkillPackage{}, SkillPackageConflictError{SkillKey: normalized.SkillKey, Version: normalized.Version}
		}
		return SkillPackage{}, err
	}
	return item, nil
}

func (r *SkillPackagesRepository) ListActive(ctx context.Context, orgID uuid.UUID) ([]SkillPackage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := r.db.Query(
		ctx,
		`SELECT org_id, skill_key, version, display_name, description, instruction_path, manifest_key, bundle_key, files_prefix, platforms, is_active, created_at, updated_at
		   FROM skill_packages
		  WHERE org_id = $1
		    AND is_active = TRUE
		  ORDER BY skill_key, version`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]SkillPackage, 0)
	for rows.Next() {
		var item SkillPackage
		if err := rows.Scan(&item.OrgID, &item.SkillKey, &item.Version, &item.DisplayName, &item.Description, &item.InstructionPath, &item.ManifestKey, &item.BundleKey, &item.FilesPrefix, &item.Platforms, &item.IsActive, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *SkillPackagesRepository) Get(ctx context.Context, orgID uuid.UUID, skillKey, version string) (*SkillPackage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	skillKey = strings.TrimSpace(skillKey)
	version = strings.TrimSpace(version)
	if orgID == uuid.Nil || skillKey == "" || version == "" {
		return nil, fmt.Errorf("org_id, skill_key and version must not be empty")
	}
	var item SkillPackage
	err := r.db.QueryRow(
		ctx,
		`SELECT org_id, skill_key, version, display_name, description, instruction_path, manifest_key, bundle_key, files_prefix, platforms, is_active, created_at, updated_at
		   FROM skill_packages
		  WHERE org_id = $1 AND skill_key = $2 AND version = $3`,
		orgID,
		skillKey,
		version,
	).Scan(&item.OrgID, &item.SkillKey, &item.Version, &item.DisplayName, &item.Description, &item.InstructionPath, &item.ManifestKey, &item.BundleKey, &item.FilesPrefix, &item.Platforms, &item.IsActive, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		if errors.Is(err, database.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}
