variable "name" { type = string }
variable "environment" { type = string }
variable "network_cidr" { type = string }
variable "kubernetes_version" { type = string }
variable "admin_principal_arn" { type = string }
variable "api_allowed_cidr" { type = string }
variable "public_api_enabled" { type = bool }
variable "node_count" { type = number }
variable "max_node_count" { type = number }
variable "node_instance_type" { type = string }
variable "enable_database" { type = bool }
variable "database_version" { type = string }
variable "addon_versions" {
  type    = map(string)
  default = {}
}

variable "origin_domain_name" { type = string }
variable "hosted_zone_id" { type = string }
