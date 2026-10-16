package catalog

import (
	"context"
	catalogv1 "example.com/aws-platform/app/gen/catalog/v1"
	appruntime "example.com/aws-platform/app/internal/runtime"
	"google.golang.org/grpc/codes"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

type Health struct {
	healthv1.UnimplementedHealthServer
	State *appruntime.State
}

func (h *Health) Check(ctx context.Context, req *healthv1.HealthCheckRequest) (*healthv1.HealthCheckResponse, error) {
	if req.GetService() != "" && req.GetService() != catalogv1.CatalogService_ServiceDesc.ServiceName {
		return nil, status.Error(codes.NotFound, "unknown service")
	}
	state := healthv1.HealthCheckResponse_SERVING
	if h.State.Draining() {
		state = healthv1.HealthCheckResponse_NOT_SERVING
	}
	return &healthv1.HealthCheckResponse{Status: state}, nil
}
