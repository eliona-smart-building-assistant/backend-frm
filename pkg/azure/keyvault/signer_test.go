package keyvault

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"testing"
)

func TestSignerRoundtrip(t *testing.T) {
	t.Parallel()

	// Skip test if required environment variables are missing.
	required := []string{
		"KEYVAULT_URL",
		"KEYVAULT_KEY_NAME",
		"AZURE_FEDERATED_TOKEN_FILE",
		"AZURE_CLIENT_ID",
		"AZURE_TENANT_ID",
	}
	for _, key := range required {
		if os.Getenv(key) == "" {
			t.Skipf("Skipping test: missing required environment variable %s", key)
		}
	}

	ctx := context.Background()
	vaultURL := os.Getenv("KEYVAULT_URL")
	keyName := os.Getenv("KEYVAULT_KEY_NAME")

	client, err := GetClient(vaultURL)
	if err != nil {
		t.Fatalf("failed to get Key Vault client: %v", err)
	}

	// Get signer (remote via Key Vault)
	signer, err := client.GetSigner(ctx, keyName, nil)
	if err != nil {
		t.Fatalf("failed to get signer: %v", err)
	}

	// Fetch public key to verify locally
	jwk, err := client.GetPublicKey(ctx, keyName, nil)
	if err != nil {
		t.Fatalf("failed to get public key: %v", err)
	}
	pub, err := JWKToPublicKey(jwk)
	if err != nil {
		t.Fatalf("failed to convert JWK to public key: %v", err)
	}

	// Prepare message and hash
	message := []byte("Hello Azure Key Vault signer!")
	digest := sha256.Sum256(message)

	// Perform signing (Key Vault signs remotely)
	sig, err := signer.Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil {
		t.Fatalf("failed to sign: %v", err)
	}

	t.Logf("Signature (base64): %s", base64.StdEncoding.EncodeToString(sig))

	// Verify the signature using the public key
	switch pub := pub.(type) {
	case *rsa.PublicKey:
		if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig); err != nil {
			t.Fatalf("RSA signature verification failed: %v", err)
		}
	default:
		t.Skipf("unsupported public key type: %T", pub)
	}

	t.Log("✅ Signature verified successfully")
}
