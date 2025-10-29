package eliona

import (
	"context"
	"strconv"

	"github.com/eliona-smart-building-assistant/backend-frm/pkg/azure/identity"
	"github.com/eliona-smart-building-assistant/backend-frm/pkg/postgres"
	"github.com/eliona-smart-building-assistant/backend-frm/pkg/utils"
)

func GetDatabasePool(ctx context.Context, appName string, poolSize int, opts ...postgres.Opt) (*postgres.Pool, error) {
	return getDatabasePool(ctx, appName, poolSize, opts...)
}

func GetDatabasePoolWithOverrideRole(ctx context.Context, role string, appName string, poolSize int, opts ...postgres.Opt) (*postgres.Pool, error) {
	return getDatabasePool(ctx, appName, poolSize, append(opts, postgres.WithOverrideRole(role))...)
}

func getDatabasePool(ctx context.Context, appName string, poolSize int, opts ...postgres.Opt) (*postgres.Pool, error) {
	azClientID := utils.EnvOrDefault("AZURE_CLIENT_ID", "")

	if azClientID != "" {
		return createWorkloadIdentityDbPool(ctx, appName, poolSize, opts...)
	}

	return createDbPool(ctx, appName, poolSize, opts...)
}

func createDbPool(ctx context.Context, appName string, poolSize int, opts ...postgres.Opt) (*postgres.Pool, error) {
	dsn := utils.EnvOrDefault("CONNECTION_STRING", "")

	pool, err := postgres.NewPool(
		ctx,
		append(opts,
			postgres.WithDSN(dsn),
			postgres.WithApplicationName(appName),
			postgres.WithMaxPoolSize(poolSize),
		)...,
	)

	return pool, err
}

func createWorkloadIdentityDbPool(ctx context.Context, appName string, poolSize int, opts ...postgres.Opt) (*postgres.Pool, error) {
	dbHost := utils.EnvOrDefault("PGHOST", "")
	dbPort, _ := strconv.Atoi(utils.EnvOrDefault("PGPORT", "5432"))
	dbUser := utils.EnvOrDefault("PGUSER", "")
	dbName := utils.EnvOrDefault("PGDATABASE", "")

	azureTokenProvider, err := identity.NewWorkloadIdentity()
	if err != nil {
		return nil, err
	}

	azureToken, err := azureTokenProvider.GetToken(ctx, identity.ScopeDefinitionDatabase)
	if err != nil {
		return nil, err
	}

	pool, err := postgres.NewPool(ctx,
		append(opts,
			postgres.WithApplicationName(appName),
			postgres.WithMaxPoolSize(poolSize),
			postgres.AllowCredentialChange(),
			postgres.WithHostname(dbHost),
			postgres.WithPort(dbPort),
			postgres.WithDatabase(dbName),
			postgres.WithCredentials(dbUser, azureToken.Token),
		)...,
	)

	if err != nil {
		return nil, err
	}

	azureTokenProvider.SetAutoRefresh(ctx, azureToken, pool.SetPassword, identity.ScopeDefinitionDatabase)

	return pool, nil
}
