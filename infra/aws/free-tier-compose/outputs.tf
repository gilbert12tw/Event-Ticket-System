output "account_id" {
  value = data.aws_caller_identity.current.account_id
}

output "selected_region" {
  value = var.aws_region
}

output "deployment_mode" {
  value = var.deployment_mode
}

output "alb_dns_name" {
  value = aws_lb.app.dns_name
}

output "alb_zone_id" {
  value = aws_lb.app.zone_id
}

output "app_target_group_arn" {
  value = aws_lb_target_group.app.arn
}

output "https_endpoint" {
  value = "https://${var.domain_name}"
}

output "acm_dns_validation_records" {
  description = "Create these records in the existing DNS provider before expecting ACM issuance."
  value = [
    for option in var.acm_certificate_arn == "" ? aws_acm_certificate.demo[0].domain_validation_options : [] : {
      name  = option.resource_record_name
      type  = option.resource_record_type
      value = option.resource_record_value
    }
  ]
}

output "rds_endpoint" {
  value     = aws_db_instance.postgres.address
  sensitive = true
}

output "s3_export_bucket" {
  value = aws_s3_bucket.exports.bucket
}

output "object_store_access_key_id" {
  value     = aws_iam_access_key.object_store.id
  sensitive = true
}

output "app_instance_ids" {
  value = { for key, instance in aws_instance.app : key => instance.id }
}

output "observability_public_ip" {
  value = aws_instance.observability.public_ip
}

output "grafana_url" {
  value = "http://${aws_instance.observability.public_ip}:3000"
}

output "budget_name" {
  value = aws_budgets_budget.phase3.name
}

output "budget_limit_usd" {
  value = aws_budgets_budget.phase3.limit_amount
}

output "budget_time_period_start" {
  value = var.budget_time_period_start
}

output "budget_time_period_end" {
  value = var.budget_time_period_end
}

output "budget_notification_thresholds_usd" {
  value = local.budget_thresholds
}
