# Cloudflare Actions For Bare-Metal HA Fallback

Router BGP/ECMP is not available in this environment, so the public HA path is:

`Browser -> Cloudflare Access -> Cloudflare Tunnel -> 3 cloudflared pods -> ingress-nginx service -> app pods`

This still gives users one HTTPS endpoint and avoids inbound firewall exposure. It does not prove the
original router BGP/ECMP requirement; `65-verify-bgp.sh` will keep failing until the MikroTik router
is configured.

## Cloudflare Dashboard/API Setup

1. Add the real domain to Cloudflare and make sure the zone status is `active`.
2. Create or choose the public hostname, for example `tickets.example.com`.
3. In Zero Trust, enroll the approved computer with WARP.
4. Create a Device Posture rule that matches the approved computer. Use the rule ID in
   `CLOUDFLARE_ACCESS_DEVICE_POSTURE_RULE_IDS`.
5. Decide which email identities are allowed. Use a comma-separated list in
   `CLOUDFLARE_ACCESS_ALLOWED_EMAILS`.
6. Create a Cloudflare API token that can manage the target zone DNS and Zero Trust resources.

## Local Values To Fill

Edit `infra/k8s/baremetal/.env.baremetal.local` and set:

```sh
CETS_PUBLIC_HOSTNAME=<real hostname>
GRAFANA_PUBLIC_HOSTNAME=<grafana hostname>
CLOUDFLARE_API_TOKEN=<token>
CLOUDFLARE_ACCOUNT_ID=<account id>
CLOUDFLARE_ZONE_ID=<zone id>
CLOUDFLARE_ZONE_NAME=<zone name>
CLOUDFLARE_ACCESS_ENABLED=true
CLOUDFLARE_ACCESS_ALLOWED_EMAILS=<approved email 1>,<approved email 2>
CLOUDFLARE_ACCESS_DEVICE_POSTURE_RULE_IDS=<device posture rule id>
```

Set `CLOUDFLARE_ACCESS_ENABLED=false` only when the app and Grafana should be
reachable by anyone on the public internet. In that mode, Terraform creates the
Tunnel and DNS records but skips Cloudflare Access applications and policies.
Grafana still has its own login, but its public login page is exposed.

Do not commit this file.

## Apply And Verify

```sh
infra/k8s/baremetal/scripts/41-cloudflare-preflight.sh
APPLY=true infra/k8s/baremetal/scripts/40-cloudflare.sh
APPLY=true infra/k8s/baremetal/scripts/50-deploy-cets.sh
infra/k8s/baremetal/scripts/62-verify-cloudflare.sh
```

Expected result:

- `cloudflared` has 3 ready replicas in namespace `cets`.
- Unauthenticated `curl` gets a Cloudflare Access challenge or deny response.
- Your approved WARP-enrolled browser can open `https://$CETS_PUBLIC_HOSTNAME`.
- Your approved WARP-enrolled browser can open `https://$GRAFANA_PUBLIC_HOSTNAME`.
- `https://$CETS_PUBLIC_HOSTNAME/healthz` reaches the app after approval.
- `https://$GRAFANA_PUBLIC_HOSTNAME/api/health` reaches Grafana after approval.

## Add An Approved Device

For this deployment, users reach the app through:

`Browser -> tickets.sky-lab.uk -> Cloudflare Access -> Cloudflare Tunnel -> Kubernetes`

That path does not require WARP private network routing or a Split Tunnel entry for the Kubernetes
LAN. The device only needs to be enrolled into the Cloudflare Zero Trust team and satisfy the Access
policy posture check.

Steps for each approved computer:

1. Install the Cloudflare One / WARP client.
2. Open WARP and choose the Zero Trust login flow.
3. Enter the account team name from `Zero Trust -> Settings -> Custom Pages` or
   `Zero Trust -> Settings -> WARP Client`.
4. Authenticate with an allowed identity, currently the email configured in
   `CLOUDFLARE_ACCESS_ALLOWED_EMAILS`.
5. Confirm the WARP client shows the device as connected to the Zero Trust organization.
6. Open `https://tickets.sky-lab.uk` in the browser and complete the Access login.

## Split Tunnel Guidance

Do not add `10.124.121.0/24` for this environment. The Kubernetes LAN is `10.121.124.0/24`, and
even that CIDR is not needed for the browser-to-public-hostname path above.

Only configure Split Tunnel Include mode if a later requirement needs approved laptops to reach
private IPs directly, such as `http://10.121.124.211`. In that separate private-routing design,
the CIDR would be `10.121.124.0/24`, and Cloudflare would also need a private network route through
the Tunnel. That is optional and not part of the current public HA endpoint.

## Remaining Non-Cloudflare Limitation

Without MikroTik BGP/ECMP, `10.121.124.211` is still useful as the internal ingress IP, and
Cloudflare Tunnel can keep the public endpoint available through outbound connectors. However,
router-level ECMP distribution across the three VMs is not proven.
