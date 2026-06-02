package ticketing

import (
	"context"
	"strings"
)

func (s *Service) AdminHROptions(ctx context.Context, actor Actor) (AdminHROptions, error) {
	if err := requireAnyRole(actor, RoleActivityAdmin, RoleHRAdmin, RoleSystemAdmin); err != nil {
		return AdminHROptions{}, err
	}
	rows, err := s.db.Query(ctx, `SELECT DISTINCT site FROM employees WHERE trim(site) <> '' ORDER BY site ASC`)
	if err != nil {
		return AdminHROptions{}, err
	}
	defer rows.Close()

	options := AdminHROptions{
		Sites: []AdminOption{{Value: "*", Label: "所有廠區"}},
	}
	for rows.Next() {
		var site string
		if err := rows.Scan(&site); err != nil {
			return AdminHROptions{}, err
		}
		site = strings.TrimSpace(site)
		if site == "" {
			continue
		}
		options.Sites = append(options.Sites, AdminOption{Value: site, Label: site})
	}
	return options, rows.Err()
}
