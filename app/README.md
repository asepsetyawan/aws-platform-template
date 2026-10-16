# Go REST API → internal gRPC catalog

Two executables in one Go module, with committed Protobuf contracts:

```text
Client → HTTPS edge/ALB → Kong :8000 → api (REST :8080)
                                        │ GetProduct (gRPC TLS, deadline 800 ms)
                                        ▼
                                     catalog (:50051, internal)
                                     admin/metrics (:9090, internal)
```

- `cmd/api`: REST gateway, JSON responses, REST metrics, gRPC client, and dependency readiness.
- `cmd/catalog`: sample in-memory product data, gRPC server, gRPC health service, admin HTTP, and RPC metrics.
- `api/catalog/v1/catalog.proto`: `CatalogService.GetProduct` contract.
- `api/openapi.yaml`: REST contract.
- `gen/catalog/v1`: generated Protobuf/client/server stubs; builds do not require protoc.
- `internal/telemetry`: JSON logs, Prometheus, OTLP tracing, and W3C context propagation.
- `integration`: real gRPC/Protobuf tests over an in-memory transport, including TLS handshakes.

## Run with Docker Compose

```bash
docker compose -f app/compose.yaml up --build
curl --fail http://localhost:8080/api/products/sku-001
curl --fail http://localhost:8080/api/hello
docker compose -f app/compose.yaml down
```

Example product response:

```json
{"id":"sku-001","name":"Cloud Architecture Handbook","price_cents":2500,"currency":"USD"}
```

Only the Kong proxy is published on localhost:8080; api/catalog ports are not published. Kong preserves the `/api/` prefix, applies a rate limit per replica, and excludes health/metrics endpoints from public routing. See the [Kong guide](../docs/KONG.md). Catalog gRPC runs on the Compose network with explicit local plaintext opt-in through `GRPC_INSECURE=true`. Kubernetes manifests use verified TLS and do not enable that flag.

## Run directly with Go

Use Go 1.27.2 as specified in `go.mod`. This native mode runs both services directly, without Kong, for application debugging. Use Compose above for the complete gateway path. Open two terminals in `app/`:

```bash
GRPC_INSECURE=true GRPC_ADDR=127.0.0.1:50051 HTTP_ADDR=127.0.0.1:9090 go run ./cmd/catalog
```

```bash
GRPC_INSECURE=true CATALOG_GRPC_ADDR=dns:///localhost:50051 HTTP_ADDR=127.0.0.1:8080 go run ./cmd/api
```

Request `/api/products/sku-001` or `/api/hello`. Both endpoints call catalog over RPC; hello returns a featured product. `sku-002` is also available, and unknown IDs return 404.

Optional direct gRPC inspection (grpcurl is not required to run the application):

```bash
grpcurl -plaintext -import-path api -proto catalog/v1/catalog.proto \
  -d '{"id":"sku-001"}' localhost:50051 catalog.v1.CatalogService/GetProduct
```

## Contracts and errors

| REST endpoint | Internal RPC | Result |
|---|---|---|
| GET /api/products/{id} | GetProduct(id) | Product JSON |
| GET /api/hello | GetProduct(sku-001) | Welcome + featured product |
| GET /readyz | grpc.health.v1.Health/Check | 200 when catalog is ready; 503 on failure/draining |
| GET /livez | No dependency call | Process liveness |
| GET /metrics | No dependency call | Prometheus metrics |

Readiness/metrics endpoints are not exposed through the public Ingress; only the `/api/` prefix is routed. Liveness does not depend on the backend, so dependency failures do not restart every API replica.

| gRPC status | HTTP |
|---|---|
| InvalidArgument | 400 |
| NotFound | 404 |
| Unavailable | 503 |
| DeadlineExceeded | 504 |
| Other internal errors | 502 |

RPCs have an explicit deadline and inherit cancellation from the REST context. There is no application retry loop; configured gRPC retry policies are disabled. The transport may still transparently retry RPCs that have not been processed. Internal error details are not sent to public clients. Sample products use integer cents rather than floating-point currency values.

## TLS and service discovery

Catalog requires `GRPC_TLS_CERT_FILE` and `GRPC_TLS_KEY_FILE`; API requires `GRPC_CA_FILE` and `GRPC_SERVER_NAME=catalog`. The server name is verified against the certificate SAN. Combining the insecure flag with TLS configuration is rejected.

In Kubernetes, the headless `catalog` Service returns Ready pod endpoints, and the `dns:///catalog:50051` client uses round_robin for HTTP/2 connections. The backend has no public Ingress/LoadBalancer. NetworkPolicy allows gRPC only from `api` pods in the same namespace, and metrics from the observability namespace.

This TLS setup authenticates the server. Add mTLS or service mesh/workload identity policies if caller identity and authorization are required. CA and leaf certificates are read at startup: roll both Deployments when rotating the CA, and catalog when rotating the leaf certificate. Automatic certificate reload is not implemented.

For local TLS practice, see [Kubernetes secret setup](../docs/SETUP.md). The development CA script creates seven-day certificates; it does not replace managed production PKI and rotation.

## Observability and shutdown

`api_http_requests_total` and REST histograms use route templates so product IDs do not become metric labels. Catalog exposes `catalog_grpc_requests_total{method,code}` and an RPC histogram. Both services write JSON logs with trace IDs; the Go API creates a new request ID and propagates it to RPC calls.

Set `OTEL_EXPORTER_OTLP_ENDPOINT=http://otel.observability.svc.cluster.local:4318` for tracing. REST → gRPC client → gRPC server spans share a trace ID. Default root trace sampling is 10%, configurable through `OTEL_TRACES_SAMPLER_ARG`; parent sampling decisions are respected.

The binary provides `/server drain` for Kubernetes preStop: it creates a readiness marker, then waits 10 seconds before SIGTERM. Request/RPC shutdown has a 25-second maximum, followed by up to 3 seconds of trace flushing, within a 40-second termination grace period. Stale markers are cleared on container restart because emptyDir can survive container restarts.

## Tests and code generation

```bash
cd app
go test -race ./...
go vet ./...
make proto
```

`make proto` requires protoc 25.1. Plugin versions are pinned in `tools.mk`. After contract changes, commit `.proto` and generated Go files together; preserve field numbers and version compatibility during staged service promotion.
