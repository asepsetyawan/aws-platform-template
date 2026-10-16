variable "account_id" {
  type = string
  validation {
    condition     = can(regex("^[0-9]{12}$", var.account_id))
    error_message = "AWS account_id must be 12 digits."
  }
}
variable "region" { type = string }
variable "hosted_zone_id" { type = string }
variable "kubernetes_version" { type = string }
variable "admin_principal_arn" { type = string }
variable "api_allowed_cidr" {
  type = string
  validation {
    condition     = can(cidrnetmask(var.api_allowed_cidr)) && endswith(var.api_allowed_cidr, "/32")
    error_message = "Use one public IPv4 /32 for controlled API access."
  }
}
variable "origin_domain_name" { type = string }
variable "database_version" { type = string }
variable "addon_versions" {
  type    = map(string)
  default = {}
}
