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

resource "cloudflare_zero_trust_tunnel_cloudflared_config" "cets" {
  account_id = var.cloudflare_account_id
  tunnel_id  = cloudflare_zero_trust_tunnel_cloudflared.cets.id

  config {
    ingress_rule {
      hostname = cloudflare_record.cets.hostname
      service  = var.origin_service
    }

    ingress_rule {
      service = "http_status:404"
    }
  }
}

resource "cloudflare_zero_trust_access_application" "cets" {
  account_id       = var.cloudflare_account_id
  name             = "Event Ticket System"
  domain           = cloudflare_record.cets.hostname
  type             = "self_hosted"
  session_duration = "8h"
}

resource "cloudflare_zero_trust_device_posture_rule" "warp_required" {
  count      = length(var.device_posture_rule_ids) == 0 ? 1 : 0
  account_id = var.cloudflare_account_id
  name       = "cets-warp-required"
  type       = "warp"
  schedule   = "1h"
  expiration = "24h"
}

locals {
  device_posture_rule_ids = length(var.device_posture_rule_ids) == 0 ? [cloudflare_zero_trust_device_posture_rule.warp_required[0].id] : var.device_posture_rule_ids
}

resource "cloudflare_zero_trust_access_policy" "allow_approved_devices" {
  account_id     = var.cloudflare_account_id
  application_id = cloudflare_zero_trust_access_application.cets.id
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
