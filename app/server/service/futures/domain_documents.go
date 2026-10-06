package futures

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	model "zhigu/server/model/futures"
)

func (d *Domain) CreateDocument(ctx context.Context, id Identity, filename, mediaType string, content []byte, idempotencyKey string) (map[string]any, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" || strings.ContainsAny(filename, "\x00/\\") {
		return nil, ErrInvalidInput
	}
	extracted, err := ExtractFuturesDocumentIsolated(ctx, filename, mediaType, content)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(content)
	contentHash := hex.EncodeToString(hash[:])
	now := d.Now().UTC()
	value := model.Document{ID: "document_" + uuid.NewString(), OwnerID: id.OwnerID, Mode: id.Mode, Filename: filename,
		MediaType: extracted.MIME, ByteSize: int64(len(content)), ContentHash: contentHash,
		StorageKey: filepath.Join("owner-"+uuid.NewString(), contentHash+".bin"), ExtractionStatus: extracted.Status,
		PageCount: extracted.PageCount, WordCount: extracted.CharacterCount, ExtractedText: extracted.Text, PurgeAt: now.Add(7 * 24 * time.Hour), CreatedAt: now}
	err = d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if existing, err := replayDocumentIdempotency(tx, id, idempotencyKey, contentHash); err != nil {
			return err
		} else if existing != nil {
			value = *existing
			return nil
		}
		var prior model.Document
		findErr := tx.Where(`owner_id=? AND mode=? AND content_hash=? AND deleted_at IS NULL`, id.OwnerID, id.Mode, contentHash).First(&prior).Error
		if findErr == nil {
			value = prior
			return nil
		}
		if !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}
		if err := tx.Create(&value).Error; err != nil {
			return err
		}
		return d.writeIdempotency(tx, id, "document.create", idempotencyKey, contentHash, value)
	})
	if err != nil {
		return nil, err
	}
	if err := d.storeDocument(value.StorageKey, content); err != nil {
		return nil, err
	}
	return documentMap(value), nil
}

func (d *Domain) GetDocument(ctx context.Context, id Identity, documentID string) (map[string]any, error) {
	var value model.Document
	err := d.DB.WithContext(ctx).Where(`id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, documentID, id.OwnerID, id.Mode).First(&value).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return documentMap(value), nil
}

func (d *Domain) DeleteDocument(ctx context.Context, id Identity, documentID, idempotencyKey string) (map[string]any, error) {
	now := d.Now().UTC()
	requestHash := hashJSON(map[string]any{"id": documentID, "action": "delete"})
	result := map[string]any{"deletion_id": "del_" + uuid.NewString(), "hidden_at": now, "purge_due_at": now.Add(24 * time.Hour)}
	var value model.Document
	err := d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if existing, ok, err := replayDeletion(ctx, tx, id, "document.delete", idempotencyKey, requestHash); err != nil {
			return err
		} else if ok {
			result = existing
			return nil
		}
		if err := tx.Where(`id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, documentID, id.OwnerID, id.Mode).First(&value).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		var referenced int64
		if err := tx.Raw(`SELECT count(*) FROM futures_drafts WHERE owner_id=? AND mode=? AND deleted_at IS NULL AND input->>'document_id'=?`, id.OwnerID, id.Mode, documentID).Scan(&referenced).Error; err != nil {
			return err
		}
		if referenced > 0 {
			return ErrReferenced
		}
		if err := tx.Model(&model.Document{}).Where(`id=? AND owner_id=? AND mode=?`, documentID, id.OwnerID, id.Mode).Update(`deleted_at`, now).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO futures_deletion_jobs(id,owner_id,mode,target_type,target_id,impact_version,purge_due_at,state) VALUES(?,?,?,?,?,?,?,'scheduled')`,
			result["deletion_id"], id.OwnerID, id.Mode, "document", documentID, contentHash(value), now.Add(24*time.Hour)).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO futures_tombstones(request_id,owner_id,object_type,object_id,hidden_at,purge_due_at) VALUES(?,?,?,?,?,?)`,
			uuid.NewString(), id.OwnerID, "document", documentID, now, now.Add(24*time.Hour)).Error; err != nil {
			return err
		}
		if err := hideIdempotentObject(ctx, tx, id, "document.create", documentID, now); err != nil {
			return err
		}
		return d.writeIdempotency(tx, id, "document.delete", idempotencyKey, requestHash, result)
	})
	if err != nil {
		return nil, err
	}
	if value.StorageKey != "" {
		_ = d.removeDocument(value.StorageKey)
	}
	return result, nil
}

func (d *Domain) storeDocument(storageKey string, content []byte) error {
	if d.StorageDir == "" {
		return nil
	}
	full := filepath.Join(d.StorageDir, filepath.FromSlash(storageKey))
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return err
	}
	return os.WriteFile(full, content, 0o600)
}

func (d *Domain) removeDocument(storageKey string) error {
	if d.StorageDir == "" {
		return nil
	}
	return os.Remove(filepath.Join(d.StorageDir, filepath.FromSlash(storageKey)))
}

func documentMap(value model.Document) map[string]any {
	return map[string]any{
		"id": value.ID, "status": value.ExtractionStatus, "mime": value.MediaType, "bytes": value.ByteSize,
		"page_count": value.PageCount, "character_count": value.WordCount, "failure_code": value.FailureCode,
	}
}

func contentHash(value model.Document) string { return value.ContentHash }

func replayDocumentIdempotency(tx *gorm.DB, id Identity, key, requestHash string) (*model.Document, error) {
	if key == "" {
		return nil, ErrInvalidInput
	}
	var row model.Idempotency
	err := tx.Where(`owner_id=? AND mode=? AND operation=? AND key=?`, id.OwnerID, id.Mode, "document.create", key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if row.RequestHash != requestHash {
		return nil, ErrIdempotencyConflict
	}
	var value model.Document
	if err := json.Unmarshal(row.Response, &value); err != nil {
		return nil, err
	}
	if !row.ExpiresAt.After(time.Now().UTC()) {
		return nil, ErrNotFound
	}
	if err := ensureIdempotentObjectVisible(context.Background(), tx, id, value.ID, value); err != nil {
		return nil, err
	}
	return &value, nil
}
