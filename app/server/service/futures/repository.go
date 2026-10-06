package futures

import (
	"context"
	"errors"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	model "zhigu/server/model/futures"
)

type DBRepository struct{ DB *gorm.DB }

func NewDBRepository(db *gorm.DB) *DBRepository { return &DBRepository{DB: db} }

func (r *DBRepository) CreateDraft(ctx context.Context, id Identity, value *model.Draft) error {
	value.OwnerID, value.Mode = id.OwnerID, id.Mode
	if len(value.Claims) == 0 {
		value.Claims = datatypes.JSON(`[]`)
	}
	return r.DB.WithContext(ctx).Create(value).Error
}

func (r *DBRepository) GetDraft(ctx context.Context, id Identity, objectID string) (*model.Draft, error) {
	var value model.Draft
	err := r.DB.WithContext(ctx).Where(`id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, objectID, id.OwnerID, id.Mode).First(&value).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &value, err
}

func (r *DBRepository) DeleteDraft(ctx context.Context, id Identity, objectID string, expectedRevision int64) error {
	res := r.DB.WithContext(ctx).Model(&model.Draft{}).
		Where(`id=? AND owner_id=? AND mode=? AND revision=? AND deleted_at IS NULL`, objectID, id.OwnerID, id.Mode, expectedRevision).
		Update(`deleted_at`, time.Now().UTC())
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *DBRepository) CreateRun(ctx context.Context, id Identity, value *model.Run) error {
	value.OwnerID, value.Mode = id.OwnerID, id.Mode
	if len(value.ClaimsSnapshot) == 0 {
		value.ClaimsSnapshot = datatypes.JSON(`[]`)
	}
	if len(value.Versions) == 0 {
		value.Versions = datatypes.JSON(`{}`)
	}
	if len(value.TaskSnapshot) == 0 {
		value.TaskSnapshot = datatypes.JSON(`{}`)
	}
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var prior model.Run
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where(`owner_id=? AND mode=? AND idempotency_key=?`, id.OwnerID, id.Mode, value.IdempotencyKey).First(&prior).Error
		if err == nil {
			if prior.RequestHash == value.RequestHash {
				*value = prior
				return nil
			}
			return ErrIdempotencyConflict
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(value)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 1 {
			return nil
		}
		err = tx.Where(`owner_id=? AND mode=? AND idempotency_key=?`, id.OwnerID, id.Mode, value.IdempotencyKey).First(&prior).Error
		if err != nil {
			return err
		}
		if prior.RequestHash != value.RequestHash {
			return ErrIdempotencyConflict
		}
		*value = prior
		return nil
	})
}

func (r *DBRepository) GetRun(ctx context.Context, id Identity, objectID string) (*model.Run, error) {
	var value model.Run
	err := r.DB.WithContext(ctx).Where(`id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, objectID, id.OwnerID, id.Mode).First(&value).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &value, err
}

func (r *DBRepository) DeleteRun(ctx context.Context, id Identity, objectID string) error {
	res := r.DB.WithContext(ctx).Model(&model.Run{}).
		Where(`id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, objectID, id.OwnerID, id.Mode).
		Update(`deleted_at`, time.Now().UTC())
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
