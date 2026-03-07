package cipher

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
	"os"
)

func readKeyDerive(keyPath string) ([]byte, error) {
	kb, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(kb)
	return sum[:], nil
}

func EncryptBytes(keyPath string, data []byte) ([]byte, error) {
	key, err := readKeyDerive(keyPath)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ct := gcm.Seal(nil, nonce, data, nil)
	out := append(nonce, ct...)
	return out, nil
}

func DecryptBytes(keyPath string, data []byte) ([]byte, error) {
	key, err := readKeyDerive(keyPath)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(data) < ns {
		return nil, errors.New("ciphertext too short")
	}
	nonce := data[:ns]
	ct := data[ns:]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, err
	}
	return pt, nil
}
