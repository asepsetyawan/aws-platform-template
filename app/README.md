# Go REST API → internal gRPC catalog

Dua executable dalam satu Go module, dengan kontrak Protobuf yang di-commit:

```text
Client → HTTPS edge/ALB → Kong :8000 → api (REST :8080)
                                        │ GetProduct (gRPC TLS, deadline 800 ms)
                                        ▼
                                     catalog (:50051, internal)
                                     admin/metrics (:9090, internal)
```

- `cmd/api`: REST gateway, JSON response, REST metrics, gRPC client dan dependency readiness.
- `cmd/catalog`: data produk contoh in-memory, gRPC server, gRPC health service, admin HTTP dan RPC metrics.
- `api/catalog/v1/catalog.proto`: kontrak `CatalogService.GetProduct`.
- `api/openapi.yaml`: kontrak REST.
- `gen/catalog/v1`: generated protobuf/client/server stubs; build tidak membutuhkan protoc.
- `internal/telemetry`: JSON logs, Prometheus, OTLP tracing dan W3C context propagation.
- `integration`: real gRPC/protobuf tests menggunakan in-memory transport, termasuk TLS handshake.

## Jalankan dengan Docker Compose

```bash
docker compose -f app/compose.yaml up --build
curl --fail http://localhost:8080/api/products/sku-001
curl --fail http://localhost:8080/api/hello
docker compose -f app/compose.yaml down
```

Contoh response produk:

```json
{"id":"sku-001","name":"Cloud Architecture Handbook","price_cents":2500,"currency":"USD"}
```

Hanya proxy Kong yang dipublish ke localhost:8080; port api/catalog tidak dipublish. Kong mempertahankan prefix `/api/`, menerapkan rate limit per replica, dan memblokir endpoint health/metrics dari public routing. Lihat [panduan Kong](../docs/KONG.md). gRPC catalog berjalan di network Compose dengan opt-in plaintext lokal `GRPC_INSECURE=true`. Manifest Kubernetes memakai TLS terverifikasi dan tidak mengaktifkan flag tersebut.

## Jalankan langsung dengan Go

Gunakan Go 1.27.2 sesuai `go.mod`. Mode native berikut menjalankan dua service langsung tanpa Kong, untuk debugging aplikasi. Untuk jalur gateway lengkap gunakan Compose di atas. Mulai dua terminal dari folder `app/`:

```bash
GRPC_INSECURE=true GRPC_ADDR=127.0.0.1:50051 HTTP_ADDR=127.0.0.1:9090 go run ./cmd/catalog
```

```bash
GRPC_INSECURE=true CATALOG_GRPC_ADDR=dns:///localhost:50051 HTTP_ADDR=127.0.0.1:8080 go run ./cmd/api
```

Buka `/api/products/sku-001` atau `/api/hello`. Kedua endpoint melakukan RPC ke catalog; hello mengembalikan featured product. `sku-002` juga tersedia, ID yang tidak ada menghasilkan 404.

Opsional inspeksi gRPC langsung (grpcurl tidak diperlukan untuk menjalankan aplikasi):

```bash
grpcurl -plaintext -import-path api -proto catalog/v1/catalog.proto \
  -d '{"id":"sku-001"}' localhost:50051 catalog.v1.CatalogService/GetProduct
```

## Kontrak dan error

| Public REST | Internal RPC | Hasil |
|---|---|---|
| GET /api/products/{id} | GetProduct(id) | Product JSON |
| GET /api/hello | GetProduct(sku-001) | Welcome + featured product |
| GET /readyz | grpc.health.v1.Health/Check | 200 jika catalog ready, 503 jika gagal/draining |
| GET /livez | Tidak memanggil dependency | Process liveness |
| GET /metrics | Tidak memanggil dependency | Prometheus metrics |

Readiness/metrics tidak diekspos melalui public Ingress; hanya prefix `/api/` dirutekan. Liveness tidak bergantung backend agar kegagalan dependency tidak menyebabkan restart seluruh gateway.

| gRPC status | HTTP |
|---|---|
| InvalidArgument | 400 |
| NotFound | 404 |
| Unavailable | 503 |
| DeadlineExceeded | 504 |
| Error internal lainnya | 502 |

RPC memakai deadline eksplisit dan cancellation dari REST context. Tidak ada application retry loop; configured gRPC retry policy dinonaktifkan. Transport masih dapat melakukan transparent retry untuk RPC yang belum diproses. Detail error internal tidak dikirim ke public client. Produk contoh memakai integer cents, bukan floating-point currency.

## TLS dan service discovery

Catalog memerlukan `GRPC_TLS_CERT_FILE` dan `GRPC_TLS_KEY_FILE`; API memerlukan `GRPC_CA_FILE` serta `GRPC_SERVER_NAME=catalog`. Server name diverifikasi terhadap SAN certificate. Kombinasi insecure flag dengan TLS ditolak.

Di Kubernetes, headless Service `catalog` mengembalikan endpoint pod yang Ready, dan client `dns:///catalog:50051` menggunakan round_robin untuk HTTP/2 connections. Backend tidak memiliki Ingress/LoadBalancer publik. NetworkPolicy hanya mengizinkan gRPC dari pod `api` pada namespace yang sama dan metrics dari namespace observability.

TLS ini memverifikasi identitas server. Jika membutuhkan identitas/authz per caller, tambahkan mTLS atau service mesh/workload identity policy. CA dan leaf certificate dibaca saat startup: rollout kedua Deployment saat rotasi CA, dan rollout catalog saat rotasi leaf. Jangan mengandalkan reload otomatis yang belum diimplementasikan.

Untuk latihan TLS lokal, lihat [setup secret Kubernetes](../docs/SETUP.md). Script CA pengembangan menghasilkan sertifikat tujuh hari, tidak menggantikan managed PKI/rotation untuk produksi.

## Observability dan shutdown

`api_http_requests_total` dan histogram REST memakai route template agar product IDs tidak menjadi metric labels. Catalog memakai `catalog_grpc_requests_total{method,code}` serta histogram RPC. Kedua service menulis JSON logs dengan trace ID; request ID baru dibuat gateway dan dipropagasikan ke RPC.

Set `OTEL_EXPORTER_OTLP_ENDPOINT=http://otel.observability.svc.cluster.local:4318` untuk tracing. Span REST → gRPC client → gRPC server menggunakan trace ID yang sama. Default root trace sampling 10%, dapat diubah melalui `OTEL_TRACES_SAMPLER_ARG`; parent sampling dihormati.

Binary menyediakan `/server drain` untuk Kubernetes preStop: marker readiness dibuat lalu menunggu 10 detik sebelum SIGTERM. Shutdown request/RPC mendapat maksimum 25 detik, lalu tracing di-flush hingga 3 detik, dalam termination grace 40 detik. Marker stale dibersihkan saat container restart karena emptyDir dapat bertahan antar restart.

## Tes dan generation

```bash
cd app
go test -race ./...
go vet ./...
make proto
```

`make proto` memerlukan protoc 25.1. Plugin versions dikunci di `tools.mk`. Setelah perubahan kontrak, commit `.proto` dan generated Go files bersama-sama; pertahankan field numbers dan kompatibilitas versi saat promosi service bertahap.
