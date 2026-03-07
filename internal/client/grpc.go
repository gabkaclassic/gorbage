package client

import (
	"crypto/tls"
	"crypto/x509"
	"os"

	"github.com/gabkaclassic/gorbage/internal/config"
	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const maxMsgSize = 1073741824

func NewGRPCConnection(cfg config.Server) (*grpc.ClientConn, error) {

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
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS13,
	}

	return grpc.NewClient(
		cfg.Address,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallSendMsgSize(maxMsgSize),
			grpc.MaxCallRecvMsgSize(maxMsgSize),
		),
	)
}
