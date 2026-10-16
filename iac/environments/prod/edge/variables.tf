variable "account_id" {
  type = string
  validation {
    condition     = can(regex("^[0-9]{12}$", var.account_id))
    error_message = "AWS account_id must be 12 digits."
  }
}
variable "region" { type = string }
variable "hosted_zone_id" { type = string }
variable "domain_name" { type = string }
variable "origin_domain_name" { type = string }
variable "origin_alb_arn" { type = string }
variable "origin_alb_dns_name" { type = string }
variable "origin_alb_zone_id" { type = string }
