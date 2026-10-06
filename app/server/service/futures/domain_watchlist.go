package futures

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type WatchlistValue struct {
	OwnerID    uint           `gorm:"column:owner_id;primaryKey"`
	Mode       string         `gorm:"column:mode;primaryKey"`
	ProductIDs datatypes.JSON `gorm:"column:product_ids"`
	Version    int64          `gorm:"column:version"`
	UpdatedAt  time.Time      `gorm:"column:updated_at"`
}

func (WatchlistValue) TableName() string { return "futures_watchlists" }

func (d *Domain) GetWatchlist(ctx context.Context, id Identity) (map[string]any, error) {
	var value WatchlistValue
	err := d.DB.WithContext(ctx).Where(`owner_id=? AND mode=?`, id.OwnerID, id.Mode).First(&value).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return map[string]any{"version": 1, "product_ids": []any{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var products any = []any{}
	_ = json.Unmarshal(value.ProductIDs, &products)
	return map[string]any{"version": value.Version, "product_ids": products}, nil
}

func (d *Domain) PutWatchlist(ctx context.Context, id Identity, expected int64, productIDs []string) (map[string]any, error) {
	if expected < 1 || len(productIDs) > 20 {
		return nil, ErrInvalidInput
	}
	seen := map[string]bool{}
	for _, productID := range productIDs {
		if productID == "" || seen[productID] {
			return nil, ErrInvalidInput
		}
		seen[productID] = true
	}
	raw, _ := json.Marshal(productIDs)
	newVersion := expected
	err := d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, productID := range productIDs {
			if err := requireAdmittedProduct(tx, productID); err != nil {
				return err
			}
		}
		var current WatchlistValue
		err := tx.Where(`owner_id=? AND mode=?`, id.OwnerID, id.Mode).First(&current).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if expected != 1 {
				return ErrRevisionConflict
			}
			return tx.Create(WatchlistValue{OwnerID: id.OwnerID, Mode: id.Mode, ProductIDs: datatypes.JSON(raw), Version: 1, UpdatedAt: d.Now().UTC()}).Error
		}
		if err != nil {
			return err
		}
		if current.Version != expected {
			return ErrRevisionConflict
		}
		newVersion = expected + 1
		return tx.Model(&WatchlistValue{}).Where(`owner_id=? AND mode=? AND version=?`, id.OwnerID, id.Mode, expected).
			Updates(map[string]any{"product_ids": datatypes.JSON(raw), "version": newVersion, "updated_at": d.Now().UTC()}).Error
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"version": newVersion, "product_ids": productIDs}, nil
}

type NotificationValue struct {
	ID            string     `gorm:"column:id;primaryKey"`
	OwnerID       uint       `gorm:"column:owner_id"`
	Mode          string     `gorm:"column:mode"`
	Type          string     `gorm:"column:type"`
	ObjectID      string     `gorm:"column:object_id"`
	ObjectVersion int64      `gorm:"column:object_version"`
	ReadAt        *time.Time `gorm:"column:read_at"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
}

func (NotificationValue) TableName() string { return "futures_notifications" }

func (d *Domain) ListNotifications(ctx context.Context, id Identity, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var values []NotificationValue
	if err := d.DB.WithContext(ctx).Where(`owner_id=? AND mode=? AND deleted_at IS NULL`, id.OwnerID, id.Mode).Order(`created_at DESC, id DESC`).Limit(limit).Find(&values).Error; err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		out = append(out, map[string]any{"id": value.ID, "type": value.Type, "object_id": value.ObjectID, "object_version": value.ObjectVersion, "read_at": value.ReadAt, "created_at": value.CreatedAt})
	}
	return out, nil
}

func (d *Domain) MarkNotification(ctx context.Context, id Identity, notificationID string) (map[string]any, error) {
	now := d.Now().UTC()
	res := d.DB.WithContext(ctx).Table(`futures_notifications`).Where(`id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, notificationID, id.OwnerID, id.Mode).Update(`read_at`, now)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected != 1 {
		return nil, ErrNotFound
	}
	return d.getNotification(ctx, id, notificationID)
}

func (d *Domain) getNotification(ctx context.Context, id Identity, notificationID string) (map[string]any, error) {
	var value NotificationValue
	if err := d.DB.WithContext(ctx).Where(`id=? AND owner_id=? AND mode=?`, notificationID, id.OwnerID, id.Mode).First(&value).Error; err != nil {
		return nil, ErrNotFound
	}
	return map[string]any{"id": value.ID, "type": value.Type, "object_id": value.ObjectID, "object_version": value.ObjectVersion, "read_at": value.ReadAt, "created_at": value.CreatedAt}, nil
}
