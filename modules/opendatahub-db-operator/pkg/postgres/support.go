/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package postgres provides the PostgreSQL DDL execution layer used by the
// SchemaClaim and DatabaseClaim reconcilers (docs/plan.md §8). It also
// exposes the password generator used by the Embedded provider's admin-secret
// bootstrap (task-08).
//
// Security invariant: no identifier or literal value is ever interpolated into
// a DDL statement without going through QuoteIdentifier or QuoteLiteral
// respectively. Never use fmt.Sprintf with a raw string to build DDL.
package postgres

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/lib/pq"
)

// QuoteIdentifier returns the identifier safely quoted for use inside a DDL
// statement (schema name, role name, database name, etc.). It uses
// pgx.Identifier.Sanitize() which follows PostgreSQL quoting rules and is the
// same approach used by CloudNativePG's own Database/DatabaseRole reconcilers.
func QuoteIdentifier(name string) string {
	return pgx.Identifier{name}.Sanitize()
}

// QuoteLiteral returns the literal value safely quoted for embedding directly
// in DDL (e.g. a password in ALTER ROLE ... WITH PASSWORD '<literal>',
// which cannot use bind parameters). It uses lib/pq's QuoteLiteral, which
// follows PostgreSQL's dollar-quoting / E-string conventions.
func QuoteLiteral(value string) string {
	return pq.QuoteLiteral(value)
}

const passwordChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// GeneratePassword generates a cryptographically random password of the given
// length using an alphanumeric character set, following the same approach used
// by Zalando's postgres-operator (pkg/util.RandomPassword). The generated
// password is suitable for use as a PostgreSQL role password.
//
// The plaintext password is returned only to the caller; it must be written
// into the credentials Secret immediately and must never be logged or cached
// beyond the single reconcile (docs/plan.md §9).
func GeneratePassword(length int) (string, error) {
	chars := make([]byte, length)
	max := big.NewInt(int64(len(passwordChars)))
	for i := range chars {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		chars[i] = passwordChars[n.Int64()]
	}
	return string(chars), nil
}

// sanitize removes the password literal from an error message before surfacing
// it in a condition, ensuring plaintext passwords never appear in status.
func sanitize(err error, password string) error {
	if err == nil || password == "" {
		return err
	}
	safe := strings.ReplaceAll(err.Error(), password, "[redacted]")
	return fmt.Errorf("%s", safe) //nolint:goerr113 // error is sanitized, not wrapped
}

// SanitizeError removes the password literal from an error before surfacing it
// outside the postgres package.
func SanitizeError(err error, password string) error {
	return sanitize(err, password)
}
