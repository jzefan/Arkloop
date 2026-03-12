package data

import (
	"context"
	"errors"

	"github.com/google/uuid"
"arkloop/services/shared/database"
)

type OrgMembershipRecord struct {
	OrgID  uuid.UUID
	UserID uuid.UUID
	Role   string
}

type OrgMembershipsRepository struct{}

func (OrgMembershipsRepository) GetByOrgAndUser(
	ctx context.Context,
	pool database.DB,
	orgID uuid.UUID,
	userID uuid.UUID,
) (*OrgMembershipRecord, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if pool == nil {
		return nil, errors.New("pool must not be nil")
	}
	if orgID == uuid.Nil {
		return nil, errors.New("org_id must not be empty")
	}
	if userID == uuid.Nil {
		return nil, errors.New("user_id must not be empty")
	}

	var record OrgMembershipRecord
	err := pool.QueryRow(
		ctx,
		`SELECT org_id, user_id, role
		   FROM org_memberships
		  WHERE org_id = $1
		    AND user_id = $2
		  LIMIT 1`,
		orgID,
		userID,
	).Scan(&record.OrgID, &record.UserID, &record.Role)
	if err != nil {
		if errors.Is(err, database.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}
