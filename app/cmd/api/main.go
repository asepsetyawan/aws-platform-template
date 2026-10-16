package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	catalogv1 "example.com/aws-platform/app/gen/catalog/v1"
	"example.com/aws-platform/app/internal/gateway"
	appruntime "example.com/aws-platform/app/internal/runtime"
	"example.com/aws-platform/app/internal/telemetry"
	"example.com/aws-platform/app/internal/transport"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	_ "google.golang.org/grpc/balancer/roundrobin"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
)

func main() {
	logger := telemetry.Logger("api")
	if handled, err := appruntime.Command(":8080", "/tmp/api-draining"); handled {
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
	logger := telemetry.Logger("api")
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	shutdown, err := telemetry.InitTracing(ctx, "platform-api")
	if err != nil {
		return err
	}
	defer func() {
		flush, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = shutdown(flush)
	}()
	creds, err := transport.ClientCredentials(os.Getenv("GRPC_CA_FILE"), os.Getenv("GRPC_SERVER_NAME"), os.Getenv("GRPC_INSECURE") == "true")
	if err != nil {
		return err
	}
	timeout, err := appruntime.Duration("GRPC_TIMEOUT", 800*time.Millisecond)
	if err != nil {
		return err
	}
	conn, err := grpc.NewClient(appruntime.Env("CATALOG_GRPC_ADDR", "dns:///catalog:50051"), grpc.WithTransportCredentials(creds), grpc.WithStatsHandler(otelgrpc.NewClientHandler()), grpc.WithDisableRetry(), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(64<<10)), grpc.WithDefaultServiceConfig(`{"loadBalancingConfig":[{"round_robin":{}}]}`))
	if err != nil {
		return err
	}
	defer conn.Close()
	state, err := appruntime.NewState(appruntime.Env("DRAIN_FILE", "/tmp/api-draining"))
	if err != nil {
		return err
	}
	handler := &gateway.Handler{Catalog: catalogv1.NewCatalogServiceClient(conn), Health: healthv1.NewHealthClient(conn), State: state, Metrics: telemetry.New("api", logger), Environment: appruntime.Env("APP_ENV", "local"), Timeout: timeout}
	server := appruntime.HTTPServer(appruntime.Env("HTTP_ADDR", ":8080"), handler.Router())
	failed := make(chan error, 1)
	go func() { failed <- server.ListenAndServe() }()
	logger.Info("REST API started", "address", server.Addr)
	select {
	case err := <-failed:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	state.Drain()
	drain, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	return server.Shutdown(drain)
}
