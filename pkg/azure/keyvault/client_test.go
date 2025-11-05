package keyvault

import (
	"context"
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
