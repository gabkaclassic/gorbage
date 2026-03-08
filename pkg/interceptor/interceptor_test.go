package interceptor

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"net"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

const testCtxKey ctxKey = "test-key"

func TestPrepareTrustedCIDRs(t *testing.T) {
	tests := []struct {
		name    string
		cidrs   []string
		want    []*net.IPNet
		wantErr bool
	}{
		{
			name:    "empty slice",
			cidrs:   []string{},
			want:    nil,
			wantErr: false,
		},
		{
			name:    "nil slice",
			cidrs:   nil,
			want:    nil,
			wantErr: false,
		},
		{
			name:  "single IPv4 CIDR",
			cidrs: []string{"192.168.1.0/24"},
			want: func() []*net.IPNet {
				_, ipnet, _ := net.ParseCIDR("192.168.1.0/24")
				return []*net.IPNet{ipnet}
			}(),
			wantErr: false,
		},
		{
			name:  "multiple IPv4 CIDRs",
			cidrs: []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
			want: func() []*net.IPNet {
				_, ipnet1, _ := net.ParseCIDR("10.0.0.0/8")
				_, ipnet2, _ := net.ParseCIDR("172.16.0.0/12")
				_, ipnet3, _ := net.ParseCIDR("192.168.0.0/16")
				return []*net.IPNet{ipnet1, ipnet2, ipnet3}
			}(),
			wantErr: false,
		},
		{
			name:  "single IPv6 CIDR",
			cidrs: []string{"2001:db8::/32"},
			want: func() []*net.IPNet {
				_, ipnet, _ := net.ParseCIDR("2001:db8::/32")
				return []*net.IPNet{ipnet}
			}(),
			wantErr: false,
		},
		{
			name:  "multiple IPv6 CIDRs",
			cidrs: []string{"2001:db8::/32", "fe80::/10", "::1/128"},
			want: func() []*net.IPNet {
				_, ipnet1, _ := net.ParseCIDR("2001:db8::/32")
				_, ipnet2, _ := net.ParseCIDR("fe80::/10")
				_, ipnet3, _ := net.ParseCIDR("::1/128")
				return []*net.IPNet{ipnet1, ipnet2, ipnet3}
			}(),
			wantErr: false,
		},
		{
			name:  "mixed IPv4 and IPv6 CIDRs",
			cidrs: []string{"192.168.1.0/24", "2001:db8::/32", "10.0.0.0/8"},
			want: func() []*net.IPNet {
				_, ipnet1, _ := net.ParseCIDR("192.168.1.0/24")
				_, ipnet2, _ := net.ParseCIDR("2001:db8::/32")
				_, ipnet3, _ := net.ParseCIDR("10.0.0.0/8")
				return []*net.IPNet{ipnet1, ipnet2, ipnet3}
			}(),
			wantErr: false,
		},
		{
			name:  "CIDR with maximum prefix",
			cidrs: []string{"192.168.1.1/32", "2001:db8::1/128"},
			want: func() []*net.IPNet {
				_, ipnet1, _ := net.ParseCIDR("192.168.1.1/32")
				_, ipnet2, _ := net.ParseCIDR("2001:db8::1/128")
				return []*net.IPNet{ipnet1, ipnet2}
			}(),
			wantErr: false,
		},
		{
			name:  "CIDR with minimum prefix",
			cidrs: []string{"0.0.0.0/0", "::/0"},
			want: func() []*net.IPNet {
				_, ipnet1, _ := net.ParseCIDR("0.0.0.0/0")
				_, ipnet2, _ := net.ParseCIDR("::/0")
				return []*net.IPNet{ipnet1, ipnet2}
			}(),
			wantErr: false,
		},
		{
			name:    "invalid CIDR format",
			cidrs:   []string{"192.168.1.0"},
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid IPv4 CIDR",
			cidrs:   []string{"256.256.256.0/24"},
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid IPv6 CIDR",
			cidrs:   []string{"2001:db8:xyz::/32"},
			want:    nil,
			wantErr: true,
		},
		{
			name:    "CIDR with invalid prefix length",
			cidrs:   []string{"192.168.1.0/33"},
			want:    nil,
			wantErr: true,
		},
		{
			name:    "empty string in slice",
			cidrs:   []string{""},
			want:    nil,
			wantErr: true,
		},
		{
			name:    "whitespace in CIDR",
			cidrs:   []string{" 192.168.1.0/24"},
			want:    nil,
			wantErr: true,
		},
		{
			name:    "multiple CIDRs with one invalid",
			cidrs:   []string{"10.0.0.0/8", "invalid", "192.168.0.0/16"},
			want:    nil,
			wantErr: true,
		},
		{
			name:  "CIDR with host bits set",
			cidrs: []string{"192.168.1.100/24"}, // Host bits are allowed in ParseCIDR, they get masked
			want: func() []*net.IPNet {
				_, ipnet, _ := net.ParseCIDR("192.168.1.100/24")
				return []*net.IPNet{ipnet}
			}(),
			wantErr: false,
		},
		{
			name:  "private network ranges",
			cidrs: []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7"},
			want: func() []*net.IPNet {
				_, ipnet1, _ := net.ParseCIDR("10.0.0.0/8")
				_, ipnet2, _ := net.ParseCIDR("172.16.0.0/12")
				_, ipnet3, _ := net.ParseCIDR("192.168.0.0/16")
				_, ipnet4, _ := net.ParseCIDR("fc00::/7")
				return []*net.IPNet{ipnet1, ipnet2, ipnet3, ipnet4}
			}(),
			wantErr: false,
		},
		{
			name:  "localhost ranges",
			cidrs: []string{"127.0.0.0/8", "::1/128"},
			want: func() []*net.IPNet {
				_, ipnet1, _ := net.ParseCIDR("127.0.0.0/8")
				_, ipnet2, _ := net.ParseCIDR("::1/128")
				return []*net.IPNet{ipnet1, ipnet2}
			}(),
			wantErr: false,
		},
		{
			name:  "multicast ranges",
			cidrs: []string{"224.0.0.0/4", "ff00::/8"},
			want: func() []*net.IPNet {
				_, ipnet1, _ := net.ParseCIDR("224.0.0.0/4")
				_, ipnet2, _ := net.ParseCIDR("ff00::/8")
				return []*net.IPNet{ipnet1, ipnet2}
			}(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := prepareTrustedCIDRs(tt.cidrs)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, got)
			} else {
				assert.NoError(t, err)

				if tt.want == nil {
					assert.Nil(t, got)
				} else {
					assert.Equal(t, len(tt.want), len(got))
					for i := range tt.want {
						assert.Equal(t, tt.want[i].String(), got[i].String())
						assert.Equal(t, tt.want[i].IP, got[i].IP)
						assert.Equal(t, tt.want[i].Mask, got[i].Mask)
					}
				}
			}
		})
	}
}

