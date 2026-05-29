package main

import (
	"math"
	"sort"
)

type result struct {
	Mode             string    `json:"mode"`
	VUs              int       `json:"vus"`
	Capacity         int       `json:"capacity"`
	WallClockMS      float64   `json:"wall_clock_ms"`
	RPS              float64   `json:"rps"`
	Outcomes         outcomes  `json:"outcomes"`
	LatencyMS        latencies `json:"latency_ms"`
	DBConfirmedCount int       `json:"db_confirmed_count"`
}

type outcomes struct {
	Confirmed  int `json:"confirmed"`
	Waitlisted int `json:"waitlisted"`
	Error      int `json:"error"`
}

type latencies struct {
	Min  float64   `json:"min"`
	P50  float64   `json:"p50"`
	P90  float64   `json:"p90"`
	P95  float64   `json:"p95"`
	P99  float64   `json:"p99"`
	Max  float64   `json:"max"`
	Mean float64   `json:"mean"`
	All  []float64 `json:"all_ms"`
}

func summarize(xs []float64) latencies {
	if len(xs) == 0 {
		return latencies{}
	}
	cp := make([]float64, len(xs))
	copy(cp, xs)
	sort.Float64s(cp)
	sum := 0.0
	for _, v := range cp {
		sum += v
	}
	return latencies{
		Min:  cp[0],
		P50:  percentile(cp, 0.50),
		P90:  percentile(cp, 0.90),
		P95:  percentile(cp, 0.95),
		P99:  percentile(cp, 0.99),
		Max:  cp[len(cp)-1],
		Mean: sum / float64(len(cp)),
		All:  cp,
	}
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
