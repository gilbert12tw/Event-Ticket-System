package ticketing

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *Service) SaveEventPoster(ctx context.Context, actor Actor, eventID string, input EventAssetInput) (EventAsset, error) {
	if err := requireRole(actor, RoleActivityAdmin); err != nil {
		return EventAsset{}, err
	}
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return EventAsset{}, badRequest("event_id is required")
	}
	if err := validateEventAssetInput(input); err != nil {
		return EventAsset{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return EventAsset{}, err
	}
	defer rollback(ctx, tx)
	if _, _, err := s.readEventWithRuleTx(ctx, tx, eventID, eventRowLockUpdate); err != nil {
		return EventAsset{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM event_assets WHERE event_id = $1`, eventID); err != nil {
		return EventAsset{}, err
	}
	assetID, err := newID("ast")
	if err != nil {
		return EventAsset{}, err
	}
	now := s.now()
	asset := EventAsset{
		AssetID:     assetID,
		EventID:     eventID,
		ObjectKey:   strings.TrimSpace(input.ObjectKey),
		FileName:    strings.TrimSpace(input.FileName),
		ContentType: strings.TrimSpace(input.ContentType),
		SizeBytes:   input.SizeBytes,
		CreatedBy:   actor.ID,
		CreatedAt:   now,
	}
	_, err = tx.Exec(ctx, `INSERT INTO event_assets
			(asset_id, event_id, object_key, file_name, content_type, size_bytes, created_by, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		asset.AssetID, asset.EventID, asset.ObjectKey, asset.FileName, asset.ContentType, asset.SizeBytes, asset.CreatedBy, asset.CreatedAt)
	if err != nil {
		return EventAsset{}, err
	}
	auditID, err := newID("aud")
	if err != nil {
		return EventAsset{}, err
	}
	if err := insertAudit(ctx, tx, newAuditRecord(auditID, actor, "event.poster.updated", "event", eventID, map[string]interface{}{
		"asset_id":     asset.AssetID,
		"content_type": asset.ContentType,
		"size_bytes":   asset.SizeBytes,
	})); err != nil {
		return EventAsset{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return EventAsset{}, err
	}
	return asset, nil
}

func (s *Service) GetEventPoster(ctx context.Context, actor Actor, eventID string) (EventAsset, error) {
	if _, err := s.GetEvent(ctx, actor, eventID, ""); err != nil {
		return EventAsset{}, err
	}
	return s.loadEventPoster(ctx, eventID)
}

func validateEventAssetInput(input EventAssetInput) error {
	if strings.TrimSpace(input.ObjectKey) == "" {
		return badRequest("object_key is required")
	}
	if strings.TrimSpace(input.FileName) == "" {
		return badRequest("file_name is required")
	}
	if strings.TrimSpace(input.ContentType) == "" {
		return badRequest("content_type is required")
	}
	if input.SizeBytes <= 0 {
		return badRequest("size_bytes must be positive")
	}
	return nil
}

func (s *Service) loadEventPoster(ctx context.Context, eventID string) (EventAsset, error) {
	var asset EventAsset
	err := s.db.QueryRow(ctx, `SELECT asset_id, event_id, object_key, file_name, content_type, size_bytes, created_by, created_at
		FROM event_assets
		WHERE event_id = $1
		ORDER BY created_at DESC
		LIMIT 1`, eventID).
		Scan(&asset.AssetID, &asset.EventID, &asset.ObjectKey, &asset.FileName, &asset.ContentType, &asset.SizeBytes, &asset.CreatedBy, &asset.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return EventAsset{}, notFound("event poster not found")
	}
	return asset, err
}
