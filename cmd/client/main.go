package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gabkaclassic/gorbage/internal/client"
	"github.com/gabkaclassic/gorbage/internal/config"
	"github.com/gabkaclassic/metrics/pkg/logger"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.ParseClientConfig()
	if err != nil {
		return fmt.Errorf("failed to parse command or config: %w", err)
	}

	logger.SetupLogger(logger.LogConfig(cfg.Log))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	defer stop()

	grpcConnection, err := client.NewGRPCConnection(cfg.Server)

	if err != nil {
		return fmt.Errorf("failed to create connection with grpc server: %w", err)
	}

	storageClient := client.NewStorageClient(grpcConnection)

	err = client.ProcessCommand(ctx, storageClient, cfg.Command)

	return err
}
