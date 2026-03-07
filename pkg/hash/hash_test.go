package hash

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSha256FromReader(t *testing.T) {
	tests := []struct {
		name     string
		content  []byte
		reader   func() io.Reader
		wantSum  string
		wantSize int64
		wantErr  bool
	}{
		{
			name:    "simple string",
			content: []byte("hello world"),
			reader: func() io.Reader {
				return strings.NewReader("hello world")
			},
			wantSum:  "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
			wantSize: 11,
			wantErr:  false,
		},
		{
			name:    "empty string",
			content: []byte(""),
			reader: func() io.Reader {
				return strings.NewReader("")
			},
			wantSum:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			wantSize: 0,
			wantErr:  false,
		},
		{
			name:    "single character",
			content: []byte("a"),
			reader: func() io.Reader {
				return strings.NewReader("a")
			},
			wantSum:  "ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb",
			wantSize: 1,
			wantErr:  false,
		},
		{
			name:    "large content",
			content: make([]byte, 1024*1024),
			reader: func() io.Reader {
				data := make([]byte, 1024*1024)
				for i := range data {
					data[i] = byte(i % 256)
				}
				return bytes.NewReader(data)
			},
			wantSum: func() string {
				data := make([]byte, 1024*1024)
				for i := range data {
					data[i] = byte(i % 256)
				}
				h := sha256.New()
				h.Write(data)
				return hex.EncodeToString(h.Sum(nil))
			}(),
			wantSize: 1024 * 1024,
			wantErr:  false,
		},
		{
			name:    "special characters",
			content: []byte("!@#$%^&*()_+{}[]|\\:;\"'<>,.?/~`"),
			reader: func() io.Reader {
				return strings.NewReader("!@#$%^&*()_+{}[]|\\:;\"'<>,.?/~`")
			},
			wantSum: func() string {
				h := sha256.New()
				h.Write([]byte("!@#$%^&*()_+{}[]|\\:;\"'<>,.?/~`"))
				return hex.EncodeToString(h.Sum(nil))
			}(),
			wantSize: 30,
			wantErr:  false,
		},
		{
			name:    "unicode characters",
			content: []byte("Hello, 世界"),
			reader: func() io.Reader {
				return strings.NewReader("Hello, 世界")
			},
			wantSum:  "a281e84c7f61393db702630c2a6807e871cd3b6896c9e56e22982d125696575c",
			wantSize: 13,
			wantErr:  false,
		},
		{
			name: "error reader",
			reader: func() io.Reader {
				return &errorReader{}
			},
			wantSum:  "",
			wantSize: 0,
			wantErr:  true,
		},
		{
			name:    "multiple chunks",
			content: []byte("this is a test string that will be read in multiple chunks"),
			reader: func() io.Reader {
				data := []byte("this is a test string that will be read in multiple chunks")
				return &chunkedReader{data: data, chunkSize: 10}
			},
			wantSum: func() string {
				h := sha256.New()
				h.Write([]byte("this is a test string that will be read in multiple chunks"))
				return hex.EncodeToString(h.Sum(nil))
			}(),
			wantSize: 58,
			wantErr:  false,
		},
		{
			name:    "binary data",
			content: []byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE, 0xFD, 0xFC},
			reader: func() io.Reader {
				return bytes.NewReader([]byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE, 0xFD, 0xFC})
			},
			wantSum: func() string {
				h := sha256.New()
				h.Write([]byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE, 0xFD, 0xFC})
				return hex.EncodeToString(h.Sum(nil))
			}(),
			wantSize: 8,
			wantErr:  false,
		},
		{
			name:    "newline characters",
			content: []byte("line1\nline2\r\nline3"),
			reader: func() io.Reader {
				return strings.NewReader("line1\nline2\r\nline3")
			},
			wantSum: func() string {
				h := sha256.New()
				h.Write([]byte("line1\nline2\r\nline3"))
				return hex.EncodeToString(h.Sum(nil))
			}(),
			wantSize: 18,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r io.Reader
			if tt.reader != nil {
				r = tt.reader()
			}

			sum, size, err := sha256FromReader(r)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Empty(t, sum)
				assert.Equal(t, int64(0), size)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantSum, sum)
				assert.Equal(t, tt.wantSize, size)
			}
		})
	}
}

