package ticketing

import (
	"strings"
	"sync"
	"time"
)

const (
	DemoClockModeReal  = "real"
	DemoClockModeFixed = "fixed"
)

type DemoClockSnapshot struct {
	Enabled   bool       `json:"enabled"`
	Mode      string     `json:"mode"`
	Now       time.Time  `json:"now"`
	RealNow   time.Time  `json:"real_now"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	Reason    string     `json:"reason,omitempty"`
}

type DemoClock struct {
	mu        sync.RWMutex
	realNow   func() time.Time
	fixed     bool
	fixedAt   time.Time
	updated   bool
	updatedAt time.Time
	reason    string
}

func NewDemoClock() *DemoClock {
	return NewDemoClockWithRealNow(func() time.Time { return time.Now().UTC() })
}

func NewDemoClockWithRealNow(realNow func() time.Time) *DemoClock {
	if realNow == nil {
		realNow = func() time.Time { return time.Now().UTC() }
	}
	return &DemoClock{realNow: func() time.Time { return realNow().UTC() }}
}

func (c *DemoClock) Now() time.Time {
	if c == nil {
		return time.Now().UTC()
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.fixed {
		return c.fixedAt
	}
	return c.realNow()
}

func (c *DemoClock) Snapshot() DemoClockSnapshot {
	if c == nil {
		now := time.Now().UTC()
		return DemoClockSnapshot{Enabled: false, Mode: DemoClockModeReal, Now: now, RealNow: now}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.snapshotLocked()
}

func (c *DemoClock) UseReal(reason string) DemoClockSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fixed = false
	c.fixedAt = time.Time{}
	c.updated = true
	c.updatedAt = c.realNow()
	c.reason = strings.TrimSpace(reason)
	return c.snapshotLocked()
}

func (c *DemoClock) UseFixed(now time.Time, reason string) DemoClockSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fixed = true
	c.fixedAt = now.UTC()
	c.updated = true
	c.updatedAt = c.realNow()
	c.reason = strings.TrimSpace(reason)
	return c.snapshotLocked()
}

func (c *DemoClock) snapshotLocked() DemoClockSnapshot {
	realNow := c.realNow()
	mode := DemoClockModeReal
	now := realNow
	if c.fixed {
		mode = DemoClockModeFixed
		now = c.fixedAt
	}
	var updatedAt *time.Time
	if c.updated {
		value := c.updatedAt
		updatedAt = &value
	}
	return DemoClockSnapshot{
		Enabled:   true,
		Mode:      mode,
		Now:       now,
		RealNow:   realNow,
		UpdatedAt: updatedAt,
		Reason:    c.reason,
	}
}
