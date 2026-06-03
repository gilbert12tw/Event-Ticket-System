package architecture_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAWSSelfManagedK8sValidatesAppEnv(t *testing.T) {
	root := repoRoot(t)
	lib := filepath.Join(root, "infra", "aws", "self-managed-k8s", "scripts", "lib.sh")

	run := func(appEnv, allowDemo string) (string, error) {
		t.Helper()
		cmd := exec.Command(
			"bash",
			"-c",
			`. "$1"; load_env; CETS_APP_ENV="$2"; ALLOW_DEMO_APP_ENV="$3"; validate_cets_app_env`,
			"aws-app-env-test",
			lib,
			appEnv,
			allowDemo,
		)
		cmd.Dir = root
		output, err := cmd.CombinedOutput()
		return string(output), err
	}

	output, err := run("aws-self-managed-k8s", "false")
	require.NoError(t, err, output)

	output, err = run("demo", "false")
	require.Error(t, err)
	assert.Contains(t, output, "enables mock profile login")

	output, err = run("demo", "true")
	require.NoError(t, err, output)

	output, err = run("production", "true")
	require.Error(t, err)
	assert.Contains(t, output, "must be aws-self-managed-k8s")
}

func TestAWSSelfManagedK8sQuickHTTPSTunnelScripts(t *testing.T) {
	root := repoRoot(t)
	scriptsRoot := filepath.Join(root, "infra", "aws", "self-managed-k8s", "scripts")

	startPath := filepath.Join(scriptsRoot, "92-start-quick-https-tunnel.sh")
	stopPath := filepath.Join(scriptsRoot, "93-stop-quick-https-tunnel.sh")
	require.FileExists(t, startPath)
	require.FileExists(t, stopPath)

	start, err := os.ReadFile(startPath)
	require.NoError(t, err)
	startText := string(start)
	assert.Contains(t, startText, "trycloudflare.com")
	assert.Contains(t, startText, "cloudflare/cloudflared")
	assert.Contains(t, startText, "nginx")
	assert.Contains(t, startText, "QUICK_HTTPS_PUBLIC_EXPOSURE_ACK")
	assert.Contains(t, startText, "cleanup_quick_https_resources")
	assert.Contains(t, startText, "--since-time")
	assert.NotContains(t, startText, "CLOUDFLARE_TUNNEL_TOKEN")

	stop, err := os.ReadFile(stopPath)
	require.NoError(t, err)
	assert.Contains(t, string(stop), "delete deployment")
}
