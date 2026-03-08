package server

import (
	"bytes"
	"context"
	"io"
	"math"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	pb "github.com/gabkaclassic/gorbage/internal/proto"
	"github.com/gabkaclassic/gorbage/pkg/interceptor"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	minioTestImage string = "minio/minio:RELEASE.2024-01-16T16-07-38Z"
	redisTestImage string = "redis:7"
)

func TestNewArtifactGRPCServer(t *testing.T) {
	tests := []struct {
		name            string
		minioConnection *minio.Client
		redisConnection *redis.Client
		uploadTTL       time.Duration
		chunkSize       int64
		wantErr         bool
		errMsg          string
	}{
		{
			name:            "successful creation",
			minioConnection: &minio.Client{},
			redisConnection: &redis.Client{},
			uploadTTL:       time.Hour,
			chunkSize:       1024,
			wantErr:         false,
		},
		{
			name:            "nil minio connection",
			minioConnection: nil,
			redisConnection: &redis.Client{},
			uploadTTL:       time.Hour,
			chunkSize:       1024,
			wantErr:         true,
			errMsg:          "minio connection can not be nil",
		},
		{
			name:            "nil redis connection",
			minioConnection: &minio.Client{},
			redisConnection: nil,
			uploadTTL:       time.Hour,
			chunkSize:       1024,
			wantErr:         true,
			errMsg:          "redis connection can not be nil",
		},
		{
			name:            "both connections nil",
			minioConnection: nil,
			redisConnection: nil,
			uploadTTL:       time.Hour,
			chunkSize:       1024,
			wantErr:         true,
			errMsg:          "minio connection can not be nil",
		},
		{
			name:            "zero upload TTL",
			minioConnection: &minio.Client{},
			redisConnection: &redis.Client{},
			uploadTTL:       0,
			chunkSize:       1024,
			wantErr:         true,
		},
		{
			name:            "negative upload TTL",
			minioConnection: &minio.Client{},
			redisConnection: &redis.Client{},
			uploadTTL:       -time.Hour,
			chunkSize:       1024,
			wantErr:         true,
		},
		{
			name:            "zero chunk size",
			minioConnection: &minio.Client{},
			redisConnection: &redis.Client{},
			uploadTTL:       time.Hour,
			chunkSize:       0,
			wantErr:         true,
		},
		{
			name:            "negative chunk size",
			minioConnection: &minio.Client{},
			redisConnection: &redis.Client{},
			uploadTTL:       time.Hour,
			chunkSize:       -1024,
			wantErr:         true,
		},
		{
			name:            "max int64 chunk size",
			minioConnection: &minio.Client{},
			redisConnection: &redis.Client{},
			uploadTTL:       time.Hour,
			chunkSize:       math.MaxInt64,
			wantErr:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewArtifactGRPCServer(
				tt.minioConnection,
				tt.redisConnection,
				tt.uploadTTL,
				tt.chunkSize,
			)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				assert.Equal(t, StorageGRPCServer{}, server)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, server.minioConnection)
				assert.NotNil(t, server.redisConnection)
				assert.Equal(t, tt.uploadTTL, server.uploadTTL)
				assert.Equal(t, uint64(tt.chunkSize), server.chunkSize)
			}
		})
	}
}

func TestGetBitmapKey(t *testing.T) {
	tests := []struct {
		name     string
		uploadID string
		want     string
	}{
		{
			name:     "valid upload ID",
			uploadID: "test-upload-123",
			want:     "uploads:test-upload-123:bitmap",
		},
		{
			name:     "empty upload ID",
			uploadID: "",
			want:     "uploads::bitmap",
		},
		{
			name:     "upload ID with special characters",
			uploadID: "test@#$%^&*()",
			want:     "uploads:test@#$%^&*():bitmap",
		},
		{
			name:     "upload ID with spaces",
			uploadID: "test upload 123",
			want:     "uploads:test upload 123:bitmap",
		},
		{
			name:     "upload ID with leading colon",
			uploadID: ":test:",
			want:     "uploads::test::bitmap",
		},
		{
			name:     "upload ID with numbers only",
			uploadID: "123456",
			want:     "uploads:123456:bitmap",
		},
		{
			name:     "upload ID with UUID format",
			uploadID: "123e4567-e89b-12d3-a456-426614174000",
			want:     "uploads:123e4567-e89b-12d3-a456-426614174000:bitmap",
		},
		{
			name:     "very long upload ID",
			uploadID: string(make([]byte, 1000)),
			want:     "uploads:" + string(make([]byte, 1000)) + ":bitmap",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getBitmapKey(tt.uploadID)
			assert.Equal(t, tt.want, got)
			assert.Contains(t, got, "uploads:")
			assert.Contains(t, got, ":bitmap")
			assert.Contains(t, got, tt.uploadID)
		})
	}
}

