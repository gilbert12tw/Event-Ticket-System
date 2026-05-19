package ticketing

func bookingNotificationPayload(reg Registration, event Event, actor Actor) map[string]interface{} {
	payload := map[string]interface{}{
		"registration_id": reg.RegistrationID,
		"event_id":        reg.EventID,
		"employee_id":     reg.EmployeeID,
		"capacity_type":   event.CapacityType,
		"family_count":    reg.FamilyCount,
	}
	if reg.Status == RegistrationConfirmed {
		addCrossCityNotificationContext(payload, event, actor)
	}
	return payload
}

func addCrossCityNotificationContext(payload map[string]interface{}, event Event, actor Actor) {
	if actor.Claims == nil {
		return
	}
	employeeCity := normalizeLocation(actor.Claims.City)
	eventCity := normalizeLocation(event.EventCity)
	if employeeCity == "" || eventCity == "" || employeeCity == eventCity {
		return
	}
	payload["warning_code"] = string(WarningCrossCity)
	payload["event_city"] = eventCity
}
