package transport

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// Explicit opt-in is required for unencrypted localhost/Compose development.
func ClientCredentials(caFile, serverName string, allowInsecure bool) (credentials.TransportCredentials, error) {
	if caFile == "" {
		if allowInsecure {
			return insecure.NewCredentials(), nil
		}
		return nil, errors.New("GRPC_CA_FILE is required; GRPC_INSECURE=true is for local development only")
	}
	if allowInsecure {
		return nil, errors.New("cannot combine GRPC_CA_FILE with GRPC_INSECURE")
	}
	if serverName == "" {
		return nil, errors.New("GRPC_SERVER_NAME is required for TLS hostname verification")
	}
	pem, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		return nil, errors.New("invalid gRPC CA certificate")
	}
	return credentials.NewTLS(&tls.Config{RootCAs: roots, ServerName: serverName, MinVersion: tls.VersionTLS12}), nil
}

func ServerCredentials(certFile, keyFile string, allowInsecure bool) (credentials.TransportCredentials, error) {
	if certFile == "" && keyFile == "" {
		if allowInsecure {
			return nil, nil
		}
		return nil, errors.New("GRPC_TLS_CERT_FILE and GRPC_TLS_KEY_FILE are required")
	}
	if allowInsecure {
		return nil, errors.New("cannot combine server certificates with GRPC_INSECURE")
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	return credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}), nil
}
