package httpx

import (
	"context"
	"crypto/subtle"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrBackupActive = errors.New("backup epoch active")

type BackupEpoch struct {
	ID        string     `gorm:"column:id;primaryKey"`
	Epoch     string     `gorm:"column:epoch;uniqueIndex"`
	Status    string     `gorm:"column:status"`
	StartedAt time.Time  `gorm:"column:started_at"`
	EndedAt   *time.Time `gorm:"column:ended_at"`
}

func (BackupEpoch) TableName() string { return "finance_backup_epochs" }

type BackupBarrier struct {
	DB     *gorm.DB
	mu     sync.RWMutex
	active atomic.Bool
	epoch  string
}

func NewBackupBarrier(db *gorm.DB) (*BackupBarrier, error) {
	barrier := &BackupBarrier{DB: db}
	var value BackupEpoch
	err := db.Where(`status='active'`).Order(`started_at DESC`).First(&value).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return barrier, nil
	}
	if err != nil {
		return nil, err
	}
	barrier.epoch = value.Epoch
	barrier.active.Store(true)
	barrier.mu.Lock()
	return barrier, nil
}

func (b *BackupBarrier) Begin(ctx context.Context, epoch string) (map[string]any, error) {
	if epoch == "" || len(epoch) > 128 {
		return nil, errors.New("invalid backup epoch")
	}
	b.mu.Lock()
	if b.active.Load() {
		b.mu.Unlock()
		return nil, ErrBackupActive
	}
	now := time.Now().UTC()
	err := b.DB.WithContext(ctx).Create(&BackupEpoch{ID: "backup_" + uuid.NewString(), Epoch: epoch, Status: "active", StartedAt: now}).Error
	if err != nil {
		b.mu.Unlock()
		return nil, err
	}
	b.epoch = epoch
	b.active.Store(true)
	return map[string]any{"epoch": epoch, "status": "active", "started_at": now, "acknowledged": true, "persisted": true}, nil
}

func (b *BackupBarrier) Status(ctx context.Context, epoch string) (map[string]any, error) {
	if !b.active.Load() || b.epoch != epoch {
		return nil, ErrBackupActive
	}
	var value BackupEpoch
	if err := b.DB.WithContext(ctx).Where(`epoch=?`, epoch).First(&value).Error; err != nil {
		return nil, err
	}
	return map[string]any{"epoch": epoch, "status": value.Status, "acknowledged": true, "persisted": true, "started_at": value.StartedAt}, nil
}

func (b *BackupBarrier) End(ctx context.Context, epoch, status string) (map[string]any, error) {
	if status != "succeeded" && status != "failed" {
		return nil, errors.New("invalid backup status")
	}
	if !b.active.Load() || b.epoch != epoch {
		return nil, ErrBackupActive
	}
	now := time.Now().UTC()
	result := b.DB.WithContext(ctx).Model(&BackupEpoch{}).Where(`epoch=? AND status='active'`, epoch).Updates(map[string]any{"status": status, "ended_at": now})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, ErrBackupActive
	}
	b.active.Store(false)
	b.epoch = ""
	b.mu.Unlock()
	return map[string]any{"epoch": epoch, "status": status, "ended_at": now, "acknowledged": true, "persisted": true}, nil
}

func (b *BackupBarrier) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/internal/ops/v1/backup/") {
			c.Next()
			return
		}
		switch c.Request.Method {
		case "GET", "HEAD", "OPTIONS":
			c.Next()
			return
		}
		b.mu.RLock()
		defer b.mu.RUnlock()
		if b.active.Load() {
			Fail(c, 503, "BACKUP_QUIESCING", "备份写入屏障已启用")
			c.Abort()
			return
		}
		c.Next()
	}
}

func RegisterBackupRoutes(engine *gin.Engine, barrier *BackupBarrier) {
	group := engine.Group("/internal/ops/v1/backup")
	group.Use(func(c *gin.Context) {
		expected := strings.TrimSpace(os.Getenv("ZHIGU_INTERNAL_TOKEN"))
		if path := os.Getenv("ZHIGU_INTERNAL_TOKEN_FILE"); expected == "" && path != "" {
			if raw, err := os.ReadFile(path); err == nil {
				expected = strings.TrimSpace(string(raw))
			}
		}
		provided := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		if expected == "" || subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) != 1 {
			Fail(c, 401, "UNAUTHENTICATED", "内部凭据无效")
			c.Abort()
			return
		}
		c.Next()
	})
	group.POST("/begin", func(c *gin.Context) {
		var input struct {
			Epoch string `json:"epoch"`
		}
		if !BindJSON(c, &input) {
			return
		}
		value, err := barrier.Begin(c.Request.Context(), input.Epoch)
		if err != nil {
			Fail(c, 409, "BACKUP_EPOCH_CONFLICT", "备份 epoch 冲突")
			return
		}
		OK(c, 200, value)
	})
	group.GET("/status", func(c *gin.Context) {
		value, err := barrier.Status(c.Request.Context(), c.Query("epoch"))
		if err != nil {
			Fail(c, 409, "BACKUP_EPOCH_CONFLICT", "备份 epoch 冲突")
			return
		}
		OK(c, 200, value)
	})
	group.POST("/end", func(c *gin.Context) {
		var input struct {
			Epoch  string `json:"epoch"`
			Status string `json:"status"`
		}
		if !BindJSON(c, &input) {
			return
		}
		value, err := barrier.End(c.Request.Context(), input.Epoch, input.Status)
		if err != nil {
			Fail(c, 409, "BACKUP_EPOCH_CONFLICT", "备份 epoch 冲突")
			return
		}
		OK(c, 200, value)
	})
}