func TestGetMetaKey(t *testing.T) {
	tests := []struct {
		name     string
		uploadID string
		want     string
	}{
		{
			name:     "valid upload ID",
			uploadID: "test-upload-123",
			want:     "uploads:test-upload-123:meta",
		},
		{
			name:     "empty upload ID",
			uploadID: "",
			want:     "uploads::meta",
		},
		{
			name:     "upload ID with special characters",
			uploadID: "test@#$%^&*()",
			want:     "uploads:test@#$%^&*():meta",
		},
		{
			name:     "upload ID with spaces",
			uploadID: "test upload 123",
			want:     "uploads:test upload 123:meta",
		},
		{
			name:     "upload ID with leading colon",
			uploadID: ":test:",
			want:     "uploads::test::meta",
		},
		{
			name:     "upload ID with numbers only",
			uploadID: "123456",
			want:     "uploads:123456:meta",
		},
		{
			name:     "upload ID with UUID format",
			uploadID: "123e4567-e89b-12d3-a456-426614174000",
			want:     "uploads:123e4567-e89b-12d3-a456-426614174000:meta",
		},
		{
			name:     "very long upload ID",
			uploadID: string(make([]byte, 1000)),
			want:     "uploads:" + string(make([]byte, 1000)) + ":meta",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getMetaKey(tt.uploadID)
			assert.Equal(t, tt.want, got)
			assert.Contains(t, got, "uploads:")
			assert.Contains(t, got, ":meta")
			assert.Contains(t, got, tt.uploadID)
		})
	}
}

func TestCheckBucket(t *testing.T) {
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        minioTestImage,
		ExposedPorts: []string{"9000/tcp"},
		Cmd:          []string{"server", "/data"},
		Env: map[string]string{
			"MINIO_ROOT_USER":     "minio",
			"MINIO_ROOT_PASSWORD": "minio123",
		},
		WaitingFor: wait.ForHTTP("/minio/health/ready").WithPort("9000/tcp"),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)

	defer container.Terminate(ctx)

	host, err := container.Host(ctx)
	require.NoError(t, err)

	port, err := container.MappedPort(ctx, "9000")
	require.NoError(t, err)

	endpoint := net.JoinHostPort(host, port.Port())

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4("minio", "minio123", ""),
		Secure: false,
	})
	require.NoError(t, err)

	tests := []struct {
		name         string
		bucket       string
		precreate    bool
		expectErr    bool
		expectBucket string
	}{
		{
			name:      "empty bucket",
			bucket:    "",
			expectErr: true,
		},
		{
			name:         "bucket exists",
			bucket:       "bucket1",
			precreate:    true,
			expectBucket: "bucket1",
		},
		{
			name:         "bucket create",
			bucket:       "bucket2",
			expectBucket: "bucket2",
		},
		{
			name:         "bucket exists second call",
			bucket:       "bucket3",
			expectBucket: "bucket3",
		},
		{
			name:         "bucket exists after create",
			bucket:       "bucket3",
			expectBucket: "bucket3",
		},
	}

	storage := StorageGRPCServer{
		minioConnection: client,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			if tt.precreate {
				err := client.MakeBucket(ctx, tt.bucket, minio.MakeBucketOptions{})
				require.NoError(t, err)
			}

			c := context.WithValue(ctx, interceptor.UserIDKey, tt.bucket)

			b, err := storage.checkBucket(c)

			if tt.expectErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expectBucket, b)

			exists, err := client.BucketExists(ctx, tt.bucket)
			assert.NoError(t, err)
			assert.True(t, exists)
		})
	}
}

