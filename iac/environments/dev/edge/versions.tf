terraform {
  required_version = ">= 1.9, < 2.0"
  cloud {
    workspaces { name = "aws-dev-edge" }
  }
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }
}
