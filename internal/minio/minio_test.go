package minio

import (
	"testing"
	"time"

	"github.com/gabkaclassic/gorbage/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestNewMinioConnection(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.Minio
		wantErr bool
	}{
		{
			name: "successful connection",
			cfg: config.Minio{
				Host:      "localhost:9000",
				AccessKey: "minioadmin",
				SecretKey: "minioadmin",
				Token:     "",
				SSL:       false,
				Timeout:   time.Second * 5,
			},
			wantErr: false,
		},
		{
			name: "empty host",
			cfg: config.Minio{
				Host:      "",
				AccessKey: "minioadmin",
				SecretKey: "minioadmin",
				Token:     "",
				SSL:       false,
				Timeout:   time.Second * 5,
			},
			wantErr: true,
		},
		{
			name: "with token",
			cfg: config.Minio{
				Host:      "localhost:9000",
				AccessKey: "minioadmin",
				SecretKey: "minioadmin",
				Token:     "test-token",
				SSL:       false,
				Timeout:   time.Second * 5,
			},
			wantErr: false,
		},
		{
			name: "zero timeout",
			cfg: config.Minio{
				Host:      "localhost:9000",
				AccessKey: "minioadmin",
				SecretKey: "minioadmin",
				Token:     "",
				SSL:       false,
				Timeout:   0,
			},
			wantErr: true,
		},
		{
			name: "host with port",
			cfg: config.Minio{
				Host:      "localhost:9000",
				AccessKey: "minioadmin",
				SecretKey: "minioadmin",
				Token:     "",
				SSL:       false,
				Timeout:   time.Second * 5,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewMinioConnection(tt.cfg)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
			}
		})
	}
}
