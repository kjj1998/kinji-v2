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

resource "aws_ssm_parameter" "enrich_llm_api_key" {
  name             = "/kinji-v2/enrich/llm-api-key"
  description      = "The LLM api key for the enrich role"
  type             = "SecureString"
  value_wo         = "placeholder"
  value_wo_version = "1"
}

resource "aws_ssm_parameter" "enrich_db_url" {
  name             = "/kinji-v2/enrich/database-url"
  description      = "The database connection url for the enrich role"
  type             = "SecureString"
  value_wo         = "placeholder"
  value_wo_version = "1"
}
