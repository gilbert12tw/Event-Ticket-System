package httpapi

import (
	"context"

	"event-ticket-system/internal/ticketing"
)

type objectStore interface {
	ticketing.ReportObjectReader
	Put(ctx context.Context, key string, contentType string, body []byte) error
}
