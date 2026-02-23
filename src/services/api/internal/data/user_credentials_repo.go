package data

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type UserCredential struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	Login        string
	PasswordHash string
	CreatedAt    time.Time
}

type UserCredentialRepository struct {
	db Querier
}

func NewUserCredentialRepository(db Querier) (*UserCredentialRepository, error) {
	if db == nil {
		return nil, errors.New("db must not be nil")
	}
	return &UserCredentialRepository{db: db}, nil
}

func (r *UserCredentialRepository) Create(
	ctx context.Context,
	userID uuid.UUID,
	login string,
	passwordHash string,
) (UserCredential, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if login == "" {
		return UserCredential{}, fmt.Errorf("login must not be empty")
	}
	if passwordHash == "" {
		return UserCredential{}, fmt.Errorf("password_hash must not be empty")
	}

	var credential UserCredential
	err := r.db.QueryRow(
		ctx,
		`INSERT INTO user_credentials (user_id, login, password_hash)
		 VALUES ($1, $2, $3)
		 RETURNING id, user_id, login, password_hash, created_at`,
		userID,
		login,
		passwordHash,
	).Scan(&credential.ID, &credential.UserID, &credential.Login, &credential.PasswordHash, &credential.CreatedAt)
	if err != nil {
		return UserCredential{}, err
	}
	return credential, nil
}

func (r *UserCredentialRepository) GetByLogin(ctx context.Context, login string) (*UserCredential, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if login == "" {
		return nil, nil
	}

	var credential UserCredential
	err := r.db.QueryRow(
		ctx,
		`SELECT id, user_id, login, password_hash, created_at
		 FROM user_credentials
		 WHERE login = $1
		 LIMIT 1`,
		login,
	).Scan(&credential.ID, &credential.UserID, &credential.Login, &credential.PasswordHash, &credential.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &credential, nil
}

func (r *UserCredentialRepository) GetByUserID(ctx context.Context, userID uuid.UUID) (*UserCredential, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	var credential UserCredential
	err := r.db.QueryRow(
		ctx,
		`SELECT id, user_id, login, password_hash, created_at
		 FROM user_credentials
		 WHERE user_id = $1
		 LIMIT 1`,
		userID,
	).Scan(&credential.ID, &credential.UserID, &credential.Login, &credential.PasswordHash, &credential.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &credential, nil
}

// ListLoginsByUserIDs 批量查询一组用户的 login，返回 userID → login 映射。
func (r *UserCredentialRepository) ListLoginsByUserIDs(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(userIDs) == 0 {
		return map[uuid.UUID]string{}, nil
	}

	rows, err := r.db.Query(
		ctx,
		`SELECT DISTINCT ON (user_id) user_id, login
		 FROM user_credentials
		 WHERE user_id = ANY($1)`,
		userIDs,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[uuid.UUID]string, len(userIDs))
	for rows.Next() {
		var uid uuid.UUID
		var login string
		if err := rows.Scan(&uid, &login); err != nil {
			return nil, err
		}
		result[uid] = login
	}
	return result, rows.Err()
}
