# Setup: from a repository to an AWS endpoint

Run all commands from the repository root. Install the tools listed in the README. AWS resources, NAT gateways, EKS, RDS, telemetry storage, and traffic incur charges.

## 1. Organization and HCP configuration

Use a separate production account; dev/staging can initially share a non-production account with separate state and clusters. Prepare a public Route53 hosted zone owned by the target account. Centralized domains/zones across accounts require changes to provider aliases and DNS trust; the default template manages DNS in the workload account.

Create six VCS-backed HCP workspaces using `platform/hcp-workspaces.yaml.example`, with the exact working directories, Terraform 1.13.4, auto-apply disabled, and appropriate approval/team access. Configure AWS dynamic credentials as described in [SECURITY.md](SECURITY.md). Set all Terraform inputs as HCP Terraform variables. `terraform.tfvars.example` documents inputs; it does not contain credentials.

For CLI-driven execution during organization bootstrap:

```bash
export TF_CLOUD_ORGANIZATION=YOUR_HCP_ORG
terraform login
terraform -chdir=iac/environments/dev/platform init
```

For VCS-backed workspaces, use PR/speculative plans and confirm applies in the HCP UI. Avoid multiple orchestrators independently auto-applying the same configuration. Copy example tfvars only for an appropriate CLI/local-variable workflow; remote VCS runs cannot read ignored files that exist only on your laptop.

Select an EKS version supported in the target region:

```bash
aws eks describe-cluster-versions --region ap-southeast-1
aws rds describe-db-engine-versions --engine postgres --region ap-southeast-1 --query 'DBEngineVersions[].EngineVersion'
aws eks describe-addon-versions --kubernetes-version YOUR_EKS_MINOR --region ap-southeast-1
```

Pin addon versions and match the cluster-autoscaler image minor version to EKS. Tests use fictional/representative versions for schema evaluation; they are not deployment version recommendations.

Populate the platform root `account_id`, region, domain `origin.dev.api.yourdomain.com`, hosted zone ID, IAM admin role ARN, API `/32` allowlist, EKS/Postgres versions, and addon map. The HCP plan must contain only resources for the target environment. The HCP `platform` output includes the cluster, VPC, `ecr_repository_url` (api), `catalog_ecr_repository_url`, buckets, certificate, and operations SNS topic.

## 2. Cluster access and platform bootstrap

Log in to AWS as a principal configured in an EKS access entry, then run:

```bash
aws eks update-kubeconfig --region ap-southeast-1 --name aws-template-dev --alias aws-template-dev
kubectl --context aws-template-dev get nodes
```

Populate `platform/argocd.yaml` with the Argo CD URL, OIDC/Dex configuration, and platform group RBAC. Store the OIDC client secret in `argocd-secret` through your organization's secret management process. Local administrator access is disabled; the installer rejects the default configuration without SSO. Argo remains a ClusterIP Service: use port-forwarding/VPN initially and design a private ingress for normal operations.

```bash
bash scripts/bootstrap-platform.sh aws-template-dev aws-template-dev ap-southeast-1 vpc-YOUR_ID vYOUR_EKS_MINOR.PATCH
```

Chart versions and checksums are recorded in `platform/charts.lock.yaml`. The installer downloads verified archives to `.generated/charts/`. Terraform provisions EBS CSI and the Pod Identity agent before Helm installation. Cluster Autoscaler uses the prepared role/service account and ASG discovery tags.

## 3. Observability and alert receiver

Create the namespace and credential secrets using secure local files. Do not commit passwords or webhooks:

```bash
kubectl --context aws-template-dev create namespace observability
kubectl --context aws-template-dev -n observability create secret generic grafana-admin \
  --from-literal=admin-user=admin --from-file=admin-password=/secure/path/grafana-password
kubectl --context aws-template-dev -n observability create secret generic oncall-webhook \
  --from-file=url=/secure/path/alertmanager-compatible-webhook-url
kubectl --context aws-template-dev -n observability create secret generic platform-alertmanager \
  --from-file=alertmanager.yaml=observability/alertmanager.yaml.example
```

The example receiver requires an endpoint that accepts Alertmanager webhook payloads; a Slack incoming-webhook URL is not directly compatible. Change the receiver configuration for PagerDuty/Slack or other integrations. Ensure the URL file has no unintended newline.

```bash
python scripts/install-observability.py --context aws-template-dev --region ap-southeast-1 \
  --loki-bucket YOUR_LOKI_BUCKET --tempo-bucket YOUR_TEMPO_BUCKET
kubectl --context aws-template-dev -n observability get pods
kubectl --context aws-template-dev -n observability port-forward svc/monitoring-grafana 3000:80
```

Verify Prometheus targets for Kong, API, and catalog; Go JSON logs in Loki; Kong → REST → gRPC traces in Tempo; the CloudWatch datasource; and a real test notification. The RDS operations SNS topic needs a separate on-call subscription/integration and endpoint confirmation; creating a topic does not prove alert delivery.

