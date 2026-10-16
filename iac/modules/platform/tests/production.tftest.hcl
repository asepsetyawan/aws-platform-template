mock_provider "aws" {
  mock_data "aws_availability_zones" { defaults = { names = ["ap-southeast-1a", "ap-southeast-1b", "ap-southeast-1c"] } }
  mock_data "aws_caller_identity" { defaults = { account_id = "123456789012" } }
  mock_data "aws_region" { defaults = { region = "ap-southeast-1" } }
  mock_resource "aws_iam_role" { defaults = { arn = "arn:aws:iam::123456789012:role/mock" } }
}
variables {
  name                = "aws-template-prod"
  environment         = "prod"
  network_cidr        = "10.60.0.0/16"
  kubernetes_version  = "1.35"
  admin_principal_arn = "arn:aws:iam::123456789012:role/PlatformAdmin"
  api_allowed_cidr    = "203.0.113.10/32"
  public_api_enabled  = false
  node_count          = 3
  max_node_count      = 12
  node_instance_type  = "m6i.large"
  enable_database     = true
  database_version    = "17.4"
  origin_domain_name  = "origin.prod.api.example.com"
  hosted_zone_id      = "Z123456789"
}
run "production_controls" {
  command = plan
  assert {
    condition     = !aws_eks_cluster.this.vpc_config[0].endpoint_public_access
    error_message = "Production API must stay private."
  }
  assert {
    condition     = length(aws_nat_gateway.cluster) == 3
    error_message = "One NAT per AZ is required."
  }
  assert {
    condition     = aws_db_instance.app[0].multi_az && aws_db_instance.app[0].storage_encrypted && aws_db_instance.app[0].deletion_protection && !aws_db_instance.app[0].publicly_accessible
    error_message = "Production database must be private, encrypted, Multi-AZ and deletion-protected."
  }
  assert {
    condition     = aws_launch_template.nodes.metadata_options[0].http_tokens == "required" && aws_launch_template.nodes.metadata_options[0].http_put_response_hop_limit == 1
    error_message = "Node instance metadata protection must be enabled."
  }
}
run "reject_session_arn" {
  command = plan
  variables { admin_principal_arn = "arn:aws:sts::123456789012:assumed-role/Admin/session" }
  expect_failures = [aws_eks_cluster.this]
}
