package postgres

import "github.com/jackc/pgx/v5"

func SanitizedIdentifier(table string) string {
	return pgx.Identifier{table}.Sanitize()
}
