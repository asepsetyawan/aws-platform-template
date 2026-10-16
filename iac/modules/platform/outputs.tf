output "cluster_name" { value = aws_eks_cluster.this.name }
output "vpc_id" { value = aws_vpc.cluster.id }
output "ecr_repository_url" { value = aws_ecr_repository.app.repository_url }
output "telemetry_buckets" { value = { for k, v in aws_s3_bucket.telemetry : k => v.id } }
output "database_endpoint" { value = try(aws_db_instance.app[0].address, null) }
output "database_admin_secret_arn" {
  value       = try(aws_db_instance.app[0].master_user_secret[0].secret_arn, null)
  description = "Admin secret for migration/bootstrap only; do not grant application this credential."
}

output "origin_certificate_arn" { value = aws_acm_certificate_validation.origin.certificate_arn }
output "cloudfront_prefix_list_id" { value = data.aws_ec2_managed_prefix_list.cloudfront.id }
output "operations_topic_arn" { value = aws_sns_topic.operations.arn }

output "public_api_enabled" { value = aws_eks_cluster.this.vpc_config[0].endpoint_public_access }

output "catalog_ecr_repository_url" { value = aws_ecr_repository.catalog.repository_url }
