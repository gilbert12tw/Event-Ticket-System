resource "random_bytes" "tunnel_secret" {
  length = 32
}

resource "cloudflare_zero_trust_tunnel_cloudflared" "cets" {
  account_id = var.cloudflare_account_id
  name       = var.tunnel_name
  secret     = random_bytes.tunnel_secret.base64
}

resource "cloudflare_record" "cets" {
  zone_id = var.cloudflare_zone_id
  name    = trimsuffix(replace(var.hostname, ".${var.cloudflare_zone_name}", ""), ".")
  type    = "CNAME"
  content = cloudflare_zero_trust_tunnel_cloudflared.cets.cname
  proxied = true
}

resource "cloudflare_record" "grafana" {
  count   = var.grafana_hostname == "" ? 0 : 1
  zone_id = var.cloudflare_zone_id
  name    = trimsuffix(replace(var.grafana_hostname, ".${var.cloudflare_zone_name}", ""), ".")
  type    = "CNAME"
  content = cloudflare_zero_trust_tunnel_cloudflared.cets.cname
  proxied = true
}

resource "cloudflare_zero_trust_tunnel_cloudflared_config" "cets" {
  account_id = var.cloudflare_account_id
  tunnel_id  = cloudflare_zero_trust_tunnel_cloudflared.cets.id

  config {
    ingress_rule {
      hostname = cloudflare_record.cets.hostname
      service  = var.origin_service
    }

    dynamic "ingress_rule" {
      for_each = var.grafana_hostname == "" ? [] : [cloudflare_record.grafana[0].hostname]
      content {
        hostname = ingress_rule.value
        service  = var.grafana_origin_service
      }
    }

    ingress_rule {
      service = "http_status:404"
    }
  }
}

resource "cloudflare_zero_trust_access_application" "cets" {
  count            = var.access_enabled ? 1 : 0
  account_id       = var.cloudflare_account_id
  name             = "Event Ticket System"
  domain           = cloudflare_record.cets.hostname
  type             = "self_hosted"
  session_duration = "8h"
}

resource "cloudflare_zero_trust_access_application" "grafana" {
  count            = var.access_enabled && var.grafana_hostname != "" ? 1 : 0
  account_id       = var.cloudflare_account_id
  name             = "Event Ticket System Grafana"
  domain           = cloudflare_record.grafana[0].hostname
  type             = "self_hosted"
  session_duration = "8h"
}

resource "cloudflare_zero_trust_device_posture_rule" "warp_required" {
  count      = var.access_enabled && length(var.device_posture_rule_ids) == 0 ? 1 : 0
  account_id = var.cloudflare_account_id
  name       = "cets-warp-required"
  type       = "warp"
  schedule   = "1h"
  expiration = "24h"
}

locals {
  device_posture_rule_ids = !var.access_enabled ? [] : length(var.device_posture_rule_ids) == 0 ? [cloudflare_zero_trust_device_posture_rule.warp_required[0].id] : var.device_posture_rule_ids
}

resource "cloudflare_zero_trust_access_policy" "allow_approved_devices" {
  count          = var.access_enabled ? 1 : 0
  account_id     = var.cloudflare_account_id
  application_id = cloudflare_zero_trust_access_application.cets[0].id
  name           = "Allow approved users and WARP devices"
  precedence     = "1"
  decision       = "allow"

  include {
    email = var.allowed_emails
  }

  dynamic "require" {
    for_each = [1]
    content {
      device_posture = local.device_posture_rule_ids
    }
  }
}

resource "cloudflare_zero_trust_access_policy" "allow_approved_devices_grafana" {
  count          = var.access_enabled && var.grafana_hostname != "" ? 1 : 0
  account_id     = var.cloudflare_account_id
  application_id = cloudflare_zero_trust_access_application.grafana[0].id
  name           = "Allow approved users and WARP devices"
  precedence     = "1"
  decision       = "allow"

  include {
    email = var.allowed_emails
  }

  dynamic "require" {
    for_each = [1]
    content {
      device_posture = local.device_posture_rule_ids
    }
  }
}
