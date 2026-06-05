package ticketing

import "time"

const checkinNotStartedReason = "event_not_started"

func checkinBusinessLocation() *time.Location {
	return time.FixedZone("Asia/Taipei", 8*60*60)
}

func isCheckinAfterEventStart(eventStartsAt time.Time, scannedAt time.Time) bool {
	if eventStartsAt.IsZero() || scannedAt.IsZero() {
		return false
	}
	return !scannedAt.Before(eventStartsAt)
}
