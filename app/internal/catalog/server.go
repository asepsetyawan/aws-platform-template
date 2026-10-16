package catalog

import (
	"context"
	"regexp"

	catalogv1 "example.com/aws-platform/app/gen/catalog/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var productID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

type Server struct {
	catalogv1.UnimplementedCatalogServiceServer
}

func (s *Server) GetProduct(ctx context.Context, req *catalogv1.GetProductRequest) (*catalogv1.GetProductResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	if !productID.MatchString(req.GetId()) {
		return nil, status.Error(codes.InvalidArgument, "invalid product ID")
	}
	var product *catalogv1.Product
	switch req.GetId() {
	case "sku-001":
		product = &catalogv1.Product{Id: "sku-001", Name: "Cloud Architecture Handbook", PriceCents: 2500, Currency: "USD"}
	case "sku-002":
		product = &catalogv1.Product{Id: "sku-002", Name: "Terraform Practice Kit", PriceCents: 3900, Currency: "USD"}
	default:
		return nil, status.Error(codes.NotFound, "product not found")
	}
	return &catalogv1.GetProductResponse{Product: product}, nil
}
