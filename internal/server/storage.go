package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strconv"
	"time"

	"log/slog"

	pb "github.com/gabkaclassic/gorbage/internal/proto"
	"github.com/gabkaclassic/gorbage/pkg/interceptor"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	originalPathKeyList string = "X-Amz-Meta-Original_path"
	isDirectoryKeyList  string = "X-Amz-Meta-Is_directory"
	sha256KeyList       string = "X-Amz-Meta-Sha256"
	sha256Key           string = "Sha256"
	originalPathKey     string = "Original_path"
	isDirectoryKey      string = "Is_directory"
)

type StorageGRPCServer struct {
	pb.UnimplementedStorageServiceServer
	redisConnection *redis.Client
	minioConnection *minio.Client
	uploadTTL       time.Duration
	chunkSize       uint64
}

func NewArtifactGRPCServer(minioConnection *minio.Client, redisConnection *redis.Client, uploadTTL time.Duration, chunkSize int64) (StorageGRPCServer, error) {

	if minioConnection == nil {
		return StorageGRPCServer{}, errors.New("minio connection can not be nil")
	}

	if redisConnection == nil {
		return StorageGRPCServer{}, errors.New("redis connection can not be nil")
	}

	if uploadTTL <= 0 {
		return StorageGRPCServer{}, errors.New("upload TTL shoud be positive")
	}
	if chunkSize <= 0 {
		return StorageGRPCServer{}, errors.New("chunk size shoud be positive")
	}

	return StorageGRPCServer{
		minioConnection: minioConnection,
		redisConnection: redisConnection,
		uploadTTL:       uploadTTL,
		chunkSize:       uint64(chunkSize),
	}, nil
}

func getBitmapKey(uploadID string) string {
	return "uploads:" + uploadID + ":bitmap"
}

func getMetaKey(uploadID string) string {
	return "uploads:" + uploadID + ":meta"
}

func (storage StorageGRPCServer) InitiateUpload(ctx context.Context, req *pb.InitiateUploadRequest) (*pb.InitiateUploadResponse, error) {
	_, err := storage.checkBucket(ctx)

	if err != nil {
		return nil, err
	}

	meta := req.GetMeta()
	if meta == nil {
		return nil, status.Error(codes.InvalidArgument, "metadata required")
	}

	uploadID := uuid.NewString()

	chunkSize := req.GetPreferredChunkSize()
	if chunkSize == 0 {
		chunkSize = storage.chunkSize
	}

	size := meta.GetSize()
	totalChunks := uint64(0)
	if size > 0 {
		totalChunks = (size + uint64(chunkSize) - 1) / uint64(chunkSize)
	}

	state := &pb.InitiateUploadRequest{
		Meta:               meta,
		PreferredChunkSize: chunkSize,
		Prefix:             req.GetPrefix(),
	}

	data, err := proto.Marshal(state)
	if err != nil {
		return nil, status.Error(codes.Internal, "marshal error")
	}

	metaKey := getMetaKey(uploadID)
	bitmapKey := getBitmapKey(uploadID)

	expiresAt := time.Now().Add(storage.uploadTTL).Unix()

	pipe := storage.redisConnection.TxPipeline()
	pipe.Set(ctx, metaKey, data, storage.uploadTTL)
	pipe.Set(ctx, bitmapKey, []byte{}, storage.uploadTTL)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}

	return &pb.InitiateUploadResponse{
		UploadId:      uploadID,
		ChunkSize:     chunkSize,
		TotalChunks:   totalChunks,
		ExpiresAtUnix: expiresAt,
	}, nil
}

