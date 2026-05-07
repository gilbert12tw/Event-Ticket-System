package ticketing

import "fmt"

func EvaluateEligibility(employee Employee, rule EligibilityRule) (bool, string) {
	if employee.EmployeeID == "" {
		return false, "employee not found"
	}
	if rule.Department != "" && rule.Department != "*" && employee.Department != rule.Department {
		return false, fmt.Sprintf("department %s is not eligible", employee.Department)
	}
	if rule.Site != "" && rule.Site != "*" && employee.Site != rule.Site {
		return false, fmt.Sprintf("site %s is not eligible", employee.Site)
	}
	if rule.MinGrade > 0 && employee.JobGrade < rule.MinGrade {
		return false, fmt.Sprintf("job grade %d is below minimum %d", employee.JobGrade, rule.MinGrade)
	}
	if rule.EmploymentStatus != "" && rule.EmploymentStatus != "*" && employee.EmploymentStatus != rule.EmploymentStatus {
		return false, fmt.Sprintf("employment status %s is not eligible", employee.EmploymentStatus)
	}
	return true, "eligible"
}

func normalizeRuleInput(input RuleInput) RuleInput {
	if input.Department == "" {
		input.Department = "*"
	}
	if input.Site == "" {
		input.Site = "*"
	}
	if input.EmploymentStatus == "" {
		input.EmploymentStatus = "active"
	}
	return input
}
