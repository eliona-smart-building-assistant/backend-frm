package keyvault

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"errors"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys"
	"io"
)

// ---------------- Remote signer ----------------

// kvSigner implements crypto.Signer but delegates signing to Azure Key Vault.
type kvSigner struct {
	client  *azkeys.Client
	ctx     context.Context
	keyName string
	version string // resolved version ("" means latest at creation time)
	pub     crypto.PublicKey
}

// Public implements crypto.Signer
func (s *kvSigner) Public() crypto.PublicKey { return s.pub }

// Sign implements crypto.Signer. It expects the **digest** according to opts.HashFunc().
func (s *kvSigner) Sign(_ io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	if s == nil || s.client == nil || s.pub == nil {
		return nil, errors.New("signer not initialized")
	}

	var alg azkeys.SignatureAlgorithm

	switch p := s.pub.(type) {
	case *rsa.PublicKey:
		// PSS vs PKCS#1 v1.5 is determined by opts type
		if psso, ok := opts.(*rsa.PSSOptions); ok {
			switch psso.Hash {
			case crypto.SHA256:
				alg = azkeys.SignatureAlgorithmPS256
			case crypto.SHA384:
				alg = azkeys.SignatureAlgorithmPS384
			case crypto.SHA512:
				alg = azkeys.SignatureAlgorithmPS512
			default:
				return nil, fmt.Errorf("unsupported RSA-PSS hash: %v", psso.Hash)
			}
		} else {
			switch opts.HashFunc() {
			case crypto.SHA256:
				alg = azkeys.SignatureAlgorithmRS256
			case crypto.SHA384:
				alg = azkeys.SignatureAlgorithmRS384
			case crypto.SHA512:
				alg = azkeys.SignatureAlgorithmRS512
			default:
				return nil, fmt.Errorf("unsupported RSA-PKCS1 hash: %v", opts.HashFunc())
			}
		}

	case *ecdsa.PublicKey:
		// Map curve/hash to ES algorithms (Key Vault returns ASN.1 DER for ECDSA)
		switch p.Curve {
		case elliptic.P256():
			if opts.HashFunc() != crypto.SHA256 {
				return nil, fmt.Errorf("ECDSA P-256 requires SHA-256, got %v", opts.HashFunc())
			}
			alg = azkeys.SignatureAlgorithmES256
		case elliptic.P384():
			if opts.HashFunc() != crypto.SHA384 {
				return nil, fmt.Errorf("ECDSA P-384 requires SHA-384, got %v", opts.HashFunc())
			}
			alg = azkeys.SignatureAlgorithmES384
		case elliptic.P521():
			if opts.HashFunc() != crypto.SHA512 {
				return nil, fmt.Errorf("ECDSA P-521 requires SHA-512, got %v", opts.HashFunc())
			}
			alg = azkeys.SignatureAlgorithmES512
		default:
			return nil, fmt.Errorf("unsupported ECDSA curve")
		}

	default:
		return nil, fmt.Errorf("unsupported public key type %T", s.pub)
	}

	resp, err := s.client.Sign(s.ctx, s.keyName, s.version, azkeys.SignParameters{
		Algorithm: &alg,
		Value:     digest, // digest (hash) of the message, not the raw bytes
	}, nil)
	if err != nil {
		return nil, err
	}
	return resp.Result, nil
}
