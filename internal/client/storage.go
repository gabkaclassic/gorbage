package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	pb "github.com/gabkaclassic/gorbage/internal/proto"
	"github.com/gabkaclassic/gorbage/pkg/cipher"
	"github.com/gabkaclassic/gorbage/pkg/hash"
	"golang.org/x/sync/errgroup"
	grpc "google.golang.org/grpc"
)

type (
	StorageClient interface {
		LoadArtifactsList(ctx context.Context, prefix string) error
		DeleteArtifact(ctx context.Context, prefix string) error
		DownloadArtifact(ctx context.Context, prefix, filepath, keyPath string, workers, chunkSize int64) error
		UploadArtifact(ctx context.Context, prefix, filepath, keyPath string, workers, chunkSize int64) error
		GetFileInfo(ctx context.Context, prefix string) error
	}
	storageClient struct {
		grpcClient pb.StorageServiceClient
	}
)

func NewStorageClient(connection *grpc.ClientConn) StorageClient {

	grpcClient := pb.NewStorageServiceClient(connection)

	return &storageClient{
		grpcClient: grpcClient,
	}
}

func (client *storageClient) LoadArtifactsList(ctx context.Context, prefix string) error {

	result, err := client.grpcClient.List(ctx, &pb.ListRequest{Prefix: prefix})

	if err != nil {
		return err
	}

	printFileList(result.GetFiles())

	return nil
}

func (client *storageClient) GetFileInfo(ctx context.Context, prefix string) error {
	artifactInfo, err := client.grpcClient.GetFileInfo(ctx, &pb.FileInfoRequest{Prefix: prefix})

	if err != nil {
		return err
	}

	printFileInfo(artifactInfo)

	return nil
}

func (client *storageClient) DeleteArtifact(ctx context.Context, prefix string) error {

	result, err := client.grpcClient.Delete(ctx, &pb.DeleteRequest{Prefix: prefix})

	if err != nil {
		return err
	}

	printDeleteResponse(result, prefix)

	return nil
}

func (client *storageClient) DownloadArtifact(ctx context.Context, prefix, filepath, keyPath string, workers, chunkSize int64) error {
	slog.Debug("start artifact download", slog.String("filepath", filepath), slog.String("keyPath", keyPath), slog.String("prefix", prefix), slog.Int64("workers", workers), slog.Int64("chunkSize", chunkSize))
	slog.Debug("get artifact info", slog.String("prefix", prefix))
	artifactInfo, err := client.grpcClient.GetFileInfo(ctx, &pb.FileInfoRequest{Prefix: prefix})
	if err != nil {
		slog.Error("get artifact info error", slog.Any("error", err), slog.String("prefix", prefix))
		return err
	}

	size := int64(artifactInfo.GetSize())

	if size == 0 {
		slog.Error("empty artifact", slog.String("prefix", prefix))
		return errors.New("empty file")
	}

	isDir := artifactInfo.GetIsDirectory()

	output, outPath, tmpPath, err := prepareOutputFile(prefix, filepath, isDir)
	if err != nil {
		return err
	}
	defer output.Close()

	slog.Debug("truncate destination file", slog.String("filepath", outPath))
	if err := output.Truncate(int64(size)); err != nil {
		slog.Error("truncate destination file error", slog.Any("error", err), slog.String("filepath", outPath))
	}

	if err := client.downloadChunks(ctx, prefix, filepath, output, size, chunkSize, workers); err != nil {
		slog.Error("download artifact error", slog.Any("error", err), slog.String("prefix", prefix))
		if isDir && tmpPath != "" {
			defer os.Remove(tmpPath)
		}
		return err
	}

	hash := artifactInfo.GetSha256()
	slog.Debug("check artifact hashsum", slog.String("sum", hash), slog.String("outFilepath", outPath), slog.String("filepath", filepath), slog.String("tmpFilepath", tmpPath))

	// if hash != "" {
	// 	if err := verifyFileHash(outPath, hash); err != nil {
	// 		return err
	// 	}
	// }

	if keyPath != "" {
		if isDir {
			encBytes, err := os.ReadFile(tmpPath)
			if err != nil {
				return err
			}
			dec, err := cipher.DecryptBytes(keyPath, encBytes)
			if err != nil {
				return err
			}
			tmpDec, err := os.CreateTemp("", "artifact-dec-*")
			if err != nil {
				return err
			}
			tmpDecPath := tmpDec.Name()
			if _, err := tmpDec.Write(dec); err != nil {
				tmpDec.Close()
				defer os.Remove(tmpDecPath)
				return err
			}
			tmpDec.Close()
			if err := unpackArchive(filepath, tmpDecPath); err != nil {
				defer os.Remove(tmpDecPath)
				return err
			}
			defer os.Remove(tmpDecPath)
			defer os.Remove(tmpPath)
		} else {
			outBytes, err := os.ReadFile(outPath)
			if err != nil {
				return err
			}
			dec, err := cipher.DecryptBytes(keyPath, outBytes)
			if err != nil {
				return err
			}
			if err := os.WriteFile(outPath, dec, 0o644); err != nil {
				return err
			}
		}
	} else {
		if isDir {
			slog.Debug("unpack artifact", slog.String("dstFilepath", filepath), slog.String("srcFilepath", tmpPath))
			if err := unpackArchive(filepath, tmpPath); err != nil {
				slog.Error("open arhive reader error", slog.Any("error", err), slog.String("dstFilepath", filepath), slog.String("srcFilepath", tmpPath))
				return err
			}

			if err := os.Remove(tmpPath); err != nil {
				slog.Error("remove temporary file error", slog.Any("error", err), slog.String("filepath", tmpPath))
				return err
			}
		}
	}

	slog.Info("artifact downloaded", slog.String("prefix", prefix), slog.String("filepath", filepath))

	return nil
}

