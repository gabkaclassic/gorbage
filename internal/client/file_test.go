package client

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareOutputFile(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name      string
		id        string
		filepath  string
		isDir     bool
		expectErr bool
	}{
		{
			name:     "regular file",
			id:       "1",
			filepath: tmpDir + "/file.txt",
			isDir:    false,
		},
		{
			name:     "temporary file for dir",
			id:       "2",
			filepath: tmpDir + "/dir",
			isDir:    true,
		},
		{
			name:      "invalid path",
			id:        "3",
			filepath:  "/invalid_path/file.txt",
			isDir:     false,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, outPath, tmpPath, err := prepareOutputFile(tt.id, tt.filepath, tt.isDir)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, output)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, output)
			assert.NotEmpty(t, outPath)
			if tt.isDir {
				assert.NotEmpty(t, tmpPath)
				_, err := os.Stat(tmpPath)
				assert.NoError(t, err)
			} else {
				assert.Empty(t, tmpPath)
				_, err := os.Stat(outPath)
				assert.NoError(t, err)
			}

			output.Close()
		})
	}
}

func TestUnpackArchive(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test.zip")
	outDir := filepath.Join(tmpDir, "out")
	require.NoError(t, os.MkdirAll(outDir, 0o755))

	createZip := func(path string) {
		f, err := os.Create(path)
		require.NoError(t, err)
		w := zip.NewWriter(f)

		f1, err := w.Create("file1.txt")
		require.NoError(t, err)
		_, err = f1.Write([]byte("data1"))
		require.NoError(t, err)

		require.NoError(t, w.Close())
		require.NoError(t, f.Close())
	}

	createZip(zipPath)

	tests := []struct {
		name      string
		zipFile   string
		dstDir    string
		expectErr bool
	}{
		{
			name:    "valid archive",
			zipFile: zipPath,
			dstDir:  outDir,
		},
		{
			name:      "zip with .. path",
			zipFile:   filepath.Join(tmpDir, "bad.zip"),
			dstDir:    outDir,
			expectErr: true,
		},
	}

	badZip := filepath.Join(tmpDir, "bad.zip")
	f, err := os.Create(badZip)
	require.NoError(t, err)
	w := zip.NewWriter(f)
	_, err = w.Create("../evil.txt")
	require.NoError(t, err)
	require.NoError(t, w.Close())
	require.NoError(t, f.Close())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := unpackArchive(tt.dstDir, tt.zipFile)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				_, err := os.Stat(filepath.Join(tt.dstDir, "file1.txt"))
				assert.NoError(t, err)
			}
		})
	}
}

func TestZipDirToFile(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("data1"), 0o644))
	subDir := filepath.Join(srcDir, "sub")
	require.NoError(t, os.MkdirAll(subDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "file2.txt"), []byte("data2"), 0o644))

	zipPath, err := zipDirToFile(srcDir)
	require.NoError(t, err)
	defer os.Remove(zipPath)

	r, err := zip.OpenReader(zipPath)
	require.NoError(t, err)
	defer r.Close()

	names := make(map[string]bool)
	for _, f := range r.File {
		names[f.Name] = true
	}

	assert.Contains(t, names, "file1.txt")
	assert.Contains(t, names, "sub/")
	assert.Contains(t, names, "sub/file2.txt")
}

func TestPrepareUploadArtifact(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("testdata"), 0o644))

	dirPath := filepath.Join(tmpDir, "dir")
	require.NoError(t, os.MkdirAll(dirPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dirPath, "f.txt"), []byte("data"), 0o644))

	tests := []struct {
		name      string
		path      string
		isDir     bool
		expectErr bool
	}{
		{
			name:  "regular file",
			path:  filePath,
			isDir: false,
		},
		{
			name:  "directory",
			path:  dirPath,
			isDir: true,
		},
		{
			name:      "nonexistent path",
			path:      filepath.Join(tmpDir, "missing"),
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, outPath, err := prepareUploadArtifact(tt.path)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, meta)
				assert.Empty(t, outPath)
				return
			}

			assert.NoError(t, err)
			if tt.isDir {
				assert.True(t, meta.IsDirectory)
				assert.NotEmpty(t, outPath)
				defer os.Remove(outPath)
				_, err := os.Stat(outPath)
				assert.NoError(t, err)
			} else {
				assert.False(t, meta.IsDirectory)
				assert.Equal(t, tt.path, outPath)
			}
			assert.NotEmpty(t, meta.Sha256)
			assert.True(t, meta.Size > 0)
		})
	}
}
