package httpapi

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"event-ticket-system/internal/ticketing"
)

const maxPosterBytes = 5 * 1024 * 1024

func handleUploadEventPoster(service TicketingService, store objectStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeError(w, http.StatusServiceUnavailable, "object store is not configured")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxPosterBytes+1024*1024)
		file, header, err := r.FormFile("poster")
		if err != nil {
			writeError(w, http.StatusBadRequest, "poster file is required")
			return
		}
		defer func() { _ = file.Close() }()
		body, err := io.ReadAll(file)
		if err != nil {
			writeError(w, http.StatusBadRequest, "poster file is invalid")
			return
		}
		if len(body) > maxPosterBytes {
			writeError(w, http.StatusBadRequest, "poster must be 5 MiB or smaller")
			return
		}
		contentType := posterContentType(header.Header.Get("Content-Type"), body)
		if !allowedPosterContentType(contentType) {
			writeError(w, http.StatusBadRequest, "poster must be jpeg, png, or webp")
			return
		}
		eventID := r.PathValue("event_id")
		objectKey := fmt.Sprintf("events/%s/poster%s", eventID, posterExtension(contentType))
		if err := store.Put(r.Context(), objectKey, contentType, body); err != nil {
			writeError(w, http.StatusServiceUnavailable, "poster storage is unavailable")
			return
		}
		asset, err := service.SaveEventPoster(r.Context(), actorFromRequest(r), eventID, ticketing.EventAssetInput{
			ObjectKey:   objectKey,
			FileName:    filepath.Base(header.Filename),
			ContentType: contentType,
			SizeBytes:   int64(len(body)),
		})
		writeServiceResult(w, http.StatusCreated, asset, err)
	}
}

func handleGetEventPoster(service TicketingService, store objectStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeError(w, http.StatusServiceUnavailable, "object store is not configured")
			return
		}
		asset, err := service.GetEventPoster(r.Context(), actorFromRequest(r), r.PathValue("event_id"))
		if err != nil {
			writeServiceResult(w, http.StatusOK, nil, err)
			return
		}
		body, contentType, err := store.Get(r.Context(), asset.ObjectKey)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "poster artifact is unavailable")
			return
		}
		if strings.TrimSpace(contentType) == "" {
			contentType = asset.ContentType
		}
		w.Header().Set(contentTypeHeader, contentType)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}
}

func posterContentType(headerValue string, body []byte) string {
	contentType := strings.TrimSpace(headerValue)
	if contentType != "" {
		return contentType
	}
	return http.DetectContentType(body)
}

func allowedPosterContentType(contentType string) bool {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/jpeg", "image/png", "image/webp":
		return true
	default:
		return false
	}
}

func posterExtension(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	default:
		return ""
	}
}
