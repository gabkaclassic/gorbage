package client

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"

	pb "github.com/gabkaclassic/gorbage/internal/proto"
	"github.com/gabkaclassic/gorbage/pkg/hash"
)

func prepareOutputFile(id, filepath string, isDir bool) (*os.File, string, string, error) {
	var output *os.File
	var outPath string
	var tmpPath string

	if isDir {
		slog.Debug("create temporary file", slog.String("filepath", filepath))
		tmp, err := os.CreateTemp("", "artifact-*.zip")
		if err != nil {
			slog.Error("create temp file error", slog.Any("error", err))
			return nil, "", "", err
		}
		output = tmp
		tmpPath = tmp.Name()
		outPath = tmpPath
		slog.Debug("temporary file created", slog.String("filepath", tmpPath))
	} else {
		slog.Debug("open destination file", slog.String("filepath", filepath))
		f, err := os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			slog.Error("open file error", slog.Any("error", err), slog.String("ID", id))
			return nil, "", "", err
		}
		output = f
		outPath = filepath
	}

	return output, outPath, tmpPath, nil
}

func verifyFileHash(filepath, expected string) error {
	sum, _, err := hash.SHA256FromFile(filepath)
	if err != nil {
		slog.Error("get destination file hash error", slog.Any("error", err), slog.String("filepath", filepath))
		return err
	}

	if sum != expected {
		slog.Error("sha mismatch", slog.String("got", sum), slog.String("expected", expected))
		return fmt.Errorf("sha mismatch: %s != %s", sum, expected)
	}

	return nil
}

func unpackArchive(dstDir, srcZip string) error {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}

	r, err := zip.OpenReader(srcZip)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if strings.Contains(f.Name, "..") {
			return fmt.Errorf("invalid file path in archive: %s", f.Name)
		}

		targetPath := dstDir + string(os.PathSeparator) + f.Name

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, f.Mode()); err != nil {
				slog.Error("create make target directory error", slog.Any("error", err), slog.String("targetPath", targetPath), slog.String("srcFilepath", srcZip))
				return err
			}
			continue
		}

		if err := os.MkdirAll(path.Dir(targetPath), 0o755); err != nil {
			slog.Error("create make target directory error", slog.Any("error", err), slog.String("targetPath", targetPath), slog.String("srcFilepath", srcZip))
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			slog.Error("open target file error", slog.Any("error", err), slog.String("targetPath", targetPath), slog.String("srcFilepath", srcZip))
			defer rc.Close()
			return err
		}

		if _, err := io.Copy(outFile, rc); err != nil {
			slog.Error("copy file content error", slog.Any("error", err), slog.String("targetPath", targetPath), slog.String("srcFilepath", srcZip))
			defer outFile.Close()
			defer rc.Close()
			return err
		}

		defer outFile.Close()
		defer rc.Close()
	}

	return nil
}

func zipDirToFile(srcDir string) (string, error) {
	tmp, err := os.CreateTemp("", "artifact-*.zip")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	zw := zip.NewWriter(tmp)

	err = filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = rel
		if info.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}
		w, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(w, f)
			closeErr := f.Close()

			if closeErr != nil {
				return closeErr
			}

			if copyErr != nil {
				return copyErr
			}
		}
		return nil
	})
	if errClose := zw.Close(); err == nil {
		err = errClose
	}
	if err2 := tmp.Close(); err == nil {
		err = err2
	}
	if err != nil {
		err = os.Remove(tmpPath)

		if err != nil {
			return "", err
		}

		return "", err
	}
	return tmpPath, nil
}

func prepareUploadArtifact(filepathArg string) (*pb.FileMetadata, string, error) {
	info, err := os.Stat(filepathArg)
	if err != nil {
		return nil, "", err
	}

	if info.IsDir() {
		tmpPath, err := zipDirToFile(filepathArg)
		if err != nil {
			return nil, "", err
		}
		sha, size, err := hash.SHA256FromFile(tmpPath)
		if err != nil {
			_ = os.Remove(tmpPath)
			return nil, "", err
		}
		meta := &pb.FileMetadata{
			OriginalPath: filepathArg,
			Size:         uint64(size),
			IsDirectory:  true,
			Sha256:       sha,
		}
		return meta, tmpPath, nil
	}

	sha, size, err := hash.SHA256FromFile(filepathArg)
	if err != nil {
		return nil, "", err
	}
	meta := &pb.FileMetadata{
		OriginalPath: filepathArg,
		Size:         uint64(size),
		IsDirectory:  false,
		Sha256:       sha,
	}
	return meta, filepathArg, nil
}
