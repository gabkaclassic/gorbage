package interceptor

import (
	"context"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type (
	ctxKey        string
	wrappedStream struct {
		grpc.ServerStream
		ctx context.Context
	}
)

const (
	requestIDKey ctxKey = "x-request-id"
	UserIDKey    ctxKey = "x-user-id"
)

func (w *wrappedStream) Context() context.Context {
	return w.ctx
}

func prepareTrustedCIDRs(cidrs []string) ([]*net.IPNet, error) {
	if len(cidrs) == 0 {
		return nil, nil
	}
	trustedCIDRs := make([]*net.IPNet, len(cidrs))

	for ind, cidr := range cidrs {
		_, trustedCIDR, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, err
		}
		trustedCIDRs[ind] = trustedCIDR
	}

	return trustedCIDRs, nil
}

func checkTrustIP(ctx context.Context, trustedCIDRs []*net.IPNet) error {
	ipStr := ""

	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if vals := md.Get("x-real-ip"); len(vals) > 0 {
			ipStr = strings.TrimSpace(vals[0])
		}
	}

	ip := net.ParseIP(ipStr)

	if ip == nil {
		requestID := ctx.Value(requestIDKey)
		slog.Info("Forbidden by untrusted IP",
			slog.String("IP", ipStr),
			slog.Any("rid", requestID),
		)
		return status.Error(codes.PermissionDenied, "forbidden")
	}

	for _, cidr := range trustedCIDRs {
		if cidr.Contains(ip) {
			return nil
		}
	}

	requestID := ctx.Value(requestIDKey)
	slog.Info("Forbidden by untrusted IP",
		slog.String("IP", ipStr),
		slog.Any("rid", requestID),
	)
	return status.Error(codes.PermissionDenied, "forbidden")
}

func TrustAddressUnaryInterceptor(cidrs []string) (grpc.UnaryServerInterceptor, error) {

	trustedCIDRs, err := prepareTrustedCIDRs(cidrs)

	if err != nil {
		return nil, err
	}

	if trustedCIDRs == nil {
		return func(
			ctx context.Context,
			req any,
			info *grpc.UnaryServerInfo,
			handler grpc.UnaryHandler,
		) (any, error) {
			return handler(ctx, req)
		}, nil
	}

	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {

		err := checkTrustIP(ctx, trustedCIDRs)

		if err != nil {
			return nil, err
		}

		return handler(ctx, req)
	}, nil
}

func TrustAddressStreamInterceptor(cidrs []string) (grpc.StreamServerInterceptor, error) {

	trustedCIDRs, err := prepareTrustedCIDRs(cidrs)

	if err != nil {
		return nil, err
	}

	if trustedCIDRs == nil {
		return func(
			srv any,
			ss grpc.ServerStream,
			info *grpc.StreamServerInfo,
			handler grpc.StreamHandler,
		) error {
			return handler(srv, ss)
		}, nil
	}

	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {

		err := checkTrustIP(ss.Context(), trustedCIDRs)

		if err != nil {
			return err
		}

		return handler(ss.Context(), ss)
	}, nil
}

// LoggingUnaryInterceptor logs gRPC request execution metrics.
//
// Logged fields:
//
//   - method: Full RPC method name.
//   - duration: Request processing time.
//
// Behaviour:
//
// Measures time from request entry to handler completion.
// Logs request execution using structured logging.
//
// Purpose:
//
// Provides observability for RPC performance monitoring.
func LoggingUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {

		md, ok := metadata.FromIncomingContext(ctx)

		var requestID string

		if ok {
			if vals := md.Get("x-request-id"); len(vals) > 0 {
				requestID = strings.TrimSpace(vals[0])
				if len(requestID) == 0 {
					requestID = uuid.NewString()
				}
			} else {
				requestID = uuid.NewString()
			}
		} else {
			requestID = uuid.NewString()
		}

		ctx = context.WithValue(ctx, requestIDKey, requestID)

		start := time.Now()

		resp, err := handler(ctx, req)

		duration := time.Since(start)

		slog.Debug("gRPC request",
			slog.String("method", info.FullMethod),
			slog.Duration("duration", duration),
			slog.String("rid", requestID),
			slog.Any("response", resp),
		)

		return resp, err
	}
}

func AuthUnaryIterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {

		p, ok := peer.FromContext(ctx)
		requestID := ctx.Value(requestIDKey)

		if !ok {
			slog.Error("get peer info from context error", slog.Any("rid", requestID))
			return nil, status.Error(codes.Internal, "internal error")
		}

		mTLSInfo, ok := p.AuthInfo.(credentials.TLSInfo)

		if !ok {
			slog.Error("get client mTLS info error", slog.Any("rid", requestID))
			return nil, status.Error(codes.Internal, "internal error")
		}

		if len(mTLSInfo.State.PeerCertificates) == 0 {
			slog.Info("empty mTLS info", slog.Any("rid", requestID))
			return nil, status.Error(codes.Unauthenticated, "invalid cert")
		}

		userID := mTLSInfo.State.PeerCertificates[0].Subject.CommonName

		if len(userID) == 0 {
			slog.Info("empty user ID", slog.Any("rid", requestID))
			return nil, status.Error(codes.Unauthenticated, "unauthorized")
		}

		ctx = context.WithValue(ctx, UserIDKey, userID)

		resp, err := handler(ctx, req)

		return resp, err
	}
}

func LoggingStreamInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {

		ctx := ss.Context()
		md, ok := metadata.FromIncomingContext(ctx)

		var requestID string

		if ok {
			if vals := md.Get("x-request-id"); len(vals) > 0 {
				requestID = strings.TrimSpace(vals[0])
				if len(requestID) == 0 {
					requestID = uuid.NewString()
				}
			} else {
				requestID = uuid.NewString()
			}
		} else {
			requestID = uuid.NewString()
		}

		ctx = context.WithValue(ctx, requestIDKey, requestID)

		wrapped := &wrappedStream{
			ServerStream: ss,
			ctx:          ctx,
		}

		start := time.Now()

		err := handler(srv, wrapped)

		duration := time.Since(start)

		slog.Info("gRPC stream",
			slog.String("method", info.FullMethod),
			slog.Duration("duration", duration),
			slog.String("rid", requestID),
		)

		return err
	}
}

func AuthStreamInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {

		ctx := ss.Context()
		p, ok := peer.FromContext(ctx)
		requestID := ctx.Value(requestIDKey)

		if !ok {
			slog.Error("get peer info from context error", slog.Any("rid", requestID))
			return status.Error(codes.Internal, "internal error")
		}

		mTLSInfo, ok := p.AuthInfo.(credentials.TLSInfo)
		if !ok {
			slog.Error("get client mTLS info error", slog.Any("rid", requestID))
			return status.Error(codes.Internal, "internal error")
		}

		if len(mTLSInfo.State.PeerCertificates) == 0 {
			slog.Info("empty mTLS info", slog.Any("rid", requestID))
			return status.Error(codes.Unauthenticated, "invalid cert")
		}

		userID := mTLSInfo.State.PeerCertificates[0].Subject.CommonName

		if len(userID) == 0 {
			slog.Info("empty user ID", slog.Any("rid", requestID))
			return status.Error(codes.Unauthenticated, "unauthorized")
		}

		ctx = context.WithValue(ctx, UserIDKey, userID)

		wrapped := &wrappedStream{
			ServerStream: ss,
			ctx:          ctx,
		}

		return handler(srv, wrapped)
	}
}
