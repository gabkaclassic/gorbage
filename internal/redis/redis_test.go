package redis

import (
	"context"
	"testing"
	"time"

	"github.com/gabkaclassic/gorbage/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestNewRedisConnection(t *testing.T) {
	tests := []struct {
		name    string
		ctx     context.Context
		cfg     config.Redis
		wantErr bool
	}{
		{
			name: "successful connection",
			ctx:  context.Background(),
			cfg: config.Redis{
				Host:     "localhost:6379",
				Password: "",
				Username: "",
				DB:       0,
			},
			wantErr: false,
		},
		{
			name: "with authentication",
			ctx:  context.Background(),
			cfg: config.Redis{
				Host:     "localhost:6379",
				Password: "password",
				Username: "user",
				DB:       0,
			},
			wantErr: true,
		},
		{
			name: "specific database",
			ctx:  context.Background(),
			cfg: config.Redis{
				Host:     "localhost:6379",
				Password: "",
				Username: "",
				DB:       5,
			},
			wantErr: false,
		},
		{
			name: "empty host",
			ctx:  context.Background(),
			cfg: config.Redis{
				Host:     "",
				Password: "",
				Username: "",
				DB:       0,
			},
			wantErr: true,
		},
		{
			name: "invalid host format",
			ctx:  context.Background(),
			cfg: config.Redis{
				Host:     "invalid-host",
				Password: "",
				Username: "",
				DB:       0,
			},
			wantErr: true,
		},
		{
			name: "host without port",
			ctx:  context.Background(),
			cfg: config.Redis{
				Host:     "localhost",
				Password: "",
				Username: "",
				DB:       0,
			},
			wantErr: true,
		},
		{
			name: "wrong port",
			ctx:  context.Background(),
			cfg: config.Redis{
				Host:     "localhost:9999",
				Password: "",
				Username: "",
				DB:       0,
			},
			wantErr: true,
		},
		{
			name: "canceled context",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			cfg: config.Redis{
				Host:     "localhost:6379",
				Password: "",
				Username: "",
				DB:       0,
			},
			wantErr: true,
		},
		{
			name: "with timeout context",
			ctx: func() context.Context {
				ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
				defer cancel()
				time.Sleep(time.Microsecond)
				return ctx
			}(),
			cfg: config.Redis{
				Host:     "localhost:6379",
				Password: "",
				Username: "",
				DB:       0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "successful connection" || tt.name == "specific database" {
				t.Skip("Skipping test that requires real Redis server")
			}

			client, err := NewRedisConnection(tt.ctx, tt.cfg)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
				if client != nil {
					defer client.Close()
				}
			}
		})
	}
}
