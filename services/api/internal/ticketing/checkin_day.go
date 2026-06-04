package ticketing

import "time"

const checkinNotEventDayReason = "event_not_checkin_day"

func checkinBusinessLocation() *time.Location {
	return time.FixedZone("Asia/Taipei", 8*60*60)
}

func isCheckinOnEventDay(eventStartsAt time.Time, scannedAt time.Time) bool {
	if eventStartsAt.IsZero() || scannedAt.IsZero() {
		return false
	}
	eventDay := eventStartsAt.In(checkinBusinessLocation())
	scanDay := scannedAt.In(checkinBusinessLocation())
	return eventDay.Year() == scanDay.Year() && eventDay.YearDay() == scanDay.YearDay()
}
