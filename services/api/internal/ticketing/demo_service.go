package ticketing

import "context"

func (s *Service) SeedDemoData(ctx context.Context) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	
	// Ensure a clean slate for demo data to prevent stale state from previous runs
	if _, err := tx.Exec(ctx, "TRUNCATE employees CASCADE"); err != nil {
		return err
	}

	employees := []Employee{
		{EmployeeID: "E1001", FullName: "Ariel Chen", Department: "Engineering", Site: "Taipei HQ", JobGrade: 6, EmploymentStatus: "active"},
		{EmployeeID: "E1002", FullName: "Ben Lin", Department: "Engineering", Site: "Taipei HQ", JobGrade: 5, EmploymentStatus: "active"},
		{EmployeeID: "E2001", FullName: "Carla Wu", Department: "Sales", Site: "Taipei HQ", JobGrade: 4, EmploymentStatus: "active"},
	}
	for _, employee := range employees {
		_, err := tx.Exec(ctx, `INSERT INTO employees (employee_id, full_name, department, site, job_grade, employment_status)
			VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (employee_id) DO UPDATE SET full_name = EXCLUDED.full_name, department = EXCLUDED.department, site = EXCLUDED.site, job_grade = EXCLUDED.job_grade, employment_status = EXCLUDED.employment_status`,
			employee.EmployeeID, employee.FullName, employee.Department, employee.Site, employee.JobGrade, employee.EmploymentStatus)
		if err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}
