package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	obaggage "go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/propagation"
)

func TestWithTraceIDBridgesCustomTraceIDToOTelBaggage(t *testing.T) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	defer otel.SetTextMapPropagator(propagation.TraceContext{})

	var capturedBag string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bag := obaggage.FromContext(r.Context())
		capturedBag = bag.Member("cets.trace_id").Value()
		w.WriteHeader(http.StatusOK)
	})
	handler := withTraceID(inner)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Trace-ID", "my-custom-trace")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, "my-custom-trace", capturedBag)
}
