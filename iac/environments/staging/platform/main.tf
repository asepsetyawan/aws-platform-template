locals { name = "aws-template-staging" }
provider "aws" {
  region              = var.region
  allowed_account_ids = [var.account_id]
  default_tags { tags = { Project = "aws-template", Environment = "staging", ManagedBy = "Terraform" } }
}

module "platform" {
  source              = "../../../modules/platform"
  name                = local.name
  environment         = "staging"
  network_cidr        = "10.50.0.0/16"
  kubernetes_version  = var.kubernetes_version
  admin_principal_arn = var.admin_principal_arn
  api_allowed_cidr    = var.api_allowed_cidr
  public_api_enabled  = true
  node_count          = 3
  max_node_count      = 12
  node_instance_type  = "t3.large"
  enable_database     = true
  database_version    = var.database_version
  addon_versions      = var.addon_versions
  origin_domain_name  = var.origin_domain_name
  hosted_zone_id      = var.hosted_zone_id
}
output "platform" { value = module.platform }