## 4. Image builds and GitOps

Configure GitHub environment variables and the AWS OIDC image publisher role as described in SECURITY.md. Run **Build and publish Go services** on main. The CI matrix tests Go, builds one amd64 image per service (`api` and `catalog`), scans it, and pushes each image to ECR with an immutable commit-SHA tag. Set `ECR_REPOSITORY_URL` for api and `ECR_CATALOG_REPOSITORY_URL` for catalog.

Use the resulting digest with **Propose image promotion** (or `python scripts/promote.py dev api ECR_API_REPOSITORY sha256:DIGEST` or `python scripts/promote.py dev catalog ECR_CATALOG_REPOSITORY sha256:DIGEST`). Select `service=api` or `catalog`; the script changes only that service's image in one overlay. The workflow opens a PR. Repeat for the other service and maintain Protobuf compatibility during staged rollouts. Review and merge. For cross-account pulls, configure an ECR repository policy or copy the same manifest/image to the destination repository and verify its digest.

Edit the environment overlay:

- `kustomization.yaml`: actual repositories and tested `api`/`catalog` digests.
- `ingress.yaml`: origin domain, regional ACM certificate ARN, and CloudFront origin-facing managed prefix list ID from platform outputs.
- `gitops/argocd/ENV/{project,application}.yaml`: actual repository URL.

Review [Kong configuration](KONG.md), image version/digest, resource sizing, and limits per replica. The Argo AppProject permits ConfigMaps; Kustomize adds a hash to the configuration name so route/plugin changes trigger a rolling update.

Before Argo sync, create the application namespace and provision two TLS secrets. For **dev practice only**, the following script creates a local CA and seven-day certificate:

```bash
kubectl --context aws-template-dev apply -f gitops/apps/overlays/dev/namespace.yaml
bash scripts/create-grpc-dev-certs.sh dev
kubectl --context aws-template-dev -n platform-dev create secret tls catalog-server-tls \
  --cert=.generated/tls/dev/tls.crt --key=.generated/tls/dev/tls.key
kubectl --context aws-template-dev -n platform-dev create secret generic catalog-client-ca \
  --from-file=ca.crt=.generated/tls/dev/ca.crt
```

Production requires organization-approved PKI/cert-manager/secret management, SAN `catalog` (or a consistent change to `GRPC_SERVER_NAME`), and managed rotation. Kubernetes does not enable `GRPC_INSECURE`. Only API mounts the CA; only catalog mounts the private server key. Operators/platform tooling manage secrets outside this Argo Application. Roll catalog after leaf certificate rotation and both services after CA rotation because TLS files are read at startup.

```bash
python scripts/preflight.py dev
kubectl --context aws-template-dev apply -f gitops/argocd/dev/project.yaml
kubectl --context aws-template-dev apply -f gitops/argocd/dev/application.yaml
kubectl --context aws-template-dev -n platform-dev rollout status deployment/catalog --timeout=180s
kubectl --context aws-template-dev -n platform-dev rollout status deployment/api --timeout=180s
kubectl --context aws-template-dev -n platform-dev rollout status deployment/kong --timeout=180s
kubectl --context aws-template-dev -n platform-dev get ingress api
```

Argo CD needs read-only repository access for private repositories, configured through a secret. Deployments omit replica counts so HPA owns scaling. Merged overlay PRs sync automatically; production application approval is enforced through the organization's protected Git review/environment workflow, rather than HCP's infrastructure apply approval.

## 5. Edge configuration and verification

Once the controller creates the internal ALB, retrieve its ARN/DNS/hosted-zone ID using `aws elbv2 describe-load-balancers` and populate the edge root. Set the public domain and an origin domain matching the regional certificate. CloudFront VPC origins require regional support and a VPC with an internet gateway; the template provides an IGW and private nodes. Review prefix list/security group limits and VPC origin status.

After plan review, apply the edge workspace, then run:

```bash
curl --fail https://dev.api.YOUR_DOMAIN/api/products/sku-001
curl --fail https://dev.api.YOUR_DOMAIN/api/hello
```

Health and `/metrics` endpoints are not exposed through the Ingress because only `/api/` is routed. Add a synthetic probe using blackbox exporter or external monitoring; `observability/synthetic-probe.yaml.example` is an example and is not enabled by the installer.

## 6. Staging and production

Use the same code revision with the target variables/workspace, certificate/domain, credentials, and a distinct kubectl context alias. Create a new plan for each environment/layer. Prod has a private-only API, Multi-AZ RDS, and deletion protection. Bootstrap operators require private network access first.

Run capacity/load tests, zone failure drills, alert delivery tests, and restore tests before accepting production traffic. Set agreed SLO/RTO/RPO objectives; repository SLO values are examples.

For promotion PRs, configure the short-lived GitHub App token described in SECURITY.md (`PROMOTION_APP_ID`, `PROMOTION_APP_PRIVATE_KEY`). The template does not create the App or change repository protections automatically.
