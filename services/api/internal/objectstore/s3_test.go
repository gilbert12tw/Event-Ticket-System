package objectstore

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
)

type recordingRoundTripper struct {
	statusCode int
	response   string
	request    *http.Request
	body       string
}

func (t *recordingRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Body != nil {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		t.body = string(body)
	}
	t.request = request
	statusCode := t.statusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(t.response)),
		Request:    request,
	}, nil
}

func TestS3CompatibleStorePutSignsAndUploadsObject(t *testing.T) {
	transport := &recordingRoundTripper{}

	store := S3CompatibleStore{
		Endpoint:  "http://minio:9000",
		Bucket:    "cets-dev",
		Region:    "us-east-1",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin_dev_password",
		Client:    &http.Client{Transport: transport},
		now:       func() time.Time { return time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC) },
	}

	require.NoError(t, store.Put(context.Background(), "exports/report 1.csv", "text/csv", []byte("event_id,title\n")))
	assert.Equal(t, "/cets-dev/exports/report%201.csv", transport.request.URL.EscapedPath())
	assert.Contains(t, transport.request.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=minioadmin/20260506/us-east-1/s3/aws4_request")
	assert.NotEmpty(t, transport.request.Header.Get("X-Amz-Content-Sha256"), "expected payload hash")
	assert.Equal(t, "event_id,title\n", transport.body)
}

func TestS3CompatibleStorePutReturnsStorageError(t *testing.T) {
	transport := &recordingRoundTripper{statusCode: http.StatusServiceUnavailable, response: "bucket unavailable"}

	store := S3CompatibleStore{
		Endpoint:  "http://minio:9000",
		Bucket:    "cets-dev",
		Region:    "us-east-1",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin_dev_password",
		Client:    &http.Client{Transport: transport},
	}

	err := store.Put(context.Background(), "exports/report.csv", "text/csv", []byte("event_id,title\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status=503")
}

func TestS3CompatibleStoreExistsSignsHeadRequest(t *testing.T) {
	transport := &recordingRoundTripper{}
	store := S3CompatibleStore{
		Endpoint:  "http://minio:9000",
		Bucket:    "cets-dev",
		Region:    "us-east-1",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin_dev_password",
		Client:    &http.Client{Transport: transport},
		now:       func() time.Time { return time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC) },
	}

	exists, err := store.Exists(context.Background(), "exports/report 1.csv")

	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, http.MethodHead, transport.request.Method)
	assert.Equal(t, "/cets-dev/exports/report%201.csv", transport.request.URL.EscapedPath())
	assert.Contains(t, transport.request.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=minioadmin/20260506/us-east-1/s3/aws4_request")
	assert.Empty(t, transport.body)
}

func TestS3CompatibleStoreExistsReturnsFalseOnNotFound(t *testing.T) {
	transport := &recordingRoundTripper{statusCode: http.StatusNotFound}
	store := S3CompatibleStore{
		Endpoint:  "http://minio:9000",
		Bucket:    "cets-dev",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin_dev_password",
		Client:    &http.Client{Transport: transport},
	}

	exists, err := store.Exists(context.Background(), "exports/missing.csv")

	require.NoError(t, err)
	assert.False(t, exists)
}

func TestS3CompatibleStoreGetSignsAndDownloadsObject(t *testing.T) {
	transport := &recordingRoundTripper{response: "event_id,title\n"}
	store := S3CompatibleStore{
		Endpoint:  "http://minio:9000",
		Bucket:    "cets-dev",
		Region:    "us-east-1",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin_dev_password",
		Client:    &http.Client{Transport: transport},
		now:       func() time.Time { return time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC) },
	}

	body, _, err := store.Get(context.Background(), "exports/report 1.csv")

	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, transport.request.Method)
	assert.Equal(t, "/cets-dev/exports/report%201.csv", transport.request.URL.EscapedPath())
	assert.Contains(t, transport.request.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=minioadmin/20260506/us-east-1/s3/aws4_request")
	assert.Equal(t, "event_id,title\n", string(body))
	assert.Empty(t, transport.body)
}

func TestS3CompatibleStoreTracesMinioWithoutSensitiveValues(t *testing.T) {
	exporter, shutdown := installObjectStoreTraceExporter(t)
	defer shutdown()
	transport := &recordingRoundTripper{}
	store := S3CompatibleStore{
		Endpoint:  "http://minio:9000",
		Bucket:    "cets-dev",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin_dev_password",
		Client:    &http.Client{Transport: transport},
	}

	require.NoError(t, store.Put(context.Background(), "events/evt_secret/poster.png", "image/png", []byte("secret-poster-body")))

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "s3.put", spans[0].Name)
	assert.Contains(t, spans[0].Attributes, attribute.String("peer.service", "minio"))
	assert.Contains(t, spans[0].Attributes, attribute.String("server.address", "minio"))
	assert.Contains(t, spans[0].Attributes, attribute.Int("http.response.status_code", http.StatusOK))
	for _, attr := range spans[0].Attributes {
		value := attr.Value.AsString()
		assert.NotContains(t, value, "evt_secret")
		assert.NotContains(t, value, "poster.png")
		assert.NotContains(t, value, "secret-poster-body")
		assert.NotContains(t, value, "minioadmin_dev_password")
	}
}

func installObjectStoreTraceExporter(t *testing.T) (*tracetest.InMemoryExporter, func()) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(provider)
	return exporter, func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(noop.NewTracerProvider())
	}
}