func (storage StorageGRPCServer) UploadPart(ctx context.Context, req *pb.UploadPartRequest) (*pb.UploadPartResponse, error) {
	bucket := ctx.Value(interceptor.UserIDKey).(string)
	if len(bucket) == 0 {
		return nil, status.Error(codes.InvalidArgument, "invalid bucket")
	}

	uploadID := req.GetUploadId()
	seq := req.GetSeq()

	slog.Info("upload chunk",
		slog.String("bucket", bucket),
		slog.String("uploadID", uploadID),
		slog.Uint64("seq", seq),
	)

	metaKey := getMetaKey(uploadID)
	bitmapKey := getBitmapKey(uploadID)

	metaBytes, err := storage.redisConnection.Get(ctx, metaKey).Bytes()
	if err != nil {
		slog.Error("upload session not found", slog.String("bucket", bucket), slog.String("uploadID", uploadID), slog.Any("error", err))
		return nil, status.Error(codes.NotFound, "upload session not found")
	}

	var initReq pb.InitiateUploadRequest
	if err := proto.Unmarshal(metaBytes, &initReq); err != nil {
		slog.Error("metadata decode error", slog.String("uploadID", uploadID), slog.Any("error", err))
		return nil, status.Error(codes.Internal, "metadata decode error")
	}

	meta := initReq.GetMeta()
	chunkSize := initReq.GetPreferredChunkSize()
	size := meta.GetSize()
	totalChunks := uint64(0)
	if size > 0 {
		totalChunks = (size + uint64(chunkSize) - 1) / uint64(chunkSize)
	}

	if seq >= totalChunks {
		slog.Error("invalid chunk seq", slog.String("uploadID", uploadID), slog.Uint64("seq", seq), slog.Uint64("totalChunks", totalChunks))
		return nil, status.Error(codes.InvalidArgument, "invalid chunk seq")
	}

	partKey := "uploads/" + uploadID + "/" + strconv.FormatUint(seq, 10)
	reader := bytes.NewReader(req.GetData())

	_, err = storage.minioConnection.PutObject(
		ctx,
		bucket,
		partKey,
		reader,
		int64(reader.Len()),
		minio.PutObjectOptions{},
	)
	if err != nil {
		slog.Error("failed to store chunk", slog.String("bucket", bucket), slog.String("partKey", partKey), slog.Any("error", err))
		return nil, status.Error(codes.Internal, "failed to store chunk")
	}
	slog.Info("chunk stored", slog.String("bucket", bucket), slog.String("partKey", partKey), slog.Uint64("seq", seq))

	if err := storage.redisConnection.SetBit(ctx, bitmapKey, int64(seq), 1).Err(); err != nil {
		slog.Error("bitmap update error", slog.String("uploadID", uploadID), slog.Any("error", err))
		return nil, status.Error(codes.Internal, "bitmap update error")
	}

	cnt, err := storage.redisConnection.BitCount(ctx, bitmapKey, &redis.BitCount{Start: 0, End: -1}).Result()
	if err != nil {
		slog.Error("bitmap count error", slog.String("uploadID", uploadID), slog.Any("error", err))
		return nil, status.Error(codes.Internal, "bitmap count error")
	}

	if uint64(cnt) == totalChunks {
		slog.Info("all chunks present, assembling", slog.String("uploadID", uploadID), slog.Uint64("totalChunks", totalChunks))

		finalKey := initReq.GetPrefix()
		pr, pw := io.Pipe()

		go func() {
			defer pw.Close()
			for i := uint64(0); i < totalChunks; i++ {
				pk := "uploads/" + uploadID + "/" + strconv.FormatUint(i, 10)
				obj, err := storage.minioConnection.GetObject(ctx, bucket, pk, minio.GetObjectOptions{})
				if err != nil {
					pw.CloseWithError(err)
					return
				}
				_, copyErr := io.Copy(pw, obj)

				if copyErr != nil {
					pw.CloseWithError(copyErr)
					return
				}

				closeErr := obj.Close()

				if closeErr != nil {
					pw.CloseWithError(closeErr)
					return
				}

			}
		}()

		userMeta := map[string]string{
			originalPathKey: meta.GetOriginalPath(),
			isDirectoryKey:  strconv.FormatBool(meta.GetIsDirectory()),
			sha256Key:       meta.GetSha256(),
		}

		_, err = storage.minioConnection.PutObject(ctx, bucket, finalKey, pr, int64(size), minio.PutObjectOptions{UserMetadata: userMeta})
		if err != nil {
			slog.Error("failed to assemble final object", slog.String("bucket", bucket), slog.String("finalKey", finalKey), slog.Any("error", err))
			pr.Close()
			return nil, status.Error(codes.Internal, "failed to assemble final object")
		}
		slog.Debug("remove temp parts", slog.String("bucket", bucket), slog.String("finalKey", finalKey))

		for i := uint64(0); i < totalChunks; i++ {
			pk := "uploads/" + uploadID + "/" + strconv.FormatUint(i, 10)
			slog.Debug("remove temp part", slog.String("bucket", bucket), slog.String("partKey", pk))
			if err := storage.minioConnection.RemoveObject(ctx, bucket, pk, minio.RemoveObjectOptions{}); err != nil {
				slog.Error("failed to remove temp part", slog.String("bucket", bucket), slog.String("partKey", pk), slog.Any("error", err))
			} else {
				slog.Debug("temp part removed", slog.String("bucket", bucket), slog.String("partKey", pk))
			}
		}

		if err := storage.redisConnection.Del(ctx, metaKey, bitmapKey).Err(); err != nil {
			slog.Error("failed to delete redis keys", slog.String("uploadID", uploadID), slog.Any("error", err))
		}

		slog.Info("upload completed", slog.String("bucket", bucket), slog.String("uploadID", uploadID), slog.String("finalKey", finalKey))
	}

	return &pb.UploadPartResponse{
		Ok: true,
	}, nil
}

