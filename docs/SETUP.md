# Setup dari repository kosong ke endpoint AWS

Semua command dimulai dari root repository. Siapkan tools yang disebut di README. AWS resource, NAT gateway, EKS, RDS, telemetry storage, dan traffic berbiaya.

## 1. Konfigurasi organisasi dan HCP

Gunakan akun prod terpisah; dev/staging dapat dimulai di akun non-prod dengan state dan cluster berbeda. Siapkan public Route53 hosted zone yang dimiliki akun target. Untuk domain/zone terpusat lintas akun, provider alias dan trust DNS harus disesuaikan; template default mengelola DNS dalam akun workload.

Buat enam VCS-backed HCP workspaces sesuai `platform/hcp-workspaces.yaml.example`, working directory persis, Terraform 1.13.4, auto-apply disabled, approval/team access sesuai risiko. Atur AWS dynamic credentials seperti [SECURITY.md](SECURITY.md). Semua input Terraform diisi sebagai HCP Terraform variables. `terraform.tfvars.example` adalah referensi input, bukan credentials.

Untuk CLI-driven execution saat bootstrap organisasi:

```bash
export TF_CLOUD_ORGANIZATION=YOUR_HCP_ORG
terraform login
terraform -chdir=iac/environments/dev/platform init
```

Untuk VCS-backed workspace, gunakan PR/speculative plan dan confirm apply di HCP UI. Jangan mencampur orchestrator yang sama-sama auto-apply. Copy contoh tfvars hanya jika menggunakan CLI/local variable workflow yang sesuai; remote VCS runs tidak membaca file ignored yang hanya ada di laptop.

Isi EKS version dari versi yang didukung region:

```bash
aws eks describe-cluster-versions --region ap-southeast-1
aws rds describe-db-engine-versions --engine postgres --region ap-southeast-1 --query 'DBEngineVersions[].EngineVersion'
aws eks describe-addon-versions --kubernetes-version YOUR_EKS_MINOR --region ap-southeast-1
```

Pin addon versions dan cocokkan cluster-autoscaler image minor dengan EKS. Contoh tests memakai versi fiktif/representatif untuk evaluasi schema; bukan rekomendasi versi deploy.

Isi root platform `account_id`, region, domain `origin.dev.api.yourdomain.com`, hosted zone ID, IAM admin role ARN, API allowlist `/32`, EKS/Postgres version, dan addon map. HCP plan harus memuat hanya resource environment target. HCP output `platform` memuat cluster, VPC, `ecr_repository_url` (api), `catalog_ecr_repository_url`, buckets, certificate, dan operations SNS topic.

## 2. Akses cluster dan bootstrap platform

Login AWS sebagai principal yang ada di EKS access entry, lalu:

```bash
aws eks update-kubeconfig --region ap-southeast-1 --name aws-template-dev --alias aws-template-dev
kubectl --context aws-template-dev get nodes
```

Isi `platform/argocd.yaml` dengan URL Argo CD, OIDC/Dex config, dan RBAC untuk group platform. Secret OIDC client diletakkan pada `argocd-secret` lewat prosedur secret management organisasi. Admin lokal dinonaktifkan; installer menolak config default tanpa SSO. Service Argo tetap ClusterIP: akses awal dengan port-forward/VPN; rancang private ingress untuk operasi normal.

```bash
bash scripts/bootstrap-platform.sh aws-template-dev aws-template-dev ap-southeast-1 vpc-YOUR_ID vYOUR_EKS_MINOR.PATCH
```

Chart versions dan checksum ada di `platform/charts.lock.yaml`. Installer mengunduh file terverifikasi ke `.generated/charts/`. Daemon EBS CSI dan Pod Identity agent dibuat Terraform sebelum Helm. Cluster Autoscaler memakai role/service account yang telah disiapkan serta tag ASG discovery.

## 3. Observability dan alert receiver

Buat namespace dan credential secrets menggunakan file lokal aman. Jangan commit password/webhook:

```bash
kubectl --context aws-template-dev create namespace observability
kubectl --context aws-template-dev -n observability create secret generic grafana-admin \
  --from-literal=admin-user=admin --from-file=admin-password=/secure/path/grafana-password
kubectl --context aws-template-dev -n observability create secret generic oncall-webhook \
  --from-file=url=/secure/path/alertmanager-compatible-webhook-url
kubectl --context aws-template-dev -n observability create secret generic platform-alertmanager \
  --from-file=alertmanager.yaml=observability/alertmanager.yaml.example
```

Contoh receiver membutuhkan endpoint yang menerima payload webhook Alertmanager; URL Slack incoming-webhook tidak langsung kompatibel. Ganti konfigurasi receiver jika menggunakan PagerDuty/Slack atau integrasi lainnya. File URL jangan mengandung newline tak diinginkan.

```bash
python scripts/install-observability.py --context aws-template-dev --region ap-southeast-1 \
  --loki-bucket YOUR_LOKI_BUCKET --tempo-bucket YOUR_TEMPO_BUCKET
kubectl --context aws-template-dev -n observability get pods
kubectl --context aws-template-dev -n observability port-forward svc/monitoring-grafana 3000:80
```

Verifikasi Prometheus targets Kong, API dan catalog, JSON logs di Loki, trace Kong → REST → gRPC di Tempo, CloudWatch datasource, dan real test notification. SNS topic operasi RDS perlu subscription/integrasi on-call tersendiri dan konfirmasi endpoint; pembuatan topic tidak berarti alert sudah diterima.

## 4. Build image dan GitOps

