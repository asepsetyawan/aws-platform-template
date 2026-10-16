# Operasi, update, dan decommission

## Release

1. PR source → CI tests/security → merge.
2. Build/scan/push dua image Go (api/catalog) → simpan digest dan commit provenance per service.
3. PR digest per service: dev → verify → staging → verify → prod. Review backward compatibility Protobuf saat service dipromosikan terpisah.
4. Argo CD sync, rollout dan synthetic check harus selesai sebelum promosi lanjut.
5. Perubahan Terraform berjalan di HCP workspace target: speculative plan, review, deployment plan/policy, approval, apply, verifikasi. Application promotion tidak menyalin state atau Terraform plan.

Revert digest melalui GitOps PR untuk rollback aplikasi. Database/schema migration harus mempunyai strategi kompatibilitas dan pemulihan sendiri. Revert Terraform code tetap membutuhkan plan baru; ia tidak mengembalikan data yang sudah hilang.

## Update dependencies

- Review image vulnerabilities; update dependency locks/base image lalu rebuild/test.
- Upgrade chart dengan version + URL + SHA256 baru dalam charts.lock.yaml; render, review CRD migration, dan test staging. Checksums membuktikan integritas file yang diunduh, bukan keamanan kontennya.
- Pin EKS/addon/node image/autoscaler minor compatibility. Review surge capacity, PDB, and node drain behavior sebelum upgrade.
- AWS node group spans AZs tetapi perlu diverifikasi distribusi node/headroom. Satu managed node group tidak menjamin satu node per AZ pada setiap keadaan.
- Sesuaikan telemetry retention, storage class/size, requests, dan sampling dengan volume nyata. Grafana/Tempo baseline single-replica memerlukan pengembangan jika observability HA adalah requirement.

## Drift dan incident

HCP health/drift checks bergantung fitur/paket; jadwalkan dan enable sesuai subscription, atau sediakan trusted scheduled plan runner. Alert pada drift, lalu review sebelum reconcile. Argo CD menangani workload GitOps drift. Tidak ada scheduled automation yang dibuat oleh template saat folder ini dibuat.

Gunakan runbooks di reliability/runbooks. Uji on-call notification secara nyata dan pastikan audit logs tersedia ketika aplikasi atau telemetry stack bermasalah.

## Decommission

1. Review data retention, snapshot, domain, image digests, dan dependencies.
2. Hapus **edge** terlebih dahulu agar CloudFront VPC origin tidak lagi bergantung pada ALB.
3. Hapus Argo Application/workloads melalui prosedur GitOps, lalu pastikan Ingress/ALB/target groups milik controller terhapus. Application YAML tidak memiliki cascade finalizer; penghapusan Application sendiri tidak otomatis menghapus workload. Gunakan managed cleanup yang eksplisit.
4. Hapus Helm workloads dan cek service/PVC resources. StorageClass Retain mempertahankan EBS PV; inventarisasi dan hapus hanya setelah keputusan retensi.
5. Prod RDS: disable deletion protection melalui perubahan yang disetujui, tentukan final snapshot name unik, lalu destroy platform.
6. Bucket versioned dan ECR force-delete disabled: arsipkan lalu kosongkan objects/versions/images jika benar-benar akan decommission. CloudTrail harus dihentikan/dihapus sebelum mengosongkan audit bucket agar tidak terus menulis; gunakan perubahan terpisah jika diperlukan.
7. Review HCP destroy plan target. Pastikan ENIs, ALBs, NAT/EIPs, EBS retained volumes, RDS snapshots, log groups, buckets, dan registry resources sesuai keputusan. Jangan hapus state sebelum cleanup selesai.
8. Terraform tetap meninggalkan resource yang lifecycle-nya memang dipertahankan, seperti snapshots atau retained PV. Dokumentasikan pemilik dan biaya lanjutan.


## gRPC TLS dan dependency recovery

Monitor certificate expiry, API dependency readiness, gRPC status codes, and deadlines. Rotate leaf/CA through approved PKI/secret management then roll affected deployments; this sample does not hot-reload TLS files. To change CA without downtime, distribute a trust bundle containing old/new roots before rotating leaf certificates and remove the old root after rollout verification. A failed catalog should make API readiness fail while liveness remains healthy. Check TLS SAN/trust and NetworkPolicy before increasing RPC deadlines or opening ingress.
