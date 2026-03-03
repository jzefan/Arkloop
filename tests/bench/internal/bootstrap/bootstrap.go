package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const registrationOpenKey = "registration.open"

func EnsureRegistrationOpen(ctx context.Context, dsn string, forceOpen bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if dsn == "" {
		return fmt.Errorf("db dsn is empty")
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err = pool.Exec(ctx,
		`INSERT INTO feature_flags (key, description, default_value)
		 VALUES ($1, $2, true)
		 ON CONFLICT (key) DO NOTHING`,
		registrationOpenKey,
		"registration mode",
	)
	if err != nil {
		return err
	}

	var enabled bool
	err = pool.QueryRow(ctx,
		`SELECT default_value FROM feature_flags WHERE key = $1`,
		registrationOpenKey,
	).Scan(&enabled)
	if err != nil {
		return err
	}

	if !enabled && forceOpen {
		_, err = pool.Exec(ctx,
			`UPDATE feature_flags SET default_value = true WHERE key = $1`,
			registrationOpenKey,
		)
		if err != nil {
			return err
		}
	}

	return nil
}