func TestInitiateUpload(t *testing.T) {
	ctx := context.Background()

	minioReq := testcontainers.ContainerRequest{
		Image:        minioTestImage,
		ExposedPorts: []string{"9000/tcp"},
		Cmd:          []string{"server", "/data"},
		Env: map[string]string{
			"MINIO_ROOT_USER":     "minio",
			"MINIO_ROOT_PASSWORD": "minio123",
		},
		WaitingFor: wait.ForHTTP("/minio/health/ready").WithPort("9000/tcp"),
	}

	minioC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: minioReq,
		Started:          true,
	})
	require.NoError(t, err)
	defer minioC.Terminate(ctx)

	minioHost, err := minioC.Host(ctx)
	require.NoError(t, err)

	minioPort, err := minioC.MappedPort(ctx, "9000")
	require.NoError(t, err)

	minioEndpoint := net.JoinHostPort(minioHost, minioPort.Port())

	minioClient, err := minio.New(minioEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4("minio", "minio123", ""),
		Secure: false,
	})
	require.NoError(t, err)

	redisReq := testcontainers.ContainerRequest{
		Image:        redisTestImage,
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor:   wait.ForListeningPort("6379/tcp"),
	}

	redisC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: redisReq,
		Started:          true,
	})
	require.NoError(t, err)
	defer redisC.Terminate(ctx)

	redisHost, err := redisC.Host(ctx)
	require.NoError(t, err)

	redisPort, err := redisC.MappedPort(ctx, "6379")
	require.NoError(t, err)

	redisClient := redis.NewClient(&redis.Options{
		Addr: net.JoinHostPort(redisHost, redisPort.Port()),
	})

	storage := StorageGRPCServer{
		minioConnection: minioClient,
		redisConnection: redisClient,
		uploadTTL:       time.Minute,
		chunkSize:       1024,
	}

	tests := []struct {
		name        string
		bucket      string
		meta        *pb.FileMetadata
		chunkSize   uint64
		expectErr   bool
		expectTotal uint64
	}{
		{
			name:      "missing meta",
			bucket:    "bucket1",
			meta:      nil,
			expectErr: true,
		},
		{
			name:   "default chunk size",
			bucket: "bucket2",
			meta: &pb.FileMetadata{
				Size: 2048,
			},
			expectTotal: 2,
		},
		{
			name:   "custom chunk size",
			bucket: "bucket3",
			meta: &pb.FileMetadata{
				Size: 4096,
			},
			chunkSize:   2048,
			expectTotal: 2,
		},
		{
			name:   "zero size file",
			bucket: "bucket4",
			meta: &pb.FileMetadata{
				Size: 0,
			},
			expectTotal: 0,
		},
		{
			name:   "single chunk",
			bucket: "bucket5",
			meta: &pb.FileMetadata{
				Size: 500,
			},
			expectTotal: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			c := context.WithValue(ctx, interceptor.UserIDKey, tt.bucket)

			req := &pb.InitiateUploadRequest{
				Meta:               tt.meta,
				PreferredChunkSize: tt.chunkSize,
			}

			resp, err := storage.InitiateUpload(c, req)

			if tt.expectErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.NotEmpty(t, resp.UploadId)
			assert.NotZero(t, resp.ExpiresAtUnix)

			if tt.chunkSize == 0 {
				assert.Equal(t, storage.chunkSize, resp.ChunkSize)
			} else {
				assert.Equal(t, tt.chunkSize, resp.ChunkSize)
			}

			assert.Equal(t, tt.expectTotal, resp.TotalChunks)

			metaKey := getMetaKey(resp.UploadId)
			bitmapKey := getBitmapKey(resp.UploadId)

			_, err = redisClient.Get(ctx, metaKey).Bytes()
			assert.NoError(t, err)

			_, err = redisClient.Get(ctx, bitmapKey).Bytes()
			assert.NoError(t, err)
		})
	}
}

