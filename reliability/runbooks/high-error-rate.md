# API high error rate / fast error-budget burn

Owner: service on-call. Start an incident when user impact is confirmed; use the team's severity policy.

1. Confirm namespace/environment, request volume, 5xx ratio, and time window. Check CloudFront/ALB and Kong response codes to distinguish edge failures from application failures.
2. Correlate deploy time and image digest with Grafana, Loki logs, and Tempo traces. Do not log credentials or customer payloads.
3. Inspect readiness/rollout for Kong, api and catalog, then gRPC status metrics, TLS certificate SAN/expiry, CA trust, Service DNS, NetworkPolicy, and node capacity with explicit kubectl context.
   Check Kong `kong_http_requests_total`, rate-limit 429s, config hash/revision, status readiness and upstream DNS/endpoints. Kong-generated errors may never reach the Go metrics. Inspect Status port 8100 health checks and ALB backend security-group rules if targets are unhealthy. Revert gateway configuration in Git when a policy/route change caused the incident.
4. If caused by a release, revert the image digest through a reviewed GitOps PR; Argo CD then reconciles the approved revision. A kubectl rollout undo alone will be overwritten by GitOps.
5. If dependency-related, inspect connection/timeouts/retries and throttle or shed load according to the service design. Do not repeatedly retry a failing database without bounds.
6. Confirm synthetic probes and error-budget burn recover. Record impact, timeline, corrective actions, and test gaps.

No automatic database restore or Terraform state rollback is implied by reverting application Git.
