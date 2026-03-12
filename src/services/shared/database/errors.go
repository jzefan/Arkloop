package database

import "errors"

// ErrNoRows is returned when a query expected exactly one row but found none.
// Adapters must translate driver-specific "no rows" errors into this sentinel.
var ErrNoRows = errors.New("database: no rows in result set")

// IsNoRows reports whether err matches ErrNoRows (works with wrapped errors).
func IsNoRows(err error) bool {
	return errors.Is(err, ErrNoRows)
}
