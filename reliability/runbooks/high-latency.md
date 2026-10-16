# API latency

1. Confirm p95/p99 and traffic; compare edge latency against API request histogram.
2. Check CPU throttling, memory, pending pods, HPA current/desired replicas, node autoscaler logs and ASG max capacity.
3. Inspect the REST → gRPC client → catalog server trace, catalog latency histogram, API GRPC_TIMEOUT (default 800 ms), and backend CPU/queueing. The sample catalog is in-memory; a database dependency requires explicit metrics/readiness/migrations when introduced.
4. Adjust capacity through reviewed configuration; verify AWS quotas and subnet IP capacity. HPA cannot help if node capacity or dependency throughput is exhausted.
5. Verify load-test results before changing thresholds; do not hide an incident by weakening the SLO.
