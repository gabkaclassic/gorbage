package client

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestNewStorageClient(t *testing.T) {
	tests := []struct {
		name      string
		conn      *grpc.ClientConn
		expectErr bool
	}{
		{
			name:      "nil connection",
			conn:      nil,
			expectErr: true,
		},
		{
			name: "valid connection",
			conn: func() *grpc.ClientConn {
				lis, err := net.Listen("tcp", "localhost:0")
				require.NoError(t, err)
				s := grpc.NewServer()
				go s.Serve(lis)
				conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
				require.NoError(t, err)
				t.Cleanup(func() {
					conn.Close()
					s.Stop()
				})
				return conn
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewStorageClient(tt.conn)
			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
			}
		})
	}
}