func (storage StorageGRPCServer) DownloadRange(ctx context.Context, req *pb.DownloadRangeRequest) (*pb.DownloadRangeResponse, error) {
	bucket := ctx.Value(interceptor.UserIDKey).(string)
	if len(bucket) == 0 {
		return nil, status.Error(codes.InvalidArgument, "invalid bucket")
	}

	offset := int64(req.GetOffset())
	limit := int64(req.GetLimit())

	slog.Info("download chunk",
		slog.String("bucket", bucket),
		slog.String("prefix", req.GetPrefix()),
		slog.Int64("limit", limit),
		slog.Int64("offset", offset),
	)

	opts := minio.GetObjectOptions{}
	opts.SetRange(offset, offset+limit-1)

	artifact, err := storage.minioConnection.GetObject(ctx, bucket, req.GetPrefix(), opts)
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}
	defer artifact.Close()

	stat, err := artifact.Stat()
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}

	buf := make([]byte, limit)
	expected := limit
	if offset+limit > stat.Size {
		expected = stat.Size - offset
	}

	n, err := artifact.ReadAt(buf, offset)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		slog.Error("read object at offset error", slog.Any("error", err), slog.Int64("offset", offset))
		return nil, status.Error(codes.Internal, "internal error")
	}

	if int64(n) != expected {
		slog.Error("invalid chunk read",
			slog.String("prefix", req.GetPrefix()),
			slog.Int64("offset", offset),
			slog.Int64("expected", expected),
			slog.Int64("read", int64(n)),
		)
		return nil, status.Error(codes.Internal, "invalid chunk size")
	}

	return &pb.DownloadRangeResponse{
		Offset:    req.Offset,
		Data:      buf[:n],
		TotalSize: uint64(stat.Size),
	}, nil
}

func (storage StorageGRPCServer) List(ctx context.Context, req *pb.ListRequest) (*pb.ListResponse, error) {

	bucket := ctx.Value(interceptor.UserIDKey).(string)
	if len(bucket) == 0 {
		return nil, status.Error(codes.InvalidArgument, "invalid bucket")
	}

	slog.Info("get artifacts list", slog.String("bucket", bucket), "prefix", req.Prefix)

	fileInfoList := make([]*pb.FileInfo, 0)

	for artifact := range storage.minioConnection.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: req.Prefix, Recursive: true, WithMetadata: true}) {
		meta := artifact.UserMetadata

		isDirectory, err := strconv.ParseBool(meta[isDirectoryKeyList])

		if err != nil {
			isDirectory = false
		}

		fileInfoList = append(fileInfoList, &pb.FileInfo{
			Prefix:        artifact.Key,
			OriginalPath:  meta[originalPathKeyList],
			Size:          uint64(artifact.Size),
			CreatedAtUnix: artifact.LastModified.Unix(),
			IsDirectory:   isDirectory,
			Sha256:        meta[sha256KeyList],
		})
	}

	return &pb.ListResponse{
		Files: fileInfoList,
	}, status.Error(codes.OK, "success get artifacts list")
}

