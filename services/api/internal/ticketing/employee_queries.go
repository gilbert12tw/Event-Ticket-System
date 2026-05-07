package ticketing

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func (s *Service) getEmployee(ctx context.Context, employeeID string) (Employee, error) {
	return scanEmployee(s.db.QueryRow(ctx, `SELECT employee_id, full_name, department, site, job_grade, employment_status FROM employees WHERE employee_id = $1`, employeeID))
}

func (s *Service) getEmployeeTx(ctx context.Context, tx pgx.Tx, employeeID string) (Employee, error) {
	return scanEmployee(tx.QueryRow(ctx, `SELECT employee_id, full_name, department, site, job_grade, employment_status FROM employees WHERE employee_id = $1`, employeeID))
}

func scanEmployee(row pgx.Row) (Employee, error) {
	var employee Employee
	err := row.Scan(&employee.EmployeeID, &employee.FullName, &employee.Department, &employee.Site, &employee.JobGrade, &employee.EmploymentStatus)
	return employee, err
}
