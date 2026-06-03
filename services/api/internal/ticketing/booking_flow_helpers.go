package ticketing

func bookingOutcomeIfFound(current string, result bookingTxResult, found bool, err error) string {
	if err == nil && found {
		return result.status
	}
	return current
}
