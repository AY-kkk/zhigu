package futures

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type OutboxValue struct {
	ID            string         `gorm:"column:id;primaryKey"`
	OwnerID       uint           `gorm:"column:owner_id"`
	Mode          string         `gorm:"column:mode"`
	DedupeKey     string         `gorm:"column:dedupe_key"`
	Payload       map[string]any `gorm:"column:payload;serializer:json"`
	Attempts      int            `gorm:"column:attempts"`
	NextAttemptAt time.Time      `gorm:"column:next_attempt_at"`
	State         string         `gorm:"column:state"`
	CreatedAt     time.Time      `gorm:"column:created_at"`
}

func (OutboxValue) TableName() string { return "futures_outbox" }

type OutboxEvent struct {
	SchemaVersion string `json:"schema_version"`
	Type          string `json:"type"`
	ObjectType    string `json:"object_type"`
	ObjectID      string `json:"object_id"`
	ObjectVersion int64  `json:"object_version"`
	RunID         string `json:"run_id,omitempty"`
}

func NewOutboxEvent(kind, objectType, objectID string, objectVersion int64, runID string) OutboxEvent {
	return OutboxEvent{SchemaVersion: "futures.outbox-event.v1", Type: kind, ObjectType: objectType, ObjectID: objectID, ObjectVersion: objectVersion, RunID: runID}
}

func parseOutboxEvent(payload map[string]any) (OutboxEvent, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return OutboxEvent{}, err
	}
	var event OutboxEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		return OutboxEvent{}, err
	}
	if event.SchemaVersion != "futures.outbox-event.v1" || event.Type == "" || event.ObjectType == "" || event.ObjectID == "" || event.ObjectVersion < 1 {
		return OutboxEvent{}, fmt.Errorf("%w: invalid outbox event", ErrInvalidInput)
	}
	return event, nil
}

func OutboxBackoff(attempt int) time.Duration {
	switch {
	case attempt <= 1:
		return 10 * time.Second
	case attempt == 2:
		return 30 * time.Second
	case attempt == 3:
		return 60 * time.Second
	default:
		return 300 * time.Second
	}
}

func OutboxStateAfterFailure(attemptsBefore, attemptsAfter int) string {
	if attemptsBefore >= 10 || attemptsAfter > 10 {
		return "dead"
	}
	return "pending"
}

func RecordOutboxFailure(ctx context.Context, db *gorm.DB, id string, now time.Time) error {
	var row OutboxValue
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(`id=? AND state='pending'`, id).First(&row).Error; err != nil {
			return err
		}
		attempts := row.Attempts + 1
		state := OutboxStateAfterFailure(row.Attempts, attempts)
		return tx.Model(&OutboxValue{}).Where(`id=?`, id).Updates(map[string]any{
			`attempts`: attempts, `state`: state, `next_attempt_at`: now.Add(OutboxBackoff(attempts)),
		}).Error
	})
}

// ProcessOutbox is at-least-once delivery. Events are isolated so one malformed
// event cannot roll back unrelated deliveries; dedupe keys prevent duplicate
// user-visible notifications.
func ProcessOutbox(ctx context.Context, db *gorm.DB, now time.Time, limit int) (int, error) {
	if limit <= 0 || limit > 2000 {
		limit = 2000
	}
	processed := 0
	for processed < limit {
		var row OutboxValue
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
				Where(`state='pending' AND next_attempt_at<=?`, now).Order(`next_attempt_at`).First(&row).Error; err != nil {
				return err
			}
			event, err := parseOutboxEvent(row.Payload)
			if err != nil {
				return err
			}
			var count int64
			if err := tx.Raw(`SELECT count(*) FROM futures_notifications WHERE dedupe_key=?`, row.DedupeKey).Scan(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				if err := tx.Exec(`INSERT INTO futures_notifications(id,owner_id,mode,type,object_id,object_version,dedupe_key,created_at) VALUES(?,?,?,?,?,?,?,?)`,
					"notification_"+uuid.NewString(), row.OwnerID, row.Mode, event.Type, event.ObjectID, event.ObjectVersion, row.DedupeKey, now).Error; err != nil {
					return err
				}
			}
			return tx.Model(&OutboxValue{}).Where(`id=?`, row.ID).Updates(map[string]any{`state`: `delivered`, `attempts`: row.Attempts + 1}).Error
		})
		if errors.Is(err, gorm.ErrRecordNotFound) {
			break
		}
		if err != nil {
			if failureErr := RecordOutboxFailure(ctx, db, row.ID, now); failureErr != nil {
				return processed, failureErr
			}
			continue
		}
		processed++
	}
	return processed, nil
}
