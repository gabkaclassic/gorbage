package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHumanSize(t *testing.T) {
	tests := []struct {
		name     string
		size     uint64
		expected string
	}{
		{
			name:     "bytes",
			size:     512,
			expected: "512B",
		},
		{
			name:     "kilobytes",
			size:     2048,
			expected: "2.0KiB",
		},
		{
			name:     "megabytes",
			size:     5 * 1024 * 1024,
			expected: "5.0MiB",
		},
		{
			name:     "gigabytes",
			size:     3 * 1024 * 1024 * 1024,
			expected: "3.0GiB",
		},
		{
			name:     "terabytes",
			size:     7 * 1024 * 1024 * 1024 * 1024,
			expected: "7.0TiB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := humanSize(tt.size)
			assert.Equal(t, tt.expected, got)
		})
	}
}
