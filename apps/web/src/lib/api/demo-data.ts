type DemoEmployee = {
  employee_id: string;
  full_name: string;
  department: string;
  site: string;
  job_grade: number;
  employment_status: "active";
};

function activeEmployee(
  employee_id: string,
  full_name: string,
  job_grade: number,
  options: Partial<Pick<DemoEmployee, "department" | "site">> = {},
): DemoEmployee {
  return {
    employee_id,
    full_name,
    department: options.department ?? "Engineering",
    site: options.site ?? "Taipei HQ",
    job_grade,
    employment_status: "active",
  };
}

export const employees = [
  activeEmployee("E1001", "Ariel Chen", 6),
  activeEmployee("E1002", "Ben Lin", 5),
  activeEmployee("E1003", "Tina Chang", 5),
  activeEmployee("E2001", "Carla Wu", 4, { department: "Sales" }),
  activeEmployee("E1004", "David Tan", 5),
  activeEmployee("E1005", "Emily Liu", 6),
  activeEmployee("E1006", "Frank Wang", 5),
  activeEmployee("E1007", "Grace Huang", 5),
  activeEmployee("E1008", "Henry Cheng", 5),
  activeEmployee("E1009", "Iris Yang", 6),
  activeEmployee("E1010", "Jack Kao", 5),
  activeEmployee("E3001", "Tainan User", 5, { site: "Tainan HQ" }),
];
