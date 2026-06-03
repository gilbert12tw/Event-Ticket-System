package architecture_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAWSSelfManagedK8sOnDemandEmptyPricingUsesFallbackGuardrail(t *testing.T) {
	root := repoRoot(t)
	generatedCostPlan := filepath.Join(root, "infra", "aws", "self-managed-k8s", "generated", "cost-plan.env")
	preserveGeneratedCostPlan(t, generatedCostPlan)

	output, err := runCostPlanWithStubAWS(t, root, map[string]string{
		"PURCHASE_OPTION":      "OnDemand",
		"ACTIVE_RUNTIME_HOURS": "336",
		"MAX_RUNTIME_HOURS":    "336",
		"BUDGET_LIMIT_USD":     "190",
		"COST_RESERVE_USD":     "15",
	})
	require.Error(t, err)
	assert.Contains(t, output, "estimated cost")
	assert.Contains(t, output, "exceeds deploy limit")
}

func preserveGeneratedCostPlan(t *testing.T, path string) {
	t.Helper()
	originalCostPlan, readErr := os.ReadFile(path)
	existed := readErr == nil
	if readErr != nil && !os.IsNotExist(readErr) {
		require.NoError(t, readErr)
	}
	t.Cleanup(func() {
		if existed {
			require.NoError(t, os.WriteFile(path, originalCostPlan, 0o600))
			return
		}
		require.NoError(t, os.RemoveAll(path))
	})
}
