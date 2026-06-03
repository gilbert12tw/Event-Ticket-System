package ticketing

func bookingOutcomeIfFound(current string, result bookingTxResult, found bool, err error) string {
	if err == nil && found {
		return result.status
	}
	return current
}

func outcomeForError(err error) string {
	if err != nil {
		return "error"
	}
	return "success"
}

func remainingForNewBooking(event Event, status string, capacity int, confirmedCount int) int {
	if event.CapacityType == CapacityTypeLimited && status == RegistrationConfirmed {
		return max(capacity-confirmedCount-1, 0)
	}
	if event.CapacityType == CapacityTypeLimited && status == RegistrationReceived {
		return max(capacity-confirmedCount, 0)
	}
	return 0
}
