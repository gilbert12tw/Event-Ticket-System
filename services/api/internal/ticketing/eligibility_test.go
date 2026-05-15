package ticketing

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluateEligibilityEligible(t *testing.T) {
	employee := Employee{EmployeeID: "E1001", Department: "Engineering", Site: "Taipei HQ", JobGrade: 6, EmploymentStatus: "active"}
	rule := EligibilityRule{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"}

	ok, reason := EvaluateEligibility(employee, rule)

	require.True(t, ok, "expected eligible, got reason %q", reason)
}

func TestEvaluateEligibilityRejectsMismatchWithReason(t *testing.T) {
	employee := Employee{EmployeeID: "E1001", Department: "Sales", Site: "Taipei HQ", JobGrade: 4, EmploymentStatus: "active"}
	rule := EligibilityRule{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"}

	ok, reason := EvaluateEligibility(employee, rule)

	require.False(t, ok, "expected ineligible")
	assert.NotEmpty(t, reason, "expected rejection reason")
}

func TestEvaluateEligibilityNormalizedSite(t *testing.T) {
	employee := Employee{EmployeeID: "E1001", Department: "Engineering", Site: "Taipei HQ", JobGrade: 6, EmploymentStatus: "active"}
	rule := EligibilityRule{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"}

	ok, reason := EvaluateEligibility(employee, rule)

	require.True(t, ok, "expected eligible with normalization, got reason %q", reason)
}

