resource "aws_budgets_budget" "monthly_spend_limit" {
  name         = "monthly-budget-total"
  budget_type  = "COST"
  time_unit    = "MONTHLY"
  limit_unit   = "USD"
  limit_amount = var.monthly_budget_usd

  # Alert 1: Triggers early if AWS forecasts you will exceed 100% of your budget
  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 100
    threshold_type             = "PERCENTAGE"
    notification_type          = "FORECASTED"
    subscriber_email_addresses = [var.alert_email]
  }

  # Alert 2: Triggers immediately when real spend hits 80% of your budget
  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 80
    threshold_type             = "PERCENTAGE"
    notification_type          = "ACTUAL"
    subscriber_email_addresses = [var.alert_email]
  }
}
