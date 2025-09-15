package eliona

import (
	"github.com/ProtonMail/gopenpgp/v3/crypto"
)

func DecryptPGPSymmetric(data []byte, password []byte) ([]byte, error) {
	decryptor, err := crypto.PGP().Decryption().Password(password).New()
	if err != nil {
		return nil, err
	}

	decrypted, err := decryptor.Decrypt(data, crypto.Bytes)
	if err != nil {
		return nil, err
	}

	return decrypted.Bytes(), err
}
