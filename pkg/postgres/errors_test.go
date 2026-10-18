package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/puddle/v2"
)

func TestWrapPgxError(t *testing.T) {
	t.Parallel()

	pgErr := &pgconn.PgError{Severity: "ERROR", Code: "23505", Message: "duplicate key"}
	parseCfgErr := pgconn.NewParseConfigError("postgres://x", "bad", errors.New("noop"))
	scanArgErr := pgx.ScanArgError{ColumnIndex: 2, FieldName: "col", Err: errors.New("cannot decode")}

	// match returns true if err was wrapped as the expected type.
	cases := []struct {
		name    string
		in      error
		match   func(error) bool
		wantNil bool
	}{
		{"nil passthrough", nil, nil, true},
		{"pg error -> QueryError", pgErr, asType[*QueryError], false},
		{"wrapped pg error -> QueryError", fmt.Errorf("exec: %w", pgErr), asType[*QueryError], false},
		{"connect error -> ConnectionError", &pgconn.ConnectError{}, asType[*ConnectionError], false},
		{"not preferred -> ConnectionError", &pgconn.NotPreferredError{}, asType[*ConnectionError], false},
		{"parse config -> ConfigParserError", parseCfgErr, asType[*ConfigParserError], false},
		{"prepare error -> QueryError", &pgconn.PrepareError{}, asType[*QueryError], false},
		{"scan arg -> ScanError", scanArgErr, asType[*ScanError], false},
		{"ctx canceled -> ContextCanceledError", context.Canceled, asType[*ContextCanceledError], false},
		{"ctx canceled wrapped -> ContextCanceledError", fmt.Errorf("acquire: %w", context.Canceled), asType[*ContextCanceledError], false},
		{"ctx deadline -> TimeoutError", context.DeadlineExceeded, asType[*TimeoutError], false},
		{"conn closed -> ConnectionClosedError", pgconn.ErrConnClosed, asType[*ConnectionClosedError], false},
		{"pool closed -> ConnectionClosedError", puddle.ErrClosedPool, asType[*ConnectionClosedError], false},
		{"tx closed -> TransactionError", pgx.ErrTxClosed, asType[*TransactionError], false},
		{"tx commit rollback -> TransactionError", pgx.ErrTxCommitRollback, asType[*TransactionError], false},
		{"pgx no rows -> NoRowsError", pgx.ErrNoRows, asType[*NoRowsError], false},
		{"sql no rows -> NoRowsError", sql.ErrNoRows, asType[*NoRowsError], false},
		{"unknown passthrough", errors.New("something else"), nil, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := wrapPgxError(tc.in)

			if tc.wantNil {
				if got != tc.in {
					t.Fatalf("expected passthrough, got different error: %v", got)
				}
				return
			}

			if !tc.match(got) {
				t.Fatalf("unexpected type: got %T: %v", got, got)
			}

			// Unwrap must preserve the original instance so errors.Is works.
			// errors.Is never calls Error(), so nil-state pgconn values are safe.
			if !errors.Is(got, tc.in) {
				t.Fatalf("wrapped error does not unwrap to original: got %v", got)
			}
		})
	}
}

// asType returns true if err wraps into the (pointer) type T.
func asType[T any](err error) bool {
	var target T
	return errors.As(err, &target)
}

func TestQueryErrorPopulatesFields(t *testing.T) {
	t.Parallel()

	src := &pgconn.PgError{Severity: "ERROR", Code: "42P01", Message: "relation does not exist"}
	wrapped := wrapPgxError(src)

	var qe *QueryError
	if !errors.As(wrapped, &qe) {
		t.Fatalf("expected *QueryError, got %T", wrapped)
	}
	if qe.Code() != "42P01" {
		t.Errorf("Code: got %q, want %q", qe.Code(), "42P01")
	}
	if qe.Message() != "relation does not exist" {
		t.Errorf("Message: got %q, want %q", qe.Message(), "relation does not exist")
	}

	// The original PgError must still be reachable via errors.As on the wrapped value.
	var pe *pgconn.PgError
	if !errors.As(wrapped, &pe) {
		t.Fatalf("underlying *pgconn.PgError not reachable via errors.As")
	}
}

func TestTimeoutPrecedence(t *testing.T) {
	t.Parallel()

	// Root-cause first: a timeout nested anywhere in the chain must classify as
	// TimeoutError. (For a real *pgconn.ConnectError whose inner err is a
	// timeout the same applies, since the timeout check precedes the
	// ConnectError branch; that exact nesting can't be built outside pgconn
	// because the err field is unexported.)
	deepTimeout := fmt.Errorf("connect attempt: %w", context.DeadlineExceeded)

	got := wrapPgxError(deepTimeout)

	var te *TimeoutError
	if !errors.As(got, &te) {
		t.Fatalf("expected *TimeoutError, got %T: %v", got, got)
	}
}
