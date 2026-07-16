package postgres

import (
	"errors"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
)

// IsPgError reports whether err is a PostgreSQL error with the given SQLSTATE
// code. Use pgerrcode constants (e.g. pgerrcode.UndefinedObject) as the code.
func IsPgError(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

// IsUndefinedObject reports whether err is SQLSTATE 42704 (undefined_object).
// PostgreSQL returns this for any missing database object — role, schema,
// function, etc. — that a statement references. Callers can use it to
// distinguish "already gone" from real failures and treat it as a no-op.
func IsUndefinedObject(err error) bool {
	return IsPgError(err, pgerrcode.UndefinedObject)
}
