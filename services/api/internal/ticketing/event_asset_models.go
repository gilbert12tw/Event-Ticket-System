package ticketing

import "time"

type EventAsset struct {
	AssetID     string    `json:"asset_id"`
	EventID     string    `json:"event_id"`
	ObjectKey   string    `json:"object_key,omitempty"`
	FileName    string    `json:"file_name"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
}

type EventAssetInput struct {
	ObjectKey   string
	FileName    string
	ContentType string
	SizeBytes   int64
}
