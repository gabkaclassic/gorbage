package cipher

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadKeyDerive(t *testing.T) {
	tests := []struct {
		name    string
		keyPath string
		setup   func() (string, func())
		want    []byte
		wantErr bool
	}{
		{
			name: "successful key read and hash",
			setup: func() (string, func()) {
				tmpfile, err := os.CreateTemp("", "testkey")
				if err != nil {
					t.Fatal(err)
				}
				content := []byte("test-key-content")
				if _, err := tmpfile.Write(content); err != nil {
					t.Fatal(err)
				}
				if err := tmpfile.Close(); err != nil {
					t.Fatal(err)
				}
				return tmpfile.Name(), func() { os.Remove(tmpfile.Name()) }
			},
			want: func() []byte {
				sum := sha256.Sum256([]byte("test-key-content"))
				return sum[:]
			}(),
			wantErr: false,
		},
		{
			name: "empty file",
			setup: func() (string, func()) {
				tmpfile, err := os.CreateTemp("", "emptykey")
				if err != nil {
					t.Fatal(err)
				}
				if err := tmpfile.Close(); err != nil {
					t.Fatal(err)
				}
				return tmpfile.Name(), func() { os.Remove(tmpfile.Name()) }
			},
			want: func() []byte {
				sum := sha256.Sum256([]byte{})
				return sum[:]
			}(),
			wantErr: false,
		},
		{
			name:    "file not found",
			keyPath: "/nonexistent/path/to/key",
			setup: func() (string, func()) {
				return "/nonexistent/path/to/key", func() {}
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "large file",
			setup: func() (string, func()) {
				tmpfile, err := os.CreateTemp("", "largekey")
				if err != nil {
					t.Fatal(err)
				}
				content := make([]byte, 1024*1024)
				for i := range content {
					content[i] = byte(i % 256)
				}
				if _, err := tmpfile.Write(content); err != nil {
					t.Fatal(err)
				}
				if err := tmpfile.Close(); err != nil {
					t.Fatal(err)
				}
				return tmpfile.Name(), func() { os.Remove(tmpfile.Name()) }
			},
			want: func() []byte {
				content := make([]byte, 1024*1024)
				for i := range content {
					content[i] = byte(i % 256)
				}
				sum := sha256.Sum256(content)
				return sum[:]
			}(),
			wantErr: false,
		},
		{
			name: "file with special characters",
			setup: func() (string, func()) {
				tmpfile, err := os.CreateTemp("", "specialkey")
				if err != nil {
					t.Fatal(err)
				}
				content := []byte("!@#$%^&*()_+{}[]|\\:;\"'<>,.?/~`")
				if _, err := tmpfile.Write(content); err != nil {
					t.Fatal(err)
				}
				if err := tmpfile.Close(); err != nil {
					t.Fatal(err)
				}
				return tmpfile.Name(), func() { os.Remove(tmpfile.Name()) }
			},
			want: func() []byte {
				sum := sha256.Sum256([]byte("!@#$%^&*()_+{}[]|\\:;\"'<>,.?/~`"))
				return sum[:]
			}(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var keyPath string
			var cleanup func()

			if tt.setup != nil {
				keyPath, cleanup = tt.setup()
				defer cleanup()
			} else {
				keyPath = tt.keyPath
			}

			got, err := readKeyDerive(keyPath)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, got)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestEncryptBytes(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		keyPath  string
		setup    func() (string, func())
		wantErr  bool
		validate func(*testing.T, []byte, error, []byte, string)
	}{
		{
			name: "successful encryption",
			data: []byte("test data to encrypt"),
			setup: func() (string, func()) {
				tmpfile, err := os.CreateTemp("", "testkey")
				if err != nil {
					t.Fatal(err)
				}
				keyContent := []byte("32bytekeyforaesencryptiontest!") // 32 bytes for AES-256
				if _, err := tmpfile.Write(keyContent); err != nil {
					t.Fatal(err)
				}
				if err := tmpfile.Close(); err != nil {
					t.Fatal(err)
				}
				return tmpfile.Name(), func() { os.Remove(tmpfile.Name()) }
			},
			wantErr: false,
			validate: func(t *testing.T, result []byte, err error, original []byte, keyPath string) {
				assert.NoError(t, err)
				assert.NotEmpty(t, result)

				key, _ := readKeyDerive(keyPath)
				block, _ := aes.NewCipher(key)
				gcm, _ := cipher.NewGCM(block)
				nonceSize := gcm.NonceSize()

				assert.True(t, len(result) > nonceSize)

				nonce := result[:nonceSize]
				ciphertext := result[nonceSize:]

				decrypted, err := gcm.Open(nil, nonce, ciphertext, nil)
				assert.NoError(t, err)
				assert.Equal(t, original, decrypted)
			},
		},
		{
			name: "empty data encryption",
			data: []byte{},
			setup: func() (string, func()) {
				tmpfile, err := os.CreateTemp("", "testkey")
				if err != nil {
					t.Fatal(err)
				}
				keyContent := []byte("32bytekeyforaesencryptiontest!")
				if _, err := tmpfile.Write(keyContent); err != nil {
					t.Fatal(err)
				}
				if err := tmpfile.Close(); err != nil {
					t.Fatal(err)
				}
				return tmpfile.Name(), func() { os.Remove(tmpfile.Name()) }
			},
			wantErr: false,
			validate: func(t *testing.T, result []byte, err error, original []byte, keyPath string) {
				assert.NoError(t, err)
				assert.NotEmpty(t, result)

				key, _ := readKeyDerive(keyPath)
				block, _ := aes.NewCipher(key)
				gcm, _ := cipher.NewGCM(block)
				nonceSize := gcm.NonceSize()

				assert.True(t, len(result) > nonceSize)

				nonce := result[:nonceSize]
				ciphertext := result[nonceSize:]

				decrypted, err := gcm.Open(nil, nonce, ciphertext, nil)
				assert.NoError(t, err)
				assert.Empty(t, decrypted)
			},
		},
		{
			name: "large data encryption",
			data: make([]byte, 1024*1024), // 1MB
			setup: func() (string, func()) {
				tmpfile, err := os.CreateTemp("", "testkey")
				if err != nil {
					t.Fatal(err)
				}
				keyContent := []byte("32bytekeyforaesencryptiontest!")
				if _, err := tmpfile.Write(keyContent); err != nil {
					t.Fatal(err)
				}
				if err := tmpfile.Close(); err != nil {
					t.Fatal(err)
				}
				return tmpfile.Name(), func() { os.Remove(tmpfile.Name()) }
			},
			wantErr: false,
			validate: func(t *testing.T, result []byte, err error, original []byte, keyPath string) {
				assert.NoError(t, err)
				assert.NotEmpty(t, result)

				key, _ := readKeyDerive(keyPath)
				block, _ := aes.NewCipher(key)
				gcm, _ := cipher.NewGCM(block)
				nonceSize := gcm.NonceSize()

				nonce := result[:nonceSize]
				ciphertext := result[nonceSize:]

				decrypted, err := gcm.Open(nil, nonce, ciphertext, nil)
				assert.NoError(t, err)
				assert.Equal(t, original, decrypted)
			},
		},
		{
			name: "invalid key path",
			data: []byte("test data"),
			setup: func() (string, func()) {
				return "/nonexistent/path/to/key", func() {}
			},
			wantErr: true,
			validate: func(t *testing.T, result []byte, err error, original []byte, keyPath string) {
				assert.Error(t, err)
				assert.Nil(t, result)
			},
		},
		{
			name: "multiple encryptions produce different results",
			data: []byte("same data to encrypt"),
			setup: func() (string, func()) {
				tmpfile, err := os.CreateTemp("", "testkey")
				if err != nil {
					t.Fatal(err)
				}
				keyContent := []byte("32bytekeyforaesencryptiontest!")
				if _, err := tmpfile.Write(keyContent); err != nil {
					t.Fatal(err)
				}
				if err := tmpfile.Close(); err != nil {
					t.Fatal(err)
				}
				return tmpfile.Name(), func() { os.Remove(tmpfile.Name()) }
			},
			wantErr: false,
			validate: func(t *testing.T, result []byte, err error, original []byte, keyPath string) {
				assert.NoError(t, err)

				result2, err2 := EncryptBytes(keyPath, original)
				assert.NoError(t, err2)
				assert.NotEqual(t, result, result2)

				key, _ := readKeyDerive(keyPath)
				block, _ := aes.NewCipher(key)
				gcm, _ := cipher.NewGCM(block)
				nonceSize := gcm.NonceSize()

				nonce1 := result[:nonceSize]
				nonce2 := result2[:nonceSize]
				assert.NotEqual(t, nonce1, nonce2)
			},
		},
		{
			name: "binary data encryption",
			data: []byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE, 0xFD, 0xFC},
			setup: func() (string, func()) {
				tmpfile, err := os.CreateTemp("", "testkey")
				if err != nil {
					t.Fatal(err)
				}
				keyContent := []byte("32bytekeyforaesencryptiontest!")
				if _, err := tmpfile.Write(keyContent); err != nil {
					t.Fatal(err)
				}
				if err := tmpfile.Close(); err != nil {
					t.Fatal(err)
				}
				return tmpfile.Name(), func() { os.Remove(tmpfile.Name()) }
			},
			wantErr: false,
			validate: func(t *testing.T, result []byte, err error, original []byte, keyPath string) {
				assert.NoError(t, err)
				assert.NotEmpty(t, result)

				key, _ := readKeyDerive(keyPath)
				block, _ := aes.NewCipher(key)
				gcm, _ := cipher.NewGCM(block)
				nonceSize := gcm.NonceSize()

				nonce := result[:nonceSize]
				ciphertext := result[nonceSize:]

				decrypted, err := gcm.Open(nil, nonce, ciphertext, nil)
				assert.NoError(t, err)
				assert.Equal(t, original, decrypted)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var keyPath string
			var cleanup func()

			if tt.setup != nil {
				keyPath, cleanup = tt.setup()
				defer cleanup()
			} else {
				keyPath = tt.keyPath
			}

			result, err := EncryptBytes(keyPath, tt.data)

			if tt.validate != nil {
				tt.validate(t, result, err, tt.data, keyPath)
			}
		})
	}
}

func TestDecryptBytes(t *testing.T) {
	tests := []struct {
		name    string
		keyPath string
		data    []byte
		setup   func() (string, func())
		want    []byte
		wantErr bool
		errMsg  string
	}{
		{
			name: "successful decryption",
			setup: func() (string, func()) {
				tmpfile, err := os.CreateTemp("", "testkey")
				if err != nil {
					t.Fatal(err)
				}
				keyContent := []byte("32bytekeyforaesencryptiontest!")
				if _, err := tmpfile.Write(keyContent); err != nil {
					t.Fatal(err)
				}
				if err := tmpfile.Close(); err != nil {
					t.Fatal(err)
				}
				return tmpfile.Name(), func() { os.Remove(tmpfile.Name()) }
			},
			data: func() []byte {
				key := []byte("32bytekeyforaesencryptiontest!")
				keySum := sha256.Sum256(key)
				block, _ := aes.NewCipher(keySum[:])
				gcm, _ := cipher.NewGCM(block)
				nonce := make([]byte, gcm.NonceSize())
				nonce[0] = 1
				ciphertext := gcm.Seal(nil, nonce, []byte("test data to decrypt"), nil)
				return append(nonce, ciphertext...)
			}(),
			want:    []byte("test data to decrypt"),
			wantErr: false,
		},
		{
			name: "decrypt empty data",
			setup: func() (string, func()) {
				tmpfile, err := os.CreateTemp("", "testkey")
				if err != nil {
					t.Fatal(err)
				}
				keyContent := []byte("32bytekeyforaesencryptiontest!")
				if _, err := tmpfile.Write(keyContent); err != nil {
					t.Fatal(err)
				}
				if err := tmpfile.Close(); err != nil {
					t.Fatal(err)
				}
				return tmpfile.Name(), func() { os.Remove(tmpfile.Name()) }
			},
			data: func() []byte {
				key := []byte("32bytekeyforaesencryptiontest!")
				keySum := sha256.Sum256(key)
				block, _ := aes.NewCipher(keySum[:])
				gcm, _ := cipher.NewGCM(block)
				nonce := make([]byte, gcm.NonceSize())
				nonce[0] = 1
				ciphertext := gcm.Seal(nil, nonce, []byte{}, nil)
				return append(nonce, ciphertext...)
			}(),
			want:    nil,
			wantErr: false,
		},
		{
			name: "decrypt large data",
			setup: func() (string, func()) {
				tmpfile, err := os.CreateTemp("", "testkey")
				if err != nil {
					t.Fatal(err)
				}
				keyContent := []byte("32bytekeyforaesencryptiontest!")
				if _, err := tmpfile.Write(keyContent); err != nil {
					t.Fatal(err)
				}
				if err := tmpfile.Close(); err != nil {
					t.Fatal(err)
				}
				return tmpfile.Name(), func() { os.Remove(tmpfile.Name()) }
			},
			data: func() []byte {
				largeData := make([]byte, 1024*1024) // 1MB
				for i := range largeData {
					largeData[i] = byte(i % 256)
				}
				key := []byte("32bytekeyforaesencryptiontest!")
				keySum := sha256.Sum256(key)
				block, _ := aes.NewCipher(keySum[:])
				gcm, _ := cipher.NewGCM(block)
				nonce := make([]byte, gcm.NonceSize())
				nonce[0] = 1
				ciphertext := gcm.Seal(nil, nonce, largeData, nil)
				return append(nonce, ciphertext...)
			}(),
			want: func() []byte {
				data := make([]byte, 1024*1024)
				for i := range data {
					data[i] = byte(i % 256)
				}
				return data
			}(),
			wantErr: false,
		},
		{
			name: "invalid key path",
			setup: func() (string, func()) {
				return "/nonexistent/path/to/key", func() {}
			},
			data:    []byte{1, 2, 3, 4},
			want:    nil,
			wantErr: true,
		},
		{
			name: "key with invalid size",
			setup: func() (string, func()) {
				tmpfile, err := os.CreateTemp("", "invalidkey")
				if err != nil {
					t.Fatal(err)
				}
				keyContent := []byte("short")
				if _, err := tmpfile.Write(keyContent); err != nil {
					t.Fatal(err)
				}
				if err := tmpfile.Close(); err != nil {
					t.Fatal(err)
				}
				return tmpfile.Name(), func() { os.Remove(tmpfile.Name()) }
			},
			data:    []byte{1, 2, 3, 4},
			want:    nil,
			wantErr: true,
		},
		{
			name: "ciphertext too short",
			setup: func() (string, func()) {
				tmpfile, err := os.CreateTemp("", "testkey")
				if err != nil {
					t.Fatal(err)
				}
				keyContent := []byte("32bytekeyforaesencryptiontest!")
				if _, err := tmpfile.Write(keyContent); err != nil {
					t.Fatal(err)
				}
				if err := tmpfile.Close(); err != nil {
					t.Fatal(err)
				}
				return tmpfile.Name(), func() { os.Remove(tmpfile.Name()) }
			},
			data:    []byte{1, 2, 3}, // Too short for nonce
			want:    nil,
			wantErr: true,
			errMsg:  "ciphertext too short",
		},
		{
			name: "corrupted ciphertext",
			setup: func() (string, func()) {
				tmpfile, err := os.CreateTemp("", "testkey")
				if err != nil {
					t.Fatal(err)
				}
				keyContent := []byte("32bytekeyforaesencryptiontest!")
				if _, err := tmpfile.Write(keyContent); err != nil {
					t.Fatal(err)
				}
				if err := tmpfile.Close(); err != nil {
					t.Fatal(err)
				}
				return tmpfile.Name(), func() { os.Remove(tmpfile.Name()) }
			},
			data: func() []byte {
				key := []byte("32bytekeyforaesencryptiontest!")
				keySum := sha256.Sum256(key)
				block, _ := aes.NewCipher(keySum[:])
				gcm, _ := cipher.NewGCM(block)
				nonce := make([]byte, gcm.NonceSize())
				ciphertext := gcm.Seal(nil, nonce, []byte("test data"), nil)
				result := append(nonce, ciphertext...)
				result[len(result)-1] ^= 0xFF // Corrupt last byte
				return result
			}(),
			want:    nil,
			wantErr: true,
		},
		{
			name: "wrong key",
			setup: func() (string, func()) {
				tmpfile, err := os.CreateTemp("", "wrongkey")
				if err != nil {
					t.Fatal(err)
				}
				keyContent := []byte("different32bytekeyforaesencrypt!")
				if _, err := tmpfile.Write(keyContent); err != nil {
					t.Fatal(err)
				}
				if err := tmpfile.Close(); err != nil {
					t.Fatal(err)
				}
				return tmpfile.Name(), func() { os.Remove(tmpfile.Name()) }
			},
			data: func() []byte {
				key := []byte("32bytekeyforaesencryptiontest!")
				keySum := sha256.Sum256(key)
				block, _ := aes.NewCipher(keySum[:])
				gcm, _ := cipher.NewGCM(block)
				nonce := make([]byte, gcm.NonceSize())
				nonce[0] = 1
				ciphertext := gcm.Seal(nil, nonce, []byte("test data"), nil)
				return append(nonce, ciphertext...)
			}(),
			want:    nil,
			wantErr: true,
		},
		{
			name: "decrypt binary data",
			setup: func() (string, func()) {
				tmpfile, err := os.CreateTemp("", "testkey")
				if err != nil {
					t.Fatal(err)
				}
				keyContent := []byte("32bytekeyforaesencryptiontest!")
				if _, err := tmpfile.Write(keyContent); err != nil {
					t.Fatal(err)
				}
				if err := tmpfile.Close(); err != nil {
					t.Fatal(err)
				}
				return tmpfile.Name(), func() { os.Remove(tmpfile.Name()) }
			},
			data: func() []byte {
				binaryData := []byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE, 0xFD, 0xFC}
				key := []byte("32bytekeyforaesencryptiontest!")
				keySum := sha256.Sum256(key)
				block, _ := aes.NewCipher(keySum[:])
				gcm, _ := cipher.NewGCM(block)
				nonce := make([]byte, gcm.NonceSize())
				nonce[0] = 1
				ciphertext := gcm.Seal(nil, nonce, binaryData, nil)
				return append(nonce, ciphertext...)
			}(),
			want:    []byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE, 0xFD, 0xFC},
			wantErr: false,
		},
		{
			name: "decrypt with different nonce values",
			setup: func() (string, func()) {
				tmpfile, err := os.CreateTemp("", "testkey")
				if err != nil {
					t.Fatal(err)
				}
				keyContent := []byte("32bytekeyforaesencryptiontest!")
				if _, err := tmpfile.Write(keyContent); err != nil {
					t.Fatal(err)
				}
				if err := tmpfile.Close(); err != nil {
					t.Fatal(err)
				}
				return tmpfile.Name(), func() { os.Remove(tmpfile.Name()) }
			},
			data: func() []byte {
				key := []byte("32bytekeyforaesencryptiontest!")
				keySum := sha256.Sum256(key)
				block, _ := aes.NewCipher(keySum[:])
				gcm, _ := cipher.NewGCM(block)
				nonce := make([]byte, gcm.NonceSize())
				for i := range nonce {
					nonce[i] = byte(i)
				}
				ciphertext := gcm.Seal(nil, nonce, []byte("test with custom nonce"), nil)
				return append(nonce, ciphertext...)
			}(),
			want:    []byte("test with custom nonce"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var keyPath string
			var cleanup func()

			if tt.setup != nil {
				keyPath, cleanup = tt.setup()
				defer cleanup()
			} else {
				keyPath = tt.keyPath
			}

			result, err := DecryptBytes(keyPath, tt.data)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, result)
			}
		})
	}
}
