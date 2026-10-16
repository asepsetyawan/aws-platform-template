data "aws_caller_identity" "current" {}

resource "aws_ecr_repository" "app" {
  name                 = "${var.name}/api"
  image_tag_mutability = "IMMUTABLE"
  force_delete         = false
  image_scanning_configuration { scan_on_push = true }
  encryption_configuration { encryption_type = "AES256" }
}

resource "aws_s3_bucket" "telemetry" {
  for_each      = toset(["loki", "tempo", "audit"])
  bucket        = "${var.name}-${data.aws_caller_identity.current.account_id}-${each.key}"
  force_destroy = false
}
resource "aws_s3_bucket_public_access_block" "telemetry" {
  for_each                = aws_s3_bucket.telemetry
  bucket                  = each.value.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}
resource "aws_s3_bucket_versioning" "telemetry" {
  for_each = aws_s3_bucket.telemetry
  bucket   = each.value.id
  versioning_configuration { status = "Enabled" }
}
resource "aws_s3_bucket_server_side_encryption_configuration" "telemetry" {
  for_each = aws_s3_bucket.telemetry
  bucket   = each.value.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm     = "aws:kms"
      kms_master_key_id = aws_kms_key.platform.arn
    }
  }
}
resource "aws_s3_bucket_policy" "tls" {
  for_each = aws_s3_bucket.telemetry
  bucket   = each.value.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = concat([{
      Sid       = "DenyInsecureTransport", Effect = "Deny", Principal = "*", Action = "s3:*"
      Resource  = [each.value.arn, "${each.value.arn}/*"]
      Condition = { Bool = { "aws:SecureTransport" = "false" } }
      }], each.key == "audit" ? [
      { Effect = "Allow", Principal = { Service = "cloudtrail.amazonaws.com" }, Action = "s3:GetBucketAcl", Resource = each.value.arn,
      Condition = { StringEquals = { "aws:SourceArn" = "arn:aws:cloudtrail:${data.aws_region.current.region}:${data.aws_caller_identity.current.account_id}:trail/${var.name}-audit" } } },
      { Effect = "Allow", Principal = { Service = "cloudtrail.amazonaws.com" }, Action = "s3:PutObject", Resource = "${each.value.arn}/AWSLogs/${data.aws_caller_identity.current.account_id}/*",
      Condition = { StringEquals = { "s3:x-amz-acl" = "bucket-owner-full-control", "aws:SourceArn" = "arn:aws:cloudtrail:${data.aws_region.current.region}:${data.aws_caller_identity.current.account_id}:trail/${var.name}-audit" } } }
    ] : [])
  })
}

resource "aws_db_subnet_group" "app" {
  count      = var.enable_database ? 1 : 0
  name       = var.name
  subnet_ids = aws_subnet.private[*].id
}
resource "aws_security_group" "database" {
  count       = var.enable_database ? 1 : 0
  name_prefix = "${var.name}-db-"
  vpc_id      = aws_vpc.cluster.id
  ingress {
    description     = "PostgreSQL from EKS workers only; constrain further with security groups for pods if needed"
    protocol        = "tcp"
    from_port       = 5432
    to_port         = 5432
    security_groups = [aws_eks_cluster.this.vpc_config[0].cluster_security_group_id]
  }
}
resource "aws_db_parameter_group" "app" {
  count       = var.enable_database ? 1 : 0
  name_prefix = "${var.name}-"
  family      = "postgres${split(".", var.database_version)[0]}"
  parameter {
    name         = "rds.force_ssl"
    value        = "1"
    apply_method = "immediate"
  }
}
resource "aws_db_instance" "app" {
  count                           = var.enable_database ? 1 : 0
  identifier                      = "${var.name}-postgres"
  engine                          = "postgres"
  engine_version                  = var.database_version
  instance_class                  = "db.t4g.medium"
  allocated_storage               = 30
  max_allocated_storage           = 100
  storage_type                    = "gp3"
  kms_key_id                      = aws_kms_key.platform.arn
  storage_encrypted               = true
  db_name                         = "platform"
  username                        = "platformadmin"
  manage_master_user_password     = true
  multi_az                        = var.environment == "prod"
  publicly_accessible             = false
  db_subnet_group_name            = aws_db_subnet_group.app[0].name
  vpc_security_group_ids          = [aws_security_group.database[0].id]
  parameter_group_name            = aws_db_parameter_group.app[0].name
  backup_retention_period         = var.environment == "prod" ? 35 : 7
  backup_window                   = "18:00-19:00"
  maintenance_window              = "sun:20:00-sun:21:00"
  deletion_protection             = var.environment == "prod"
  skip_final_snapshot             = false
  final_snapshot_identifier       = "${var.name}-final"
  enabled_cloudwatch_logs_exports = ["postgresql", "upgrade"]
  copy_tags_to_snapshot           = true
}

