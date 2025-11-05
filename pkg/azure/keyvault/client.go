package keyvault

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"errors"
	"fmt"
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

	// immutable cache by version: "<keyName>|<version>" -> *azkeys.JSONWebKey
	byVersion sync.Map

	// TTL cache for "latest" per keyName (not strictly needed for signer, kept from your code)
	muLatest sync.RWMutex
	latest   map[string]*azkeys.JSONWebKey

	// cached signers: "<keyName>|<version>" -> crypto.Signer
	signers sync.Map
}

// GetClient returns a cached Key Vault client for the given vaultURL.
func GetClient(vaultURL string) (*Client, error) {
	vaultURL = strings.TrimSpace(vaultURL)
	if vaultURL == "" {
		return nil, fmt.Errorf("vaultURL is required")
	}

	if v, ok := clientCache.Load(vaultURL); ok {
		return v.(*Client), nil
	}

	c, err := NewClient(vaultURL)
	if err != nil {
		return nil, err
	}

	actual, _ := clientCache.LoadOrStore(vaultURL, c)
	return actual.(*Client), nil
}

// NewClient returns a new Key Vault client for the given vaultURL.
// vaultURL example: "https://<your-vault>.vault.azure.net/"
func NewClient(vaultURL string) (*Client, error) {
	wi := &azidentity.WorkloadIdentityCredentialOptions{
		TenantID:      strings.TrimSpace(os.Getenv("AZURE_TENANT_ID")),
		ClientID:      strings.TrimSpace(os.Getenv("AZURE_CLIENT_ID")),
		TokenFilePath: strings.TrimSpace(os.Getenv("AZURE_FEDERATED_TOKEN_FILE")),
	}
	cred, err := azidentity.NewWorkloadIdentityCredential(wi)
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

// ---------------- Public key helpers ----------------

// GetPublicKey returns the JWK for a specific *version* (if provided) of a key.
// Result is cached forever for that (keyName, version) pair.
// Pass version=nil or version="" to resolve "latest".
func (c *Client) GetPublicKey(ctx context.Context, keyName string, version *string) (*azkeys.JSONWebKey, error) {
	v := ""
	if version != nil {
		v = *version
	}
	cacheKey := keyName + "|" + v
	if got, ok := c.byVersion.Load(cacheKey); ok {
		return got.(*azkeys.JSONWebKey), nil
	}

	resp, err := c.client.GetKey(ctx, keyName, v, nil)
	if err != nil {
		return nil, err
	}
	if resp.Key == nil || resp.Key.Kty == nil {
		return nil, errors.New("invalid or missing JWK fields")
	}

	if v == "" && resp.Key.KID != nil {
		if _, _, rv := ParseKeyID(resp.Key.KID); rv != "" {
			cacheKey = keyName + "|" + rv
		}
	}
	c.byVersion.Store(cacheKey, resp.Key)

	return resp.Key, nil
}

// GetSigner returns a cached crypto.Signer that uses Key Vault to sign with the given key/version.
// If version == nil or empty, it resolves the current version and caches the signer under that version.
func (c *Client) GetSigner(ctx context.Context, keyName string, version *string) (crypto.Signer, error) {
	if strings.TrimSpace(keyName) == "" {
		return nil, fmt.Errorf("keyName is required")
	}
	ver := ""
	if version != nil {
		ver = strings.TrimSpace(*version)
	}
	cacheKey := keyName + "|" + ver

	// If we already have a signer cached under this (name|version) key, return it.
	if s, ok := c.signers.Load(cacheKey); ok {
		return s.(crypto.Signer), nil
	}

	// Fetch JWK (also seeds byVersion cache). If latest, resolve concrete version.
	resp, err := c.client.GetKey(ctx, keyName, ver, nil)
	if err != nil {
		return nil, err
	}
	if resp.Key == nil || resp.Key.Kty == nil {
		return nil, errors.New("invalid or missing JWK fields")
	}

	pub, err := JWKToPublicKey(resp.Key)
	if err != nil {
		return nil, err
	}

	resolvedVersion := ver
	if resolvedVersion == "" && resp.Key.KID != nil {
		if _, _, rv := ParseKeyID(resp.Key.KID); rv != "" {
			resolvedVersion = rv
			c.byVersion.Store(keyName+"|"+rv, resp.Key)
		}
	}

	kvs := &kvSigner{
		client:  c.client,
		ctx:     ctx,
		keyName: keyName,
		version: resolvedVersion,
		pub:     pub,
	}

	// Cache under the requested key (name|ver). If ver == "", cache under "" and also under resolved version for future direct lookups.
	actual, _ := c.signers.LoadOrStore(cacheKey, kvs)
	if resolvedVersion != "" && ver == "" {
		c.signers.Store(keyName+"|"+resolvedVersion, actual)
	}

	return actual.(crypto.Signer), nil
}

// ParseKeyID extracts the Vault URL, key name, and version (if present)
// from an Azure Key Vault Key ID (KID). It works for both *azkeys.ID and string values.
//
// Examples:
//
//	vaultURL, keyName, version := keyvault.ParseKeyID("https://myvault.vault.azure.net/keys/my-key/abcd1234")
//	vaultURL, keyName, version := keyvault.ParseKeyID(resp.Key.KID)
//
// Results:
//
//	vaultURL = "https://myvault.vault.azure.net"
//	keyName  = "my-key"
//	version  = "abcd1234"
//
// If the Key ID has no version (e.g. ends with /keys/my-key), version will be "".
func ParseKeyID(kid any) (vaultURL, keyName, version string) {
	var id string

	switch v := kid.(type) {
	case string:
		id = strings.TrimSpace(v)
	case *string:
		if v != nil {
			id = strings.TrimSpace(*v)
		}
	case azkeys.ID:
		id = strings.TrimSpace(string(v))
	case *azkeys.ID:
		if v != nil {
			id = strings.TrimSpace(string(*v))
		}
	default:
		return "", "", ""
	}

	if id == "" {
		return "", "", ""
	}

	// Expected format:
	// https://<vault>.vault.azure.net/keys/<keyName>/<version?>
	parts := strings.Split(strings.Trim(id, "/"), "/")
	if len(parts) < 3 {
		// Unexpected shape — just return the whole thing as vaultURL
		return id, "", ""
	}

	// Find the "keys" segment
	for i := 0; i < len(parts); i++ {
		if strings.EqualFold(parts[i], "keys") {
			vaultURL = strings.Join(parts[:i], "/")
			keyName = parts[i+1]
			if len(parts) > i+2 {
				version = parts[i+2]
			}
			return
		}
	}

	// Fallback: not a normal key path
	return id, "", ""
}

// JWKToPublicKey converts an Azure Key Vault JSONWebKey to a crypto.PublicKey.
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
		return nil, fmt.Errorf("unsupported key type %q", *jwk.Kty)
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
		// Not in stdlib; add btcec if you need secp256k1
		return nil, fmt.Errorf("unsupported EC curve %q (secp256k1 requires external lib)", *jwk.Crv)
	default:
		return nil, fmt.Errorf("unsupported EC curve %q", *jwk.Crv)
	}
	x := new(big.Int).SetBytes(jwk.X)
	y := new(big.Int).SetBytes(jwk.Y)
	if !curve.IsOnCurve(x, y) {
		return nil, fmt.Errorf("EC point not on curve %v", *jwk.Crv)
	}
	return &ecdsa.PublicKey{Curve: curve, X: x, Y: y}, nil
}
