# Identity and security configuration

Use separate AWS accounts for prod and non-prod; split dev/staging further when required. Manage organization guardrails, account provisioning, and central audit accounts outside application lifecycle.

## HCP Terraform

Create one VCS-backed workspace for each environment/layer listed in `platform/hcp-workspaces.yaml.example`. Configure exact workspace working directory, Terraform version, variables, team permissions, and approval policy. Trigger paths must include both the root and its shared module (`iac/modules/platform` or `iac/modules/edge`). Shared module changes require plans for all affected roots.

Use HCP AWS dynamic provider credentials. Configure the AWS OIDC provider and separate plan/apply roles with trust limited to exact HCP organization/project/workspace/run phase. Typical HCP environment variables:

```text
TFC_AWS_PROVIDER_AUTH=true
TFC_AWS_PLAN_ROLE_ARN=arn:aws:iam::ACCOUNT:role/TerraformPlan
TFC_AWS_APPLY_ROLE_ARN=arn:aws:iam::ACCOUNT:role/TerraformApply
```

HCP provider aliases need corresponding alias-tagged credentials configuration if you assign different credential sets; the regional and `global` AWS providers in edge use the same target account. Verify both region providers receive credentials. Do not store static AWS keys or application secrets in repository files.

The plan role must read the API resources needed to refresh/diff. The apply role requires the relevant EKS/VPC/EC2/IAM/ECR/RDS/ACM/Route53/CloudFront/WAF/S3/CloudTrail permissions. IAM creation is a privileged platform responsibility: constrain role names, pass-role targets, and permissions boundaries using organization-specific policies. Do not substitute AdministratorAccess as a production design.

## GitHub image publication

Set an AWS OIDC trust for the exact repository and `dev` environment (audience `sts.amazonaws.com`, subject `repo:ORG/REPO:environment:dev`). Limit the GitHub environment to main and approved operators. Grant publisher only `ecr:GetAuthorizationToken` plus upload/PutImage actions on both intended ECR repositories. It does not need EKS administrator permissions.

Configure GitHub environment variables `AWS_REGION`, `AWS_IMAGE_PUBLISH_ROLE_ARN`, `ECR_REPOSITORY_URL` (api), `ECR_CATALOG_REPOSITORY_URL`. Build workflow publishes only on a manual main-branch run and scans the exact local image before pushing. Immutable SHA tags mean rerunning publication for an existing commit can fail; reuse the existing digest after verification rather than replacing its tag.

Copy CODEOWNERS.example to CODEOWNERS with real teams. Enable protected main, required checks/reviews, restricted workflow changes, and environment approval rules supported by your GitHub plan. The promotion workflow creates a PR; it cannot itself prove staging test evidence. Require reviewers to verify provenance, scan result, and promotion history. Fork PR jobs have no AWS credentials.

When using one shared ECR repository across accounts, both node-role IAM permissions and a repository policy granting the chosen consumer accounts/roles are required. Terraform provides per-environment repositories by default; either copy the verified image into the target account and verify digest, or configure a deliberate shared-registry policy. The same SHA256 manifest is promoted; do not rebuild from source for each environment.

## Kubernetes and edge

Prod EKS API is private. Operators/bootstrap/Argo CD run with VPC connectivity (VPN, peering, or an approved private runner). Argo CD only needs cluster access from inside its own cluster. HCP Terraform manages AWS APIs and does not install Kubernetes workloads.

ALB is internal, its HTTPS certificate is regional ACM, and CloudFront uses a VPC origin. Ingress permits the CloudFront origin-facing managed prefix list; verify regional VPC-origin support and security group quotas. No public origin bypass is needed. CloudFront viewer ACM certificate and WAF use us-east-1. DNS hosted zone must be in the target account and you must own the domain.

Kong NetworkPolicy permits proxy traffic on 8000 and ALB status health checks on 8100 from 10.0.0.0/8; narrow this CIDR to the actual environment VPC. Status/metrics are also accessible from observability, never through the public route. API NetworkPolicy permits HTTP only from same-namespace Kong pods and observability, and egress to DNS/OTLP plus catalog gRPC port 50051. Catalog permits gRPC only from same-namespace API pods, and admin/metrics port 9090 only from observability. Catalog uses a headless internal Service and has no public Ingress. Narrow CIDRs for your network. AWS VPC CNI network policy is enabled; confirm supported node kernel/addon versions and actual enforcement before relying on it. Add explicit dependency egress rules when extending the application.

Node IMDS requires tokens and hop limit 1. Pod Identity roles are scoped per service account. VPC CNI permission currently remains on the node role for bootstrap; move it to a dedicated workload identity after validating addon lifecycle. Sample API gets no cloud permissions or database master credential.

ACR/AKS/Azure are not part of this template.

## Scanner exception

Trivy rule AVD-AWS-0040 is waived only at the shared EKS resource until 2026-12-31 because dev/staging deliberately permit a restricted `/32` public API. Prod root is private-only and has a regression assertion in its mock plan; the module tests also check this control. Reassess/renew the exception deliberately or move all environments to private API. It is not a waiver for production public access. Customer-managed KMS encryption is configured for EKS secrets, S3 telemetry/audit, CloudTrail, and platform data; key permissions require runtime verification during bootstrap.

## Promotion bot and required checks

Install a GitHub App scoped to this repository with Contents and Pull requests write permissions. Set `PROMOTION_APP_ID` and `PROMOTION_APP_PRIVATE_KEY` in the protected GitHub environment/repository configuration. The workflow mints a short-lived installation token for branch/PR creation. This is deliberate: PRs created with the default GITHUB_TOKEN do not normally trigger subsequent CI workflows, which can leave required checks pending. Keep the App token scoped and prevent untrusted PR jobs from accessing its private key.


## Internal gRPC transport

Kubernetes mounts server TLS cert/key only in catalog and trusted CA only in API. API verifies certificate hostname `catalog`; there is no insecure fallback. Plaintext opt-in exists only in the local development examples. This is server-authenticated TLS, not mTLS: NetworkPolicy is the current caller boundary. Use managed PKI, rotation and workload identity/mTLS when service-level caller authentication is required. TLS secrets are operator-managed, not committed; rollout is required after certificate/CA rotation.

Public REST is routed through Kong to API only, and `/metrics`/health are omitted from public Ingress. Kong Admin API and Manager are disabled. The baseline rate limit is 600 requests/minute per service per Kong replica, not a shared user quota. Authentication, authorization and rate limits specific to a real product must be added before handling customer data. The catalog's sample in-memory products are not production persistence.
