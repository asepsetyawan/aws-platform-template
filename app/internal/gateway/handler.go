package gateway

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"regexp"
	"time"

	catalogv1 "example.com/aws-platform/app/gen/catalog/v1"
	appruntime "example.com/aws-platform/app/internal/runtime"
	"example.com/aws-platform/app/internal/telemetry"
	"google.golang.org/grpc/codes"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var productID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

type Handler struct {
	Catalog     catalogv1.CatalogServiceClient
	Health      healthv1.HealthClient
	State       *appruntime.State
	Metrics     *telemetry.Metrics
	Environment string
	Timeout     time.Duration
}

type product struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	PriceCents int64  `json:"price_cents"`
	Currency   string `json:"currency"`
}

func (h *Handler) Router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", h.State.Live)
	mux.HandleFunc("GET /readyz", h.ready)
	mux.Handle("GET /metrics", h.Metrics.Handler())
	mux.Handle("GET /api/hello", h.Metrics.HTTP("/api/hello", http.HandlerFunc(h.hello)))
	mux.Handle("GET /api/products/{id}", h.Metrics.HTTP("/api/products/{id}", http.HandlerFunc(h.getProduct)))
	mux.Handle("/", h.Metrics.HTTP("__unmatched__", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, 404, map[string]string{"error": "not found"})
	})))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", rand.Text())
		w.Header().Set("X-Content-Type-Options", "nosniff")
		mux.ServeHTTP(w, r)
	})
}
func (h *Handler) ready(w http.ResponseWriter, r *http.Request) {
	if h.State.Draining() {
		jsonResponse(w, 503, map[string]string{"status": "draining"})
		return
	}
	timeout := h.Timeout
	if timeout > 500*time.Millisecond {
		timeout = 500 * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	response, err := h.Health.Check(ctx, &healthv1.HealthCheckRequest{Service: catalogv1.CatalogService_ServiceDesc.ServiceName})
	if err != nil || response.GetStatus() != healthv1.HealthCheckResponse_SERVING {
		jsonResponse(w, 503, map[string]string{"status": "dependency unavailable"})
		return
	}
	jsonResponse(w, 200, map[string]string{"status": "ready"})
}
func (h *Handler) fetch(w http.ResponseWriter, r *http.Request, id string) (*product, bool) {
	if !productID.MatchString(id) {
		jsonResponse(w, 400, map[string]string{"error": "invalid product ID"})
		return nil, false
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.Timeout)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "x-request-id", w.Header().Get("X-Request-ID"))
	response, err := h.Catalog.GetProduct(ctx, &catalogv1.GetProductRequest{Id: id})
	if err != nil {
		code, message := HTTPError(err)
		h.Metrics.Logger.WarnContext(ctx, "catalog call failed", "grpc_code", status.Code(err).String(), "http_status", code, "request_id", w.Header().Get("X-Request-ID"))
		jsonResponse(w, code, map[string]string{"error": message})
		return nil, false
	}
	if response.GetProduct() == nil {
		jsonResponse(w, 502, map[string]string{"error": "invalid upstream response"})
		return nil, false
	}
	p := response.GetProduct()
	return &product{ID: p.GetId(), Name: p.GetName(), PriceCents: p.GetPriceCents(), Currency: p.GetCurrency()}, true
}
func (h *Handler) getProduct(w http.ResponseWriter, r *http.Request) {
	p, ok := h.fetch(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	jsonResponse(w, 200, p)
}
func (h *Handler) hello(w http.ResponseWriter, r *http.Request) {
	p, ok := h.fetch(w, r, "sku-001")
	if !ok {
		return
	}
	jsonResponse(w, 200, map[string]any{"message": "Hello from AWS EKS", "environment": h.Environment, "featured_product": p})
}
func HTTPError(err error) (int, string) {
	switch status.Code(err) {
	case codes.NotFound:
		return 404, "product not found"
	case codes.InvalidArgument:
		return 400, "invalid product ID"
	case codes.DeadlineExceeded:
		return 504, "catalog deadline exceeded"
	case codes.Unavailable:
		return 503, "catalog unavailable"
	case codes.Canceled:
		return 503, "request canceled"
	default:
		return 502, "catalog request failed"
	}
}
func jsonResponse(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}
