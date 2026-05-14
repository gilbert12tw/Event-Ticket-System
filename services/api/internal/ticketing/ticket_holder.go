package ticketing

func ticketHolderFromTicket(ticket Ticket) TicketHolder {
	return TicketHolder{
		DisplayName: ticket.EmployeeName,
		Department:  ticket.Department,
		City:        ticket.City,
	}
}

func ticketHolderFromEmployee(employee Employee) TicketHolder {
	return TicketHolder{
		DisplayName: employee.FullName,
		Department:  employee.Department,
		City:        employee.Site,
	}
}
