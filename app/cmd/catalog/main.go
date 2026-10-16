package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	catalogv1 "example.com/aws-platform/app/gen/catalog/v1"
	"example.com/aws-platform/app/internal/catalog"
	appruntime "example.com/aws-platform/app/internal/runtime"
	"example.com/aws-platform/app/internal/telemetry"
	"example.com/aws-platform/app/internal/transport"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
)

func main() {
	logger := telemetry.Logger("catalog")
	if handled, err := appruntime.Command(":9090", "/tmp/catalog-draining"); handled {
		if err != nil {
			logger.Error("command failed", "error", err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		logger.Error("service failed", "error", err)
		os.Exit(1)
	}
}
func run() error {
	logger := telemetry.Logger("catalog")
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	shutdown, err := telemetry.InitTracing(ctx, "platform-catalog")
	if err != nil {
		return err
	}
	defer func() {
		flush, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = shutdown(flush)
	}()
	creds, err := transport.ServerCredentials(os.Getenv("GRPC_TLS_CERT_FILE"), os.Getenv("GRPC_TLS_KEY_FILE"), os.Getenv("GRPC_INSECURE") == "true")
	if err != nil {
		return err
	}
	state, err := appruntime.NewState(appruntime.Env("DRAIN_FILE", "/tmp/catalog-draining"))
	if err != nil {
		return err
	}
	metrics := telemetry.New("catalog", logger)
	opts := []grpc.ServerOption{grpc.StatsHandler(otelgrpc.NewServerHandler()), grpc.UnaryInterceptor(metrics.UnaryServer()), grpc.MaxRecvMsgSize(4 << 10), grpc.MaxHeaderListSize(16 << 10), grpc.MaxSendMsgSize(64 << 10)}
	if creds != nil {
		opts = append(opts, grpc.Creds(creds))
	}
	rpc := grpc.NewServer(opts...)
	catalogv1.RegisterCatalogServiceServer(rpc, &catalog.Server{})
	healthv1.RegisterHealthServer(rpc, &catalog.Health{State: state})
	listener, err := net.Listen("tcp", appruntime.Env("GRPC_ADDR", ":50051"))
	if err != nil {
		return err
	}
	defer listener.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", state.Live)
	mux.HandleFunc("GET /readyz", state.Ready)
	mux.Handle("GET /metrics", metrics.Handler())
	admin := appruntime.HTTPServer(appruntime.Env("HTTP_ADDR", ":9090"), mux)
	failed := make(chan error, 2)
	go func() { failed <- rpc.Serve(listener) }()
	go func() { failed <- admin.ListenAndServe() }()
	logger.Info("gRPC catalog started", "address", listener.Addr().String())
	var serviceErr error
	select {
	case serviceErr = <-failed:
	case <-ctx.Done():
	}
	state.Drain()
	done := make(chan struct{})
	go func() { rpc.GracefulStop(); close(done) }()
	drain, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	_ = admin.Shutdown(drain)
	select {
	case <-done:
	case <-drain.Done():
		rpc.Stop()
	}
	if errors.Is(serviceErr, http.ErrServerClosed) || errors.Is(serviceErr, grpc.ErrServerStopped) {
		return nil
	}
	return serviceErr
}