func (client *storageClient) downloadChunks(ctx context.Context, prefix, filepath string, output *os.File, size, chunkSize, workers int64) error {
	segs := (size + chunkSize - 1) / chunkSize
	if workers > segs {
		workers = segs
	}

	type job struct {
		offset int64
		limit  int64
	}

	jobs := make(chan job)
	errs, ctx := errgroup.WithContext(ctx)
	var downloaded int64

	for w := int64(0); w < workers; w++ {
		errs.Go(func() error {
			for j := range jobs {
				expected := j.limit

				slog.Debug("download artifact chunk",
					slog.Int64("offset", j.offset),
					slog.Int64("limit", j.limit),
				)

				resp, err := client.grpcClient.DownloadRange(ctx, &pb.DownloadRangeRequest{
					Prefix: prefix,
					Offset: uint64(j.offset),
					Limit:  uint64(j.limit),
				})
				if err != nil {
					slog.Error("download artifact request error", slog.Any("error", err), slog.String("prefix", prefix))
					return err
				}

				got := int64(len(resp.Data))

				slog.Debug("artifact chunk received",
					slog.Int64("offset", j.offset),
					slog.Int64("expected", expected),
					slog.Int64("received", got),
				)

				if got != expected {
					return fmt.Errorf("invalid chunk size: expected %d got %d", expected, got)
				}

				n, err := output.WriteAt(resp.Data, j.offset)
				if err != nil {
					slog.Error("write artifact data to destination file error", slog.Any("error", err), slog.String("filepath", filepath))
					return err
				}

				if int64(n) != got {
					return fmt.Errorf("partial write: expected %d wrote %d", got, n)
				}

				atomic.AddInt64(&downloaded, got)

				slog.Debug("artifact chunk downloaded",
					slog.Int64("offset", j.offset),
					slog.Int64("downloaded", downloaded),
				)
			}
			return nil
		})
	}

	for i := int64(0); i < segs; i++ {
		offset := i * chunkSize
		limit := chunkSize
		if offset+limit > size {
			limit = size - offset
		}

		select {
		case jobs <- job{offset: offset, limit: limit}:
		case <-ctx.Done():
			close(jobs)
			return ctx.Err()
		}
	}

	close(jobs)

	return errs.Wait()
}

func (client *storageClient) uploadChunks(ctx context.Context, cancelUpload context.CancelFunc, uploadID, filepath string, size, chunkSize int64, workers int64) error {

	segs := (size + chunkSize - 1) / chunkSize
	if workers > segs {
		workers = segs
	}
	sem := make(chan struct{}, workers)
	errs, uploadCtx := errgroup.WithContext(ctx)
	var uploaded int64

	slog.Debug("open artifact file descriptor", slog.String("filepath", filepath), slog.Int64("preferedChunkSize", chunkSize), slog.Int64("workers", workers))
	file, err := os.Open(filepath)

	if err != nil {
		slog.Error("open artifact file descriptor error", slog.Any("error", err), slog.String("filepath", filepath), slog.Int64("preferedChunkSize", chunkSize), slog.Int64("workers", workers))
		return err
	}
	defer file.Close()
	slog.Debug("start upload artifact by chunks", slog.String("filepath", filepath), slog.Int64("preferedChunkSize", chunkSize), slog.Int64("workers", workers))

	for i := int64(0); i < segs; i++ {
		offset := i * chunkSize
		limit := chunkSize
		if offset+limit > size {
			limit = size - offset
		}
		seq := i

		sem <- struct{}{}
		off := offset
		lim := limit
		errs.Go(func() error {
			slog.Debug("start upload artifact chunk", slog.String("uploadID", uploadID))
			defer func() { <-sem }()
			data := make([]byte, lim)
			n, err := file.ReadAt(data, off)

			if err != nil && err != io.EOF {
				return err
			}

			resp, err := client.grpcClient.UploadPart(uploadCtx, &pb.UploadPartRequest{
				UploadId: uploadID,
				Data:     data[:n],
				Seq:      uint64(seq),
			})

			if err != nil {
				return err
			}

			if resp.GetOk() {
				atomic.AddInt64(&uploaded, lim)
			} else {
				slog.Info("artifact chunk upload failed: cancel upload", slog.String("uploadID", uploadID), slog.Bool("ok", resp.GetOk()))
				cancelUpload()
			}

			slog.Debug("artifact chunk uploaded", slog.String("uploadID", uploadID), slog.Int64("uploaded", uploaded), slog.Int64("size", size), slog.Bool("ok", resp.GetOk()))

			return nil
		})
	}

	err = errs.Wait()

	slog.Debug("upload artifact finised", slog.String("filepath", filepath), slog.Int64("preferedChunkSize", chunkSize), slog.Int64("workers", workers), slog.Any("error", err))

	return err
}

