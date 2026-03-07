package hash

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

func sha256FromReader(r io.Reader) (sum string, size int64, err error) {
	h := sha256.New()
	n, err := io.Copy(h, r)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func SHA256FromFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	return sha256FromReader(f)
}

func SHA256FromBytes(data []byte) (string, int64) {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), int64(len(data))
}
