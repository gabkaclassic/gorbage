package client

import (
	"context"
	"errors"
	"log/slog"

	"github.com/gabkaclassic/gorbage/internal/config"
)

func ProcessCommand(ctx context.Context, storageClient StorageClient, cmd config.Command) error {

	switch cmd.Name {
	case "pull":
		slog.Debug(
			"pull artifact",
			slog.String("prefix", cmd.Prefix),
			slog.String("filepath", cmd.FilePath),
		)

		if len(cmd.Prefix) == 0 {
			return errors.New("artifact prefix is required to pull")
		}
		if len(cmd.FilePath) == 0 {
			return errors.New("file path is required to pull")
		}

		err := storageClient.DownloadArtifact(ctx, cmd.Prefix, cmd.FilePath, cmd.CipherKeyPath, cmd.Workers, cmd.ChunkSize)
		if err != nil {
			return err
		}
	case "push":
		slog.Debug(
			"push artifact",
			slog.String("dest", cmd.FilePath),
		)

		if len(cmd.FilePath) == 0 {
			return errors.New("file path is required to push")
		}
		if len(cmd.Prefix) == 0 {
			cmd.Prefix = cmd.FilePath
		}
		err := storageClient.UploadArtifact(ctx, cmd.Prefix, cmd.FilePath, cmd.CipherKeyPath, cmd.Workers, cmd.ChunkSize)
		if err != nil {
			return err
		}
	case "del":
		slog.Debug(
			"delete artifact",
			slog.String("ID", cmd.Prefix),
		)
		if len(cmd.Prefix) == 0 {
			return errors.New("artifact prefix is required to delete")
		}
		err := storageClient.DeleteArtifact(ctx, cmd.Prefix)
		if err != nil {
			return err
		}
	case "ls":
		slog.Debug(
			"get artifacts list",
		)
		err := storageClient.LoadArtifactsList(ctx, cmd.FilePath)
		if err != nil {
			return err
		}
	case "inf":
		slog.Debug(
			"get artifact info",
		)
		err := storageClient.GetFileInfo(ctx, cmd.Prefix)
		if err != nil {
			return err
		}
	default:
		return errors.New("unknown command")
	}

	return nil
}
