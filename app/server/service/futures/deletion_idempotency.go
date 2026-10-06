package futures

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	model "zhigu/server/model/futures"
)

func replayDeletion(ctx context.Context, tx *gorm.DB, id Identity, operation, key, requestHash string) (map[string]any, bool, error) {
	if key == "" {
		return nil, false, ErrInvalidInput
	}
	var row model.Idempotency
	err := tx.WithContext(ctx).Where(`owner_id=? AND mode=? AND operation=? AND key=?`, id.OwnerID, id.Mode, operation, key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if row.RequestHash != requestHash {
		return nil, false, ErrIdempotencyConflict
	}
	var result map[string]any
	if err := json.Unmarshal(row.Response, &result); err != nil {
		return nil, false, err
	}
	return result, true, nil
}

func ensureIdempotentObjectVisible(ctx context.Context, tx *gorm.DB, id Identity, objectID string, response any) error {
	raw, err := json.Marshal(response)
	if err != nil {
		return err
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	if fmt.Sprint(decoded["id"]) != objectID {
		return nil
	}
	var tombstones int64
	if err := tx.WithContext(ctx).Raw(`SELECT count(*) FROM futures_tombstones WHERE owner_id=? AND object_id=?`, id.OwnerID, objectID).Scan(&tombstones).Error; err != nil {
		return err
	}
	if tombstones > 0 {
		return ErrNotFound
	}
	return nil
}

func hideIdempotentObject(ctx context.Context, tx *gorm.DB, id Identity, operation, objectID string, hiddenAt time.Time) error {
	return tx.WithContext(ctx).Exec(`UPDATE futures_idempotency_keys SET response=jsonb_build_object('id',?::text,'deleted',true,'hidden_at',?::timestamptz) WHERE owner_id=? AND mode=? AND operation=? AND response->>'id'=?`,
		objectID, hiddenAt, id.OwnerID, id.Mode, operation, objectID).Error
}
