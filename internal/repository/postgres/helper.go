package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

const postgresUniqueViolationCode = "23505"

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolationCode
}
