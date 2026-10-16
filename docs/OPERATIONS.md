# Operations, updates, and decommissioning

## Releases

1. Source PR → CI tests/security checks → merge.
2. Build/scan/push both Go images (api/catalog) → record each service's digest and commit provenance.
3. Promote each service's digest through PRs: dev → verify → staging → verify → prod. Review Protobuf backward compatibility when promoting services separately.
4. Complete Argo CD sync, rollout, and synthetic checks before the next promotion.
5. Run Terraform changes in the target HCP workspace: speculative plan, review, deployment plan/policy, approval, apply, verification. Application promotion does not copy Terraform state or plans.

Roll back applications by reverting the digest through a GitOps PR. Database/schema migrations need their own compatibility and recovery strategies. Reverting Terraform code still requires a new plan; it does not restore lost data.

## Dependency updates

- Review image vulnerabilities; update dependency locks/base images, then rebuild and test.
- Upgrade charts with a new version, URL, and SHA256 in charts.lock.yaml; render them, review CRD migrations, and test in staging. Checksums establish downloaded file integrity, not content security.
- Pin compatible EKS/addon/node image/autoscaler minor versions. Review surge capacity, PDBs, and node draining before upgrades.
- The AWS node group spans AZs, but verify node distribution and spare capacity. One managed node group does not guarantee one node per AZ in every situation.
- Adjust telemetry retention, storage class/size, resource requests, and sampling to actual volumes. The single-replica Grafana/Tempo baseline needs additional work if observability HA is required.

## Drift and incidents

HCP health/drift checks depend on product features and subscription. Schedule and enable them where supported, or provide a trusted scheduled plan runner. Alert on drift and review before reconciliation. Argo CD handles GitOps workload drift. Creating this template did not create scheduled automation.

Use the runbooks in reliability/runbooks. Test real on-call notifications and ensure audit logs remain available during application or telemetry stack failures.

## Decommissioning

1. Review data retention, snapshots, domains, image digests, and dependencies.
2. Remove **edge** first so the CloudFront VPC origin no longer depends on the ALB.
3. Remove the Argo Application/workloads through your GitOps procedure, then verify that the controller's Ingress/ALB/target groups are deleted. The Application YAML has no cascade finalizer; deleting the Application alone does not automatically remove workloads. Use an explicit managed cleanup process.
4. Remove Helm workloads and inspect Service/PVC resources. The Retain storage class preserves EBS PVs; inventory them and delete only after deciding retention requirements.
5. For prod RDS, disable deletion protection through an approved change, choose a unique final snapshot name, then destroy the platform.
6. Versioned buckets and ECR repositories have force deletion disabled: archive and then empty objects/versions/images only when decommissioning is intended. Stop/delete CloudTrail before emptying the audit bucket so it does not keep writing; use a separate change if needed.
7. Review the target HCP destroy plan. Account for ENIs, ALBs, NAT/EIPs, retained EBS volumes, RDS snapshots, log groups, buckets, and registry resources. Do not remove state before cleanup is complete.
8. Terraform leaves resources intentionally retained by their lifecycle, such as snapshots or retained PVs. Document their owners and ongoing costs.

## gRPC TLS and dependency recovery

Monitor certificate expiry, API dependency readiness, gRPC status codes, and deadlines. Rotate leaf/CA through approved PKI/secret management then roll affected deployments; this sample does not hot-reload TLS files. To change CA without downtime, distribute a trust bundle containing old/new roots before rotating leaf certificates and remove the old root after rollout verification. A failed catalog should make API readiness fail while liveness remains healthy. Check TLS SAN/trust and NetworkPolicy before increasing RPC deadlines or opening ingress.
