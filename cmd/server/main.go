package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/gabkaclassic/gorbage/internal/config"
	"github.com/gabkaclassic/gorbage/internal/minio"
	"github.com/gabkaclassic/gorbage/internal/redis"
	"github.com/gabkaclassic/gorbage/internal/server"
	"github.com/gabkaclassic/metrics/pkg/logger"
)

var (
	buildVersion string
	buildDate    string
	buildCommit  string
)

func main() {
	printTags()

	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func printTags() {
	if buildVersion == "" {
		buildVersion = "N/A"
	}
	if buildDate == "" {
		buildDate = "N/A"
	}
	if buildCommit == "" {
		buildCommit = "N/A"
	}

	fmt.Printf("Build version: %s\n", buildVersion)
	fmt.Printf("Build date: %s\n", buildDate)
	fmt.Printf("Build commit: %s\n", buildCommit)
}

func run() error {
	cfg, err := config.ParseServerConfig()
	if err != nil {
		return fmt.Errorf("failed to parse server configuration: %w", err)
	}
	logger.SetupLogger(logger.LogConfig(cfg.Log))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	defer stop()

	minioConnection, err := minio.NewMinioConnection(cfg.Minio)

	if err != nil {
		return fmt.Errorf("failed to create minio connection: %w", err)
	}

	redisConnection, err := redis.NewRedisConnection(ctx, cfg.Redis)

	if err != nil {
		return fmt.Errorf("failed to create redis connection: %w", err)
	}

	artifactStorage, err := server.NewArtifactGRPCServer(minioConnection, redisConnection, cfg.UploadTTL, cfg.ChunkSize)
	if err != nil {
		return fmt.Errorf("failed to setup artifact storage: %w", err)
	}

	grpcServer, err := server.SetupGRPCServer(cfg.GRPC, artifactStorage)
	if err != nil {
		return fmt.Errorf("failed to setup gRPC server: %w", err)
	}

	go grpcServer.Run(ctx, cfg.GRPC.Address)

	<-ctx.Done()
	slog.Info("Shutdown complete")

	return nil
}
