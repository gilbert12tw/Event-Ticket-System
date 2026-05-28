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

	assert.False(t, meta.IsStale)
	assert.Equal(t, "read_model", meta.Source)
	assert.GreaterOrEqual(t, meta.ReadModelLagSeconds, 29)
	assert.LessOrEqual(t, meta.ReadModelLagSeconds, 31)
	assert.Equal(t, &updatedAt, meta.GeneratedAt)
}

func TestFreshnessFromProjection_Stale(t *testing.T) {
	now := time.Now()
	updatedAt := now.Add(-90 * time.Second)
	threshold := 60

	meta := FreshnessFromProjection(&updatedAt, threshold)

	assert.True(t, meta.IsStale)
	assert.Equal(t, "read_model", meta.Source)
	assert.GreaterOrEqual(t, meta.ReadModelLagSeconds, 89)
	assert.LessOrEqual(t, meta.ReadModelLagSeconds, 91)
	assert.Equal(t, &updatedAt, meta.GeneratedAt)
}

func TestFreshnessFromProjection_Missing(t *testing.T) {
	meta := FreshnessFromProjection(nil, 60)

	assert.True(t, meta.IsStale)
	assert.Equal(t, "unavailable", meta.Source)
	assert.Equal(t, -1, meta.ReadModelLagSeconds)
	assert.Nil(t, meta.GeneratedAt)
}

func TestFreshnessThresholdRespected(t *testing.T) {
	now := time.Now()
	updatedAt := now.Add(-30 * time.Second)
	threshold := 20

	meta := FreshnessFromProjection(&updatedAt, threshold)

	assert.True(t, meta.IsStale)
}