func TestSHA256FromFile(t *testing.T) {
	tests := []struct {
		name     string
		content  []byte
		setup    func() (string, func())
		wantSum  string
		wantSize int64
		wantErr  bool
	}{
		{
			name:    "successful file read",
			content: []byte("hello world"),
			setup: func() (string, func()) {
				tmpfile, err := os.CreateTemp("", "testfile")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := tmpfile.Write([]byte("hello world")); err != nil {
					t.Fatal(err)
				}
				if err := tmpfile.Close(); err != nil {
					t.Fatal(err)
				}
				return tmpfile.Name(), func() { os.Remove(tmpfile.Name()) }
			},
			wantSum:  "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
			wantSize: 11,
			wantErr:  false,
		},
		{
			name:    "empty file",
			content: []byte{},
			setup: func() (string, func()) {
				tmpfile, err := os.CreateTemp("", "emptyfile")
				if err != nil {
					t.Fatal(err)
				}
				if err := tmpfile.Close(); err != nil {
					t.Fatal(err)
				}
				return tmpfile.Name(), func() { os.Remove(tmpfile.Name()) }
			},
			wantSum:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			wantSize: 0,
			wantErr:  false,
		},
		{
			name:    "file with single character",
			content: []byte("a"),
			setup: func() (string, func()) {
				tmpfile, err := os.CreateTemp("", "singlechar")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := tmpfile.Write([]byte("a")); err != nil {
					t.Fatal(err)
				}
				if err := tmpfile.Close(); err != nil {
					t.Fatal(err)
				}
				return tmpfile.Name(), func() { os.Remove(tmpfile.Name()) }
			},
			wantSum:  "ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb",
			wantSize: 1,
			wantErr:  false,
		},
		{
			name:    "large file",
			content: make([]byte, 1024*1024),
			setup: func() (string, func()) {
				tmpfile, err := os.CreateTemp("", "largefile")
				if err != nil {
					t.Fatal(err)
				}
				data := make([]byte, 1024*1024)
				for i := range data {
					data[i] = byte(i % 256)
				}
				if _, err := tmpfile.Write(data); err != nil {
					t.Fatal(err)
				}
				if err := tmpfile.Close(); err != nil {
					t.Fatal(err)
				}
				return tmpfile.Name(), func() { os.Remove(tmpfile.Name()) }
			},
			wantSum: func() string {
				data := make([]byte, 1024*1024)
				for i := range data {
					data[i] = byte(i % 256)
				}
				h := sha256.New()
				h.Write(data)
				return hex.EncodeToString(h.Sum(nil))
			}(),
			wantSize: 1024 * 1024,
			wantErr:  false,
		},
		{
			name:    "file with special characters",
			content: []byte("!@#$%^&*()_+{}[]|\\:;\"'<>,.?/~`"),
			setup: func() (string, func()) {
				tmpfile, err := os.CreateTemp("", "specialchars")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := tmpfile.Write([]byte("!@#$%^&*()_+{}[]|\\:;\"'<>,.?/~`")); err != nil {
					t.Fatal(err)
				}
				if err := tmpfile.Close(); err != nil {
					t.Fatal(err)
				}
				return tmpfile.Name(), func() { os.Remove(tmpfile.Name()) }
			},
			wantSum: func() string {
				h := sha256.New()
				h.Write([]byte("!@#$%^&*()_+{}[]|\\:;\"'<>,.?/~`"))
				return hex.EncodeToString(h.Sum(nil))
			}(),
			wantSize: 30,
			wantErr:  false,
		},
		{
			name:    "file with unicode characters",
			content: []byte("Hello, 世界"),
			setup: func() (string, func()) {
				tmpfile, err := os.CreateTemp("", "unicode")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := tmpfile.Write([]byte("Hello, 世界")); err != nil {
					t.Fatal(err)
				}
				if err := tmpfile.Close(); err != nil {
					t.Fatal(err)
				}
				return tmpfile.Name(), func() { os.Remove(tmpfile.Name()) }
			},
			wantSum:  "a281e84c7f61393db702630c2a6807e871cd3b6896c9e56e22982d125696575c",
			wantSize: 13,
			wantErr:  false,
		},
		{
			name:    "file with newline characters",
			content: []byte("line1\nline2\r\nline3"),
			setup: func() (string, func()) {
				tmpfile, err := os.CreateTemp("", "newlines")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := tmpfile.Write([]byte("line1\nline2\r\nline3")); err != nil {
					t.Fatal(err)
				}
				if err := tmpfile.Close(); err != nil {
					t.Fatal(err)
				}
				return tmpfile.Name(), func() { os.Remove(tmpfile.Name()) }
			},
			wantSum: func() string {
				h := sha256.New()
				h.Write([]byte("line1\nline2\r\nline3"))
				return hex.EncodeToString(h.Sum(nil))
			}(),
			wantSize: 18,
			wantErr:  false,
		},
		{
			name:    "binary file",
			content: []byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE, 0xFD, 0xFC},
			setup: func() (string, func()) {
				tmpfile, err := os.CreateTemp("", "binary")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := tmpfile.Write([]byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE, 0xFD, 0xFC}); err != nil {
					t.Fatal(err)
				}
				if err := tmpfile.Close(); err != nil {
					t.Fatal(err)
				}
				return tmpfile.Name(), func() { os.Remove(tmpfile.Name()) }
			},
			wantSum: func() string {
				h := sha256.New()
				h.Write([]byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE, 0xFD, 0xFC})
				return hex.EncodeToString(h.Sum(nil))
			}(),
			wantSize: 8,
			wantErr:  false,
		},
		{
			name:    "file with multiple lines",
			content: []byte("line1\nline2\nline3\nline4\nline5"),
			setup: func() (string, func()) {
				tmpfile, err := os.CreateTemp("", "multiline")
				if err != nil {
					t.Fatal(err)
				}
				content := "line1\nline2\nline3\nline4\nline5"
				if _, err := tmpfile.Write([]byte(content)); err != nil {
					t.Fatal(err)
				}
				if err := tmpfile.Close(); err != nil {
					t.Fatal(err)
				}
				return tmpfile.Name(), func() { os.Remove(tmpfile.Name()) }
			},
			wantSum: func() string {
				h := sha256.New()
				h.Write([]byte("line1\nline2\nline3\nline4\nline5"))
				return hex.EncodeToString(h.Sum(nil))
			}(),
			wantSize: 29,
			wantErr:  false,
		},
		{
			name: "file not found",
			setup: func() (string, func()) {
				return "/nonexistent/path/to/file", func() {}
			},
			wantSum:  "",
			wantSize: 0,
			wantErr:  true,
		},
		{
			name:    "file with read permissions only",
			content: []byte("read only content"),
			setup: func() (string, func()) {
				tmpfile, err := os.CreateTemp("", "readonly")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := tmpfile.Write([]byte("read only content")); err != nil {
					t.Fatal(err)
				}
				if err := tmpfile.Close(); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(tmpfile.Name(), 0444); err != nil {
					t.Fatal(err)
				}
				return tmpfile.Name(), func() {
					os.Chmod(tmpfile.Name(), 0644)
					os.Remove(tmpfile.Name())
				}
			},
			wantSum: func() string {
				h := sha256.New()
				h.Write([]byte("read only content"))
				return hex.EncodeToString(h.Sum(nil))
			}(),
			wantSize: 17,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var filePath string
			var cleanup func()

			if tt.setup != nil {
				filePath, cleanup = tt.setup()
				defer cleanup()
			}

			sum, size, err := SHA256FromFile(filePath)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Empty(t, sum)
				assert.Equal(t, int64(0), size)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantSum, sum)
				assert.Equal(t, tt.wantSize, size)
			}
		})
	}
}

