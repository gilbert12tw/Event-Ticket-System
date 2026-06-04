package httpapi

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"

	"event-ticket-system/internal/ticketing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeObjectStore struct {
	key         string
	contentType string
	body        []byte
}

func (s *fakeObjectStore) Put(_ context.Context, key string, contentType string, body []byte) error {
	s.key = key
	s.contentType = contentType
	s.body = body
	return nil
}

func (s *fakeObjectStore) Get(_ context.Context, key string) ([]byte, string, error) {
	s.key = key
	return s.body, s.contentType, nil
}

func TestUploadEventPosterStoresObjectAndMetadata(t *testing.T) {
	service := &fakeTicketingService{}
	store := &fakeObjectStore{}
	router := testRouter(Dependencies{Ticketing: service, ObjectStore: store})
	body, contentType := posterMultipartBody(t, "poster.png", "image/png", []byte("png-body"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events/evt_1/poster", body)
	req.Header.Set("Content-Type", contentType)
	authorizeRequest(t, req, ticketing.RoleActivityAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.Equal(t, "events/evt_1/poster.png", store.key)
	assert.Equal(t, "image/png", store.contentType)
	assert.Equal(t, []byte("png-body"), store.body)
	assert.Equal(t, "events/evt_1/poster.png", service.posterInput.ObjectKey)
	assert.Equal(t, int64(len("png-body")), service.posterInput.SizeBytes)
	assertEnvelope(t, rec.Body.String(), `"asset_id":"ast_1"`)
}

func TestGetEventPosterServesStoredImage(t *testing.T) {
	service := &fakeTicketingService{}
	store := &fakeObjectStore{body: []byte("png-body"), contentType: "image/png"}
	router := testRouter(Dependencies{Ticketing: service, ObjectStore: store})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/evt_1/poster", nil)
	authorizeRequest(t, req, ticketing.RoleEmployee)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "events/evt_1/poster.png", store.key)
	assert.Equal(t, "image/png", rec.Header().Get("Content-Type"))
	assert.Equal(t, "png-body", rec.Body.String())
}

func TestUploadEventPosterRejectsUnsupportedType(t *testing.T) {
	service := &fakeTicketingService{}
	store := &fakeObjectStore{}
	router := testRouter(Dependencies{Ticketing: service, ObjectStore: store})
	body, contentType := posterMultipartBody(t, "poster.txt", "text/plain", []byte("not image"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events/evt_1/poster", body)
	req.Header.Set("Content-Type", contentType)
	authorizeRequest(t, req, ticketing.RoleActivityAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Empty(t, store.key)
}

func TestUploadEventPosterRejectsOversizedFile(t *testing.T) {
	service := &fakeTicketingService{}
	store := &fakeObjectStore{}
	router := testRouter(Dependencies{Ticketing: service, ObjectStore: store})
	body, contentType := posterMultipartBody(t, "poster.png", "image/png", bytes.Repeat([]byte("x"), maxPosterBytes+1))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events/evt_1/poster", body)
	req.Header.Set("Content-Type", contentType)
	authorizeRequest(t, req, ticketing.RoleActivityAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Empty(t, store.key)
}

func posterMultipartBody(t *testing.T, fileName string, fileContentType string, body []byte) (*bytes.Buffer, string) {
	t.Helper()
	buffer := &bytes.Buffer{}
	writer := multipart.NewWriter(buffer)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="poster"; filename="`+fileName+`"`)
	header.Set("Content-Type", fileContentType)
	part, err := writer.CreatePart(header)
	require.NoError(t, err)
	_, err = part.Write(body)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return buffer, writer.FormDataContentType()
}
