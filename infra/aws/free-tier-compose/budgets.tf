resource "aws_budgets_budget" "phase3" {
  name              = "${var.project_name}-two-week-guardrail"
  budget_type       = "COST"
  limit_amount      = "180"
  limit_unit        = "USD"
  time_period_start = var.budget_time_period_start
  time_period_end   = var.budget_time_period_end
  time_unit         = "MONTHLY"

  cost_types {
    include_credit             = true
    include_discount           = true
    include_other_subscription = true
    include_recurring          = true
    include_refund             = true
    include_subscription       = true
    include_support            = true
    include_tax                = true
    include_upfront            = true
    use_blended                = false
  }

  dynamic "notification" {
    for_each = toset(local.budget_thresholds)

    content {
      comparison_operator        = "GREATER_THAN"
      threshold                  = notification.value
      threshold_type             = "ABSOLUTE_VALUE"
      notification_type          = "ACTUAL"
      subscriber_email_addresses = [var.budget_email]
    }
  }
}