func TestUploadPart(t *testing.T) {
	ctx := context.Background()

	minioReq := testcontainers.ContainerRequest{
		Image:        minioTestImage,
		ExposedPorts: []string{"9000/tcp"},
		Cmd:          []string{"server", "/data"},
		Env: map[string]string{
			"MINIO_ROOT_USER":     "minio",
			"MINIO_ROOT_PASSWORD": "minio123",
		},
		WaitingFor: wait.ForHTTP("/minio/health/ready").WithPort("9000/tcp"),
	}

	minioC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: minioReq,
		Started:          true,
	})
	require.NoError(t, err)
	defer minioC.Terminate(ctx)

	minioHost, err := minioC.Host(ctx)
	require.NoError(t, err)

	minioPort, err := minioC.MappedPort(ctx, "9000")
	require.NoError(t, err)

	minioEndpoint := net.JoinHostPort(minioHost, minioPort.Port())

	minioClient, err := minio.New(minioEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4("minio", "minio123", ""),
		Secure: false,
	})
	require.NoError(t, err)

	redisReq := testcontainers.ContainerRequest{
		Image:        redisTestImage,
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor:   wait.ForListeningPort("6379/tcp"),
	}

	redisC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: redisReq,
		Started:          true,
	})
	require.NoError(t, err)
	defer redisC.Terminate(ctx)

	redisHost, err := redisC.Host(ctx)
	require.NoError(t, err)

	redisPort, err := redisC.MappedPort(ctx, "6379")
	require.NoError(t, err)

	redisClient := redis.NewClient(&redis.Options{
		Addr: net.JoinHostPort(redisHost, redisPort.Port()),
	})

	storage := StorageGRPCServer{
		minioConnection: minioClient,
		redisConnection: redisClient,
		uploadTTL:       time.Minute,
		chunkSize:       1024,
	}

	tests := []struct {
		name        string
		bucket      string
		setup       func(uploadID string)
		req         func(uploadID string) *pb.UploadPartRequest
		expectErr   bool
		expectAssem bool
	}{
		{
			name:   "invalid bucket",
			bucket: "",
			req: func(uploadID string) *pb.UploadPartRequest {
				return &pb.UploadPartRequest{UploadId: uploadID, Seq: 0, Data: []byte("a")}
			},
			expectErr: true,
		},
		{
			name:   "session not found",
			bucket: "bucket1",
			req: func(uploadID string) *pb.UploadPartRequest {
				return &pb.UploadPartRequest{UploadId: uploadID, Seq: 0, Data: []byte("a")}
			},
			expectErr: true,
		},
		{
			name:   "invalid seq",
			bucket: "bucket2",
			setup: func(uploadID string) {
				meta := &pb.FileMetadata{Size: 10}
				init := &pb.InitiateUploadRequest{Meta: meta, PreferredChunkSize: 10}
				data, _ := proto.Marshal(init)
				redisClient.Set(ctx, getMetaKey(uploadID), data, time.Minute)
				redisClient.Set(ctx, getBitmapKey(uploadID), []byte{}, time.Minute)
				minioClient.MakeBucket(ctx, "bucket2", minio.MakeBucketOptions{})
			},
			req: func(uploadID string) *pb.UploadPartRequest {
				return &pb.UploadPartRequest{UploadId: uploadID, Seq: 2, Data: []byte("a")}
			},
			expectErr: true,
		},
		{
			name:   "upload single chunk assemble",
			bucket: "bucket3",
			setup: func(uploadID string) {
				meta := &pb.FileMetadata{
					Size:         4,
					OriginalPath: "f",
					IsDirectory:  false,
					Sha256:       "x",
				}
				init := &pb.InitiateUploadRequest{
					Meta:               meta,
					PreferredChunkSize: 10,
					Prefix:             "final",
				}
				data, _ := proto.Marshal(init)
				redisClient.Set(ctx, getMetaKey(uploadID), data, time.Minute)
				redisClient.Set(ctx, getBitmapKey(uploadID), []byte{}, time.Minute)
				minioClient.MakeBucket(ctx, "bucket3", minio.MakeBucketOptions{})
			},
			req: func(uploadID string) *pb.UploadPartRequest {
				return &pb.UploadPartRequest{UploadId: uploadID, Seq: 0, Data: []byte("data")}
			},
			expectAssem: true,
		},
		{
			name:   "upload first chunk only",
			bucket: "bucket4",
			setup: func(uploadID string) {
				meta := &pb.FileMetadata{
					Size:         20,
					OriginalPath: "f",
					IsDirectory:  false,
					Sha256:       "x",
				}
				init := &pb.InitiateUploadRequest{
					Meta:               meta,
					PreferredChunkSize: 10,
					Prefix:             "final",
				}
				data, _ := proto.Marshal(init)
				redisClient.Set(ctx, getMetaKey(uploadID), data, time.Minute)
				redisClient.Set(ctx, getBitmapKey(uploadID), []byte{}, time.Minute)
				minioClient.MakeBucket(ctx, "bucket4", minio.MakeBucketOptions{})
			},
			req: func(uploadID string) *pb.UploadPartRequest {
				return &pb.UploadPartRequest{UploadId: uploadID, Seq: 0, Data: []byte("chunk")}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			uploadID := uuid.NewString()

			if tt.setup != nil {
				tt.setup(uploadID)
			}

			c := context.WithValue(ctx, interceptor.UserIDKey, tt.bucket)

			resp, err := storage.UploadPart(c, tt.req(uploadID))

			if tt.expectErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.True(t, resp.Ok)

			if tt.expectAssem {
				obj, err := minioClient.GetObject(ctx, tt.bucket, "final", minio.GetObjectOptions{})
				assert.NoError(t, err)
				_, err = io.ReadAll(obj)
				assert.NoError(t, err)
			}
		})
	}
}

