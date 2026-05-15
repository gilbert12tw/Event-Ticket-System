package ticketing

import "time"

type NoShowPolicy struct {
	Threshold        int
	CooldownDuration time.Duration
	GracePeriod      time.Duration
}

func DefaultNoShowPolicy() NoShowPolicy {
	return NoShowPolicy{
		Threshold:        1,
		CooldownDuration: 90 * 24 * time.Hour,
		GracePeriod:      24 * time.Hour,
	}
}

func (p NoShowPolicy) Normalize() NoShowPolicy {
	defaults := DefaultNoShowPolicy()
	if p.Threshold <= 0 {
		p.Threshold = defaults.Threshold
	}
	if p.CooldownDuration <= 0 {
		p.CooldownDuration = defaults.CooldownDuration
	}
	if p.GracePeriod <= 0 {
		p.GracePeriod = defaults.GracePeriod
	}
	return p
}
