output "hostname" {
  value = cloudflare_record.cets.hostname
}

output "grafana_hostname" {
  value = var.grafana_hostname == "" ? "" : cloudflare_record.grafana[0].hostname
}

output "tunnel_id" {
  value = cloudflare_zero_trust_tunnel_cloudflared.cets.id
}

output "tunnel_token" {
  value     = cloudflare_zero_trust_tunnel_cloudflared.cets.tunnel_token
  sensitive = true
}
