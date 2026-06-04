package observability

import (
	"io"
	"sort"
	"strconv"
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

	r.mu.Lock()
	keys := make([]redisOpKey, 0, len(r.redisOp))
	for key := range r.redisOp {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return redisOpLabelSet(keys[i]) < redisOpLabelSet(keys[j])
	})
	snapshots := make(map[redisOpKey]histogram, len(keys))
	for _, key := range keys {
		current := r.redisOp[key]
		snapshots[key] = histogram{
			Buckets: append([]uint64(nil), current.Buckets...),
			Count:   current.Count,
			Sum:     current.Sum,
		}
	}
	r.mu.Unlock()

	for _, key := range keys {
		labels := redisOpLabelSet(key)
		h := snapshots[key]
		writeFormat(w, "cets_redis_operation_total{%s} %d\n", labels, h.Count)
		for i, bucket := range httpBuckets {
			writeFormat(w, "cets_redis_operation_seconds_bucket{%s,le=%q} %d\n", labels, formatBucket(bucket), h.Buckets[i])
		}
		writeFormat(w, "cets_redis_operation_seconds_bucket{%s,le=\"+Inf\"} %d\n", labels, h.Count)
		writeFormat(w, "cets_redis_operation_seconds_sum{%s} %s\n", labels, strconv.FormatFloat(h.Sum, 'f', -1, 64))
		writeFormat(w, "cets_redis_operation_seconds_count{%s} %d\n", labels, h.Count)
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
