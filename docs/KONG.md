# Kong API Gateway

```text
Client → Route53 → CloudFront/WAF → internal ALB (HTTPS)
                                      ↓ HTTP :8000
                                    Kong
                                      ↓ HTTP :8080
                                    Go api
                                      ↓ gRPC/TLS :50051
                                    Go catalog
```

Kong OSS `kong:3.9.4` (multi-architecture manifest digest pinned) runs DB-less, without Postgres, Kong Ingress Controller, or a control-plane dependency. AWS Load Balancer Controller still owns the internal ALB. Argo CD owns Kong Deployment/Service/HPA/PDB/NetworkPolicy and a generated ConfigMap. The two business services remain Go `api` and `catalog`; Kong is a platform component. Kong does not translate REST to gRPC; Go api still performs that operation.

The raster AWS reference image predates Kong. The README Mermaid and flow above describe the current implementation.

## Configuration and delivery

- Kubernetes source: `gitops/apps/base/kong/kong.yaml`; Compose: `app/kong.yaml` (same routes, no local OTLP exporter).
- `/api/` routes to `http://api:8080`, with `strip_path: false`; health and metrics have no public route.
- Upstream connect timeout 1 s, read/write timeout 5 s, retries 0. Go keeps its 800 ms RPC deadline. There is no retry amplification at Kong.
- Admin API and Manager listeners are disabled. No administrative port is exposed.
- Rate limiting is deliberately service-wide: 600 requests/minute **per Kong replica**, using local counters. Scaling/restarts change the effective limit; it is not a shared quota or user-specific protection. WAF and this coarse limit are complementary. Configure authenticated consumers and Redis-backed quotas if the product needs a consistent global limit. Do not trust caller-supplied forwarding headers for client identity; trusted proxy/client-IP handling must be designed before adding IP-based policies.
- No user authentication/authorization plugin is enabled by this template. Select and configure your IdP/auth policy before serving protected data.
- A config change changes the Kustomize ConfigMap hash and rolls Kong. Existing/new policies coexist during the rollout. Avoid incompatible policy changes and retain backwards compatibility.
- Promote gateway changes through reviewed Git revisions from dev to staging/prod. Kong uses an upstream image, so the Go build/promotion workflows still select only api/catalog. Update Kong version and verified image digest through a separate reviewed manifest change, and mirror to ECR if your registry policy requires it. Version and multi-architecture digest are fixed in Compose, Kubernetes and CI. Registry manifest digest was verified on 2026-10-16. Review provenance and security updates before production.

## Reliability and transport

Dev HPA minimum 2, staging/prod minimum 3; maximum 6/12, CPU target 65%. PDB allows one unavailable pod, rolling updates allow zero unavailable, and topology spread separates AZs/nodes. Set capacity using load tests including gateway/plugin overhead.

Kong readiness uses `/status/ready`; liveness/startup use `/status`. ALB checks status port 8100, not the public proxy port. Ensure ALB backend security-group rules permit the health-check port. Kong readiness checks Kong configuration, not downstream catalog health; API readiness controls Service endpoints. Synthetic `/api/products/sku-001` checks are required to detect failures across the complete chain.

On termination, preStop waits 10 s for endpoint/ALB propagation. Nginx worker shutdown timeout is 30 s within a 60 s Kubernetes grace period. Verify real long-request draining with the AWS target deregistration settings before release.

TLS terminates at the ALB; ALB→Kong and Kong→api are HTTP inside the private cluster, protected by NetworkPolicy. api→catalog remains verified TLS. If policy requires encryption on every hop, add verified internal TLS or mesh mTLS and update the ALB backend protocol. This template does not claim encrypted HTTP hops.

## Observability and checks

Prometheus scrapes Kong Status API `/metrics` via `platform-kong` ServiceMonitor. Status-code, latency, and bandwidth metrics are enabled. Alloy collects Kong stdout/stderr; gateway access logs are Nginx text, while Go logs remain JSON. OpenTelemetry exports Kong spans to `otel.observability.svc.cluster.local:4318/v1/traces` with W3C propagation and 10% root sampling. Verify Kong→api→catalog parent relationships in Tempo after deployment; telemetry queues are not durable.

Useful queries: `sum(rate(kong_http_requests_total{code=~"5.."}[5m]))`, `sum(rate(kong_http_requests_total{code="429"}[5m]))`, and `histogram_quantile(0.95, sum by (le) (rate(kong_request_latency_ms_bucket[5m])))`. Kong latency metrics use milliseconds. Existing app alerts still cover Go responses; deploy the gateway alert rules alongside them to catch Kong-generated failures.

```bash
docker compose -f app/compose.yaml up --build --wait
python scripts/smoke_gateway.py
docker compose -f app/compose.yaml logs kong
docker compose -f app/compose.yaml down
```

CI parses both declarative files with the pinned Kong image and runs the Compose smoke test. It checks product retrieval through Kong and gRPC, a missing product, and rejection of public health/metrics. The configuration contract check renders all three environments. These checks do not substitute for AWS load/failover/auth tests.

## Official references

- [DB-less deployment](https://developer.konghq.com/gateway/db-less-mode/)
- [Kong OSS releases](https://github.com/Kong/kong/releases)
- [Health probes](https://developer.konghq.com/gateway/traffic-control/health-check-probes/)
- [Prometheus](https://developer.konghq.com/plugins/prometheus/)
- [OpenTelemetry](https://developer.konghq.com/plugins/opentelemetry/)
