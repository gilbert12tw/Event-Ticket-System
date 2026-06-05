package deploy

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBaremetalCloudflareSupportsTicketAndGrafanaHostnames(t *testing.T) {
	cloudflare := readFilesUnder(t, filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "cloudflare"))
	scripts := readText(t, filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "scripts", "40-cloudflare.sh")) +
		readText(t, filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "scripts", "41-cloudflare-preflight.sh")) +
		readText(t, filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "scripts", "62-verify-cloudflare.sh"))
	envExample := readText(t, filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", ".env.example"))
	docs := readText(t, filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "README.md")) +
		readText(t, filepath.Join("..", "..", "..", "infra", "k8s", "baremetal", "CLOUDFLARE_ACTIONS.md"))
	combined := cloudflare + "\n" + scripts + "\n" + envExample + "\n" + docs

	for _, fragment := range []string{
		"variable \"grafana_hostname\"",
		"variable \"grafana_origin_service\"",
		"variable \"access_enabled\"",
		"http://kube-prometheus-stack-grafana.observability.svc.cluster.local:80",
		"cloudflare_record\" \"grafana\"",
		"cloudflare_zero_trust_access_application\" \"grafana\"",
		"allow_approved_devices_grafana",
		"var.access_enabled ? 1 : 0",
		`CLOUDFLARE_ACCESS_ENABLED=false`,
		"Cloudflare Access is disabled; public hostnames will not require Access login",
		"public endpoint did not reach origin health",
		"grafana_hostname: $grafana_hostname",
		"GRAFANA_PUBLIC_HOSTNAME",
		"https://$GRAFANA_PUBLIC_HOSTNAME/api/health",
		"grafana_hostname",
	} {
		assert.Contains(t, combined, fragment, "Cloudflare Grafana contract is missing %q", fragment)
	}
}
