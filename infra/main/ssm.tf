resource "aws_ssm_parameter" "ingest_db_url" {
  name             = "/kinji-v2/ingest/database-url"
  description      = "The database connection url for the ingest role"
  type             = "SecureString"
  value_wo         = "placeholder"
  value_wo_version = "1"
}

resource "aws_ssm_parameter" "mint_token_db_url" {
  name             = "/kinji-v2/mint-token/database-url"
  description      = "The database connection url for the mint_token role"
  type             = "SecureString"
  value_wo         = "placeholder"
  value_wo_version = "1"
}
