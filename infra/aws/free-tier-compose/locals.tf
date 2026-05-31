locals {
  is_demo_ha        = var.deployment_mode == "demo-ha"
  app_node_count    = local.is_demo_ha ? var.demo_app_node_count : 1
  network_azs       = slice(data.aws_availability_zones.available.names, 0, 2)
  app_azs           = local.is_demo_ha ? local.network_azs : slice(local.network_azs, 0, 1)
  rds_multi_az      = local.is_demo_ha && var.enable_rds_multi_az_in_demo
  public_subnets    = { for index, az in local.network_azs : az => cidrsubnet("10.43.0.0/16", 8, index) }
  private_subnets   = { for index, az in local.network_azs : az => cidrsubnet("10.43.0.0/16", 8, index + 20) }
  budget_thresholds = [1, 25, 90, 150, 180]

  common_tags = {
    Project     = var.project_name
    Phase       = "phase3"
    ManagedBy   = "terraform"
    CostOwner   = "codex-demo"
    TTL         = var.ttl_expires_on
    Environment = var.deployment_mode
  }
}
