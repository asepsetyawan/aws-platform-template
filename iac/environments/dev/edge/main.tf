locals { name = "aws-template-dev" }
provider "aws" {
  region              = var.region
  allowed_account_ids = [var.account_id]
  default_tags { tags = { Project = "aws-template", Environment = "dev", ManagedBy = "Terraform" } }
}

provider "aws" {
  alias               = "global"
  region              = "us-east-1"
  allowed_account_ids = [var.account_id]
}
module "edge" {
  source              = "../../../modules/edge"
  providers           = { aws = aws, aws.global = aws.global }
  name                = local.name
  domain_name         = var.domain_name
  origin_domain_name  = var.origin_domain_name
  hosted_zone_id      = var.hosted_zone_id
  origin_alb_arn      = var.origin_alb_arn
  origin_alb_dns_name = var.origin_alb_dns_name
  origin_alb_zone_id  = var.origin_alb_zone_id
}
output "url" { value = module.edge.url }
