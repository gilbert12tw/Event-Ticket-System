package observability

import (
	"io"
	"runtime"
	"strconv"
	"syscall"
)

func (r *Registry) writeRuntimeResourceMetrics(w io.Writer) {
	writeLine(w, "# HELP process_cpu_seconds_total Total user and system CPU time spent in seconds.")
	writeLine(w, "# TYPE process_cpu_seconds_total counter")
	writeFormat(w, "process_cpu_seconds_total{%s} %s\n",
		resourceLabelSet(r.identity),
		strconv.FormatFloat(processCPUSeconds(), 'f', -1, 64))

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	writeLine(w, "# HELP go_memstats_heap_alloc_bytes Bytes of allocated heap objects.")
	writeLine(w, "# TYPE go_memstats_heap_alloc_bytes gauge")
	writeFormat(w, "go_memstats_heap_alloc_bytes{%s} %d\n",
		resourceLabelSet(r.identity), mem.HeapAlloc)
}

func resourceLabelSet(identity registryIdentity) string {
	return `service="` + escapeLabel(identity.Service) + `",replica="` + escapeLabel(identity.Replica) + `"`
}

func processCPUSeconds() float64 {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		return 0
	}
	return timevalSeconds(usage.Utime) + timevalSeconds(usage.Stime)
}

func timevalSeconds(value syscall.Timeval) float64 {
	return float64(value.Sec) + float64(value.Usec)/1_000_000
}
