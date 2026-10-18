package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/puddle/v2"
)

// ConnectionError wraps errors that occur while establishing a connection to
// the PostgreSQL server (pgconn.ConnectError, pgconn.NotPreferredError).
type ConnectionError struct {
	origin error
}

func (e ConnectionError) Error() string {
	return e.origin.Error()
}

func (e ConnectionError) Unwrap() error {
	return e.origin
}

// QueryError wraps errors returned by the PostgreSQL server for a query
// (pgconn.PgError) and prepared-statement failures (pgconn.PrepareError).
// Code and Message are populated from the underlying PgError when available.
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

func (e QueryError) Message() string {
	return e.message
}

func (e QueryError) Unwrap() error {
	return e.origin
}

// ConfigParserError wraps errors that occur while parsing the connection
// string / configuration (pgconn.ParseConfigError).
type ConfigParserError struct {
	origin error
}

func (e ConfigParserError) Error() string {
	return e.origin.Error()
}

func (e ConfigParserError) Unwrap() error {
	return e.origin
}

// ConnectionClosedError wraps errors signaling that an operation was attempted
// on an already-closed connection (pgconn.ErrConnClosed) or a closed pool
// (puddle.ErrClosedPool).
type ConnectionClosedError struct {
	origin error
}

func (e ConnectionClosedError) Error() string {
	return e.origin.Error()
}

func (e ConnectionClosedError) Unwrap() error {
	return e.origin
}

// TimeoutError wraps errors caused by a context deadline or a network
// timeout. Covers pgconn-reported timeouts (via pgconn.Timeout) as well as
// pool acquire timeouts that surface as context.DeadlineExceeded.
type TimeoutError struct {
	origin error
}

func (e TimeoutError) Error() string {
	return e.origin.Error()
}

func (e TimeoutError) Unwrap() error {
	return e.origin
}

// ContextCanceledError wraps errors caused by context cancellation
// (context.Canceled), e.g. an in-flight query or pool acquire that was
// canceled by the caller.
type ContextCanceledError struct {
	origin error
}

func (e ContextCanceledError) Error() string {
	return e.origin.Error()
}

func (e ContextCanceledError) Unwrap() error {
	return e.origin
}

// NoRowsError wraps pgx.ErrNoRows / sql.ErrNoRows, returned when a query that
// was expected to return a row returned none.
type NoRowsError struct {
	origin error
}

func (e NoRowsError) Error() string {
	return e.origin.Error()
}

func (e NoRowsError) Unwrap() error {
	return e.origin
}

// ScanError wraps errors that occur while scanning or decoding row values into
// Go types (pgx.ScanArgError and underlying decoding errors).
type ScanError struct {
	origin error
}

func (e ScanError) Error() string {
	return e.origin.Error()
}

func (e ScanError) Unwrap() error {
	return e.origin
}

// TransactionError wraps transaction-level sentinel errors from pgx
// (pgx.ErrTxClosed, pgx.ErrTxCommitRollback).
type TransactionError struct {
	origin error
}

func (e TransactionError) Error() string {
	return e.origin.Error()
}

func (e TransactionError) Unwrap() error {
	return e.origin
}

// wrapPgxError classifies a pgx/pgconn error into one of the typed wrapper
// errors defined in this package. The wrapped error always preserves the
// original via Unwrap, so errors.Is / errors.As against pgx types still works.
//
// Precedence is root-cause first: cancellation and timeouts are checked before
// the structural type that carried them, so a connect attempt that times out
// is reported as TimeoutError rather than ConnectionError.
func wrapPgxError(err error) error {
	if err == nil {
		return nil
	}

	// Context cancellation — checked first since pgconn normalizes a
	// cancel-induced timeout back to context.Canceled.
	if errors.Is(err, context.Canceled) {
		return &ContextCanceledError{origin: err}
	}

	// Timeout — pgconn-reported (context deadline / net timeout) or pool
	// acquire timeout (context.DeadlineExceeded from puddle).
	if pgconn.Timeout(err) || errors.Is(err, context.DeadlineExceeded) {
		return &TimeoutError{origin: err}
	}

	// No rows — pgx.ErrNoRows already wraps sql.ErrNoRows, but check both
	// so plain sql.ErrNoRows is covered too.
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
		return &NoRowsError{origin: err}
	}

	// Closed connection or closed pool.
	if errors.Is(err, pgconn.ErrConnClosed) || errors.Is(err, puddle.ErrClosedPool) {
		return &ConnectionClosedError{origin: err}
	}

	// Transaction-level sentinels.
	if errors.Is(err, pgx.ErrTxClosed) || errors.Is(err, pgx.ErrTxCommitRollback) {
		return &TransactionError{origin: err}
	}

	// Server-side query error.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return &QueryError{
			origin:  err,
			code:    pgErr.Code,
			message: pgErr.Message,
		}
	}

	// Prepared-statement failure (server-side, during prepare).
	var prepErr *pgconn.PrepareError
	if errors.As(err, &prepErr) {
		return &QueryError{origin: err}
	}

	// Scan / decode failure.
	var scanErr pgx.ScanArgError
	if errors.As(err, &scanErr) {
		return &ScanError{origin: err}
	}

	// Connection establishment failure.
	var connErr *pgconn.ConnectError
	if errors.As(err, &connErr) {
		return &ConnectionError{origin: err}
	}

	// Standby / preferred-host resolution failure during connection.
	var notPrefErr *pgconn.NotPreferredError
	if errors.As(err, &notPrefErr) {
		return &ConnectionError{origin: err}
	}

	// Connection-string parse failure.
	var cfgErr *pgconn.ParseConfigError
	if errors.As(err, &cfgErr) {
		return &ConfigParserError{origin: err}
	}

	// Unknown error — pass through unchanged.
	return err
}
