package minio

import (
	"github.com/gabkaclassic/gorbage/internal/config"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func NewMinioConnection(cfg config.Minio) (*minio.Client, error) {
	client, err := minio.New(cfg.Host, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, cfg.Token),
		Secure: cfg.SSL,
	})

	if err != nil {
		return nil, err
	}
	_, err = client.HealthCheck(cfg.Timeout)

	if err != nil {
		return nil, err
	}

	return client, nil
}