Siapkan GitHub environment variables dan AWS OIDC image publisher role sesuai SECURITY.md. Jalankan **Build and publish Go services** pada main. CI matrix menguji Go, membangun satu image amd64 per service (`api` dan `catalog`), memindainya, lalu push masing-masing image ke ECR dengan immutable commit-SHA tag. Atur `ECR_REPOSITORY_URL` untuk api dan `ECR_CATALOG_REPOSITORY_URL` untuk catalog.

Gunakan digest hasil build untuk workflow **Propose image promotion** (atau `python scripts/promote.py dev api ECR_API_REPOSITORY sha256:DIGEST` atau `python scripts/promote.py dev catalog ECR_CATALOG_REPOSITORY sha256:DIGEST`). Pilih input `service=api` atau `catalog`; script hanya mengganti image service itu pada satu overlay. Workflow membuka PR. Ulangi untuk image service lainnya dan pertahankan kompatibilitas Protobuf selama rollout bertahap. Review dan merge. Untuk pull lintas akun, siapkan ECR repository policy atau copy manifest/image yang sama ke repository tujuan dan verifikasi digest.

Edit overlay environment:

- `kustomization.yaml`: dua repository nyata dan digest `api`/`catalog` yang sudah diuji.
- `ingress.yaml`: origin domain, regional ACM certificate ARN, CloudFront origin-facing managed prefix list ID dari output platform.
- `gitops/argocd/ENV/{project,application}.yaml`: URL repository nyata.

Review [konfigurasi Kong](KONG.md), image version/digest, resource sizing, dan limit per replica. Argo AppProject mengizinkan ConfigMap; Kustomize memberi hash nama konfigurasi agar perubahan route/plugin memicu rolling update.

Sebelum Argo sync, buat namespace aplikasi dan sediakan dua TLS secrets. Untuk latihan **dev saja**, script berikut membuat CA lokal dan certificate tujuh hari:

```bash
kubectl --context aws-template-dev apply -f gitops/apps/overlays/dev/namespace.yaml
bash scripts/create-grpc-dev-certs.sh dev
kubectl --context aws-template-dev -n platform-dev create secret tls catalog-server-tls \
  --cert=.generated/tls/dev/tls.crt --key=.generated/tls/dev/tls.key
kubectl --context aws-template-dev -n platform-dev create secret generic catalog-client-ca \
  --from-file=ca.crt=.generated/tls/dev/ca.crt
```

Production memakai PKI/cert-manager/secret management yang disetujui organisasi, dengan SAN `catalog` (atau ubah `GRPC_SERVER_NAME` konsisten) dan rotasi terkelola. Kubernetes tidak mengaktifkan `GRPC_INSECURE`. CA hanya dimount API; private server key hanya dimount catalog. Secrets dikelola operator/platform di luar Application Argo ini. Lakukan rollout catalog setelah rotasi leaf certificate dan rollout kedua service saat rotasi CA karena file TLS dibaca saat startup.

```bash
python scripts/preflight.py dev
kubectl --context aws-template-dev apply -f gitops/argocd/dev/project.yaml
kubectl --context aws-template-dev apply -f gitops/argocd/dev/application.yaml
kubectl --context aws-template-dev -n platform-dev rollout status deployment/catalog --timeout=180s
kubectl --context aws-template-dev -n platform-dev rollout status deployment/api --timeout=180s
kubectl --context aws-template-dev -n platform-dev rollout status deployment/kong --timeout=180s
kubectl --context aws-template-dev -n platform-dev get ingress api
```

Argo CD harus punya repo access read-only untuk private repo, dikonfigurasi lewat secret. Deployment tidak menyatakan replicas agar HPA mengelola scale. Merge PR ke overlay akan di-sync otomatis; approval produksi ada pada protected Git review/environment workflow yang dikonfigurasi organisasi, bukan pada tombol apply HCP untuk aplikasi.

## 5. Edge dan verifikasi

Setelah controller menghasilkan internal ALB, ambil ARN/DNS/hosted-zone ID melalui `aws elbv2 describe-load-balancers` dan isi root edge. Isi public domain serta origin domain yang cocok regional certificate. CloudFront VPC origin membutuhkan dukungan region dan VPC dengan internet gateway; template sudah menyediakan IGW dan private nodes. Review limit/security groups prefix list dan status VPC origin.

Apply workspace edge setelah plan/review, kemudian:

```bash
curl --fail https://dev.api.YOUR_DOMAIN/api/products/sku-001
curl --fail https://dev.api.YOUR_DOMAIN/api/hello
```

Endpoint `/metrics` dan health tidak diekspos Ingress karena routing hanya `/api/`. Tambahkan synthetic probe memakai blackbox exporter atau layanan monitoring eksternal; `observability/synthetic-probe.yaml.example` adalah contoh yang belum diaktifkan installer.

## 6. Staging/prod

Gunakan revisi kode yang sama, variables/workspace target, certificate/domain, credentials, serta alias kubectl context berbeda. Buat plan baru per environment/layer. Prod memiliki private-only API, Multi-AZ RDS, dan deletion protection. Operator bootstrap membutuhkan akses jaringan privat terlebih dahulu.

Jalankan capacity/load test, zone failure drill, alert delivery test, dan restore test sebelum menerima production traffic. Isi SLO/RTO/RPO yang disepakati; angka SLO dalam repo adalah contoh.

For promotion PRs, configure the short-lived GitHub App token described in SECURITY.md (`PROMOTION_APP_ID`, `PROMOTION_APP_PRIVATE_KEY`). The template does not create the App or change repository protections automatically.
