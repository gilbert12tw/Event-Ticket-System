package observability

import (
	"io"
	"strings"
	"time"
)

type redisOpKey struct {
	Operation string
	Result    string
}

func (r *Registry) ObserveRedisOperation(operation string, result string, duration time.Duration) {
	if r == nil {
		return
	}
	key := redisOpKey{
		Operation: boundedRedisOperation(operation),
		Result:    boundedRedisResult(result),
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	h := r.redisOp[key]
	if h == nil {
		h = &histogram{Buckets: make([]uint64, len(httpBuckets))}
		r.redisOp[key] = h
	}
	observeDuration(h, duration.Seconds())
}

func (r *Registry) writeRedisOperationMetrics(w io.Writer) {
	writeLine(w, "# HELP cets_redis_operation_total Redis gate script calls by operation and result.")
	writeLine(w, "# TYPE cets_redis_operation_total counter")
	writeLine(w, "# HELP cets_redis_operation_seconds Redis gate script call duration by operation and result.")
	writeLine(w, "# TYPE cets_redis_operation_seconds histogram")

	keys, snapshots := sortedHistogramSnapshots(&r.mu, r.redisOp, redisOpLabelSet)
	for _, key := range keys {
		labels := redisOpLabelSet(key)
		h := snapshots[key]
		writeFormat(w, "cets_redis_operation_total{%s} %d\n", labels, h.Count)
		writeHistogramSeries(w, "cets_redis_operation_seconds", labels, h)
	}
}

func redisOpLabelSet(key redisOpKey) string {
	return `operation="` + escapeLabel(key.Operation) + `",result="` + escapeLabel(key.Result) + `"`
}

func boundedRedisOperation(op string) string {
	switch strings.TrimSpace(op) {
	case "reserve", "commit", "release",
		"compensation_release", "compensation_drop", "compensation_cap",
		"zrangebyscore", "scan":
		return op
	default:
		return "unknown"
	}
}

func boundedRedisResult(result string) string {
	switch strings.TrimSpace(result) {
	case "ok", "error", "timeout":
		return result
	default:
		return "unknown"
	}
}
