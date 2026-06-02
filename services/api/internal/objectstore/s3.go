package objectstore

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
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
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	request.Header.Set("Content-Type", contentType)

	payloadHash := sha256Hex(body)
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	request.Header.Set("X-Amz-Content-Sha256", payloadHash)
	request.Header.Set("X-Amz-Date", amzDate)
	request.Header.Set("Authorization", s.authorization(request, canonicalURI, payloadHash, amzDate, dateStamp, region))

	response, err := s.httpClient().Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
	return fmt.Errorf("object storage put failed: status=%d body=%s", response.StatusCode, strings.TrimSpace(string(responseBody)))
}

func (s S3CompatibleStore) Exists(ctx context.Context, key string) (bool, error) {
	endpoint, canonicalURI, region, now, err := s.requestParts(key)
	if err != nil {
		return false, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, endpoint.String(), nil)
	if err != nil {
		return false, err
	}
	payloadHash := sha256Hex(nil)
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	request.Header.Set("X-Amz-Content-Sha256", payloadHash)
	request.Header.Set("X-Amz-Date", amzDate)
	request.Header.Set("Authorization", s.authorization(request, canonicalURI, payloadHash, amzDate, dateStamp, region))

	response, err := s.httpClient().Do(request)
	if err != nil {
		return false, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return true, nil
	}
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
	return false, fmt.Errorf("object storage head failed: status=%d body=%s", response.StatusCode, strings.TrimSpace(string(responseBody)))
}

func (s S3CompatibleStore) Get(ctx context.Context, key string) ([]byte, string, error) {
	endpoint, canonicalURI, region, now, err := s.requestParts(key)
	if err != nil {
		return nil, "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, "", err
	}
	payloadHash := sha256Hex(nil)
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	request.Header.Set("X-Amz-Content-Sha256", payloadHash)
	request.Header.Set("X-Amz-Date", amzDate)
	request.Header.Set("Authorization", s.authorization(request, canonicalURI, payloadHash, amzDate, dateStamp, region))

	response, err := s.httpClient().Do(request)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		body, err := io.ReadAll(response.Body)
		return body, response.Header.Get("Content-Type"), err
	}
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
	return nil, "", fmt.Errorf("object storage get failed: status=%d body=%s", response.StatusCode, strings.TrimSpace(string(responseBody)))
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