func TestCheckTrustIP(t *testing.T) {
	tests := []struct {
		name         string
		ctx          context.Context
		trustedCIDRs []*net.IPNet
		wantErr      bool
		errCode      codes.Code
	}{
		{
			name: "trusted IPv4 in CIDR",
			ctx: func() context.Context {
				md := metadata.New(map[string]string{"x-real-ip": "192.168.1.100"})
				return metadata.NewIncomingContext(context.Background(), md)
			}(),
			trustedCIDRs: func() []*net.IPNet {
				_, cidr, _ := net.ParseCIDR("192.168.1.0/24")
				return []*net.IPNet{cidr}
			}(),
			wantErr: false,
		},
		{
			name: "trusted IPv4 with spaces",
			ctx: func() context.Context {
				md := metadata.New(map[string]string{"x-real-ip": " 192.168.1.100 "})
				return metadata.NewIncomingContext(context.Background(), md)
			}(),
			trustedCIDRs: func() []*net.IPNet {
				_, cidr, _ := net.ParseCIDR("192.168.1.0/24")
				return []*net.IPNet{cidr}
			}(),
			wantErr: false,
		},
		{
			name: "trusted IPv6 in CIDR",
			ctx: func() context.Context {
				md := metadata.New(map[string]string{"x-real-ip": "2001:db8::1"})
				return metadata.NewIncomingContext(context.Background(), md)
			}(),
			trustedCIDRs: func() []*net.IPNet {
				_, cidr, _ := net.ParseCIDR("2001:db8::/32")
				return []*net.IPNet{cidr}
			}(),
			wantErr: false,
		},
		{
			name: "multiple trusted CIDRs - first matches",
			ctx: func() context.Context {
				md := metadata.New(map[string]string{"x-real-ip": "10.0.0.5"})
				return metadata.NewIncomingContext(context.Background(), md)
			}(),
			trustedCIDRs: func() []*net.IPNet {
				_, cidr1, _ := net.ParseCIDR("10.0.0.0/8")
				_, cidr2, _ := net.ParseCIDR("192.168.1.0/24")
				_, cidr3, _ := net.ParseCIDR("172.16.0.0/12")
				return []*net.IPNet{cidr1, cidr2, cidr3}
			}(),
			wantErr: false,
		},
		{
			name: "multiple trusted CIDRs - last matches",
			ctx: func() context.Context {
				md := metadata.New(map[string]string{"x-real-ip": "172.16.5.5"})
				return metadata.NewIncomingContext(context.Background(), md)
			}(),
			trustedCIDRs: func() []*net.IPNet {
				_, cidr1, _ := net.ParseCIDR("10.0.0.0/8")
				_, cidr2, _ := net.ParseCIDR("192.168.1.0/24")
				_, cidr3, _ := net.ParseCIDR("172.16.0.0/12")
				return []*net.IPNet{cidr1, cidr2, cidr3}
			}(),
			wantErr: false,
		},
		{
			name: "IP exactly at CIDR boundary",
			ctx: func() context.Context {
				md := metadata.New(map[string]string{"x-real-ip": "192.168.1.0"})
				return metadata.NewIncomingContext(context.Background(), md)
			}(),
			trustedCIDRs: func() []*net.IPNet {
				_, cidr, _ := net.ParseCIDR("192.168.1.0/24")
				return []*net.IPNet{cidr}
			}(),
			wantErr: false,
		},
		{
			name: "IP exactly at CIDR broadcast",
			ctx: func() context.Context {
				md := metadata.New(map[string]string{"x-real-ip": "192.168.1.255"})
				return metadata.NewIncomingContext(context.Background(), md)
			}(),
			trustedCIDRs: func() []*net.IPNet {
				_, cidr, _ := net.ParseCIDR("192.168.1.0/24")
				return []*net.IPNet{cidr}
			}(),
			wantErr: false,
		},
		{
			name: "untrusted IPv4",
			ctx: func() context.Context {
				md := metadata.New(map[string]string{"x-real-ip": "8.8.8.8"})
				ctx := metadata.NewIncomingContext(context.Background(), md)
				return context.WithValue(ctx, requestIDKey, "test-request-123")
			}(),
			trustedCIDRs: func() []*net.IPNet {
				_, cidr, _ := net.ParseCIDR("192.168.1.0/24")
				return []*net.IPNet{cidr}
			}(),
			wantErr: true,
			errCode: codes.PermissionDenied,
		},
		{
			name: "untrusted IPv6",
			ctx: func() context.Context {
				md := metadata.New(map[string]string{"x-real-ip": "2001:4860:4860::8888"})
				ctx := metadata.NewIncomingContext(context.Background(), md)
				return context.WithValue(ctx, requestIDKey, "test-request-123")
			}(),
			trustedCIDRs: func() []*net.IPNet {
				_, cidr, _ := net.ParseCIDR("2001:db8::/32")
				return []*net.IPNet{cidr}
			}(),
			wantErr: true,
			errCode: codes.PermissionDenied,
		},
		{
			name: "empty trusted CIDRs slice",
			ctx: func() context.Context {
				md := metadata.New(map[string]string{"x-real-ip": "192.168.1.100"})
				return metadata.NewIncomingContext(context.Background(), md)
			}(),
			trustedCIDRs: []*net.IPNet{},
			wantErr:      true,
			errCode:      codes.PermissionDenied,
		},
		{
			name: "nil trusted CIDRs",
			ctx: func() context.Context {
				md := metadata.New(map[string]string{"x-real-ip": "192.168.1.100"})
				return metadata.NewIncomingContext(context.Background(), md)
			}(),
			trustedCIDRs: nil,
			wantErr:      true,
			errCode:      codes.PermissionDenied,
		},
		{
			name: "missing x-real-ip header",
			ctx: func() context.Context {
				md := metadata.New(map[string]string{})
				ctx := metadata.NewIncomingContext(context.Background(), md)
				return context.WithValue(ctx, requestIDKey, "test-request-123")
			}(),
			trustedCIDRs: func() []*net.IPNet {
				_, cidr, _ := net.ParseCIDR("192.168.1.0/24")
				return []*net.IPNet{cidr}
			}(),
			wantErr: true,
			errCode: codes.PermissionDenied,
		},
		{
			name: "empty x-real-ip header",
			ctx: func() context.Context {
				md := metadata.New(map[string]string{"x-real-ip": ""})
				ctx := metadata.NewIncomingContext(context.Background(), md)
				return context.WithValue(ctx, requestIDKey, "test-request-123")
			}(),
			trustedCIDRs: func() []*net.IPNet {
				_, cidr, _ := net.ParseCIDR("192.168.1.0/24")
				return []*net.IPNet{cidr}
			}(),
			wantErr: true,
			errCode: codes.PermissionDenied,
		},
		{
			name: "invalid IP in header",
			ctx: func() context.Context {
				md := metadata.New(map[string]string{"x-real-ip": "not-an-ip"})
				ctx := metadata.NewIncomingContext(context.Background(), md)
				return context.WithValue(ctx, requestIDKey, "test-request-123")
			}(),
			trustedCIDRs: func() []*net.IPNet {
				_, cidr, _ := net.ParseCIDR("192.168.1.0/24")
				return []*net.IPNet{cidr}
			}(),
			wantErr: true,
			errCode: codes.PermissionDenied,
		},
		{
			name: "no metadata in context",
			ctx: func() context.Context {
				return context.WithValue(context.Background(), requestIDKey, "test-request-123")
			}(),
			trustedCIDRs: func() []*net.IPNet {
				_, cidr, _ := net.ParseCIDR("192.168.1.0/24")
				return []*net.IPNet{cidr}
			}(),
			wantErr: true,
			errCode: codes.PermissionDenied,
		},
		{
			name: "IP in multiple CIDRs",
			ctx: func() context.Context {
				md := metadata.New(map[string]string{"x-real-ip": "10.0.0.1"})
				return metadata.NewIncomingContext(context.Background(), md)
			}(),
			trustedCIDRs: func() []*net.IPNet {
				_, cidr1, _ := net.ParseCIDR("0.0.0.0/0")
				_, cidr2, _ := net.ParseCIDR("10.0.0.0/8")
				return []*net.IPNet{cidr1, cidr2}
			}(),
			wantErr: false,
		},
		{
			name: "IPv6 loopback in IPv6 CIDR",
			ctx: func() context.Context {
				md := metadata.New(map[string]string{"x-real-ip": "::1"})
				return metadata.NewIncomingContext(context.Background(), md)
			}(),
			trustedCIDRs: func() []*net.IPNet {
				_, cidr, _ := net.ParseCIDR("::1/128")
				return []*net.IPNet{cidr}
			}(),
			wantErr: false,
		},
		{
			name: "with request ID in context",
			ctx: func() context.Context {
				md := metadata.New(map[string]string{"x-real-ip": "10.0.0.1"})
				ctx := metadata.NewIncomingContext(context.Background(), md)
				return context.WithValue(ctx, requestIDKey, "test-request-456")
			}(),
			trustedCIDRs: func() []*net.IPNet {
				_, cidr, _ := net.ParseCIDR("10.0.0.0/8")
				return []*net.IPNet{cidr}
			}(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkTrustIP(tt.ctx, tt.trustedCIDRs)

			if tt.wantErr {
				assert.Error(t, err)
				st, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tt.errCode, st.Code())
				assert.Equal(t, "forbidden", st.Message())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestTrustAddressUnaryInterceptor(t *testing.T) {
	tests := []struct {
		name     string
		cidrs    []string
		setup    func() ([]string, func())
		wantErr  bool
		validate func(*testing.T, grpc.UnaryServerInterceptor, error)
	}{
		{
			name:    "valid IPv4 CIDRs",
			cidrs:   []string{"192.168.1.0/24", "10.0.0.0/8"},
			wantErr: false,
			validate: func(t *testing.T, interceptor grpc.UnaryServerInterceptor, err error) {
				assert.NotNil(t, interceptor)

				ctx := metadata.NewIncomingContext(
					context.Background(),
					metadata.New(map[string]string{"x-real-ip": "192.168.1.100"}),
				)
				handler := func(ctx context.Context, req any) (any, error) {
					return "response", nil
				}
				resp, err := interceptor(ctx, "request", &grpc.UnaryServerInfo{}, handler)
				assert.NoError(t, err)
				assert.Equal(t, "response", resp)

				ctx = metadata.NewIncomingContext(
					context.Background(),
					metadata.New(map[string]string{"x-real-ip": "8.8.8.8"}),
				)
				resp, err = interceptor(ctx, "request", &grpc.UnaryServerInfo{}, handler)
				assert.Error(t, err)
				assert.Nil(t, resp)
				st, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, codes.PermissionDenied, st.Code())
			},
		},
		{
			name:    "valid IPv6 CIDRs",
			cidrs:   []string{"2001:db8::/32", "fe80::/10"},
			wantErr: false,
			validate: func(t *testing.T, interceptor grpc.UnaryServerInterceptor, err error) {
				assert.NotNil(t, interceptor)

				ctx := metadata.NewIncomingContext(
					context.Background(),
					metadata.New(map[string]string{"x-real-ip": "2001:db8::1"}),
				)
				handler := func(ctx context.Context, req any) (any, error) {
					return "response", nil
				}
				resp, err := interceptor(ctx, "request", &grpc.UnaryServerInfo{}, handler)
				assert.NoError(t, err)
				assert.Equal(t, "response", resp)

				ctx = metadata.NewIncomingContext(
					context.Background(),
					metadata.New(map[string]string{"x-real-ip": "2001:4860::1"}),
				)
				resp, err = interceptor(ctx, "request", &grpc.UnaryServerInfo{}, handler)
				assert.Error(t, err)
				assert.Nil(t, resp)
			},
		},
		{
			name:    "mixed IPv4 and IPv6 CIDRs",
			cidrs:   []string{"192.168.1.0/24", "2001:db8::/32"},
			wantErr: false,
			validate: func(t *testing.T, interceptor grpc.UnaryServerInterceptor, err error) {
				assert.NotNil(t, interceptor)

				handler := func(ctx context.Context, req any) (any, error) {
					return "response", nil
				}

				// Test trusted IPv4
				ctx := metadata.NewIncomingContext(
					context.Background(),
					metadata.New(map[string]string{"x-real-ip": "192.168.1.50"}),
				)
				resp, err := interceptor(ctx, "request", &grpc.UnaryServerInfo{}, handler)
				assert.NoError(t, err)
				assert.Equal(t, "response", resp)

				// Test trusted IPv6
				ctx = metadata.NewIncomingContext(
					context.Background(),
					metadata.New(map[string]string{"x-real-ip": "2001:db8::abc"}),
				)
				resp, err = interceptor(ctx, "request", &grpc.UnaryServerInfo{}, handler)
				assert.NoError(t, err)
				assert.Equal(t, "response", resp)
			},
		},
		{
			name:    "empty CIDRs slice",
			cidrs:   []string{},
			wantErr: false,
			validate: func(t *testing.T, interceptor grpc.UnaryServerInterceptor, err error) {
				assert.NotNil(t, interceptor)

				ctx := metadata.NewIncomingContext(
					context.Background(),
					metadata.New(map[string]string{"x-real-ip": "untrusted-ip"}),
				)
				handler := func(ctx context.Context, req any) (any, error) {
					return "response", nil
				}
				resp, err := interceptor(ctx, "request", &grpc.UnaryServerInfo{}, handler)
				assert.NoError(t, err)
				assert.Equal(t, "response", resp)
			},
		},
		{
			name:    "nil CIDRs",
			cidrs:   nil,
			wantErr: false,
			validate: func(t *testing.T, interceptor grpc.UnaryServerInterceptor, err error) {
				assert.NotNil(t, interceptor)

				ctx := metadata.NewIncomingContext(
					context.Background(),
					metadata.New(map[string]string{"x-real-ip": "any-ip"}),
				)
				handler := func(ctx context.Context, req any) (any, error) {
					return "response", nil
				}
				resp, err := interceptor(ctx, "request", &grpc.UnaryServerInfo{}, handler)
				assert.NoError(t, err)
				assert.Equal(t, "response", resp)
			},
		},
		{
			name:    "invalid CIDR format",
			cidrs:   []string{"invalid-cidr"},
			wantErr: true,
			validate: func(t *testing.T, interceptor grpc.UnaryServerInterceptor, err error) {
				assert.Nil(t, interceptor)
				assert.Error(t, err)
			},
		},
		{
			name:    "one valid and one invalid CIDR",
			cidrs:   []string{"192.168.1.0/24", "invalid"},
			wantErr: true,
			validate: func(t *testing.T, interceptor grpc.UnaryServerInterceptor, err error) {
				assert.Nil(t, interceptor)
				assert.Error(t, err)
			},
		},
		{
			name:    "CIDR with host bits set",
			cidrs:   []string{"192.168.1.100/24"}, // Host bits are allowed
			wantErr: false,
			validate: func(t *testing.T, interceptor grpc.UnaryServerInterceptor, err error) {
				assert.NotNil(t, interceptor)

				ctx := metadata.NewIncomingContext(
					context.Background(),
					metadata.New(map[string]string{"x-real-ip": "192.168.1.200"}),
				)
				handler := func(ctx context.Context, req any) (any, error) {
					return "response", nil
				}
				resp, err := interceptor(ctx, "request", &grpc.UnaryServerInfo{}, handler)
				assert.NoError(t, err)
				assert.Equal(t, "response", resp)
			},
		},
		{
			name:    "private network ranges",
			cidrs:   []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
			wantErr: false,
			validate: func(t *testing.T, interceptor grpc.UnaryServerInterceptor, err error) {
				assert.NotNil(t, interceptor)

				handler := func(ctx context.Context, req any) (any, error) {
					return "response", nil
				}

				ips := []string{"10.10.10.10", "172.20.20.20", "192.168.50.50"}
				for _, ip := range ips {
					ctx := metadata.NewIncomingContext(
						context.Background(),
						metadata.New(map[string]string{"x-real-ip": ip}),
					)
					resp, err := interceptor(ctx, "request", &grpc.UnaryServerInfo{}, handler)
					assert.NoError(t, err)
					assert.Equal(t, "response", resp)
				}

				ctx := metadata.NewIncomingContext(
					context.Background(),
					metadata.New(map[string]string{"x-real-ip": "8.8.8.8"}),
				)
				resp, err := interceptor(ctx, "request", &grpc.UnaryServerInfo{}, handler)
				assert.Error(t, err)
				assert.Nil(t, resp)
			},
		},
		{
			name:    "localhost ranges",
			cidrs:   []string{"127.0.0.0/8", "::1/128"},
			wantErr: false,
			validate: func(t *testing.T, interceptor grpc.UnaryServerInterceptor, err error) {
				assert.NotNil(t, interceptor)

				handler := func(ctx context.Context, req any) (any, error) {
					return "response", nil
				}

				ctx := metadata.NewIncomingContext(
					context.Background(),
					metadata.New(map[string]string{"x-real-ip": "127.0.0.1"}),
				)
				resp, err := interceptor(ctx, "request", &grpc.UnaryServerInfo{}, handler)
				assert.NoError(t, err)
				assert.Equal(t, "response", resp)

				ctx = metadata.NewIncomingContext(
					context.Background(),
					metadata.New(map[string]string{"x-real-ip": "::1"}),
				)
				resp, err = interceptor(ctx, "request", &grpc.UnaryServerInfo{}, handler)
				assert.NoError(t, err)
				assert.Equal(t, "response", resp)
			},
		},
		{
			name:    "default route (all IPs)",
			cidrs:   []string{"0.0.0.0/0", "::/0"},
			wantErr: false,
			validate: func(t *testing.T, interceptor grpc.UnaryServerInterceptor, err error) {
				assert.NotNil(t, interceptor)

				handler := func(ctx context.Context, req any) (any, error) {
					return "response", nil
				}

				ips := []string{"8.8.8.8", "1.1.1.1", "192.168.1.1", "10.0.0.1", "2001:db8::1", "::1"}
				for _, ip := range ips {
					ctx := metadata.NewIncomingContext(
						context.Background(),
						metadata.New(map[string]string{"x-real-ip": ip}),
					)
					resp, err := interceptor(ctx, "request", &grpc.UnaryServerInfo{}, handler)
					assert.NoError(t, err)
					assert.Equal(t, "response", resp)
				}
			},
		},
		{
			name:    "interceptor passes request context and info",
			cidrs:   []string{"192.168.1.0/24"},
			wantErr: false,
			validate: func(t *testing.T, interceptor grpc.UnaryServerInterceptor, err error) {
				assert.NotNil(t, interceptor)

				expectedInfo := &grpc.UnaryServerInfo{
					FullMethod: "/test.Service/Method",
					Server:     "test-server",
				}

				ctx := metadata.NewIncomingContext(
					context.Background(),
					metadata.New(map[string]string{"x-real-ip": "192.168.1.100"}),
				)
				ctx = context.WithValue(ctx, testCtxKey, "test-value")

				handler := func(ctx context.Context, req any) (any, error) {
					assert.Equal(t, "test-value", ctx.Value(testCtxKey))
					return "response", nil
				}

				resp, err := interceptor(ctx, "request", expectedInfo, handler)
				assert.NoError(t, err)
				assert.Equal(t, "response", resp)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cidrs []string
			var cleanup func()

			if tt.setup != nil {
				cidrs, cleanup = tt.setup()
				defer cleanup()
			} else {
				cidrs = tt.cidrs
			}

			interceptor, err := TrustAddressUnaryInterceptor(cidrs)

			if tt.validate != nil {
				tt.validate(t, interceptor, err)
			}
		})
	}
}

func TestTrustAddressStreamInterceptor(t *testing.T) {
	tests := []struct {
		name     string
		cidrs    []string
		setup    func() ([]string, func())
		wantErr  bool
		validate func(*testing.T, grpc.StreamServerInterceptor, error)
	}{
		{
			name:    "valid IPv4 CIDRs",
			cidrs:   []string{"192.168.1.0/24", "10.0.0.0/8"},
			wantErr: false,
			validate: func(t *testing.T, interceptor grpc.StreamServerInterceptor, err error) {
				assert.NotNil(t, interceptor)

				ctx := metadata.NewIncomingContext(
					context.Background(),
					metadata.New(map[string]string{"x-real-ip": "192.168.1.100"}),
				)
				ss := &mockServerStream{ctx: ctx}
				handler := func(srv any, stream grpc.ServerStream) error {
					return nil
				}
				err = interceptor(nil, ss, &grpc.StreamServerInfo{}, handler)
				assert.NoError(t, err)

				ctx = metadata.NewIncomingContext(
					context.Background(),
					metadata.New(map[string]string{"x-real-ip": "8.8.8.8"}),
				)
				ss = &mockServerStream{ctx: ctx}
				err = interceptor(nil, ss, &grpc.StreamServerInfo{}, handler)
				assert.Error(t, err)
				st, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, codes.PermissionDenied, st.Code())
			},
		},
		{
			name:    "valid IPv6 CIDRs",
			cidrs:   []string{"2001:db8::/32", "fe80::/10"},
			wantErr: false,
			validate: func(t *testing.T, interceptor grpc.StreamServerInterceptor, err error) {
				assert.NotNil(t, interceptor)

				ctx := metadata.NewIncomingContext(
					context.Background(),
					metadata.New(map[string]string{"x-real-ip": "2001:db8::1"}),
				)
				ss := &mockServerStream{ctx: ctx}
				handler := func(srv any, stream grpc.ServerStream) error {
					return nil
				}
				err = interceptor(nil, ss, &grpc.StreamServerInfo{}, handler)
				assert.NoError(t, err)

				ctx = metadata.NewIncomingContext(
					context.Background(),
					metadata.New(map[string]string{"x-real-ip": "2001:4860::1"}),
				)
				ss = &mockServerStream{ctx: ctx}
				err = interceptor(nil, ss, &grpc.StreamServerInfo{}, handler)
				assert.Error(t, err)
			},
		},
		{
			name:    "mixed IPv4 and IPv6 CIDRs",
			cidrs:   []string{"192.168.1.0/24", "2001:db8::/32"},
			wantErr: false,
			validate: func(t *testing.T, interceptor grpc.StreamServerInterceptor, err error) {
				assert.NotNil(t, interceptor)

				handler := func(srv any, stream grpc.ServerStream) error {
					return nil
				}

				ctx := metadata.NewIncomingContext(
					context.Background(),
					metadata.New(map[string]string{"x-real-ip": "192.168.1.50"}),
				)
				ss := &mockServerStream{ctx: ctx}
				err = interceptor(nil, ss, &grpc.StreamServerInfo{}, handler)
				assert.NoError(t, err)

				ctx = metadata.NewIncomingContext(
					context.Background(),
					metadata.New(map[string]string{"x-real-ip": "2001:db8::abc"}),
				)
				ss = &mockServerStream{ctx: ctx}
				err = interceptor(nil, ss, &grpc.StreamServerInfo{}, handler)
				assert.NoError(t, err)
			},
		},
		{
			name:    "empty CIDRs slice",
			cidrs:   []string{},
			wantErr: false,
			validate: func(t *testing.T, interceptor grpc.StreamServerInterceptor, err error) {
				assert.NotNil(t, interceptor)

				ctx := metadata.NewIncomingContext(
					context.Background(),
					metadata.New(map[string]string{"x-real-ip": "untrusted-ip"}),
				)
				ss := &mockServerStream{ctx: ctx}
				handler := func(srv any, stream grpc.ServerStream) error {
					return nil
				}
				err = interceptor(nil, ss, &grpc.StreamServerInfo{}, handler)
				assert.NoError(t, err)
			},
		},
		{
			name:    "nil CIDRs",
			cidrs:   nil,
			wantErr: false,
			validate: func(t *testing.T, interceptor grpc.StreamServerInterceptor, err error) {
				assert.NotNil(t, interceptor)

				ctx := metadata.NewIncomingContext(
					context.Background(),
					metadata.New(map[string]string{"x-real-ip": "any-ip"}),
				)
				ss := &mockServerStream{ctx: ctx}
				handler := func(srv any, stream grpc.ServerStream) error {
					return nil
				}
				err = interceptor(nil, ss, &grpc.StreamServerInfo{}, handler)
				assert.NoError(t, err)
			},
		},
		{
			name:    "invalid CIDR format",
			cidrs:   []string{"invalid-cidr"},
			wantErr: true,
			validate: func(t *testing.T, interceptor grpc.StreamServerInterceptor, err error) {
				assert.Nil(t, interceptor)
				assert.Error(t, err)
			},
		},
		{
			name:    "one valid and one invalid CIDR",
			cidrs:   []string{"192.168.1.0/24", "invalid"},
			wantErr: true,
			validate: func(t *testing.T, interceptor grpc.StreamServerInterceptor, err error) {
				assert.Nil(t, interceptor)
				assert.Error(t, err)
			},
		},
		{
			name:    "CIDR with host bits set",
			cidrs:   []string{"192.168.1.100/24"},
			wantErr: false,
			validate: func(t *testing.T, interceptor grpc.StreamServerInterceptor, err error) {
				assert.NotNil(t, interceptor)

				ctx := metadata.NewIncomingContext(
					context.Background(),
					metadata.New(map[string]string{"x-real-ip": "192.168.1.200"}),
				)
				ss := &mockServerStream{ctx: ctx}
				handler := func(srv any, stream grpc.ServerStream) error {
					return nil
				}
				err = interceptor(nil, ss, &grpc.StreamServerInfo{}, handler)
				assert.NoError(t, err)
			},
		},
		{
			name:    "private network ranges",
			cidrs:   []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
			wantErr: false,
			validate: func(t *testing.T, interceptor grpc.StreamServerInterceptor, err error) {
				assert.NotNil(t, interceptor)

				handler := func(srv any, stream grpc.ServerStream) error {
					return nil
				}

				ips := []string{"10.10.10.10", "172.20.20.20", "192.168.50.50"}
				for _, ip := range ips {
					ctx := metadata.NewIncomingContext(
						context.Background(),
						metadata.New(map[string]string{"x-real-ip": ip}),
					)
					ss := &mockServerStream{ctx: ctx}
					err = interceptor(nil, ss, &grpc.StreamServerInfo{}, handler)
					assert.NoError(t, err)
				}

				ctx := metadata.NewIncomingContext(
					context.Background(),
					metadata.New(map[string]string{"x-real-ip": "8.8.8.8"}),
				)
				ss := &mockServerStream{ctx: ctx}
				err = interceptor(nil, ss, &grpc.StreamServerInfo{}, handler)
				assert.Error(t, err)
			},
		},
		{
			name:    "localhost ranges",
			cidrs:   []string{"127.0.0.0/8", "::1/128"},
			wantErr: false,
			validate: func(t *testing.T, interceptor grpc.StreamServerInterceptor, err error) {
				assert.NotNil(t, interceptor)

				handler := func(srv any, stream grpc.ServerStream) error {
					return nil
				}

				ctx := metadata.NewIncomingContext(
					context.Background(),
					metadata.New(map[string]string{"x-real-ip": "127.0.0.1"}),
				)
				ss := &mockServerStream{ctx: ctx}
				err = interceptor(nil, ss, &grpc.StreamServerInfo{}, handler)
				assert.NoError(t, err)

				ctx = metadata.NewIncomingContext(
					context.Background(),
					metadata.New(map[string]string{"x-real-ip": "::1"}),
				)
				ss = &mockServerStream{ctx: ctx}
				err = interceptor(nil, ss, &grpc.StreamServerInfo{}, handler)
				assert.NoError(t, err)
			},
		},
		{
			name:    "default route",
			cidrs:   []string{"0.0.0.0/0", "::/0"},
			wantErr: false,
			validate: func(t *testing.T, interceptor grpc.StreamServerInterceptor, err error) {
				assert.NotNil(t, interceptor)

				handler := func(srv any, stream grpc.ServerStream) error {
					return nil
				}

				ips := []string{"8.8.8.8", "1.1.1.1", "192.168.1.1", "10.0.0.1", "2001:db8::1", "::1"}
				for _, ip := range ips {
					ctx := metadata.NewIncomingContext(
						context.Background(),
						metadata.New(map[string]string{"x-real-ip": ip}),
					)
					ss := &mockServerStream{ctx: ctx}
					err = interceptor(nil, ss, &grpc.StreamServerInfo{}, handler)
					assert.NoError(t, err)
				}
			},
		},
		{
			name:    "missing x-real-ip header",
			cidrs:   []string{"192.168.1.0/24"},
			wantErr: false,
			validate: func(t *testing.T, interceptor grpc.StreamServerInterceptor, err error) {
				assert.NotNil(t, interceptor)

				ctx := metadata.NewIncomingContext(
					context.Background(),
					metadata.New(map[string]string{}),
				)
				ss := &mockServerStream{ctx: ctx}
				handler := func(srv any, stream grpc.ServerStream) error {
					return nil
				}
				err = interceptor(nil, ss, &grpc.StreamServerInfo{}, handler)
				assert.Error(t, err)
			},
		},
		{
			name:    "stream handler returns error",
			cidrs:   []string{"192.168.1.0/24"},
			wantErr: false,
			validate: func(t *testing.T, interceptor grpc.StreamServerInterceptor, err error) {
				assert.NotNil(t, interceptor)

				ctx := metadata.NewIncomingContext(
					context.Background(),
					metadata.New(map[string]string{"x-real-ip": "192.168.1.100"}),
				)
				ss := &mockServerStream{ctx: ctx}
				expectedErr := errors.New("handler error")
				handler := func(srv any, stream grpc.ServerStream) error {
					return expectedErr
				}
				err = interceptor(nil, ss, &grpc.StreamServerInfo{}, handler)
				assert.Equal(t, expectedErr, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cidrs []string
			var cleanup func()

			if tt.setup != nil {
				cidrs, cleanup = tt.setup()
				defer cleanup()
			} else {
				cidrs = tt.cidrs
			}

			interceptor, err := TrustAddressStreamInterceptor(cidrs)

			if tt.validate != nil {
				tt.validate(t, interceptor, err)
			}
		})
	}
}

func TestLoggingUnaryInterceptor(t *testing.T) {
	tests := []struct {
		name        string
		ctx         context.Context
		expectedRID string
		validate    func(*testing.T, context.Context, string)
	}{
		{
			name: "request ID provided in metadata",
			ctx: metadata.NewIncomingContext(
				context.Background(),
				metadata.New(map[string]string{"x-request-id": "test-request-123"}),
			),
			expectedRID: "test-request-123",
			validate: func(t *testing.T, ctx context.Context, rid string) {
				assert.Equal(t, "test-request-123", rid)
				val := ctx.Value(requestIDKey)
				assert.NotNil(t, val)
				assert.Equal(t, "test-request-123", val.(string))
			},
		},
		{
			name: "request ID with spaces",
			ctx: metadata.NewIncomingContext(
				context.Background(),
				metadata.New(map[string]string{"x-request-id": "  test-request-123  "}),
			),
			expectedRID: "test-request-123",
			validate: func(t *testing.T, ctx context.Context, rid string) {
				assert.Equal(t, "test-request-123", rid)
				val := ctx.Value(requestIDKey)
				assert.NotNil(t, val)
				assert.Equal(t, "test-request-123", val.(string))
			},
		},
		{
			name: "no request ID in metadata",
			ctx: metadata.NewIncomingContext(
				context.Background(),
				metadata.New(map[string]string{}),
			),
			validate: func(t *testing.T, ctx context.Context, rid string) {
				assert.NotEmpty(t, rid)
				val := ctx.Value(requestIDKey)
				assert.NotNil(t, val)
				assert.Equal(t, rid, val.(string))

				_, err := uuid.Parse(rid)
				assert.NoError(t, err)
			},
		},
		{
			name: "no metadata in context",
			ctx:  context.Background(),
			validate: func(t *testing.T, ctx context.Context, rid string) {
				assert.NotEmpty(t, rid)
				val := ctx.Value(requestIDKey)
				assert.NotNil(t, val)
				assert.Equal(t, rid, val.(string))

				_, err := uuid.Parse(rid)
				assert.NoError(t, err)
			},
		},
		{
			name: "empty request ID in metadata",
			ctx: metadata.NewIncomingContext(
				context.Background(),
				metadata.New(map[string]string{"x-request-id": ""}),
			),
			validate: func(t *testing.T, ctx context.Context, rid string) {
				assert.NotEmpty(t, rid)
				val := ctx.Value(requestIDKey)
				assert.NotNil(t, val)
				assert.Equal(t, rid, val.(string))

				_, err := uuid.Parse(rid)
				assert.NoError(t, err)
			},
		},
		{
			name: "request ID already in context",
			ctx: func() context.Context {
				ctx := context.WithValue(context.Background(), requestIDKey, "existing-rid")
				return metadata.NewIncomingContext(
					ctx,
					metadata.New(map[string]string{"x-request-id": "new-rid"}),
				)
			}(),
			expectedRID: "new-rid",
			validate: func(t *testing.T, ctx context.Context, rid string) {
				assert.Equal(t, "new-rid", rid)
				val := ctx.Value(requestIDKey)
				assert.NotNil(t, val)
				assert.Equal(t, "new-rid", val.(string))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interceptor := LoggingUnaryInterceptor()

			var handlerCtx context.Context
			handler := func(ctx context.Context, req any) (any, error) {
				handlerCtx = ctx
				return "response", nil
			}

			resp, err := interceptor(tt.ctx, "request", &grpc.UnaryServerInfo{
				FullMethod: "/test.Service/Method",
			}, handler)

			assert.NoError(t, err)
			assert.Equal(t, "response", resp)

			if tt.validate != nil {
				val := handlerCtx.Value(requestIDKey)
				assert.NotNil(t, val)
				tt.validate(t, handlerCtx, val.(string))
			}
		})
	}
}

func TestLoggingStreamInterceptor(t *testing.T) {
	tests := []struct {
		name     string
		ctx      context.Context
		validate func(*testing.T, context.Context, string)
	}{
		{
			name: "request ID provided in metadata",
			ctx: metadata.NewIncomingContext(
				context.Background(),
				metadata.New(map[string]string{"x-request-id": "test-request-123"}),
			),
			validate: func(t *testing.T, ctx context.Context, rid string) {
				assert.Equal(t, "test-request-123", rid)
				val := ctx.Value(requestIDKey)
				assert.NotNil(t, val)
				assert.Equal(t, "test-request-123", val.(string))
			},
		},
		{
			name: "request ID with spaces",
			ctx: metadata.NewIncomingContext(
				context.Background(),
				metadata.New(map[string]string{"x-request-id": "  test-request-123  "}),
			),
			validate: func(t *testing.T, ctx context.Context, rid string) {
				assert.Equal(t, "test-request-123", rid)
				val := ctx.Value(requestIDKey)
				assert.NotNil(t, val)
				assert.Equal(t, "test-request-123", val.(string))
			},
		},
		{
			name: "empty request ID in metadata",
			ctx: metadata.NewIncomingContext(
				context.Background(),
				metadata.New(map[string]string{"x-request-id": ""}),
			),
			validate: func(t *testing.T, ctx context.Context, rid string) {
				assert.NotEmpty(t, rid)
				val := ctx.Value(requestIDKey)
				assert.NotNil(t, val)
				assert.Equal(t, rid, val.(string))

				_, err := uuid.Parse(rid)
				assert.NoError(t, err)
			},
		},
		{
			name: "no request ID in metadata",
			ctx: metadata.NewIncomingContext(
				context.Background(),
				metadata.New(map[string]string{}),
			),
			validate: func(t *testing.T, ctx context.Context, rid string) {
				assert.NotEmpty(t, rid)
				val := ctx.Value(requestIDKey)
				assert.NotNil(t, val)
				assert.Equal(t, rid, val.(string))

				_, err := uuid.Parse(rid)
				assert.NoError(t, err)
			},
		},
		{
			name: "multiple request IDs in metadata",
			ctx: metadata.NewIncomingContext(
				context.Background(),
				metadata.New(map[string]string{"x-request-id": "first"}),
			),
			validate: func(t *testing.T, ctx context.Context, rid string) {
				assert.Equal(t, "first", rid)
				val := ctx.Value(requestIDKey)
				assert.NotNil(t, val)
				assert.Equal(t, "first", val.(string))
			},
		},
		{
			name: "no metadata in context",
			ctx:  context.Background(),
			validate: func(t *testing.T, ctx context.Context, rid string) {
				assert.NotEmpty(t, rid)
				val := ctx.Value(requestIDKey)
				assert.NotNil(t, val)
				assert.Equal(t, rid, val.(string))

				_, err := uuid.Parse(rid)
				assert.NoError(t, err)
			},
		},
		{
			name: "request ID already in context",
			ctx: func() context.Context {
				ctx := context.WithValue(context.Background(), requestIDKey, "existing-rid")
				return metadata.NewIncomingContext(
					ctx,
					metadata.New(map[string]string{"x-request-id": "new-rid"}),
				)
			}(),
			validate: func(t *testing.T, ctx context.Context, rid string) {
				assert.Equal(t, "new-rid", rid)
				val := ctx.Value(requestIDKey)
				assert.NotNil(t, val)
				assert.Equal(t, "new-rid", val.(string))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interceptor := LoggingStreamInterceptor()

			baseStream := &mockServerStream{ctx: tt.ctx}
			var wrappedCtx context.Context

			handler := func(srv any, stream grpc.ServerStream) error {
				wrappedCtx = stream.Context()
				return nil
			}

			err := interceptor(nil, baseStream, &grpc.StreamServerInfo{
				FullMethod: "/test.Service/Method",
			}, handler)

			assert.NoError(t, err)

			if tt.validate != nil {
				val := wrappedCtx.Value(requestIDKey)
				assert.NotNil(t, val)
				tt.validate(t, wrappedCtx, val.(string))
			}
		})
	}
}

func TestAuthUnaryIterceptor(t *testing.T) {
	tests := []struct {
		name       string
		ctx        context.Context
		setupPeer  func() *peer.Peer
		wantUserID string
		wantErr    bool
		errCode    codes.Code
	}{
		{
			name: "successful authentication with valid cert",
			ctx:  context.WithValue(context.Background(), requestIDKey, "test-rid"),
			setupPeer: func() *peer.Peer {
				cert := &x509.Certificate{
					Subject: pkix.Name{
						CommonName: "test-user",
					},
				}
				return &peer.Peer{
					AuthInfo: credentials.TLSInfo{
						State: tls.ConnectionState{
							PeerCertificates: []*x509.Certificate{cert},
						},
					},
				}
			},
			wantUserID: "test-user",
			wantErr:    false,
		},
		{
			name: "no peer in context",
			ctx:  context.WithValue(context.Background(), requestIDKey, "test-rid"),
			setupPeer: func() *peer.Peer {
				return &peer.Peer{}
			},
			wantErr: true,
			errCode: codes.Internal,
		},
		{
			name: "no TLS info in peer",
			ctx:  context.WithValue(context.Background(), requestIDKey, "test-rid"),
			setupPeer: func() *peer.Peer {
				return &peer.Peer{
					AuthInfo: nil,
				}
			},
			wantErr: true,
			errCode: codes.Internal,
		},
		{
			name: "empty peer certificates",
			ctx:  context.WithValue(context.Background(), requestIDKey, "test-rid"),
			setupPeer: func() *peer.Peer {
				return &peer.Peer{
					AuthInfo: credentials.TLSInfo{
						State: tls.ConnectionState{
							PeerCertificates: []*x509.Certificate{},
						},
					},
				}
			},
			wantErr: true,
			errCode: codes.Unauthenticated,
		},
		{
			name: "empty common name in certificate",
			ctx:  context.WithValue(context.Background(), requestIDKey, "test-rid"),
			setupPeer: func() *peer.Peer {
				cert := &x509.Certificate{
					Subject: pkix.Name{
						CommonName: "",
					},
				}
				return &peer.Peer{
					AuthInfo: credentials.TLSInfo{
						State: tls.ConnectionState{
							PeerCertificates: []*x509.Certificate{cert},
						},
					},
				}
			},
			wantErr: true,
			errCode: codes.Unauthenticated,
		},
		{
			name: "multiple certificates - use first",
			ctx:  context.WithValue(context.Background(), requestIDKey, "test-rid"),
			setupPeer: func() *peer.Peer {
				cert1 := &x509.Certificate{
					Subject: pkix.Name{
						CommonName: "first-user",
					},
				}
				cert2 := &x509.Certificate{
					Subject: pkix.Name{
						CommonName: "second-user",
					},
				}
				return &peer.Peer{
					AuthInfo: credentials.TLSInfo{
						State: tls.ConnectionState{
							PeerCertificates: []*x509.Certificate{cert1, cert2},
						},
					},
				}
			},
			wantUserID: "first-user",
			wantErr:    false,
		},
		{
			name: "no request ID in context",
			ctx:  context.Background(),
			setupPeer: func() *peer.Peer {
				cert := &x509.Certificate{
					Subject: pkix.Name{
						CommonName: "test-user",
					},
				}
				return &peer.Peer{
					AuthInfo: credentials.TLSInfo{
						State: tls.ConnectionState{
							PeerCertificates: []*x509.Certificate{cert},
						},
					},
				}
			},
			wantUserID: "test-user",
			wantErr:    false,
		},
		{
			name: "certificate with long common name",
			ctx:  context.WithValue(context.Background(), requestIDKey, "test-rid"),
			setupPeer: func() *peer.Peer {
				longName := string(make([]byte, 1000))
				cert := &x509.Certificate{
					Subject: pkix.Name{
						CommonName: longName,
					},
				}
				return &peer.Peer{
					AuthInfo: credentials.TLSInfo{
						State: tls.ConnectionState{
							PeerCertificates: []*x509.Certificate{cert},
						},
					},
				}
			},
			wantUserID: string(make([]byte, 1000)),
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interceptor := AuthUnaryIterceptor()

			peerCtx := peer.NewContext(tt.ctx, tt.setupPeer())

			var capturedUserID string
			handler := func(ctx context.Context, req any) (any, error) {
				val := ctx.Value(UserIDKey)
				if val != nil {
					capturedUserID = val.(string)
				}
				return "response", nil
			}

			resp, err := interceptor(peerCtx, "request", &grpc.UnaryServerInfo{
				FullMethod: "/test.Service/Method",
			}, handler)

			if tt.wantErr {
				assert.Error(t, err)
				st, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tt.errCode, st.Code())
				assert.Nil(t, resp)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, "response", resp)
				assert.Equal(t, tt.wantUserID, capturedUserID)
			}
		})
	}
}

