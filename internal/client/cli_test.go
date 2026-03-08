package client

import (
	"context"
	"testing"

	"github.com/gabkaclassic/gorbage/internal/config"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
)

func TestProcessCommand(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		cmd       config.Command
		setup     func(*MockStorageClient)
		expectErr bool
	}{
		{
			name: "pull ok",
			cmd: config.Command{
				Name:     "pull",
				Prefix:   "a",
				FilePath: "b",
			},
			setup: func(m *MockStorageClient) {
				m.On("DownloadArtifact", mock.Anything, "a", "b", "", int64(0), int64(0)).Return(nil).Once()
			},
		},
		{
			name: "push ok",
			cmd: config.Command{
				Name:     "push",
				Prefix:   "b",
				FilePath: "a",
			},
			setup: func(m *MockStorageClient) {
				m.On("UploadArtifact", mock.Anything, "b", "a", "", int64(0), int64(0)).Return(nil).Once()
			},
		},
		{
			name: "delete ok",
			cmd: config.Command{
				Name:   "del",
				Prefix: "a",
			},
			setup: func(m *MockStorageClient) {
				m.On("DeleteArtifact", mock.Anything, "a").Return(nil).Once()
			},
		},
		{
			name: "list ok",
			cmd: config.Command{
				Name: "ls",
			},
			setup: func(m *MockStorageClient) {
				m.On("LoadArtifactsList", mock.Anything, "").Return(nil).Once()
			},
		},
		{
			name: "info ok",
			cmd: config.Command{
				Name:   "inf",
				Prefix: "a",
			},
			setup: func(m *MockStorageClient) {
				m.On("GetFileInfo", mock.Anything, "a").Return(nil).Once()
			},
		},
		{
			name: "unknown command",
			cmd: config.Command{
				Name: "xxx",
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			mockClient := NewMockStorageClient(t)

			if tt.setup != nil {
				tt.setup(mockClient)
			}

			err := ProcessCommand(ctx, mockClient, tt.cmd)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockClient.AssertExpectations(t)
		})
	}
}
