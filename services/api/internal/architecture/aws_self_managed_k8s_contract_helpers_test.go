package architecture_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func runCostPlanWithStubAWS(t *testing.T, root string, env map[string]string) (string, error) {
	t.Helper()
	tempDir := t.TempDir()
	stubPath := filepath.Join(tempDir, "aws")
	stub := `#!/usr/bin/env bash
set -euo pipefail
args="$*"
if [[ "$args" == *"sts get-caller-identity"* ]]; then
  if [[ "$args" == *"--query Account"* ]]; then
    printf '123456789012\n'
  else
    printf '{"Account":"123456789012"}\n'
  fi
  exit 0
fi
if [[ "$args" == *"ec2 describe-spot-price-history"* ]]; then
  if [ -n "${AWS_STUB_SPOT_PRICE:-}" ]; then
    printf '%s\n' "$AWS_STUB_SPOT_PRICE"
  elif [[ "$args" == *"m7a.2xlarge"* ]]; then
    printf '0.120000\n'
  elif [[ "$args" == *"m7i.2xlarge"* ]]; then
    printf '0.110000\n'
  elif [[ "$args" == *"m6a.2xlarge"* ]]; then
    printf '0.100000\n'
  else
    printf '0.090000\n'
  fi
  exit 0
fi
if [[ "$args" == *"pricing get-products"* ]]; then
  printf '{}\n'
  exit 0
fi
printf 'unexpected aws call: %s\n' "$args" >&2
exit 2
`
	require.NoError(t, os.WriteFile(stubPath, []byte(stub), 0o755))

	cmd := exec.Command("bash", filepath.Join(root, "infra", "aws", "self-managed-k8s", "scripts", "10-cost-plan.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PATH="+tempDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"AWS_ACCOUNT_ID=123456789012",
		"AWS_REGION=us-west-2",
		"AWS_ALLOWED_REGIONS=us-west-2",
		"AWS_STACK_PREFIX=cets-aws-k8s-test",
		"NODE_COUNT=3",
		"CONTROL_PLANE_INSTANCE_TYPE=m6a.2xlarge",
		"FALLBACK_INSTANCE_TYPE_1=m7a.2xlarge",
		"FALLBACK_INSTANCE_TYPE_2=m7i.2xlarge",
		"PURCHASE_OPTION=Spot",
		"APP_INGRESS_MODE=nlb",
		"ROOT_VOLUME_GB=120",
		"WORKER_MAX_CAPACITY=0",
	)
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func readArchitectureText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func readFilesUnderForArchitectureTest(t *testing.T, root string) string {
	t.Helper()
	var builder strings.Builder
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		builder.WriteString(filepath.ToSlash(path))
		builder.WriteByte('\n')
		builder.Write(data)
		builder.WriteByte('\n')
		return nil
	})
	require.NoError(t, err)
	return builder.String()
}