func TestDownloadRange(t *testing.T) {
	ctx := context.Background()

	minioReq := testcontainers.ContainerRequest{
		Image:        minioTestImage,
		ExposedPorts: []string{"9000/tcp"},
		Cmd:          []string{"server", "/data"},
		Env: map[string]string{
			"MINIO_ROOT_USER":     "minio",
			"MINIO_ROOT_PASSWORD": "minio123",
		},
		WaitingFor: wait.ForHTTP("/minio/health/ready").WithPort("9000/tcp"),
	}

	minioC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: minioReq,
		Started:          true,
	})
	require.NoError(t, err)
	defer minioC.Terminate(ctx)

	host, err := minioC.Host(ctx)
	require.NoError(t, err)

	port, err := minioC.MappedPort(ctx, "9000")
	require.NoError(t, err)

	endpoint := net.JoinHostPort(host, port.Port())

	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4("minio", "minio123", ""),
		Secure: false,
	})
	require.NoError(t, err)

	storage := StorageGRPCServer{
		minioConnection: minioClient,
	}

	bucket := "bucket1"
	err = minioClient.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
	require.NoError(t, err)

	data := []byte("abcdefghijklmnopqrstuvwxyz")

	_, err = minioClient.PutObject(
		ctx,
		bucket,
		"file",
		bytes.NewReader(data),
		int64(len(data)),
		minio.PutObjectOptions{},
	)
	require.NoError(t, err)

	tests := []struct {
		name        string
		bucket      string
		offset      uint64
		limit       uint64
		expectErr   bool
		expectData  []byte
		expectTotal uint64
	}{
		{
			name:      "invalid bucket",
			bucket:    "",
			offset:    0,
			limit:     5,
			expectErr: true,
		},
		{
			name:        "first chunk",
			bucket:      bucket,
			offset:      0,
			limit:       5,
			expectData:  []byte("abcde"),
			expectTotal: uint64(len(data)),
		},
		{
			name:        "middle chunk",
			bucket:      bucket,
			offset:      5,
			limit:       5,
			expectData:  []byte("fghij"),
			expectTotal: uint64(len(data)),
		},
		{
			name:        "tail chunk",
			bucket:      bucket,
			offset:      23,
			limit:       5,
			expectData:  []byte("xyz"),
			expectTotal: uint64(len(data)),
		},
		{
			name:        "full file",
			bucket:      bucket,
			offset:      0,
			limit:       uint64(len(data)),
			expectData:  data,
			expectTotal: uint64(len(data)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			c := context.WithValue(ctx, interceptor.UserIDKey, tt.bucket)

			req := &pb.DownloadRangeRequest{
				Prefix: "file",
				Offset: tt.offset,
				Limit:  tt.limit,
			}

			resp, err := storage.DownloadRange(c, req)

			if tt.expectErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expectTotal, resp.TotalSize)
			assert.Equal(t, tt.offset, resp.Offset)
			assert.Equal(t, tt.expectData, resp.Data)
		})
	}
}

