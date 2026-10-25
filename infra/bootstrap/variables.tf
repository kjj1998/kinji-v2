variable "alert_email" {
  type        = string
  description = "Alert email for AWS budget monitoring"
  default     = "example.email@gmail.com"
}

variable "monthly_budget_usd" {
  type        = number
  description = "The monthly budget (in USD) to spend on AWS"
  default     = 5
}