func TestAuthStreamInterceptor(t *testing.T) {
	tests := []struct {
		name       string
		ctx        context.Context
		setupPeer  func() *peer.Peer
		wantUserID string
		wantErr    bool
		errCode    codes.Code
	}{
		{
			name: "successful authentication with valid cert",
			ctx:  context.WithValue(context.Background(), requestIDKey, "test-rid"),
			setupPeer: func() *peer.Peer {
				cert := &x509.Certificate{
					Subject: pkix.Name{
						CommonName: "test-user",
					},
				}
				return &peer.Peer{
					AuthInfo: credentials.TLSInfo{
						State: tls.ConnectionState{
							PeerCertificates: []*x509.Certificate{cert},
						},
					},
				}
			},
			wantUserID: "test-user",
			wantErr:    false,
		},
		{
			name: "no peer in context",
			ctx:  context.WithValue(context.Background(), requestIDKey, "test-rid"),
			setupPeer: func() *peer.Peer {
				return &peer.Peer{}
			},
			wantErr: true,
			errCode: codes.Internal,
		},
		{
			name: "no TLS info in peer",
			ctx:  context.WithValue(context.Background(), requestIDKey, "test-rid"),
			setupPeer: func() *peer.Peer {
				return &peer.Peer{
					AuthInfo: nil,
				}
			},
			wantErr: true,
			errCode: codes.Internal,
		},
		{
			name: "empty peer certificates",
			ctx:  context.WithValue(context.Background(), requestIDKey, "test-rid"),
			setupPeer: func() *peer.Peer {
				return &peer.Peer{
					AuthInfo: credentials.TLSInfo{
						State: tls.ConnectionState{
							PeerCertificates: []*x509.Certificate{},
						},
					},
				}
			},
			wantErr: true,
			errCode: codes.Unauthenticated,
		},
		{
			name: "empty common name in certificate",
			ctx:  context.WithValue(context.Background(), requestIDKey, "test-rid"),
			setupPeer: func() *peer.Peer {
				cert := &x509.Certificate{
					Subject: pkix.Name{
						CommonName: "",
					},
				}
				return &peer.Peer{
					AuthInfo: credentials.TLSInfo{
						State: tls.ConnectionState{
							PeerCertificates: []*x509.Certificate{cert},
						},
					},
				}
			},
			wantErr: true,
			errCode: codes.Unauthenticated,
		},
		{
			name: "multiple certificates - use first",
			ctx:  context.WithValue(context.Background(), requestIDKey, "test-rid"),
			setupPeer: func() *peer.Peer {
				cert1 := &x509.Certificate{
					Subject: pkix.Name{
						CommonName: "first-user",
					},
				}
				cert2 := &x509.Certificate{
					Subject: pkix.Name{
						CommonName: "second-user",
					},
				}
				return &peer.Peer{
					AuthInfo: credentials.TLSInfo{
						State: tls.ConnectionState{
							PeerCertificates: []*x509.Certificate{cert1, cert2},
						},
					},
				}
			},
			wantUserID: "first-user",
			wantErr:    false,
		},
		{
			name: "no request ID in context",
			ctx:  context.Background(),
			setupPeer: func() *peer.Peer {
				cert := &x509.Certificate{
					Subject: pkix.Name{
						CommonName: "test-user",
					},
				}
				return &peer.Peer{
					AuthInfo: credentials.TLSInfo{
						State: tls.ConnectionState{
							PeerCertificates: []*x509.Certificate{cert},
						},
					},
				}
			},
			wantUserID: "test-user",
			wantErr:    false,
		},
		{
			name: "certificate with long common name",
			ctx:  context.WithValue(context.Background(), requestIDKey, "test-rid"),
			setupPeer: func() *peer.Peer {
				longName := string(make([]byte, 1000))
				cert := &x509.Certificate{
					Subject: pkix.Name{
						CommonName: longName,
					},
				}
				return &peer.Peer{
					AuthInfo: credentials.TLSInfo{
						State: tls.ConnectionState{
							PeerCertificates: []*x509.Certificate{cert},
						},
					},
				}
			},
			wantUserID: string(make([]byte, 1000)),
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interceptor := AuthStreamInterceptor()

			peerCtx := peer.NewContext(tt.ctx, tt.setupPeer())
			baseStream := &mockServerStream{ctx: peerCtx}

			var wrappedCtx context.Context
			var capturedUserID string

			handler := func(srv any, stream grpc.ServerStream) error {
				wrappedCtx = stream.Context()
				val := wrappedCtx.Value(UserIDKey)
				if val != nil {
					capturedUserID = val.(string)
				}
				return nil
			}

			err := interceptor(nil, baseStream, &grpc.StreamServerInfo{
				FullMethod: "/test.Service/Method",
			}, handler)

			if tt.wantErr {
				assert.Error(t, err)
				st, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tt.errCode, st.Code())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantUserID, capturedUserID)

				val := wrappedCtx.Value(requestIDKey)
				if tt.ctx.Value(requestIDKey) != nil {
					assert.Equal(t, tt.ctx.Value(requestIDKey), val)
				}
			}
		})
	}
}

type mockServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (m *mockServerStream) Context() context.Context {
	return m.ctx
}
