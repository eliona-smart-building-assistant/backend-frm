package keyvault

import (
	"context"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys"
	"os"
	"testing"
	"time"
)

func TestGetPublicKey(t *testing.T) {
	t.Parallel()

	// Skip if required env vars are missing (keeps `go test ./...` fast by default).
	required := []string{
		"KEYVAULT_URL",
		"KEYVAULT_KEY_NAME",
		"AZURE_FEDERATED_TOKEN_FILE",
		"AZURE_CLIENT_ID",
		"AZURE_TENANT_ID",
	}
	for _, k := range required {
		if _, ok := os.LookupEnv(k); !ok {
			t.Skipf("skipping: %s not set", k)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	client, err := GetClient(os.Getenv("KEYVAULT_URL"))
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}

	// Version is optional: pass nil to use the latest.
	var version *string
	if v := os.Getenv("KEYVAULT_KEY_VERSION"); v != "" {
		version = &v
	}

	publicKey, err := client.GetPublicKey(ctx, os.Getenv("KEYVAULT_KEY_NAME"), version)
	if err != nil {
		t.Fatalf("GetPublicKey: %v", err)
	}
	if publicKey == nil {
		t.Fatal("GetPublicKey: got nil JWK")
	}
	if publicKey.KID == nil || *publicKey.KID == "" {
		t.Fatal("GetPublicKey: missing KID")
	}
	if publicKey.Kty == nil || *publicKey.Kty == "" {
		t.Fatal("GetPublicKey: missing Kty")
	}
	t.Logf("fetched key: kid=%s, kty=%s", *publicKey.KID, *publicKey.Kty)

	key, err := JWKToPublicKey(publicKey)
	if err != nil {
		t.Fatalf("JWKToPublicKey: %v", err)
	}
	if key == nil {
		t.Fatal("JWKToPublicKey: got nil key")
	}
	t.Logf("converted public key type: %T", key)
}

func TestParseKeyID(t *testing.T) {
	tests := []struct {
		name      string
		input     any
		wantVault string
		wantKey   string
		wantVer   string
	}{
		{
			name:      "full key id with version",
			input:     "https://myvault.vault.azure.net/keys/my-key/abcd1234",
			wantVault: "https://myvault.vault.azure.net",
			wantKey:   "my-key",
			wantVer:   "abcd1234",
		},
		{
			name:      "key id without version",
			input:     "https://myvault.vault.azure.net/keys/my-key",
			wantVault: "https://myvault.vault.azure.net",
			wantKey:   "my-key",
			wantVer:   "",
		},
		{
			name:      "uppercase KEYS segment",
			input:     "https://myvault.vault.azure.net/KEYS/testkey/xyz",
			wantVault: "https://myvault.vault.azure.net",
			wantKey:   "testkey",
			wantVer:   "xyz",
		},
		{
			name:      "input is *azkeys.ID",
			input:     azkeys.ID("https://other.vault.azure.net/keys/foo/123"),
			wantVault: "https://other.vault.azure.net",
			wantKey:   "foo",
			wantVer:   "123",
		},
		{
			name:      "malformed url no keys segment",
			input:     "https://weird.vault.azure.net/notkeys/foo/bar",
			wantVault: "https://weird.vault.azure.net/notkeys/foo/bar",
			wantKey:   "",
			wantVer:   "",
		},
		{
			name:      "empty input",
			input:     "",
			wantVault: "",
			wantKey:   "",
			wantVer:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVault, gotKey, gotVer := ParseKeyID(tt.input)
			if gotVault != tt.wantVault || gotKey != tt.wantKey || gotVer != tt.wantVer {
				t.Errorf("ParseKeyID(%v) = (%q, %q, %q), want (%q, %q, %q)",
					tt.input, gotVault, gotKey, gotVer,
					tt.wantVault, tt.wantKey, tt.wantVer)
			}
		})
	}
}
