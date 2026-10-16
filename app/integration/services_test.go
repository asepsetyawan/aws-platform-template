package integration

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	catalogv1 "example.com/aws-platform/app/gen/catalog/v1"
	"example.com/aws-platform/app/internal/catalog"
	"example.com/aws-platform/app/internal/gateway"
	appruntime "example.com/aws-platform/app/internal/runtime"
	"example.com/aws-platform/app/internal/telemetry"
	"example.com/aws-platform/app/internal/transport"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type failingCatalog struct {
	catalogv1.UnimplementedCatalogServiceServer
	code codes.Code
	wait bool
}

func (s *failingCatalog) GetProduct(ctx context.Context, _ *catalogv1.GetProductRequest) (*catalogv1.GetProductResponse, error) {
	if s.wait {
		<-ctx.Done()
		return nil, status.FromContextError(ctx.Err()).Err()
	}
	return nil, status.Error(s.code, "private backend diagnostics must never leak")
}

type stack struct {
	handler       http.Handler
	state         *appruntime.State
	serverMetrics *telemetry.Metrics
	conn          *grpc.ClientConn
}

func start(t *testing.T, implementation catalogv1.CatalogServiceServer, timeout time.Duration, serverOptions []grpc.ServerOption, clientOptions []grpc.DialOption) stack {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	metrics := telemetry.New("catalog", logger)
	state := &appruntime.State{DrainFile: filepath.Join(t.TempDir(), "draining")}
	opts := append([]grpc.ServerOption{grpc.StatsHandler(otelgrpc.NewServerHandler()), grpc.UnaryInterceptor(metrics.UnaryServer())}, serverOptions...)
	server := grpc.NewServer(opts...)
	catalogv1.RegisterCatalogServiceServer(server, implementation)
	healthv1.RegisterHealthServer(server, &catalog.Health{State: state})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })
	dialOptions := []grpc.DialOption{grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithStatsHandler(otelgrpc.NewClientHandler()), grpc.WithDisableRetry()}
	if len(clientOptions) == 0 {
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	dialOptions = append(dialOptions, clientOptions...)
	conn, err := grpc.NewClient("passthrough:///bufnet", dialOptions...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	h := &gateway.Handler{Catalog: catalogv1.NewCatalogServiceClient(conn), Health: healthv1.NewHealthClient(conn), State: &appruntime.State{DrainFile: filepath.Join(t.TempDir(), "draining")}, Metrics: telemetry.New("api", logger), Environment: "test", Timeout: timeout}
	return stack{handler: h.Router(), state: state, serverMetrics: metrics, conn: conn}
}
func request(handler http.Handler, path string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
	return response
}

func TestRESTCallsRealGRPCAndExportsBoundedMetrics(t *testing.T) {
	s := start(t, &catalog.Server{}, time.Second, nil, nil)
	response := request(s.handler, "/api/products/sku-001")
	if response.Code != 200 || !strings.Contains(response.Body.String(), "Cloud Architecture Handbook") {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body)
	}
	if response.Header().Get("X-Request-ID") == "" {
		t.Fatal("missing request ID")
	}
	if got := request(s.handler, "/api/hello"); got.Code != 200 || !strings.Contains(got.Body.String(), "featured_product") {
		t.Fatalf("hello must call catalog: %s", got.Body)
	}
	if got := request(s.handler, "/api/products/not-found"); got.Code != 404 {
		t.Fatalf("got %d", got.Code)
	}
	if got := request(s.handler, "/api/products/%20"); got.Code != 400 {
		t.Fatalf("got %d", got.Code)
	}
	_ = request(s.handler, "/missing/user-123")
	metrics := request(s.handler, "/metrics").Body.String()
	if !strings.Contains(metrics, `route="/api/products/{id}"`) || strings.Contains(metrics, "sku-001") || strings.Contains(metrics, "user-123") {
		t.Fatalf("metric route cardinality is not bounded: %s", metrics)
	}
	families, err := s.serverMetrics.Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, family := range families {
		if family.GetName() == "catalog_grpc_requests_total" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing internal gRPC metrics")
	}
}
func TestRPCErrorMappingAndRedaction(t *testing.T) {
	for _, test := range []struct {
		code codes.Code
		want int
	}{{codes.NotFound, 404}, {codes.InvalidArgument, 400}, {codes.Unavailable, 503}, {codes.Internal, 502}} {
		t.Run(test.code.String(), func(t *testing.T) {
			s := start(t, &failingCatalog{code: test.code}, time.Second, nil, nil)
			response := request(s.handler, "/api/products/sku-001")
			if response.Code != test.want {
				t.Fatalf("got %d want %d", response.Code, test.want)
			}
			if strings.Contains(response.Body.String(), "private backend") {
				t.Fatal("backend diagnostics leaked")
			}
		})
	}
}
func TestDeadlineCancelsInternalCall(t *testing.T) {
	s := start(t, &failingCatalog{wait: true}, 40*time.Millisecond, nil, nil)
	started := time.Now()
	response := request(s.handler, "/api/products/sku-001")
	if response.Code != 504 {
		t.Fatalf("got %d want 504: %s", response.Code, response.Body)
	}
	if time.Since(started) > time.Second {
		t.Fatal("internal deadline was not bounded")
	}
}
func TestDrainingDependencyChangesReadinessNotLiveness(t *testing.T) {
	s := start(t, &catalog.Server{}, time.Second, nil, nil)
	if got := request(s.handler, "/readyz").Code; got != 200 {
		t.Fatalf("got %d", got)
	}
	s.state.Drain()
	if got := request(s.handler, "/readyz").Code; got != 503 {
		t.Fatalf("got %d", got)
	}
	if got := request(s.handler, "/livez").Code; got != 200 {
		t.Fatalf("got %d", got)
	}
}
func TestTraceContinuesFromRESTThroughGRPC(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter), sdktrace.WithSampler(sdktrace.AlwaysSample()))
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	s := start(t, &catalog.Server{}, time.Second, nil, nil)
	req := httptest.NewRequest("GET", "/api/products/sku-001", nil)
	req.Header.Set("traceparent", "00-11111111111111111111111111111111-2222222222222222-01")
	response := httptest.NewRecorder()
	s.handler.ServeHTTP(response, req)
	if response.Code != 200 {
		t.Fatalf("got %d", response.Code)
	}
	deadline := time.Now().Add(time.Second)
	var httpSpan, rpcClient, rpcServer *tracetest.SpanStub
	for time.Now().Before(deadline) {
		spans := exporter.GetSpans()
		for i := range spans {
			span := spans[i]
			if span.Name == "GET /api/products/{id}" {
				copy := span
				httpSpan = &copy
			}
			if strings.Contains(span.Name, "CatalogService/GetProduct") && span.SpanKind == trace.SpanKindClient {
				copy := span
				rpcClient = &copy
			}
			if strings.Contains(span.Name, "CatalogService/GetProduct") && span.SpanKind == trace.SpanKindServer {
				copy := span
				rpcServer = &copy
			}
		}
		if httpSpan != nil && rpcClient != nil && rpcServer != nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if httpSpan == nil || rpcClient == nil || rpcServer == nil {
		t.Fatalf("missing distributed spans: %#v", exporter.GetSpans())
	}
	if httpSpan.SpanContext.TraceID().String() != "11111111111111111111111111111111" || rpcClient.Parent.SpanID() != httpSpan.SpanContext.SpanID() || rpcServer.Parent.SpanID() != rpcClient.SpanContext.SpanID() {
		t.Fatal("REST → gRPC parent/trace propagation is broken")
	}
}

