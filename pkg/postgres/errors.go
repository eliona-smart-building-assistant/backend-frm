package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

type ConnectionError struct {
	origin error
}

func (e ConnectionError) Error() string {
	return e.origin.Error()
}

func (e ConnectionError) Unwrap() error {
	return e.origin
}

type QueryError struct {
	origin  error
	code    string
	message string
}

func (e QueryError) Error() string {
	return e.origin.Error()
}

func (e QueryError) Code() string {
	return e.code
}

func (e QueryError) Unwrap() error {
	return e.origin
}

type ConfigParserError struct {
	origin error
}

func (e ConfigParserError) Error() string {
	return e.origin.Error()
}

func (e ConfigParserError) Unwrap() error {
	return e.origin
}

func wrapPgxError(err error) error {
	if err == nil {
		return nil
	}

	var pgxConnErr *pgconn.ConnectError
	if errors.As(err, &pgxConnErr) {
		return &ConnectionError{
			origin: err,
		}
	}

	var pgxErr *pgconn.PgError
	if errors.As(err, &pgxErr) {
		return &QueryError{
			origin: err,
		}
	}

	var pgxCfgParseErr *pgconn.ParseConfigError
	if errors.As(err, &pgxCfgParseErr) {
		return &ConfigParserError{
			err,
		}
	}

	// Unknown error?
	return err
}
