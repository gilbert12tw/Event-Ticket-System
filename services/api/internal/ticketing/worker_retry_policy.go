package ticketing

import (
	"hash/fnv"
	"strconv"
	"time"
)

type OutboxRetryPolicy struct {
	MaxAttempts int
	BackoffBase time.Duration
	BackoffMax  time.Duration
}

func outboxRetryPolicyFromOptions(options OutboxProcessorOptions) OutboxRetryPolicy {
	if options.RetryPolicy != nil {
		return normalizeOutboxRetryPolicy(*options.RetryPolicy)
	}
	maxAttempts := options.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultOutboxMaxAttempts
	}
	return OutboxRetryPolicy{
		MaxAttempts: maxAttempts,
		BackoffBase: time.Minute,
		BackoffMax:  time.Duration(maxAttempts) * time.Minute,
	}
}

func normalizeOutboxRetryPolicy(policy OutboxRetryPolicy) OutboxRetryPolicy {
	if policy.MaxAttempts < 0 {
		policy.MaxAttempts = 0
	}
	if policy.BackoffBase <= 0 {
		policy.BackoffBase = time.Minute
	}
	if policy.BackoffMax <= 0 {
		policy.BackoffMax = policy.BackoffBase
	}
	if policy.BackoffMax < policy.BackoffBase {
		policy.BackoffMax = policy.BackoffBase
	}
	return policy
}

func (p OutboxRetryPolicy) exhausted(attempts int) bool {
	if p.MaxAttempts <= 0 {
		return true
	}
	return attempts >= p.MaxAttempts
}

func (p OutboxRetryPolicy) backoff(attempts int) time.Duration {
	p = normalizeOutboxRetryPolicy(p)
	if attempts <= 0 {
		attempts = 1
	}
	backoff := p.BackoffBase
	for i := 1; i < attempts; i++ {
		if backoff >= p.BackoffMax/2 {
			return p.BackoffMax
		}
		backoff *= 2
		if backoff >= p.BackoffMax {
			return p.BackoffMax
		}
	}
	return backoff
}

func (p OutboxRetryPolicy) backoffForOutbox(outboxID string, attempts int) time.Duration {
	p = normalizeOutboxRetryPolicy(p)
	backoff := p.backoff(attempts)
	if backoff >= p.BackoffMax {
		return p.BackoffMax
	}
	jitterWindow := backoff / 4
	if jitterWindow <= 0 {
		return backoff
	}
	jittered := backoff + outboxRetryJitter(outboxID, attempts, jitterWindow)
	if jittered > p.BackoffMax {
		return p.BackoffMax
	}
	return jittered
}

func outboxRetryJitter(outboxID string, attempts int, window time.Duration) time.Duration {
	if window <= 0 {
		return 0
	}
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(outboxID))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(strconv.Itoa(attempts)))
	return time.Duration(hash.Sum64() % uint64(window))
}