func TestList(t *testing.T) {
	ctx := context.Background()

	minioReq := testcontainers.ContainerRequest{
		Image:        minioTestImage,
		ExposedPorts: []string{"9000/tcp"},
		Cmd:          []string{"server", "/data"},
		Env: map[string]string{
			"MINIO_ROOT_USER":     "minio",
			"MINIO_ROOT_PASSWORD": "minio123",
		},
		WaitingFor: wait.ForHTTP("/minio/health/ready").WithPort("9000/tcp"),
	}

	minioC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: minioReq,
		Started:          true,
	})
	require.NoError(t, err)
	defer minioC.Terminate(ctx)

	host, err := minioC.Host(ctx)
	require.NoError(t, err)

	port, err := minioC.MappedPort(ctx, "9000")
	require.NoError(t, err)

	endpoint := net.JoinHostPort(host, port.Port())

	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4("minio", "minio123", ""),
		Secure: false,
	})
	require.NoError(t, err)

	storage := StorageGRPCServer{
		minioConnection: minioClient,
	}

	bucket := "bucket1"
	err = minioClient.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
	require.NoError(t, err)

	put := func(key string, data []byte, meta map[string]string) {
		_, err := minioClient.PutObject(
			ctx,
			bucket,
			key,
			bytes.NewReader(data),
			int64(len(data)),
			minio.PutObjectOptions{UserMetadata: meta},
		)
		require.NoError(t, err)
	}

	put("a/file1", []byte("data1"), map[string]string{
		originalPathKey: "orig1",
		isDirectoryKey:  "false",
		sha256Key:       "s1",
	})

	put("a/file2", []byte("data22"), map[string]string{
		originalPathKey: "orig2",
		isDirectoryKey:  "false",
		sha256Key:       "s2",
	})

	put("b/dir", []byte(""), map[string]string{
		originalPathKey: "dir",
		isDirectoryKey:  "true",
		sha256Key:       "",
	})

	tests := []struct {
		name        string
		bucket      string
		prefix      string
		expectErr   bool
		expectCount int
	}{
		{
			name:      "invalid bucket",
			bucket:    "",
			prefix:    "",
			expectErr: true,
		},
		{
			name:        "list all",
			bucket:      bucket,
			prefix:      "",
			expectCount: 3,
		},
		{
			name:        "list prefix a",
			bucket:      bucket,
			prefix:      "a/",
			expectCount: 2,
		},
		{
			name:        "list prefix b",
			bucket:      bucket,
			prefix:      "b/",
			expectCount: 1,
		},
		{
			name:        "list empty prefix",
			bucket:      bucket,
			prefix:      "none/",
			expectCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			c := context.WithValue(ctx, interceptor.UserIDKey, tt.bucket)

			resp, err := storage.List(c, &pb.ListRequest{
				Prefix: tt.prefix,
			})

			if tt.expectErr {
				assert.Error(t, err)
				return
			}

			assert.Equal(t, codes.OK, status.Code(err))
			assert.Len(t, resp.Files, tt.expectCount)
		})
	}
}