func (storage StorageGRPCServer) GetFileInfo(ctx context.Context, req *pb.FileInfoRequest) (*pb.FileInfo, error) {

	bucket := ctx.Value(interceptor.UserIDKey).(string)

	if len(bucket) == 0 {
		return nil, status.Error(codes.InvalidArgument, "invalid bucket")
	}

	slog.Info("get artifact info", slog.String("bucket", bucket), "prefix", req.Prefix)
	artifact, err := storage.minioConnection.GetObject(ctx, bucket, req.Prefix, minio.GetObjectOptions{})
	if err != nil {
		slog.Error("get artifact info error", slog.String("bucket", bucket), "prefix", req.Prefix, slog.Any("error", err))
		return nil, status.Error(codes.Internal, "get artifact info error")
	}

	info, err := artifact.Stat()
	if err != nil {
		slog.Error("get artifact attributes error", slog.String("bucket", bucket), "prefix", req.Prefix, slog.Any("error", err))
		return nil, status.Error(codes.Internal, "get artifact info error")
	}

	isDirectory, err := strconv.ParseBool(info.UserMetadata[isDirectoryKey])

	if err != nil {
		isDirectory = false
	}
	return &pb.FileInfo{
		Prefix:        req.GetPrefix(),
		OriginalPath:  info.UserMetadata[originalPathKey],
		Size:          uint64(info.Size),
		CreatedAtUnix: info.LastModified.Unix(),
		IsDirectory:   isDirectory,
		Sha256:        info.UserMetadata[sha256Key],
	}, nil
}

func (storage StorageGRPCServer) Delete(ctx context.Context, req *pb.DeleteRequest) (*pb.DeleteResponse, error) {

	bucket := ctx.Value(interceptor.UserIDKey).(string)
	if len(bucket) == 0 {
		return nil, status.Error(codes.InvalidArgument, "invalid bucket")
	}

	slog.Info("delete artifact", slog.String("bucket", bucket), "prefix", req.Prefix)

	err := storage.minioConnection.RemoveObject(ctx, bucket, req.GetPrefix(), minio.RemoveObjectOptions{ForceDelete: true})

	if err != nil {
		slog.Error("delete artifact error", slog.String("bucket", bucket), "prefix", req.Prefix, slog.Any("error", err))
		return nil, status.Error(codes.Internal, "delete artifact error")
	}

	return &pb.DeleteResponse{
		Success: true,
	}, status.Error(codes.OK, "success delete artifact")
}

func (storage StorageGRPCServer) checkBucket(ctx context.Context) (string, error) {
	bucket := ctx.Value(interceptor.UserIDKey).(string)
	slog.Debug("check bucket", slog.String("bucket", bucket))
	if len(bucket) == 0 {
		return "", status.Error(codes.InvalidArgument, "invalid bucket")
	}

	exists, err := storage.minioConnection.BucketExists(ctx, bucket)

	if err != nil {
		slog.Error("check exists bucket error", slog.Bool("exists", exists), slog.String("bucket", bucket), slog.Any("error", err))
		return "", err
	}

	if !exists {
		slog.Info("create bucket", slog.String("bucket", bucket))
		err = storage.minioConnection.MakeBucket(ctx, bucket, minio.MakeBucketOptions{ForceCreate: true})

		if err != nil {
			slog.Error("create bucket error", slog.String("bucket", bucket), slog.Any("error", err))
			return "", err
		}
	}

	return bucket, nil
}
