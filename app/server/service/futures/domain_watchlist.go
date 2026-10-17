package futures

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
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

type NotificationFilter struct {
	Type          string
	Read          *bool
	ObjectID      string
	ObjectVersion *int64
}

func (f NotificationFilter) String() string {
	read := "any"
	if f.Read != nil {
		read = strconv.FormatBool(*f.Read)
	}
	version := "any"
	if f.ObjectVersion != nil {
		version = strconv.FormatInt(*f.ObjectVersion, 10)
	}
	return "type=" + f.Type + ";read=" + read + ";object_id=" + f.ObjectID + ";object_version=" + version
}

func (d *Domain) ListNotificationsPage(ctx context.Context, id Identity, filter NotificationFilter, options PageOptions) (Page[map[string]any], error) {
	if options.Limit < 1 || options.Limit > 100 {
		return Page[map[string]any]{}, ErrInvalidInput
	}
	query := d.DB.WithContext(ctx).Where(`owner_id=? AND mode=? AND deleted_at IS NULL`, id.OwnerID, id.Mode)
	if filter.Type != "" {
		query = query.Where(`type=?`, filter.Type)
	}
	if filter.Read != nil {
		if *filter.Read {
			query = query.Where(`read_at IS NOT NULL`)
		} else {
			query = query.Where(`read_at IS NULL`)
		}
	}
	if filter.ObjectID != "" {
		query = query.Where(`object_id=?`, filter.ObjectID)
	}
	if filter.ObjectVersion != nil {
		query = query.Where(`object_version=?`, *filter.ObjectVersion)
	}
	if options.Cursor != nil {
		cursorTime, err := time.Parse(time.RFC3339Nano, options.Cursor.Primary)
		if err != nil {
			return Page[map[string]any]{}, ErrInvalidInput
		}
		query = query.Where(`created_at < ? OR (created_at = ? AND id < ?)`, cursorTime, cursorTime, options.Cursor.Secondary)
	}
	var values []NotificationValue
	if err := query.Order(`created_at DESC, id DESC`).Limit(options.Limit + 1).Find(&values).Error; err != nil {
		return Page[map[string]any]{}, err
	}
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		out = append(out, notificationMap(value))
	}
	return BuildPage(id, "notifications", filter.String(), options, out, func(value map[string]any) (string, string, string) {
		return TimePagePosition(value["created_at"].(time.Time), fmt.Sprint(value["id"]), 0)
	})
}

func notificationMap(value NotificationValue) map[string]any {
	return map[string]any{"id": value.ID, "type": value.Type, "object_id": value.ObjectID, "object_version": value.ObjectVersion, "read_at": value.ReadAt, "created_at": value.CreatedAt}
}

func (d *Domain) MarkNotificationsRead(ctx context.Context, id Identity, ids []string) (map[string]any, error) {
	if len(ids) == 0 || len(ids) > 100 {
		return nil, ErrInvalidBatch
	}
	now := d.Now().UTC()
	var updated []string
	err := d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := `UPDATE futures_notifications SET read_at=COALESCE(read_at, ?) WHERE owner_id=? AND mode=? AND deleted_at IS NULL AND id IN (?) RETURNING id`
		if err := tx.Raw(query, now, id.OwnerID, id.Mode, ids).Scan(&updated).Error; err != nil {
			return err
		}
		if len(updated) != len(ids) {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(updated)
	return map[string]any{"ids": updated, "read_at": now}, nil
}
