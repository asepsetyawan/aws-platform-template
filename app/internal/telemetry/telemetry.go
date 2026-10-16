package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func Logger(service string) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", service)
}

func InitTracing(ctx context.Context, service string) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return func(context.Context) error { return nil }, nil
	}
	ratio := 0.1
	if value := os.Getenv("OTEL_TRACES_SAMPLER_ARG"); value != "" {
		var err error
		ratio, err = strconv.ParseFloat(value, 64)
		if err != nil || ratio < 0 || ratio > 1 {
			return nil, fmt.Errorf("OTEL_TRACES_SAMPLER_ARG must be between 0 and 1")
		}
	}
	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}
	res := resource.NewWithAttributes("", attribute.String("service.name", service), attribute.String("deployment.environment.name", os.Getenv("APP_ENV")))
	provider := sdktrace.NewTracerProvider(sdktrace.WithResource(res), sdktrace.WithBatcher(exporter), sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))))
	otel.SetTracerProvider(provider)
	return provider.Shutdown, nil
}

type Metrics struct {
	Registry     *prometheus.Registry
	HTTPRequests *prometheus.CounterVec
	HTTPDuration *prometheus.HistogramVec
	RPCRequests  *prometheus.CounterVec
	RPCDuration  *prometheus.HistogramVec
	Logger       *slog.Logger
}

func New(service string, logger *slog.Logger) *Metrics {
	m := &Metrics{Registry: prometheus.NewRegistry(), Logger: logger}
	m.Registry.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	if service == "api" {
		m.HTTPRequests = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "api_http_requests_total", Help: "Application REST requests"}, []string{"method", "route", "status"})
		m.HTTPDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "api_http_request_duration_seconds", Help: "REST request duration", Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2, 5}}, []string{"method", "route"})
		m.Registry.MustRegister(m.HTTPRequests, m.HTTPDuration)
	} else {
		m.RPCRequests = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "catalog_grpc_requests_total", Help: "Internal gRPC requests"}, []string{"method", "code"})
		m.RPCDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "catalog_grpc_request_duration_seconds", Help: "Internal gRPC duration", Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2}}, []string{"method"})
		m.Registry.MustRegister(m.RPCRequests, m.RPCDuration)
	}
	return m
}
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
		w.ResponseWriter.WriteHeader(code)
	}
}
func (w *responseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}
func (w *responseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Route is a fixed template supplied by the router, never an arbitrary user URL.
func (m *Metrics) HTTP(route string, next http.Handler) http.Handler {
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		rw := &responseWriter{ResponseWriter: w}
		next.ServeHTTP(rw, r)
		code := rw.status
		if code == 0 {
			code = 200
		}
		method := r.Method
		switch method {
		case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		default:
			method = "OTHER"
		}
		m.HTTPRequests.WithLabelValues(method, route, strconv.Itoa(code)).Inc()
		m.HTTPDuration.WithLabelValues(method, route).Observe(time.Since(started).Seconds())
		span := trace.SpanContextFromContext(r.Context())
		m.Logger.Info("request", "method", method, "route", route, "status", code, "duration_ms", time.Since(started).Milliseconds(), "trace_id", span.TraceID().String(), "request_id", rw.Header().Get("X-Request-ID"))
	})
	return otelhttp.NewHandler(wrapped, route)
}
func (m *Metrics) UnaryServer() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		started := time.Now()
		response, err := handler(ctx, req)
		// Health checks do not consume the application error budget.
		if info.FullMethod != "/grpc.health.v1.Health/Check" {
			code := status.Code(err).String()
			m.RPCRequests.WithLabelValues(info.FullMethod, code).Inc()
			m.RPCDuration.WithLabelValues(info.FullMethod).Observe(time.Since(started).Seconds())
			span := trace.SpanContextFromContext(ctx)
			requestID := ""
			if values := metadata.ValueFromIncomingContext(ctx, "x-request-id"); len(values) > 0 && len(values[0]) <= 128 {
				requestID = values[0]
			}
			m.Logger.Info("rpc", "method", info.FullMethod, "code", code, "duration_ms", time.Since(started).Milliseconds(), "trace_id", span.TraceID().String(), "request_id", requestID)
		}
		return response, err
	}
}