func certificates(t *testing.T) (string, string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), DNSNames: []string{"catalog"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, ca, &leafKey.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leafKeyDER, err := x509.MarshalPKCS8PrivateKey(leafKey)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	caFile, certFile, keyFile := filepath.Join(dir, "ca.crt"), filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key")
	for path, content := range map[string][]byte{caFile: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), certFile: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}), keyFile: pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: leafKeyDER})} {
		if err := os.WriteFile(path, content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return caFile, certFile, keyFile
}
func TestInternalGRPCUsesVerifiedTLS(t *testing.T) {
	ca, cert, key := certificates(t)
	serverCreds, err := transport.ServerCredentials(cert, key, false)
	if err != nil {
		t.Fatal(err)
	}
	clientCreds, err := transport.ClientCredentials(ca, "catalog", false)
	if err != nil {
		t.Fatal(err)
	}
	s := start(t, &catalog.Server{}, time.Second, []grpc.ServerOption{grpc.Creds(serverCreds)}, []grpc.DialOption{grpc.WithTransportCredentials(clientCreds)})
	if response := request(s.handler, "/api/products/sku-001"); response.Code != 200 {
		t.Fatalf("TLS call failed: %d %s", response.Code, response.Body)
	}
	wrongName, err := transport.ClientCredentials(ca, "wrong-host", false)
	if err != nil {
		t.Fatal(err)
	}
	bad := start(t, &catalog.Server{}, time.Second, []grpc.ServerOption{grpc.Creds(serverCreds)}, []grpc.DialOption{grpc.WithTransportCredentials(wrongName)})
	if response := request(bad.handler, "/api/products/sku-001"); response.Code != 503 {
		t.Fatalf("must reject wrong TLS hostname: %d", response.Code)
	}
}

func TestCatalogValidatesDirectRPCInputs(t *testing.T) {
	s := start(t, &catalog.Server{}, time.Second, nil, nil)
	client := catalogv1.NewCatalogServiceClient(s.conn)
	for _, id := range []string{"", "invalid ID", strings.Repeat("x", 65)} {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_, err := client.GetProduct(ctx, &catalogv1.GetProductRequest{Id: id})
		cancel()
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("expected InvalidArgument for %q, got %v", id, err)
		}
	}
}
