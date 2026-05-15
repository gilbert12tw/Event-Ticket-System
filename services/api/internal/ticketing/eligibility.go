package ticketing

import (
	"fmt"
	"strings"
)

func EvaluateEligibility(employee Employee, rule EligibilityRule) (bool, string) {
	if employee.EmployeeID == "" {
		return false, "employee not found"
	}
	if rule.Department != "" && rule.Department != "*" && employee.Department != rule.Department {
		return false, fmt.Sprintf("department %s is not eligible", employee.Department)
	}
	normalizedEmployeeSite := normalizeLocation(employee.Site)
	normalizedRuleSite := normalizeLocation(rule.Site)
	if normalizedRuleSite != "" && normalizedRuleSite != "*" && normalizedEmployeeSite != normalizedRuleSite {
		return false, fmt.Sprintf("site %s is not eligible", employee.Site)
	}

	if rule.MinGrade > 0 && employee.JobGrade < rule.MinGrade {
		return false, fmt.Sprintf("job grade %d is below minimum %d", employee.JobGrade, rule.MinGrade)
	}
	if rule.EmploymentStatus != "" && rule.EmploymentStatus != "*" && employee.EmploymentStatus != rule.EmploymentStatus {
		return false, fmt.Sprintf("employment status %s is not eligible", employee.EmploymentStatus)
	}

	return true, ""
}

func normalizeLocation(input string) string {
	input = strings.TrimSpace(input)
	if input == "" || input == "*" {
		return input
	}
	inputLower := strings.ToLower(input)
	for _, c := range knownCities {
		if strings.Contains(inputLower, strings.ToLower(c)) {
			return c
		}
	}
	return input
}
