package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

const (
	defaultPingTimeout = 1 * time.Second
)

type Pool struct {
	pool                  *pgxpool.Pool
	maxPoolSize           int
	port                  int
	dsn                   string
	hostname              string
	appName               string
	credMu                sync.Mutex
	allowCredentialChange bool
	asyncCommits          bool
	resetOnAcquire        bool
	overrideRole          string
	afterConnectFuncs     []func(ctx context.Context, conn *pgx.Conn) error
	afterReleaseFuncs     []func(conn *pgx.Conn) bool
	login                 string
	password              string
	database              string
}

func defaultPool() *Pool {
	hostname, _ := os.Hostname()

	return &Pool{
		maxPoolSize: 4,
		appName:     hostname,
	}
}

func NewPool(ctx context.Context, opts ...Opt) (*Pool, error) {
	pool := defaultPool()

	for _, opt := range opts {
		opt(pool)
	}

	var poolCfg *pgxpool.Config
	var err error

	if len(pool.dsn) > 0 {
		poolCfg, err = poolConfigFromDSN(pool.dsn)
	} else {
		poolCfg, err = poolConfigFromFields(pool.hostname, pool.port, pool.login, pool.password, pool.database)
	}

	if err != nil {
		return nil, wrapPgxError(err)
	}

	poolCfg.ConnConfig.RuntimeParams["application_name"] = pool.appName
	poolCfg.MaxConns = int32(pool.maxPoolSize)

	if pool.allowCredentialChange {
		poolCfg.BeforeConnect = func(ctx context.Context, connCfg *pgx.ConnConfig) error {
			pool.credMu.Lock()
			connCfg.User = pool.login
			connCfg.Password = pool.password
			pool.credMu.Unlock()

			return nil
		}
	}

	if pool.overrideRole != "" {
		pool.afterConnectFuncs = append(pool.afterConnectFuncs, func(ctx context.Context, conn *pgx.Conn) error {
			_, execErr := conn.Exec(ctx, "SET ROLE "+pool.overrideRole)
			return execErr
		})
	}

	if pool.asyncCommits {
		pool.afterConnectFuncs = append(pool.afterConnectFuncs, func(ctx context.Context, conn *pgx.Conn) error {
			_, execErr := conn.Exec(ctx, "SET SYNCHRONOUS_COMMIT TO OFF")
			return execErr
		})
	}

	if pool.resetOnAcquire {
		poolCfg.PrepareConn = func(ctx context.Context, conn *pgx.Conn) (bool, error) {
			_, execErr := conn.Exec(ctx, "SET request.jwt.claims='{}';")

			return true, execErr
		}
	}

	if len(pool.afterConnectFuncs) > 0 {
		poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
			for _, fn := range pool.afterConnectFuncs {
				execErr := fn(ctx, conn)
				if execErr != nil {
					return execErr
				}
			}

			return nil
		}
	}

	if len(pool.afterReleaseFuncs) > 0 {
		poolCfg.AfterRelease = func(conn *pgx.Conn) bool {
			for _, fn := range pool.afterReleaseFuncs {
				if !fn(conn) {
					return false
				}
			}

			return true
		}
	}

	pool.pool, err = pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, wrapPgxError(err)
	}

	err = pool.pool.Ping(ctx)
	if err != nil {
		return nil, wrapPgxError(err)
	}

	return pool, nil
}

func poolConfigFromFields(host string, port int, user string, password string, db string) (*pgxpool.Config, error) {
	values := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s", host, port, user, password, db)
	return pgxpool.ParseConfig(values)
}

func poolConfigFromDSN(dsn string) (*pgxpool.Config, error) {
	return pgxpool.ParseConfig(dsn)
}

func (p *Pool) Close(ctx context.Context) error {
	p.pool.Close()

	return nil
}

func (p *Pool) Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	ct, err := p.pool.Exec(ctx, query, args...)

	return ct, wrapPgxError(err)
}

func (p *Pool) Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	rows, err := p.pool.Query(ctx, query, args...)

	return rows, wrapPgxError(err)
}

func (p *Pool) ExecAs(ctx context.Context, role string, query string, args ...interface{}) (pgconn.CommandTag, error) {
	conn, err := p.AcquireConn(ctx)
	if err != nil {
		return pgconn.CommandTag{}, err
	}

	defer conn.Release()

	_, err = conn.Exec(ctx, "SET ROLE "+role)
	if err != nil {
		return pgconn.CommandTag{}, err
	}

	ct, err := p.Exec(ctx, query, args...)

	_, _ = conn.Exec(ctx, "RESET ROLE")

	if err != nil {
		return pgconn.CommandTag{}, err
	}

	return ct, nil
}

func (p *Pool) QueryAs(ctx context.Context, role string, query string, args ...interface{}) (pgx.Rows, error) {
	conn, err := p.AcquireConn(ctx)
	if err != nil {
		return nil, err
	}

	defer conn.Release()

	_, err = conn.Exec(ctx, "SET ROLE "+role)
	if err != nil {
		return nil, err
	}

	rows, err := p.Query(ctx, query, args...)

	_, _ = conn.Exec(ctx, "RESET ROLE")

	if err != nil {
		return nil, err
	}

	return rows, nil
}

// AcquireConn returns a lower-level connection. You must release it via .Release().
func (p *Pool) AcquireConn(ctx context.Context) (*pgxpool.Conn, error) {
	return p.pool.Acquire(ctx)
}

// Pool return lower-level Pool object from pgx
func (p *Pool) Pool() *pgxpool.Pool {
	return p.pool
}

// StdlibDB returns a *sql.DB that uses the underlying pgx pool.
func (p *Pool) StdlibDB() *sql.DB {
	db := stdlib.OpenDBFromPool(p.pool)

	return db
}

func (p *Pool) Tx(ctx context.Context) (pgx.Tx, error) {
	return p.pool.BeginTx(ctx, pgx.TxOptions{})
}

// SetCredentials updates login and password that will be used for connections of an already created pool.
// All idle connections are reset and will be recreated with new credentials.
// Already acquired connections are not affected.
func (p *Pool) SetCredentials(login string, password string) {
	if !p.allowCredentialChange {
		return
	}

	p.credMu.Lock()

	p.login = login
	p.password = password

	p.pool.Reset()

	p.credMu.Unlock()
}

func (p *Pool) SetPassword(password string) {
	if !p.allowCredentialChange {
		return
	}

	p.credMu.Lock()

	p.password = password

	p.pool.Reset()

	p.credMu.Unlock()
}
