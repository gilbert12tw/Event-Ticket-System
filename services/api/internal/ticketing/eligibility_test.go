package ticketing

import "testing"

func TestEvaluateEligibilityEligible(t *testing.T) {
	employee := Employee{EmployeeID: "E1001", Department: "Engineering", Site: "Taipei", JobGrade: 6, EmploymentStatus: "active"}
	rule := EligibilityRule{Department: "Engineering", Site: "Taipei", MinGrade: 5, EmploymentStatus: "active"}

	ok, reason := EvaluateEligibility(employee, rule)

	if !ok {
		t.Fatalf("expected eligible, got reason %q", reason)
	}
}

func TestEvaluateEligibilityRejectsMismatchWithReason(t *testing.T) {
	employee := Employee{EmployeeID: "E1001", Department: "Sales", Site: "Taipei", JobGrade: 4, EmploymentStatus: "active"}
	rule := EligibilityRule{Department: "Engineering", Site: "Taipei", MinGrade: 5, EmploymentStatus: "active"}

	ok, reason := EvaluateEligibility(employee, rule)

	if ok {
		t.Fatal("expected ineligible")
	}
	if reason == "" {
		t.Fatal("expected rejection reason")
	}
}
