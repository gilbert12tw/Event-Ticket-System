variable "cloudflare_api_token" {
  type      = string
  sensitive = true
}

variable "cloudflare_account_id" {
  type = string
}

variable "cloudflare_zone_id" {
  type = string
}

variable "cloudflare_zone_name" {
  type = string
}

variable "hostname" {
  type = string
}

variable "tunnel_name" {
  type    = string
  default = "cets-baremetal"
}

variable "origin_service" {
  type    = string
  default = "http://ingress-nginx-controller.ingress-nginx.svc.cluster.local:80"
}

variable "allowed_emails" {
  type = list(string)
}

variable "device_posture_rule_ids" {
  type    = list(string)
  default = []
}
