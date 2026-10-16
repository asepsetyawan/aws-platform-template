resource "aws_route53_record" "origin" {
  zone_id = var.hosted_zone_id
  name    = var.origin_domain_name
  type    = "A"
  alias {
    name                   = var.origin_alb_dns_name
    zone_id                = var.origin_alb_zone_id
    evaluate_target_health = false
  }
}
resource "aws_cloudfront_vpc_origin" "app" {
  vpc_origin_endpoint_config {
    name                   = var.name
    arn                    = var.origin_alb_arn
    http_port              = 80
    https_port             = 443
    origin_protocol_policy = "https-only"
    origin_ssl_protocols {
      items    = ["TLSv1.2"]
      quantity = 1
    }
  }
}
resource "aws_acm_certificate" "viewer" {
  provider          = aws.global
  domain_name       = var.domain_name
  validation_method = "DNS"
  lifecycle { create_before_destroy = true }
}
resource "aws_route53_record" "viewer_validation" {
  zone_id = var.hosted_zone_id
  name    = one(aws_acm_certificate.viewer.domain_validation_options).resource_record_name
  type    = one(aws_acm_certificate.viewer.domain_validation_options).resource_record_type
  records = [one(aws_acm_certificate.viewer.domain_validation_options).resource_record_value]
  ttl     = 60
}
resource "aws_acm_certificate_validation" "viewer" {
  provider                = aws.global
  certificate_arn         = aws_acm_certificate.viewer.arn
  validation_record_fqdns = [aws_route53_record.viewer_validation.fqdn]
}
resource "aws_wafv2_web_acl" "app" {
  provider = aws.global
  name     = var.name
  scope    = "CLOUDFRONT"
  default_action {
    allow {}
  }
  visibility_config {
    cloudwatch_metrics_enabled = true
    metric_name                = var.name
    sampled_requests_enabled   = true
  }
  rule {
    name     = "AWSCommonRules"
    priority = 1
    override_action {
      none {}
    }
    statement {
      managed_rule_group_statement {
        name        = "AWSManagedRulesCommonRuleSet"
        vendor_name = "AWS"
      }
    }
    visibility_config {
      cloudwatch_metrics_enabled = true
      metric_name                = "${var.name}-common"
      sampled_requests_enabled   = true
    }
  }
}
data "aws_cloudfront_cache_policy" "disabled" { name = "Managed-CachingDisabled" }
data "aws_cloudfront_origin_request_policy" "request" { name = "Managed-AllViewerExceptHostHeader" }
resource "aws_cloudfront_distribution" "app" {
  enabled    = true
  aliases    = [var.domain_name]
  web_acl_id = aws_wafv2_web_acl.app.arn
  origin {
    domain_name = var.origin_domain_name
    origin_id   = "eks-alb"
    vpc_origin_config { vpc_origin_id = aws_cloudfront_vpc_origin.app.id }
  }
  default_cache_behavior {
    target_origin_id         = "eks-alb"
    viewer_protocol_policy   = "redirect-to-https"
    allowed_methods          = ["GET", "HEAD", "OPTIONS", "PUT", "POST", "PATCH", "DELETE"]
    cached_methods           = ["GET", "HEAD"]
    cache_policy_id          = data.aws_cloudfront_cache_policy.disabled.id
    origin_request_policy_id = data.aws_cloudfront_origin_request_policy.request.id
    compress                 = true
  }
  restrictions {
    geo_restriction { restriction_type = "none" }
  }
  viewer_certificate {
    acm_certificate_arn      = aws_acm_certificate_validation.viewer.certificate_arn
    ssl_support_method       = "sni-only"
    minimum_protocol_version = "TLSv1.2_2021"
  }
  depends_on = [aws_route53_record.origin]
}
resource "aws_route53_record" "app" {
  zone_id = var.hosted_zone_id
  name    = var.domain_name
  type    = "A"
  alias {
    name                   = aws_cloudfront_distribution.app.domain_name
    zone_id                = aws_cloudfront_distribution.app.hosted_zone_id
    evaluate_target_health = false
  }
}
output "url" { value = "https://${var.domain_name}" }
