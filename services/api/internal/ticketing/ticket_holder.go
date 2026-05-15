package ticketing

func ticketHolderFromTicket(ticket Ticket) TicketHolder {
	return TicketHolder{
		DisplayName: ticket.EmployeeName,
		Department:  ticket.Department,
		City:        ticket.City,
	}
}
