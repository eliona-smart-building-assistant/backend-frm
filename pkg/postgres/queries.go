package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var ErrTooManyRows = errors.New("too many rows")

type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

type Execer interface {
	Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
}

func CollectRowsToStruct[T any](ctx context.Context, q Querier, sql string, args ...any) ([]T, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, wrapPgxError(err)
	}

	collected, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[T])

	return collected, wrapPgxError(err)
}

func CollectOneRowToStruct[T any](ctx context.Context, q Querier, sql string, args ...any) (T, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		var empty T
		return empty, wrapPgxError(err)
	}

	collected, err := pgx.CollectOneRow(rows, pgx.RowToStructByNameLax[T])
	if err != nil {
		var empty T
		return empty, wrapPgxError(err)
	}

	if rows.CommandTag().RowsAffected() > 1 {
		return collected, ErrTooManyRows
	}

	return collected, nil
}

func CollectColumn[T any](ctx context.Context, q Querier, sql string, args ...any) ([]T, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, wrapPgxError(err)
	}

	collected, err := pgx.CollectRows(rows, pgx.RowTo[T])
	if err != nil {
		return nil, wrapPgxError(err)
	}

	return collected, nil
}

func CollectSingleValue[T any](ctx context.Context, q Querier, sql string, args ...any) (T, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		var empty T
		return empty, wrapPgxError(err)
	}

	collected, err := pgx.CollectOneRow(rows, pgx.RowTo[T])
	if err != nil {
		var empty T
		return empty, wrapPgxError(err)
	}

	if rows.CommandTag().RowsAffected() > 1 {
		return collected, ErrTooManyRows
	}

	return collected, nil
}
