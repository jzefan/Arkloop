//go:build desktop

package auth

import (
	"context"

	"arkloop/services/api/internal/data"
)

func interceptDesktopUser(ctx context.Context, userRepo *data.UserRepository) (*data.User, bool) {
	user, err := userRepo.GetByID(ctx, DesktopUserID)
	if err != nil || user == nil {
		return nil, false
	}
	return user, true
}

func interceptDesktopActor() (VerifiedAccessToken, bool) {
	return DesktopVerifiedAccessToken(), true
}
