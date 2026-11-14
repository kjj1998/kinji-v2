data "archive_file" "ingest" {
  type        = "zip"
  source_file = "${path.module}/../../lambdas/build/ingest/bootstrap"
  output_path = "${path.module}/build/ingest.zip"
}

resource "aws_cloudwatch_log_group" "ingest" {
  name              = "/aws/lambda/kinji-v2-ingest"
  retention_in_days = 14
}

data "aws_iam_policy_document" "ingest_assume_role_policy" {
  statement {
    actions = ["sts:AssumeRole"]
    effect  = "Allow"

    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "ingest" {
  name               = "kinji-v2-ingest"
  assume_role_policy = data.aws_iam_policy_document.ingest_assume_role_policy.json
}

data "aws_iam_policy_document" "ingest_access_policy" {
  statement {
    effect  = "Allow"
    actions = ["logs:CreateLogStream", "logs:PutLogEvents"]
    resources = [
      "${aws_cloudwatch_log_group.ingest.arn}:*",
    ]
  }

  statement {
    effect  = "Allow"
    actions = ["ssm:GetParameter"]
    resources = [
      aws_ssm_parameter.ingest_db_url.arn
    ]
  }

  statement {
    effect  = "Allow"
    actions = ["sqs:SendMessage"]
    resources = [
      aws_sqs_queue.enrich.arn
    ]
  }
}


resource "aws_iam_role_policy" "ingest_permissions" {
  name   = "kinji-v2-ingest-access"
  role   = aws_iam_role.ingest.id
  policy = data.aws_iam_policy_document.ingest_access_policy.json
}

resource "aws_lambda_function" "ingest" {
  function_name                  = "kinji-v2-ingest"
  role                           = aws_iam_role.ingest.arn
  runtime                        = "provided.al2023"
  handler                        = "bootstrap"
  architectures                  = ["arm64"]
  filename                       = data.archive_file.ingest.output_path
  source_code_hash               = data.archive_file.ingest.output_base64sha256
  timeout                        = 10
  memory_size                    = 128
  reserved_concurrent_executions = 5
  environment {
    variables = {
      DATABASE_URL_PARAM = aws_ssm_parameter.ingest_db_url.name
      ENRICH_QUEUE_URL   = aws_sqs_queue.enrich.url
    }
  }

  depends_on = [aws_cloudwatch_log_group.ingest, aws_iam_role_policy.ingest_permissions]
}

resource "aws_lambda_function_url" "ingest" {
  function_name      = aws_lambda_function.ingest.function_name
  authorization_type = "NONE"
}

resource "aws_lambda_permission" "ingest_url_invoke_url" {
  function_name          = aws_lambda_function.ingest.function_name
  statement_id           = "AllowPublicFunctionUrl"
  action                 = "lambda:InvokeFunctionUrl"
  principal              = "*"
  function_url_auth_type = "NONE"
}

resource "aws_lambda_permission" "ingest_url_invoke_function" {
  function_name            = aws_lambda_function.ingest.function_name
  statement_id             = "AllowInvokeViaFunctionUrl"
  action                   = "lambda:InvokeFunction"
  principal                = "*"
  invoked_via_function_url = true
}

