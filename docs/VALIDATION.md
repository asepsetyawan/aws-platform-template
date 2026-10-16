# Validation record

Updated locally on 2026-10-16 after adding Kong to the two Go services. Existing Go/Terraform results below are retained from the application migration; those sources were unchanged. No AWS/HCP/GitHub deployment was performed.

| Check | Result |
|---|---|
| Go integration/unit tests with race detector | 10 top-level tests + RPC error subcases passed |
| REST → real Protobuf/gRPC call, deadline/error mapping, dependency readiness | Passed |
| gRPC TLS handshake and rejection of wrong certificate hostname | Passed |
| REST → gRPC client → gRPC server trace context/parent propagation | Passed |
| Native two-process smoke with TLS | Product 200/404, backend metrics, dependency failure 503, liveness 200 passed |
| Linux amd64 static builds for api/catalog (CGO disabled) | Both compiled successfully |
| go vet, gofmt, go.mod/go.sum consistency | Passed |
| Per-service/per-environment image promotion helper | 2 unit tests passed |
| Terraform environment/layer mock plans and invalid-account inputs | 12 runs passed; affected platform roots rechecked after adding catalog ECR |
| Terraform production security invariants and rejected STS session admin ARN | 2 runs passed |
| Kustomize Kong + two-service routing/TLS/NetworkPolicy/HPA/PDB/config-rollout contracts | dev/staging/prod passed |
| Prometheus rule syntax | 9 rules valid |
| API SLO burn, catalog failures and Kong proxy/metrics failures | 4 scenarios passed |
| Trivy IaC HIGH/CRITICAL checks | No unwaived findings; existing documented non-prod API exception |
| Terraform formatting and shell syntax | Passed |

Platform chart values remain unchanged: the 9 pinned Argo CD/controller/autoscaler/metrics/monitoring/Loki/Tempo/OTel/Alloy charts were previously rendered successfully. They still require real cluster verification.

Tools: Go 1.27.2, grpc-go 1.84.0, Protobuf Go 1.36.12, OpenTelemetry SDK 1.47.0, protoc 25.1, protoc-gen-go-grpc 1.6.2; Terraform 1.13.4, AWS provider 6.68.0, Helm 3.19.0, promtool 3.5.0, Trivy 0.76.0. Go dependency checksums, plugin versions, chart checksums and action commits are recorded in source lock files. Python/PyYAML remains only for platform helper scripts.

Kong addition checks: Compose configuration parses, all three Kustomize overlays and gateway isolation/config hash contracts pass. The Docker Registry multi-architecture manifest for Kong 3.9.4 was verified and its digest pinned in Compose, Deployment and CI. Kong declarative schema parsing and full Compose gateway smoke are configured in CI but **were not executed locally**, because Docker daemon is unavailable. Runtime auth/rate-limit/load/draining/network enforcement still require verification.

## What remains environment-dependent

- Docker build and Compose execution: Docker daemon is unavailable locally. Native/static Go builds and real two-process TLS communication were verified instead.
- Real AWS API plan/apply, IAM/KMS/quotas, regional versions, certificates/DNS, Pod Identity and controllers.
- HCP organization/VCS/dynamic credentials and approval policies.
- GitHub Actions execution, GitHub App, ECR push/cross-account pulls, promotion PR and repository/environment protections.
- Kubernetes admission, TLS secret provisioning/rotation, DNS load balancing across multiple catalog pods, NetworkPolicy enforcement, autoscaling and placement.
- Live metrics/log/trace delivery to backends, actual on-call notifications, capacity/zone-failure/backup/restore/regional-DR exercises.

Mock plans and rendered manifests are configuration checks, not evidence of deployed AWS availability. Trivy root inputs include placeholders/unknowns; rescan personalized plans in your environment. Application container vulnerability scanning runs in CI after the image is built.