data "aws_region" "current" {}
resource "aws_cloudtrail" "audit" {
  name                          = "${var.name}-audit"
  s3_bucket_name                = aws_s3_bucket.telemetry["audit"].id
  include_global_service_events = true
  is_multi_region_trail         = true
  kms_key_id                    = aws_kms_key.platform.arn
  enable_log_file_validation    = true
  depends_on                    = [aws_s3_bucket_policy.tls]
}
resource "aws_sns_topic" "operations" {
  name              = "${var.name}-operations"
  kms_master_key_id = aws_kms_key.operations.arn
}
resource "aws_cloudwatch_metric_alarm" "database_cpu" {
  count               = var.enable_database ? 1 : 0
  alarm_name          = "${var.name}-postgres-cpu"
  namespace           = "AWS/RDS"
  metric_name         = "CPUUtilization"
  dimensions          = { DBInstanceIdentifier = aws_db_instance.app[0].identifier }
  statistic           = "Average"
  period              = 300
  evaluation_periods  = 3
  comparison_operator = "GreaterThanThreshold"
  threshold           = 80
  treat_missing_data  = "missing"
  alarm_actions       = [aws_sns_topic.operations.arn]
}
resource "aws_acm_certificate" "origin" {
  domain_name       = var.origin_domain_name
  validation_method = "DNS"
  lifecycle { create_before_destroy = true }
}
resource "aws_route53_record" "origin_validation" {
  zone_id = var.hosted_zone_id
  name    = one(aws_acm_certificate.origin.domain_validation_options).resource_record_name
  type    = one(aws_acm_certificate.origin.domain_validation_options).resource_record_type
  records = [one(aws_acm_certificate.origin.domain_validation_options).resource_record_value]
  ttl     = 60
}
resource "aws_acm_certificate_validation" "origin" {
  certificate_arn         = aws_acm_certificate.origin.arn
  validation_record_fqdns = [aws_route53_record.origin_validation.fqdn]
}
data "aws_ec2_managed_prefix_list" "cloudfront" {
  name = "com.amazonaws.global.cloudfront.origin-facing"
}

resource "aws_kms_key" "operations" {
  description             = "Encrypt operation notifications; allow CloudWatch alarm publishing"
  enable_key_rotation     = true
  deletion_window_in_days = 30
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      { Effect = "Allow", Principal = { AWS = "arn:aws:iam::${data.aws_caller_identity.current.account_id}:root" }, Action = "kms:*", Resource = "*" },
      { Effect = "Allow", Principal = { Service = "cloudwatch.amazonaws.com" }, Action = ["kms:Decrypt", "kms:GenerateDataKey*"], Resource = "*", Condition = { StringEquals = { "aws:SourceAccount" = data.aws_caller_identity.current.account_id } } }
    ]
  })
}

resource "aws_kms_key" "platform" {
  description             = "Customer-managed encryption for platform data, EKS secrets and audit logs"
  enable_key_rotation     = true
  deletion_window_in_days = 30
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      { Effect = "Allow", Principal = { AWS = "arn:aws:iam::${data.aws_caller_identity.current.account_id}:root" }, Action = "kms:*", Resource = "*" },
      { Effect = "Allow", Principal = { Service = "cloudtrail.amazonaws.com" }, Action = ["kms:GenerateDataKey*", "kms:DescribeKey"], Resource = "*", Condition = { StringEquals = {
        "aws:SourceArn" = "arn:aws:cloudtrail:${data.aws_region.current.region}:${data.aws_caller_identity.current.account_id}:trail/${var.name}-audit"
      } } },
      { Effect = "Allow", Principal = { Service = "logs.${data.aws_region.current.region}.amazonaws.com" }, Action = ["kms:Encrypt*", "kms:Decrypt*", "kms:ReEncrypt*", "kms:GenerateDataKey*", "kms:Describe*"], Resource = "*", Condition = { ArnEquals = {
        "kms:EncryptionContext:aws:logs:arn" = "arn:aws:logs:${data.aws_region.current.region}:${data.aws_caller_identity.current.account_id}:log-group:/aws/eks/${var.name}/cluster"
      } } }
    ]
  })
}

resource "aws_ecr_repository" "catalog" {
  name                 = "${var.name}/catalog"
  image_tag_mutability = "IMMUTABLE"
  force_delete         = false
  image_scanning_configuration { scan_on_push = true }
  encryption_configuration { encryption_type = "AES256" }
}
