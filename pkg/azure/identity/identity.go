package identity

import (
	"context"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/eliona-smart-building-assistant/backend-frm/pkg/log"
	"github.com/eliona-smart-building-assistant/backend-frm/pkg/utils"
)

const (
	ScopeDefinitionDatabase = "https://ossrdbms-aad.database.windows.net/.default"
)

type CallbackFn func(string)

type WorkloadIdentityProvider struct {
	credential *azidentity.WorkloadIdentityCredential
	tenantId   string
	logger     log.Logger
}

func NewWorkloadIdentity( /*logger log.Logger*/ ) (*WorkloadIdentityProvider, error) {
	tenantId := utils.EnvOrDefault("AZURE_TENANT_ID", "")
	clientId := utils.EnvOrDefault("AZURE_CLIENT_ID", "")

	credential, err := azidentity.NewWorkloadIdentityCredential(&azidentity.WorkloadIdentityCredentialOptions{
		ClientID:      clientId,
		TenantID:      tenantId,
		TokenFilePath: utils.EnvOrDefault("AZURE_FEDERATED_TOKEN_FILE", ""),
	})

	if err != nil {
		return nil, err
	}

	//ownLogger := logger.With().
	//	Str("module", "workload-identity").
	//	Str("azure_tenant_id", tenantId).
	//	Str("azure_client_id", clientId).
	//	Logger()

	return &WorkloadIdentityProvider{credential: credential, tenantId: tenantId /*, logger: &ownLogger*/}, nil
}

func (w WorkloadIdentityProvider) GetToken(ctx context.Context, scopes ...string) (azcore.AccessToken, error) {
	opts := policy.TokenRequestOptions{Scopes: scopes, TenantID: w.tenantId}

	token, err := w.credential.GetToken(ctx, opts)
	if err != nil {
		return azcore.AccessToken{}, err
	}

	return token, nil
}

func (w WorkloadIdentityProvider) GetTokenWithAutoRefresh(ctx context.Context, callback CallbackFn, scopes ...string) (azcore.AccessToken, error) {
	token, err := w.GetToken(ctx, scopes...)
	if err != nil {
		return azcore.AccessToken{}, err
	}

	w.SetAutoRefresh(ctx, token, callback, scopes...)

	return token, nil
}

func (w WorkloadIdentityProvider) SetAutoRefresh(ctx context.Context, token azcore.AccessToken, callback CallbackFn, scopes ...string) {
	go func() {
		next := getNext(token)

		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(next):
				newToken, err := w.GetToken(ctx, scopes...)
				if err != nil {
					//w.logger.Error().Err(err).Msg("failed to refresh token")
					//continue
					panic(err)
				}

				callback(newToken.Token)
				next = getNext(newToken)
			}
		}
	}()
}

func getNext(token azcore.AccessToken) time.Duration {
	refreshAt := token.ExpiresOn.Add(-5 * time.Minute)
	if !token.RefreshOn.IsZero() {
		refreshAt = token.RefreshOn
	}

	return refreshAt.Sub(time.Now())
}
