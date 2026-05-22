package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"event-ticket-system/internal/ticketing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteServiceResultWritesSuccessEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()

	writeServiceResult(rec, http.StatusCreated, map[string]string{"id": "evt_1"}, nil)

	require.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, true, body["success"])
	assert.Nil(t, body["error"])
	assert.Equal(t, map[string]interface{}{"id": "evt_1"}, body["data"])
}

func TestWriteServiceResultWritesAppErrorEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()

	writeServiceResult(rec, http.StatusCreated, nil, ticketing.AppError{Status: http.StatusConflict, Message: "already booked"})

	require.Equal(t, http.StatusConflict, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, false, body["success"])
	assert.Nil(t, body["data"])
	assert.Equal(t, "already booked", body["error"])
}

func TestWriteServiceResultNormalizesNilSlices(t *testing.T) {
	rec := httptest.NewRecorder()
	var rows []ticketing.Ticket

	writeServiceResult(rec, http.StatusOK, rows, nil)

	require.Equal(t, http.StatusOK, rec.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, []interface{}{}, body["data"])
}
