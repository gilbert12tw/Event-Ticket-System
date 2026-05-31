package deploy

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestPrometheusServiceLevelRulesCoverSLISLOAndErrorBudget(t *testing.T) {
	rulesFile, err := os.ReadFile("observability/rules/cets-service-level.yml")
	require.NoError(t, err)

	var rules prometheusRulesFile
	require.NoError(t, yaml.Unmarshal(rulesFile, &rules))
	require.Len(t, rules.Groups, 1)
	assert.Equal(t, "cets-service-level-objectives", rules.Groups[0].Name)

	records := map[string]prometheusAlertRule{}
	for _, rule := range rules.Groups[0].Rules {
		records[rule.Record] = rule
	}
	requiredRecords := []string{
		"cets:sli_api_success_rate:ratio5m",
		"cets:sli_api_p99_latency_seconds:5m",
		"cets:slo_api_success_rate:target_ratio",
		"cets:slo_api_p99_latency:target_seconds",
		"cets:error_budget_api_success_rate:burn_rate5m",
		"cets:error_budget_api_success_rate:remaining_ratio5m",
	}
	for _, record := range requiredRecords {
		rule, ok := records[record]
		require.True(t, ok, "missing service-level recording rule %s", record)
		assert.NotEmpty(t, rule.Expr, "recording rule %s must have a PromQL expression", record)
	}

	combinedExpr := string(rulesFile)
	for _, fragment := range []string{
		"cets_http_requests_total{status_class=~\"5xx\"}",
		"cets_http_request_seconds_bucket",
		"vector(0.999)",
		"vector(1)",
		"/ 0.001",
		"clamp_min(1 - cets:error_budget_api_success_rate:burn_rate5m, 0)",
		"slo: cets-api-success-rate",
		"sli: api_success_rate",
	} {
		assert.Contains(t, combinedExpr, fragment, "service-level rules must include %q", fragment)
	}
}
