mock_provider "aws" {
  mock_data "aws_availability_zones" { defaults = { names = ["ap-southeast-1a", "ap-southeast-1b", "ap-southeast-1c"] } }
  mock_data "aws_caller_identity" { defaults = { account_id = "123456789012" } }
  mock_data "aws_region" { defaults = { region = "ap-southeast-1" } }
  mock_resource "aws_iam_role" { defaults = { arn = "arn:aws:iam::123456789012:role/mock" } }
  mock_resource "aws_acm_certificate" {
    defaults = { domain_validation_options = [{ domain_name = "origin.dev.api.example.com", resource_record_name = "_validation.example.com", resource_record_type = "CNAME", resource_record_value = "_value.acm-validations.aws" }] }
  }
}
variables {
  account_id          = "123456789012"
  region              = "ap-southeast-1"
  hosted_zone_id      = "Z123456789"
  origin_domain_name  = "origin.dev.api.example.com"
  kubernetes_version  = "1.35"
  database_version    = "17.4"
  admin_principal_arn = "arn:aws:iam::123456789012:role/PlatformAdmin"
  api_allowed_cidr    = "203.0.113.10/32"
}

run "environment_plan" {
  command = plan
  assert {
    condition     = output.platform.cluster_name == "aws-template-dev"
    error_message = "Environment isolation output does not match root."
  }
}
run "reject_invalid_account" {
  command = plan
  variables { account_id = "invalid" }
  expect_failures = [var.account_id]
}
