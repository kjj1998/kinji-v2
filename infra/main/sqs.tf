resource "aws_sqs_queue" "enrich_dlq" {
  name                      = "kinji-v2-enrich-dlq"
  message_retention_seconds = 1209600
  sqs_managed_sse_enabled   = true
}

resource "aws_sqs_queue_redrive_allow_policy" "enrich_dlq" {
  queue_url = aws_sqs_queue.enrich_dlq.id

  redrive_allow_policy = jsonencode({
    redrivePermission = "byQueue"
    sourceQueueArns   = [aws_sqs_queue.enrich.arn]
  })
}

resource "aws_sqs_queue" "enrich" {
  name                       = "kinji-v2-enrich"
  visibility_timeout_seconds = 360
  receive_wait_time_seconds  = 20
  sqs_managed_sse_enabled    = true

  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.enrich_dlq.arn
    maxReceiveCount     = 5
  })
}
