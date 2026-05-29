package ticketing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFreshnessFromProjection_Fresh(t *testing.T) {
	now := time.Now()
	updatedAt := now.Add(-30 * time.Second)
	threshold := 60

	meta := FreshnessFromProjection(&updatedAt, threshold)

	assert.False(t, meta.Degraded)
	assert.Equal(t, ReportSourceReportingProjection, meta.Source)
	assert.GreaterOrEqual(t, meta.LagSeconds, 29)
	assert.LessOrEqual(t, meta.LagSeconds, 31)
	assert.Equal(t, &updatedAt, meta.AsOf)
}

func TestFreshnessFromProjection_Stale(t *testing.T) {
	now := time.Now()
	updatedAt := now.Add(-90 * time.Second)
	threshold := 60

	meta := FreshnessFromProjection(&updatedAt, threshold)

	assert.True(t, meta.Degraded)
	assert.Equal(t, ReportSourceReportingProjection, meta.Source)
	assert.GreaterOrEqual(t, meta.LagSeconds, 89)
	assert.LessOrEqual(t, meta.LagSeconds, 91)
	assert.Equal(t, &updatedAt, meta.AsOf)
}

func TestFreshnessFromProjection_Missing(t *testing.T) {
	meta := FreshnessFromProjection(nil, 60)

	assert.True(t, meta.Degraded)
	assert.Equal(t, ReportSourceUnavailable, meta.Source)
	assert.Equal(t, -1, meta.LagSeconds)
	assert.Nil(t, meta.AsOf)
}

func TestFreshnessThresholdRespected(t *testing.T) {
	now := time.Now()
	updatedAt := now.Add(-30 * time.Second)
	threshold := 20

	meta := FreshnessFromProjection(&updatedAt, threshold)

	assert.True(t, meta.Degraded)
}
