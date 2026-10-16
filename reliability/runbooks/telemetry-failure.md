# Telemetry pipeline failure

1. Check whether the application is failing or only telemetry collection is failing using an independent synthetic probe.
2. Inspect ServiceMonitor discovery, Prometheus target health, collector dropped/export-failed counters, Alloy logs, and backend readiness.
3. Verify Pod Identity agent, service account names, S3 bucket policy, IAM, egress, DNS, disk usage, and retention configuration.
4. Inspect Alertmanager routing plus a real test notification; an active alert in Prometheus does not prove delivery.
5. Collector queues are memory-backed in this template: restarts may lose queued spans. Tempo is single replica; Prometheus storage is local per replica, without long-term deduplicating storage.
6. Escalate if visibility is lost during an incident. Use CloudWatch/EKS audit logs as an independent data source where appropriate.

For the Go services, check both ServiceMonitors: API HTTP metrics on service port `http`, catalog metrics on `metrics` (9090). gRPC port 50051 is not a Prometheus endpoint. A healthy distributed trace contains a REST server span, gRPC client span, and catalog server span sharing a trace ID. API and catalog logs carry the same generated request ID for product calls.
