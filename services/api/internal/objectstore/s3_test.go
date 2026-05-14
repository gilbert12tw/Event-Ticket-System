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
)

type recordingRoundTripper struct {
	statusCode int
	response   string
	request    *http.Request
	body       string
}

func (t *recordingRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	t.request = request
	t.body = string(body)
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
