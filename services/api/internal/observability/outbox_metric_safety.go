package observability

import (
	"sort"
	"strings"
)

type outboxMetricKey struct {
	EventType  string
	WorkerKind string
	Status     string
}

type outboxMetricAggregate struct {
	Key             outboxMetricKey
	Count           int64
	OldestLag       float64
	RetryCount      int64
	DeadLetterCount int64
	LeaseHeld       float64
}

type outboxLagHistogramKey struct {
	EventType  string
	WorkerKind string
}

type outboxLagHistogramAggregate struct {
	Key     outboxLagHistogramKey
	Buckets []int64
	Count   int64
	Sum     float64
}

func normalizeOutboxMetricKey(eventType string, workerKind string, status string) outboxMetricKey {
	return outboxMetricKey{
		EventType:  safeOutboxMetricEventType(eventType),
		WorkerKind: safeOutboxMetricWorkerKind(eventType, workerKind),
		Status:     safeOutboxMetricStatus(status),
	}
}

func normalizeOutboxLagHistogramKey(eventType string, workerKind string) outboxLagHistogramKey {
	return outboxLagHistogramKey{
		EventType:  safeOutboxMetricEventType(eventType),
		WorkerKind: safeOutboxMetricWorkerKind(eventType, workerKind),
	}
}

func safeOutboxMetricWorkerKind(eventType string, workerKind string) string {
	if safeOutboxMetricEventType(eventType) == "unknown" {
		return "unknown"
	}
	return safeWorkerMetricKind(workerKind)
}

func safeOutboxMetricStatus(status string) string {
	switch strings.TrimSpace(status) {
	case "pending", "processing", "dead_letter":
		return strings.TrimSpace(status)
	default:
		return "unknown"
	}
}

func addOutboxMetricAggregate(aggregates map[outboxMetricKey]outboxMetricAggregate, key outboxMetricKey, count int64, oldestLag float64, retryCount int64, deadLetterCount int64, leaseHeld float64) {
	aggregate := aggregates[key]
	aggregate.Key = key
	aggregate.Count += count
	if oldestLag > aggregate.OldestLag {
		aggregate.OldestLag = oldestLag
	}
	aggregate.RetryCount += retryCount
	aggregate.DeadLetterCount += deadLetterCount
	if leaseHeld > aggregate.LeaseHeld {
		aggregate.LeaseHeld = leaseHeld
	}
	aggregates[key] = aggregate
}

func sortedOutboxMetricAggregates(aggregates map[outboxMetricKey]outboxMetricAggregate) []outboxMetricAggregate {
	rows := make([]outboxMetricAggregate, 0, len(aggregates))
	for _, aggregate := range aggregates {
		rows = append(rows, aggregate)
	}
	sort.Slice(rows, func(i, j int) bool {
		left := rows[i].Key
		right := rows[j].Key
		if left.WorkerKind != right.WorkerKind {
			return left.WorkerKind < right.WorkerKind
		}
		if left.EventType != right.EventType {
			return left.EventType < right.EventType
		}
		return left.Status < right.Status
	})
	return rows
}

func addOutboxLagHistogramAggregate(aggregates map[outboxLagHistogramKey]outboxLagHistogramAggregate, key outboxLagHistogramKey, bucketCounts []int64, count int64, sum float64) {
	aggregate := aggregates[key]
	aggregate.Key = key
	if aggregate.Buckets == nil {
		aggregate.Buckets = make([]int64, len(bucketCounts))
	}
	for i, bucketCount := range bucketCounts {
		aggregate.Buckets[i] += bucketCount
	}
	aggregate.Count += count
	aggregate.Sum += sum
	aggregates[key] = aggregate
}

func sortedOutboxLagHistogramAggregates(aggregates map[outboxLagHistogramKey]outboxLagHistogramAggregate) []outboxLagHistogramAggregate {
	rows := make([]outboxLagHistogramAggregate, 0, len(aggregates))
	for _, aggregate := range aggregates {
		rows = append(rows, aggregate)
	}
	sort.Slice(rows, func(i, j int) bool {
		left := rows[i].Key
		right := rows[j].Key
		if left.WorkerKind != right.WorkerKind {
			return left.WorkerKind < right.WorkerKind
		}
		return left.EventType < right.EventType
	})
	return rows
}
