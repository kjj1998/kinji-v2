output "ingest_url" {
  description = "The public URL of the ingest lambda."
  value       = aws_lambda_function_url.ingest.function_url
}
