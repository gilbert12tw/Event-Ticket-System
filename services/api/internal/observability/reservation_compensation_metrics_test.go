package observability

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReservationCompensationMetricsExposeActionAndDriftCounters(t *testing.T) {
	db := fakeSQLMetricsDB{
		reservationCompensationRows: [][]any{
			{"compensation", "release", "released", int64(5)},
			{"compensation", "drop", "dropped", int64(2)},
			{"compensation", "lookup", "error", int64(1)},
			{"counter_drift", "", "capped", int64(3)},
			{"counter_drift", "", "unchanged", int64(9)},
		},
	}

	var body bytes.Buffer
	NewRegistry().WritePrometheus(context.Background(), &body, db)
	metrics := body.String()

	assert.Contains(t, metrics, `# TYPE cets_reservation_compensation_total counter`)
	assert.Contains(t, metrics, `# TYPE cets_reservation_counter_drift_total counter`)
	assert.Contains(t, metrics, `cets_reservation_compensation_total{action="release",result="released"} 5`)
	assert.Contains(t, metrics, `cets_reservation_compensation_total{action="drop",result="dropped"} 2`)
	assert.Contains(t, metrics, `cets_reservation_compensation_total{action="lookup",result="error"} 1`)
	assert.Contains(t, metrics, `cets_reservation_counter_drift_total{result="capped"} 3`)
	assert.Contains(t, metrics, `cets_reservation_counter_drift_total{result="unchanged"} 9`)
}

func TestReservationCompensationMetricsBoundUnsafeLabels(t *testing.T) {
	db := fakeSQLMetricsDB{
		reservationCompensationRows: [][]any{
			// An unexpected action/result (e.g. accidental PII or injection)
			// must collapse to "unknown", never widen metric cardinality.
			{"compensation", "evil e1001@cets.local", "token eyJhbGci", int64(4)},
			{"counter_drift", "", "drifted e2002@cets.local", int64(7)},
		},
	}

	var body bytes.Buffer
	NewRegistry().WritePrometheus(context.Background(), &body, db)
	metrics := body.String()

	assert.Contains(t, metrics, `cets_reservation_compensation_total{action="unknown",result="unknown"} 4`)
	assert.Contains(t, metrics, `cets_reservation_counter_drift_total{result="unknown"} 7`)
	assert.NotContains(t, metrics, "e1001@cets.local")
	assert.NotContains(t, metrics, "e2002@cets.local")
	assert.NotContains(t, metrics, "eyJhbGci")
}