func (client *storageClient) UploadArtifact(ctx context.Context, prefix, filepath, keyPath string, workers, chunkSize int64) error {
	slog.Debug("start upload artifact", slog.String("filepath", filepath), slog.String("prefix", prefix), slog.String("keyPath", keyPath), slog.Int64("preferedChunkSize", chunkSize), slog.Int64("workers", workers))

	meta, newFilePath, err := prepareUploadArtifact(filepath)

	if err != nil {
		slog.Error("prepare upload artifact error", slog.Any("error", err), slog.String("prefix", prefix), slog.String("filepath", newFilePath), slog.Int64("preferedChunkSize", chunkSize), slog.Int64("workers", workers))
		return err
	}

	if meta.GetIsDirectory() {
		defer os.Remove(newFilePath)
	}

	if keyPath != "" {
		plain, err := os.ReadFile(newFilePath)
		if err != nil {
			slog.Error("read file for encryption error", slog.Any("error", err), slog.String("filepath", newFilePath), slog.String("prefix", prefix))
			return err
		}
		enc, err := cipher.EncryptBytes(keyPath, plain)
		if err != nil {
			slog.Error("encrypt file error", slog.Any("error", err), slog.String("prefix", prefix), slog.String("filepath", newFilePath))
			return err
		}
		tmpEnc, err := os.CreateTemp("", "artifact-enc-*")
		if err != nil {
			slog.Error("encrypt create temporary file error", slog.Any("error", err), slog.String("prefix", prefix), slog.String("filepath", newFilePath))
			return err
		}
		tmpEncPath := tmpEnc.Name()
		slog.Debug("encrypt create temporary file", slog.String("prefix", prefix), slog.String("filepath", newFilePath), slog.String("tmpPath", tmpEncPath))
		if _, err := tmpEnc.Write(enc); err != nil {
			tmpEnc.Close()
			defer os.Remove(tmpEncPath)
			return err
		}
		err = tmpEnc.Close()

		if err != nil {
			slog.Error("encrypt close temporary file error", slog.Any("error", err), slog.String("prefix", prefix), slog.String("filepath", newFilePath), slog.String("tmpPath", tmpEncPath))
			return err
		}

		if !meta.GetIsDirectory() {
			meta.Size = uint64(len(enc))
		} else {
			meta.Size = uint64(len(enc))
		}
		meta.Sha256, _ = hash.SHA256FromBytes(enc)
		newFilePath = tmpEncPath
		defer os.Remove(tmpEncPath)
	}

	slog.Debug("send initiate artifact upload request", slog.String("prefix", prefix), slog.String("filepath", newFilePath), slog.Int64("preferedChunkSize", chunkSize), slog.Int64("workers", workers))
	resp, err := client.grpcClient.InitiateUpload(ctx, &pb.InitiateUploadRequest{
		Meta:               meta,
		Prefix:             prefix,
		PreferredChunkSize: uint64(chunkSize),
	})

	if err != nil {
		slog.Error("initiate upload request error", slog.String("prefix", prefix), slog.Any("error", err), slog.String("filepath", newFilePath), slog.Int64("preferedChunkSize", chunkSize), slog.Int64("workers", workers))
		return err
	}

	uploadID := resp.GetUploadId()
	actualChunkSize := int64(resp.GetChunkSize())
	size := int64(meta.GetSize())
	expiresAt := resp.GetExpiresAtUnix()
	uploadCtx, cancelUpload := context.WithDeadline(ctx, time.Unix(expiresAt, 0))

	return client.uploadChunks(uploadCtx, cancelUpload, uploadID, newFilePath, size, actualChunkSize, workers)
}
