package postgres

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultPingTimeout = 1 * time.Second
)

type Pool struct {
	pool        *pgxpool.Pool
	maxPoolSize int
	port        int
	dsn         string
	hostname    string
	appName     string
	credMu      sync.Mutex
	login       string
	password    string
	database    string
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
	poolCfg.BeforeConnect = func(ctx context.Context, connCfg *pgx.ConnConfig) error {
		pool.credMu.Lock()
		connCfg.User = pool.login
		connCfg.Password = pool.password
		pool.credMu.Unlock()

		return nil
	}

	pool.pool, err = pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, wrapPgxError(err)
	}

	pingCtx, pingCtxCancel := context.WithTimeout(ctx, defaultPingTimeout)
	defer pingCtxCancel()

	err = pool.pool.Ping(pingCtx)
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

func (p *Pool) Close() {
	p.pool.Close()
}

func (p *Pool) Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	ct, err := p.pool.Exec(ctx, query, args...)

	return ct, wrapPgxError(err)
}

func (p *Pool) Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	rows, err := p.pool.Query(ctx, query, args...)

	return rows, wrapPgxError(err)
}

// SetCredentials updates login and password that will be used for connections of already created pool.
// All idle connections are reset and will be recreated with new credentials.
// Already acquired connections are not affected.
func (p *Pool) SetCredentials(login string, password string) {
	p.credMu.Lock()

	p.login = login
	p.password = password

	p.pool.Reset()

	p.credMu.Unlock()
}

func (p *Pool) SetPassword(password string) {
	p.credMu.Lock()

	p.password = password

	p.pool.Reset()

	p.credMu.Unlock()
}
