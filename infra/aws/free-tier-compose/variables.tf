variable "aws_profile" {
  description = "Least-privilege AWS CLI profile. Root credentials are only allowed for one-time bootstrap outside this stack."
  type        = string
}

variable "aws_region" {
  description = "Selected low-cost US region after live cost check."
  type        = string

  validation {
    condition     = contains(["us-east-1", "us-east-2", "us-west-2"], var.aws_region)
    error_message = "aws_region must be one of us-east-1, us-east-2, or us-west-2."
  }
}

variable "deployment_mode" {
  description = "Deployment mode for scheduled cost control."
  type        = string
  default     = "dev-single-az"

  validation {
    condition     = contains(["dev-single-az", "demo-ha"], var.deployment_mode)
    error_message = "deployment_mode must be dev-single-az or demo-ha."
  }
}

variable "project_name" {
  description = "Resource name prefix."
  type        = string
  default     = "cets-phase3"
}

variable "domain_name" {
  description = "Existing demo domain or subdomain for ALB HTTPS."
  type        = string
}

variable "acm_certificate_arn" {
  description = "Optional pre-issued ACM certificate ARN. If empty, this stack creates a DNS-validated certificate and outputs validation records."
  type        = string
  default     = ""
}

variable "budget_email" {
  description = "Email address subscribed to AWS Budgets notifications."
  type        = string
  sensitive   = true
}

variable "budget_time_period_start" {
  description = "UTC budget window start in AWS Budgets YYYY-MM-DD_HH:MM format. Default is 2026-06-01 00:00 Asia/Taipei."
  type        = string
  default     = "2026-05-31_16:00"

  validation {
    condition     = can(regex("^20[0-9][0-9]-[0-1][0-9]-[0-3][0-9]_[0-2][0-9]:[0-5][0-9]$", var.budget_time_period_start))
    error_message = "budget_time_period_start must use AWS Budgets YYYY-MM-DD_HH:MM UTC format."
  }
}

variable "budget_time_period_end" {
  description = "UTC budget window end in AWS Budgets YYYY-MM-DD_HH:MM format. Default is 2026-06-15 00:00 Asia/Taipei."
  type        = string
  default     = "2026-06-14_16:00"

  validation {
    condition     = can(regex("^20[0-9][0-9]-[0-1][0-9]-[0-3][0-9]_[0-2][0-9]:[0-5][0-9]$", var.budget_time_period_end))
    error_message = "budget_time_period_end must use AWS Budgets YYYY-MM-DD_HH:MM UTC format."
  }
}

variable "allowlist_cidrs" {
  description = "CIDR blocks allowed to reach SSH and restricted observability/admin endpoints."
  type        = list(string)
}

variable "public_app_cidrs" {
  description = "CIDR blocks allowed to reach public HTTPS app endpoint. Use 0.0.0.0/0 only when explicitly accepted for demo."
  type        = list(string)
  default     = ["0.0.0.0/0"]
}

variable "app_instance_type" {
  description = "Low-cost ARM EC2 instance type for app nodes."
  type        = string
  default     = "t4g.micro"
}

variable "observability_instance_type" {
  description = "Low-cost ARM EC2 instance type for the LGTM/control node. Increase only if the cost gate passes."
  type        = string
  default     = "t4g.small"
}

variable "demo_app_node_count" {
  description = "App node count during demo-ha. A third node is allowed only after cost gate passes."
  type        = number
  default     = 2

  validation {
    condition     = var.demo_app_node_count >= 2 && var.demo_app_node_count <= 3
    error_message = "demo_app_node_count must be 2 or 3."
  }
}

variable "ssh_key_name" {
  description = "Optional EC2 key pair name. Prefer SSM Session Manager when possible."
  type        = string
  default     = null
}

variable "repository_url" {
  description = "Git URL the EC2 bootstrap can clone."
  type        = string
}

variable "repository_branch" {
  description = "Branch to deploy."
  type        = string
  default     = "feature/phase3-compose-ha-lgtm-pr"
}

variable "db_name" {
  description = "RDS database name."
  type        = string
  default     = "cets"
}

variable "db_username" {
  description = "RDS database username."
  type        = string
  default     = "cets"
}

variable "db_password" {
  description = "RDS database password."
  type        = string
  sensitive   = true
}

variable "token_signing_secret" {
  description = "Ticket token signing secret. Stored in Terraform state; use a dedicated encrypted backend for real deployments."
  type        = string
  sensitive   = true
}

variable "provider_token_secret" {
  description = "Provider token secret. Stored in Terraform state; use a dedicated encrypted backend for real deployments."
  type        = string
  sensitive   = true
}

variable "grafana_admin_user" {
  description = "Grafana admin user for demo observability."
  type        = string
  default     = "admin"
}

variable "grafana_admin_password" {
  description = "Grafana admin password. Stored in Terraform state; use a dedicated encrypted backend for real deployments."
  type        = string
  sensitive   = true
}

variable "enable_rds_multi_az_in_demo" {
  description = "Enable RDS Multi-AZ only during demo-ha when the cost gate passes."
  type        = bool
  default     = true
}

variable "ttl_expires_on" {
  description = "ISO date for cleanup tracking. Example: 2026-06-12."
  type        = string
  default     = "2026-06-12"
}
