package objectstore

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"event-ticket-system/internal/observability"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	amzContentSHA256Header = "X-Amz-Content-Sha256"
	amzDateHeader          = "X-Amz-Date"
)

type S3CompatibleStore struct {
	Endpoint  string
	Bucket    string
	Region    string
	AccessKey string
	SecretKey string
	Client    *http.Client
	now       func() time.Time
}

func (s S3CompatibleStore) Put(ctx context.Context, key string, contentType string, body []byte) error {
	endpoint, canonicalURI, region, now, err := s.requestParts(key)
	if err != nil {
		return err
	}
	ctx, span := observability.StartDependencySpan(ctx, observability.DependencySpanConfig{
		System:      "s3",
		ServiceName: "minio",
		Operation:   "put",
	})
	defer func() { observability.EndDependencySpan(span, err) }()
	request, err := s.newSignedRequest(ctx, http.MethodPut, endpoint, canonicalURI, region, now, body)
	if err != nil {
		return err
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	request.Header.Set("Content-Type", contentType)

	response, err := s.httpClient().Do(request)
	if err != nil {
		return err
	}
	observability.SetDependencyHTTPStatus(span, response.StatusCode)
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
	err = fmt.Errorf("object storage put failed: status=%d body=%s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	return err
}

func (s S3CompatibleStore) Exists(ctx context.Context, key string) (bool, error) {
	endpoint, canonicalURI, region, now, err := s.requestParts(key)
	if err != nil {
		return false, err
	}
	ctx, span := observability.StartDependencySpan(ctx, observability.DependencySpanConfig{
		System:      "s3",
		ServiceName: "minio",
		Operation:   "head",
	})
	defer func() { observability.EndDependencySpan(span, err) }()
	request, err := s.newSignedRequest(ctx, http.MethodHead, endpoint, canonicalURI, region, now, nil)
	if err != nil {
		return false, err
	}
	response, err := s.httpClient().Do(request)
	if err != nil {
		return false, err
	}
	observability.SetDependencyHTTPStatus(span, response.StatusCode)
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return true, nil
	}
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
	err = fmt.Errorf("object storage head failed: status=%d body=%s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	return false, err
}

func (s S3CompatibleStore) Get(ctx context.Context, key string) ([]byte, string, error) {
	endpoint, canonicalURI, region, now, err := s.requestParts(key)
	if err != nil {
		return nil, "", err
	}
	ctx, span := observability.StartDependencySpan(ctx, observability.DependencySpanConfig{
		System:      "s3",
		ServiceName: "minio",
		Operation:   "get",
	})
	defer func() { observability.EndDependencySpan(span, err) }()
	request, err := s.newSignedRequest(ctx, http.MethodGet, endpoint, canonicalURI, region, now, nil)
	if err != nil {
		return nil, "", err
	}
	response, err := s.httpClient().Do(request)
	if err != nil {
		return nil, "", err
	}
	observability.SetDependencyHTTPStatus(span, response.StatusCode)
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		var body []byte
		body, err = io.ReadAll(response.Body)
		return body, response.Header.Get("Content-Type"), err
	}
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
	err = fmt.Errorf("object storage get failed: status=%d body=%s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	return nil, "", err
}

func (s S3CompatibleStore) requestParts(key string) (*url.URL, string, string, time.Time, error) {
	if strings.TrimSpace(key) == "" {
		return nil, "", "", time.Time{}, fmt.Errorf("object key is required")
	}
	endpoint, err := url.Parse(strings.TrimRight(s.Endpoint, "/"))
	if err != nil {
		return nil, "", "", time.Time{}, err
	}
	if endpoint.Scheme == "" || endpoint.Host == "" {
		return nil, "", "", time.Time{}, fmt.Errorf("object storage endpoint must include scheme and host")
	}
	if strings.TrimSpace(s.Bucket) == "" {
		return nil, "", "", time.Time{}, fmt.Errorf("object storage bucket is required")
	}
	if strings.TrimSpace(s.AccessKey) == "" || strings.TrimSpace(s.SecretKey) == "" {
		return nil, "", "", time.Time{}, fmt.Errorf("object storage credentials are required")
	}
	region := strings.TrimSpace(s.Region)
	if region == "" {
		region = "us-east-1"
	}
	now := time.Now().UTC()
	if s.now != nil {
		now = s.now().UTC()
	}
	objectPath := "/" + strings.Trim(s.Bucket, "/") + "/" + strings.TrimLeft(key, "/")
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + objectPath
	endpoint.RawQuery = ""
	return endpoint, endpoint.EscapedPath(), region, now, nil
}

// newSignedRequest builds a request with the SigV4 payload hash, date, and
// Authorization headers set. body must be nil for bodyless methods so the
// empty-payload hash is signed.
func (s S3CompatibleStore) newSignedRequest(ctx context.Context, method string, endpoint *url.URL, canonicalURI string, region string, now time.Time, body []byte) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), reader)
	if err != nil {
		return nil, err
	}
	payloadHash := sha256Hex(body)
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	request.Header.Set(amzContentSHA256Header, payloadHash)
	request.Header.Set(amzDateHeader, amzDate)
	request.Header.Set("Authorization", s.authorization(request, canonicalURI, payloadHash, amzDate, dateStamp, region))
	return request, nil
}

func (s S3CompatibleStore) httpClient() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return http.DefaultClient
}

func (s S3CompatibleStore) authorization(request *http.Request, canonicalURI string, payloadHash string, amzDate string, dateStamp string, region string) string {
	host := request.URL.Host
	canonicalHeaders := "host:" + host + "\n" +
		"x-amz-content-sha256:" + payloadHash + "\n" +
		"x-amz-date:" + amzDate + "\n"
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalRequest := request.Method + "\n" +
		canonicalURI + "\n\n" +
		canonicalHeaders + "\n" +
		signedHeaders + "\n" +
		payloadHash

	scope := dateStamp + "/" + region + "/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" +
		amzDate + "\n" +
		scope + "\n" +
		sha256Hex([]byte(canonicalRequest))
	signature := hex.EncodeToString(hmacSHA256(signingKey(s.SecretKey, dateStamp, region), []byte(stringToSign)))

	return "AWS4-HMAC-SHA256 Credential=" + s.AccessKey + "/" + scope +
		", SignedHeaders=" + signedHeaders +
		", Signature=" + signature
}

func signingKey(secret string, dateStamp string, region string) []byte {
	dateKey := hmacSHA256([]byte("AWS4"+secret), []byte(dateStamp))
	regionKey := hmacSHA256(dateKey, []byte(region))
	serviceKey := hmacSHA256(regionKey, []byte("s3"))
	return hmacSHA256(serviceKey, []byte("aws4_request"))
}

func hmacSHA256(key []byte, value []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(value)
	return mac.Sum(nil)
}

func sha256Hex(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
