package ticketing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestOutboxRetryPolicyAppliesDeterministicJitter(t *testing.T) {
	policy := OutboxRetryPolicy{
		MaxAttempts: 5,
		BackoffBase: time.Second,
		BackoffMax:  10 * time.Second,
	}
	base := policy.backoff(2)

	first := policy.backoffForOutbox("outbox-alpha", 2)
	second := policy.backoffForOutbox("outbox-alpha", 2)
	other := policy.backoffForOutbox("outbox-beta", 2)

	assert.Equal(t, second, first)
	assert.NotEqual(t, other, first)
	assert.GreaterOrEqual(t, first, base)
	assert.LessOrEqual(t, first, base+base/4)
	assert.GreaterOrEqual(t, other, base)
	assert.LessOrEqual(t, other, base+base/4)
}

func TestOutboxRetryPolicyJitterDoesNotExceedBackoffMax(t *testing.T) {
	policy := OutboxRetryPolicy{
		MaxAttempts: 5,
		BackoffBase: time.Second,
		BackoffMax:  2 * time.Second,
	}

	got := policy.backoffForOutbox("outbox-capped", 3)

	assert.Equal(t, 2*time.Second, got)
}
