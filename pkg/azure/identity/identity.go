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
	stopChan   chan struct{}
}

func NewWorkloadIdentity() (*WorkloadIdentityProvider, error) {
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

	return &WorkloadIdentityProvider{credential: credential, tenantId: tenantId, stopChan: make(chan struct{})}, nil
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

	w.SetAutoRefresh(token, callback, scopes...)

	return token, nil
}

func (w WorkloadIdentityProvider) SetAutoRefresh(token azcore.AccessToken, callback CallbackFn, scopes ...string) {
	go func() {
		next := getNext(token)

		for {
			select {
			case <-w.stopChan:
				return
			case <-time.After(next):
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				newToken, err := w.GetToken(ctx, scopes...)
				cancel()
				if err != nil {
					panic(err)
				}

				callback(newToken.Token)
				next = getNext(newToken)
			}
		}
	}()
}

func getNext(token azcore.AccessToken) time.Duration {
	refreshAt := token.ExpiresOn.Add(-10 * time.Minute)
	if !token.RefreshOn.IsZero() {
		refreshAt = token.RefreshOn
	}

	return refreshAt.Sub(time.Now())
}