func TestGetFileInfo(t *testing.T) {
	ctx := context.Background()

	minioReq := testcontainers.ContainerRequest{
		Image:        minioTestImage,
		ExposedPorts: []string{"9000/tcp"},
		Cmd:          []string{"server", "/data"},
		Env: map[string]string{
			"MINIO_ROOT_USER":     "minio",
			"MINIO_ROOT_PASSWORD": "minio123",
		},
		WaitingFor: wait.ForHTTP("/minio/health/ready").WithPort("9000/tcp"),
	}

	minioC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: minioReq,
		Started:          true,
	})
	require.NoError(t, err)
	defer minioC.Terminate(ctx)

	host, err := minioC.Host(ctx)
	require.NoError(t, err)

	port, err := minioC.MappedPort(ctx, "9000")
	require.NoError(t, err)

	endpoint := net.JoinHostPort(host, port.Port())

	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4("minio", "minio123", ""),
		Secure: false,
	})
	require.NoError(t, err)

	storage := StorageGRPCServer{
		minioConnection: minioClient,
	}

	bucket := "bucket1"
	err = minioClient.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
	require.NoError(t, err)

	put := func(key string, data []byte, meta map[string]string) {
		_, err := minioClient.PutObject(
			ctx,
			bucket,
			key,
			bytes.NewReader(data),
			int64(len(data)),
			minio.PutObjectOptions{UserMetadata: meta},
		)
		require.NoError(t, err)
	}

	put("file1", []byte("data1"), map[string]string{
		originalPathKey: "orig1",
		isDirectoryKey:  "false",
		sha256Key:       "s1",
	})

	put("dir1", []byte(""), map[string]string{
		originalPathKey: "dir",
		isDirectoryKey:  "true",
		sha256Key:       "",
	})

	tests := []struct {
		name       string
		bucket     string
		prefix     string
		expectErr  bool
		expectSize uint64
		expectDir  bool
	}{
		{
			name:      "invalid bucket",
			bucket:    "",
			prefix:    "file1",
			expectErr: true,
		},
		{
			name:       "file info",
			bucket:     bucket,
			prefix:     "file1",
			expectSize: 5,
			expectDir:  false,
		},
		{
			name:       "directory info",
			bucket:     bucket,
			prefix:     "dir1",
			expectSize: 0,
			expectDir:  true,
		},
		{
			name:      "not exists",
			bucket:    bucket,
			prefix:    "missing",
			expectErr: true,
		},
		{
			name:       "invalid directory meta",
			bucket:     bucket,
			prefix:     "file1",
			expectSize: 5,
			expectDir:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			c := context.WithValue(ctx, interceptor.UserIDKey, tt.bucket)

			resp, err := storage.GetFileInfo(c, &pb.FileInfoRequest{
				Prefix: tt.prefix,
			})

			if tt.expectErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.prefix, resp.Prefix)
			assert.Equal(t, tt.expectSize, resp.Size)
			assert.Equal(t, tt.expectDir, resp.IsDirectory)
		})
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()

	minioReq := testcontainers.ContainerRequest{
		Image:        minioTestImage,
		ExposedPorts: []string{"9000/tcp"},
		Cmd:          []string{"server", "/data"},
		Env: map[string]string{
			"MINIO_ROOT_USER":     "minio",
			"MINIO_ROOT_PASSWORD": "minio123",
		},
		WaitingFor: wait.ForHTTP("/minio/health/ready").WithPort("9000/tcp"),
	}

	minioC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: minioReq,
		Started:          true,
	})
	require.NoError(t, err)
	defer minioC.Terminate(ctx)

	host, err := minioC.Host(ctx)
	require.NoError(t, err)

	port, err := minioC.MappedPort(ctx, "9000")
	require.NoError(t, err)

	endpoint := net.JoinHostPort(host, port.Port())

	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4("minio", "minio123", ""),
		Secure: false,
	})
	require.NoError(t, err)

	storage := StorageGRPCServer{
		minioConnection: minioClient,
	}

	bucket := "bucket1"
	err = minioClient.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
	require.NoError(t, err)

	put := func(key string) {
		_, err := minioClient.PutObject(
			ctx,
			bucket,
			key,
			bytes.NewReader([]byte("data")),
			4,
			minio.PutObjectOptions{},
		)
		require.NoError(t, err)
	}

	put("file1")
	put("file2")

	tests := []struct {
		name        string
		bucket      string
		prefix      string
		expectErr   bool
		expectExist bool
	}{
		{
			name:      "invalid bucket",
			bucket:    "",
			prefix:    "file1",
			expectErr: true,
		},
		{
			name:        "delete existing",
			bucket:      bucket,
			prefix:      "file1",
			expectExist: false,
		},
		{
			name:        "delete second file",
			bucket:      bucket,
			prefix:      "file2",
			expectExist: false,
		},
		{
			name:        "delete missing",
			bucket:      bucket,
			prefix:      "missing",
			expectExist: false,
		},
		{
			name:        "delete twice",
			bucket:      bucket,
			prefix:      "file1",
			expectExist: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			c := context.WithValue(ctx, interceptor.UserIDKey, tt.bucket)

			resp, err := storage.Delete(c, &pb.DeleteRequest{
				Prefix: tt.prefix,
			})

			if tt.expectErr {
				assert.Error(t, err)
				return
			}

			assert.Equal(t, codes.OK, status.Code(err))
			assert.True(t, resp.Success)

			_, err = minioClient.StatObject(ctx, bucket, tt.prefix, minio.StatObjectOptions{})

			if tt.expectExist {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
