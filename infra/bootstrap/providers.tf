provider "aws" {
  region = "ap-southeast-1"
  default_tags {
    tags = {
      Project     = "kinji-v2"
      Environment = "Prod"
    }
  }
}
