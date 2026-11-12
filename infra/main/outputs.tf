output "ingest_url" {
  description = "The public URL of the ingest lambda."
  value       = aws_lambda_function_url.ingest.function_url
}

output "enrich_queue_url" {
  description = "The URL of the enrich SQS queue"
  value       = aws_sqs_queue.enrich.url
}

output "enrich_queue_arn" {
  description = "The ARN of the enrich SQS queue"
  value       = aws_sqs_queue.enrich.arn
}
