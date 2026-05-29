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

func recipientEmployeeIDForOutbox(eventType string, payload map[string]interface{}) string {
	if eventType == "notification.requested.v2" {
		if employeeID := stringFromPayload(payload, "recipient_employee_id"); employeeID != "" {
			return employeeID
		}
	}
	return stringFromPayload(payload, "employee_id")
}

type outboxDeliveryChannels struct {
	inApp bool
	email bool
	valid bool
}

func deliveryChannelsForOutbox(eventType string, payload map[string]interface{}, senderAvailable bool) outboxDeliveryChannels {
	if eventType != "notification.requested.v2" {
		return outboxDeliveryChannels{inApp: true, email: senderAvailable, valid: true}
	}
	switch stringFromPayload(payload, "channel") {
	case "email":
		return outboxDeliveryChannels{email: senderAvailable, valid: true}
	case "in_app":
		return outboxDeliveryChannels{inApp: true, valid: true}
	default:
		return outboxDeliveryChannels{}
	}
}
