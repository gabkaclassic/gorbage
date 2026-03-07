package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"os"

	"log/slog"

	"github.com/gabkaclassic/gorbage/internal/config"
	pb "github.com/gabkaclassic/gorbage/internal/proto"
	"github.com/gabkaclassic/gorbage/pkg/interceptor"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const maxMsgSize = 1073741824

type GRPCServer struct {
	srv *grpc.Server
}

func SetupGRPCServer(
	cfg config.GRPC,
	artifactServer StorageGRPCServer,
) (*GRPCServer, error) {

	trustUnaryInterceptor, err := interceptor.TrustAddressUnaryInterceptor(cfg.TrustedCIDRs)
	if err != nil {
		return nil, err
	}

	trustStreamInterceptor, err := interceptor.TrustAddressStreamInterceptor(cfg.TrustedCIDRs)
	if err != nil {
		return nil, err
	}

	cert, err := tls.LoadX509KeyPair(cfg.TLS.CertPath, cfg.TLS.KeyPath)
	if err != nil {
		return nil, err
	}

	caCert, err := os.ReadFile(cfg.TLS.CAPath)
	if err != nil {
		return nil, err
	}

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caCert)

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    caPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
		ServerName:   "localhost",
	}

	creds := credentials.NewTLS(tlsConfig)

	server := grpc.NewServer(
		grpc.Creds(creds),
		grpc.MaxSendMsgSize(maxMsgSize),
		grpc.MaxRecvMsgSize(maxMsgSize),
		grpc.ChainUnaryInterceptor(
			interceptor.LoggingUnaryInterceptor(),
			trustUnaryInterceptor,
			interceptor.AuthUnaryIterceptor(),
		),
		grpc.ChainStreamInterceptor(
			interceptor.LoggingStreamInterceptor(),
			trustStreamInterceptor,
			interceptor.AuthStreamInterceptor(),
		),
	)

	pb.RegisterStorageServiceServer(server, artifactServer)

	return &GRPCServer{srv: server}, nil
}

func (g *GRPCServer) Run(ctx context.Context, addr string) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("failed to listen gRPC", "addr", addr, "error", err)
		return
	}

	serveDone := make(chan struct{})

	go func() {
		defer close(serveDone)

		if err := g.srv.Serve(lis); err != nil {
			slog.Error("gRPC server serve error", "error", err)
		}
	}()

	slog.Info("gRPC server started", "addr", addr)

	<-ctx.Done()
	slog.Info("gRPC shutdown requested")

	g.srv.GracefulStop()
	<-serveDone

	slog.Info("gRPC server stopped gracefully")
}
