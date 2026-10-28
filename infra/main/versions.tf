terraform {
  required_version = ">= 1.17.0, < 1.18.0"
  required_providers {

    aws = {
      source  = "hashicorp/aws"
      version = "6.70.0"
    }
  }
  backend "s3" {
    bucket       = "kinji-v2-tfstate-271407076537"
    key          = "main/terraform.tfstate"
    region       = "ap-southeast-1"
    use_lockfile = true
  }
}
