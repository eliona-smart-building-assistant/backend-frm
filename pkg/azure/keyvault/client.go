package keyvault

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"errors"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys"
	"math/big"
	"os"
	"strings"
	"sync"
)

var (
	clientCache sync.Map // map[string]*Client
)

type Client struct {
	client   *azkeys.Client
	vaultURL string

	// immutable cache by version: "<keyName>|<version>" -> *rsa.PublicKey
	byVersion sync.Map

	// TTL cache for "latest" per keyName
	latest map[string]*azkeys.JSONWebKey
}

// GetClient returns a cached Key Vault client for the given vaultURL.
// It reuses an existing client if already created; otherwise, it calls NewClient().
func GetClient(vaultURL string) (*Client, error) {
	vaultURL = strings.TrimSpace(vaultURL)
	if vaultURL == "" {
		return nil, fmt.Errorf("vaultURL is required")
	}

	// Fast path: return cached client if present
	if v, ok := clientCache.Load(vaultURL); ok {
		return v.(*Client), nil
	}

	// Slow path: create new client
	c, err := NewClient(vaultURL)
	if err != nil {
		return nil, err
	}

	// Store and return the one actually stored (avoid races)
	actual, _ := clientCache.LoadOrStore(vaultURL, c)
	return actual.(*Client), nil
}

// NewClient returns a new Key Vault client for the given vaultURL.
// vaultURL example: "https://<your-vault>.vault.azure.net/"
func NewClient(vaultURL string) (*Client, error) {
	var cred azcore.TokenCredential
	wi := &azidentity.WorkloadIdentityCredentialOptions{
		TenantID:      strings.TrimSpace(os.Getenv("AZURE_TENANT_ID")),
		ClientID:      strings.TrimSpace(os.Getenv("AZURE_CLIENT_ID")),
		TokenFilePath: strings.TrimSpace(os.Getenv("AZURE_FEDERATED_TOKEN_FILE")),
	}
	var err error
	cred, err = azidentity.NewWorkloadIdentityCredential(wi)
	if err != nil {
		return nil, fmt.Errorf("workload identity credential init failed: %w", err)
	}

	kc, err := azkeys.NewClient(vaultURL, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating azkeys client: %w", err)
	}

	return &Client{
		client:   kc,
		vaultURL: strings.TrimSpace(vaultURL),
		latest:   make(map[string]*azkeys.JSONWebKey),
	}, nil
}

// GetPublicKey returns the RSA public key for a specific *version* (if provided) of a key.
// The result is cached forever for that (keyName, version) pair.
func (c *Client) GetPublicKey(ctx context.Context, keyName string, version *string) (*azkeys.JSONWebKey, error) {
	v := ""
	if version != nil {
		v = *version
	}
	cacheKey := keyName + "|" + v
	if v, ok := c.byVersion.Load(cacheKey); ok {
		return v.(*azkeys.JSONWebKey), nil
	}

	resp, err := c.client.GetKey(ctx, keyName, v, nil)
	if err != nil {
		return nil, err
	}

	if resp.Key == nil || resp.Key.Kty == nil {
		return nil, errors.New("invalid or missing JWK fields")
	}

	c.byVersion.Store(cacheKey, resp.Key)
	return resp.Key, nil
}

// JWKToPublicKey converts an Azure Key Vault JSONWebKey to a crypto.PublicKey.
// Supports RSA / RSA-HSM and EC / EC-HSM (P-256, P-256K, P-384, P-521).
func JWKToPublicKey(jwk *azkeys.JSONWebKey) (crypto.PublicKey, error) {
	if jwk == nil || jwk.Kty == nil {
		return nil, errors.New("invalid or missing JWK fields")
	}

	switch *jwk.Kty {
	case azkeys.KeyTypeRSA, azkeys.KeyTypeRSAHSM:
		return jwkToRSAPublic(jwk)

	case azkeys.KeyTypeEC, azkeys.KeyTypeECHSM:
		return jwkToECPublic(jwk)

	case azkeys.KeyTypeOct, azkeys.KeyTypeOctHSM:
		return nil, errors.New("symmetric (oct) keys have no public component")

	default:
		return nil, fmt.Errorf("%w: %q", errors.New("unsupported key type"), *jwk.Kty)
	}
}

func jwkToRSAPublic(jwk *azkeys.JSONWebKey) (*rsa.PublicKey, error) {
	if len(jwk.N) == 0 || len(jwk.E) == 0 {
		return nil, errors.New("invalid or missing JWK fields")
	}
	// big-endian bytes -> int
	e := 0
	for _, b := range jwk.E {
		e = (e << 8) | int(b)
	}
	if e <= 1 || e%2 == 0 {
		return nil, fmt.Errorf("invalid RSA exponent: %d", e)
	}
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(jwk.N),
		E: e,
	}, nil
}

func jwkToECPublic(jwk *azkeys.JSONWebKey) (*ecdsa.PublicKey, error) {
	if jwk.Crv == nil || len(jwk.X) == 0 || len(jwk.Y) == 0 {
		return nil, errors.New("invalid or missing JWK fields")
	}

	var curve elliptic.Curve
	switch *jwk.Crv {
	case azkeys.CurveNameP256:
		curve = elliptic.P256()
	case azkeys.CurveNameP384:
		curve = elliptic.P384()
	case azkeys.CurveNameP521:
		curve = elliptic.P521()
	case azkeys.CurveNameP256K:
		// Go stdlib doesn't provide P-256K (secp256k1) on elliptic.
		// If you need it, plug in a third-party curve implementation (e.g., btcsuite/btcd/btcec/v2)
		// and replace this branch with that curve.
		return nil, fmt.Errorf("%w: curve %q requires external implementation (secp256k1)", errors.New("unsupported EC curve"), *jwk.Crv)
	default:
		return nil, fmt.Errorf("%w: %q", errors.New("unsupported EC curve"), *jwk.Crv)
	}

	x := new(big.Int).SetBytes(jwk.X)
	y := new(big.Int).SetBytes(jwk.Y)

	// Optional sanity check: point must be on the curve
	if !curve.IsOnCurve(x, y) {
		return nil, fmt.Errorf("EC point not on curve %v", *jwk.Crv)
	}

	return &ecdsa.PublicKey{Curve: curve, X: x, Y: y}, nil
}