func TestSHA256FromBytes(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		wantSum  string
		wantSize int64
	}{
		{
			name:     "simple string",
			data:     []byte("hello world"),
			wantSum:  "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
			wantSize: 11,
		},
		{
			name:     "empty bytes",
			data:     []byte{},
			wantSum:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			wantSize: 0,
		},
		{
			name:     "nil bytes",
			data:     nil,
			wantSum:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			wantSize: 0,
		},
		{
			name:     "single character",
			data:     []byte("a"),
			wantSum:  "ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb",
			wantSize: 1,
		},
		{
			name:     "single byte",
			data:     []byte{0x00},
			wantSum:  "6e340b9cffb37a989ca544e6bb780a2c78901d3fb33738768511a30617afa01d",
			wantSize: 1,
		},
		{
			name:     "multiple bytes",
			data:     []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05},
			wantSum:  "17e88db187afd62c16e5debf3e6527cd006bc012bc90b51a810cd80c2d511f43",
			wantSize: 6,
		},
		{
			name:     "special characters",
			data:     []byte("!@#$%^&*()_+{}[]|\\:;\"'<>,.?/~`"),
			wantSum:  "4d9480a233f2d6aec59f2294f6d55b4688216c8d3adf45ea1c8fce2f30cd7591",
			wantSize: 30,
		},
		{
			name:     "unicode characters",
			data:     []byte("Hello, 世界"),
			wantSum:  "a281e84c7f61393db702630c2a6807e871cd3b6896c9e56e22982d125696575c",
			wantSize: 13,
		},
		{
			name:     "newline characters",
			data:     []byte("line1\nline2\r\nline3"),
			wantSum:  "50ba38bb6d7f20c4ccbbfea952d4cad1ba80f4f65cc6dd9e2e30b0b533224c1c",
			wantSize: 18,
		},
		{
			name: "maximum byte values",
			data: []byte{0xFF, 0xFF, 0xFF, 0xFF},
			wantSum: func() string {
				sum := sha256.Sum256([]byte{0xFF, 0xFF, 0xFF, 0xFF})
				return hex.EncodeToString(sum[:])
			}(),
			wantSize: 4,
		},
		{
			name: "mixed byte values",
			data: []byte{0x00, 0xFF, 0x7F, 0x80, 0x01, 0xFE},
			wantSum: func() string {
				sum := sha256.Sum256([]byte{0x00, 0xFF, 0x7F, 0x80, 0x01, 0xFE})
				return hex.EncodeToString(sum[:])
			}(),
			wantSize: 6,
		},
		{
			name:     "alphanumeric string",
			data:     []byte("abcdefghijklmnopqrstuvwxyz0123456789"),
			wantSum:  "011fc2994e39d251141540f87a69092b3f22a86767f7283de7eeedb3897bedf6",
			wantSize: 36,
		},
		{
			name:     "whitespace characters",
			data:     []byte(" \t\n\r\v\f"),
			wantSum:  "bf0070d8cde6a1173987796177950669806efd2cb9f3377676c7b7bc930afbec",
			wantSize: 6,
		},
		{
			name:     "repeated pattern",
			data:     []byte("abcabcabcabcabcabcabcabcabcabcabcabc"),
			wantSum:  "7fec9f12d0682abad5858d5a59592280fc2b6c8d70010e219728e84fe7c6138b",
			wantSize: 36,
		},
		{
			name: "binary representation of numbers",
			data: []byte{0x01, 0x02, 0x04, 0x08, 0x10, 0x20, 0x40, 0x80},
			wantSum: func() string {
				sum := sha256.Sum256([]byte{0x01, 0x02, 0x04, 0x08, 0x10, 0x20, 0x40, 0x80})
				return hex.EncodeToString(sum[:])
			}(),
			wantSize: 8,
		},
		{
			name: "string with null bytes",
			data: []byte("hello\x00world"),
			wantSum: func() string {
				sum := sha256.Sum256([]byte("hello\x00world"))
				return hex.EncodeToString(sum[:])
			}(),
			wantSize: 11,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sum, size := SHA256FromBytes(tt.data)

			assert.Equal(t, tt.wantSum, sum)
			assert.Equal(t, tt.wantSize, size)

			assert.Len(t, sum, 64)
		})
	}
}

type errorReader struct{}

func (r *errorReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("simulated read error")
}

type chunkedReader struct {
	data      []byte
	chunkSize int
	offset    int
}

func (r *chunkedReader) Read(p []byte) (n int, err error) {
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}

	end := r.offset + r.chunkSize
	if end > len(r.data) {
		end = len(r.data)
	}

	n = copy(p, r.data[r.offset:end])
	r.offset = end

	if r.offset >= len(r.data) {
		return n, io.EOF
	}
	return n, nil
}
