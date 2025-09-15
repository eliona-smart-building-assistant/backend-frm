package eliona

import (
	"reflect"
	"testing"

	"github.com/ProtonMail/gopenpgp/v3/crypto"
)

func TestDecryptPGPSymmetric(t *testing.T) {
	password := []byte("this_is_a_password")
	message := []byte("this is a message")

	encryptor, err := crypto.PGP().Encryption().Password(password).New()

	if err != nil {
		t.Error(err)
	}

	encrypted, err := encryptor.Encrypt(message)
	if err != nil {
		t.Error(err)
	}

	decrypted, err := DecryptPGPSymmetric(encrypted.Bytes(), password)
	if err != nil {
		t.Error(err)
	}

	equal := reflect.DeepEqual(decrypted, message)

	if !equal {
		t.Error("decrypted message is not equal to encrypted message")
	}
}
