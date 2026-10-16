mock_provider "aws" {
  mock_data "aws_availability_zones" { defaults = { names = ["ap-southeast-1a", "ap-southeast-1b", "ap-southeast-1c"] } }
  mock_data "aws_caller_identity" { defaults = { account_id = "123456789012" } }
  mock_data "aws_region" { defaults = { region = "ap-southeast-1" } }
  mock_resource "aws_iam_role" { defaults = { arn = "arn:aws:iam::123456789012:role/mock" } }
  mock_resource "aws_acm_certificate" {
    defaults = { domain_validation_options = [{ domain_name = "origin.prod.api.example.com", resource_record_name = "_validation.example.com", resource_record_type = "CNAME", resource_record_value = "_value.acm-validations.aws" }] }
  }
}
mock_provider "aws" {
  alias = "global"
  mock_resource "aws_acm_certificate" {
    defaults = { domain_validation_options = [{ domain_name = "api.example.com", resource_record_name = "_validation.example.com", resource_record_type = "CNAME", resource_record_value = "_value.acm-validations.aws" }] }
  }
}
variables {
  account_id          = "123456789012"
  region              = "ap-southeast-1"
  hosted_zone_id      = "Z123456789"
  origin_domain_name  = "origin.prod.api.example.com"
  domain_name         = "prod.api.example.com"
  origin_alb_arn      = "arn:aws:elasticloadbalancing:ap-southeast-1:123456789012:loadbalancer/app/example/1234567890123456"
  origin_alb_dns_name = "internal-example.ap-southeast-1.elb.amazonaws.com"
  origin_alb_zone_id  = "Z123456789"
}

run "environment_plan" {
  command = plan
  assert {
    condition     = output.url == "https://prod.api.example.com"
    error_message = "Environment isolation output does not match root."
  }
}
run "reject_invalid_account" {
  command = plan
  variables { account_id = "invalid" }
  expect_failures = [var.account_id]
}
