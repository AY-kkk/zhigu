package finance

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	modelfinance "zhigu/server/model/finance"
)

type ModelProxy struct {
	DB     *gorm.DB
	Budget BudgetService
	Client *http.Client
}

func NewModelProxy(db *gorm.DB, budget BudgetService) *ModelProxy {
	return &ModelProxy{
		DB:     db,
		Budget: budget,
		Client: &http.Client{
			Timeout: 45 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return NewError(400, "validation", "SSRF_BLOCKED", "禁止重定向")
			},
		},
	}
}

func (p *ModelProxy) Complete(ctx context.Context, requestID, bodyHash, tokenHash string, body []byte, protocol string) ([]byte, error) {
	var tok modelfinance.InternalToken
	if err := p.DB.Where("token_hash = ?", tokenHash).Take(&tok).Error; err != nil {
		return nil, NewError(401, "forbidden", "INVALID_TASK_TOKEN", "内部令牌无效")
	}
	if tok.RevokedAt != nil || time.Now().After(tok.ExpiresAt) {
		return nil, NewError(401, "forbidden", "TOKEN_REVOKED", "内部令牌已撤销")
	}
	var run modelfinance.ResearchRun
	if err := p.DB.Where("id = ?", tok.RunID).Take(&run).Error; err != nil {
		return nil, NewError(404, "not_found", "RUN_NOT_FOUND", "研究不存在")
	}
	if !runAllowsToolsAt(run, time.Now().UTC()) {
		return nil, NewError(409, "conflict", "RUN_CLOSED", "研究已结束")
	}
	wantProto, err := runProtocol(run)
	if err != nil {
		return nil, err
	}
	gotProto, err := NormalizeProtocol(protocol)
	if err != nil {
		return nil, err
	}
	if gotProto != wantProto {
		return nil, NewError(409, "conflict", "MODEL_PROTOCOL_MISMATCH", "入口与冻结协议不符")
	}
	var task modelfinance.ResearchTask
	if err := p.DB.Where("id = ? AND run_id = ?", tok.TaskID, tok.RunID).Take(&task).Error; err != nil {
		return nil, NewError(403, "forbidden", "TASK_RUN_MISMATCH", "任务不属于该研究")
	}
	var cached modelfinance.ModelCache
	q := p.DB.Where("run_id = ? AND task_id = ? AND request_id = ?", tok.RunID, tok.TaskID, requestID).Take(&cached)
	if q.Error == nil {
		if cached.BodyHash != bodyHash || cached.OwnerID != run.OwnerID {
			return nil, NewError(409, "conflict", "REQUEST_ID_CONFLICT", "相同请求 ID 对应不同正文或归属")
		}
		if cached.Status == "succeeded" {
			return cached.Response, nil
		}
		if cached.Status == "pending" {
			return nil, NewError(409, "conflict", "IN_FLIGHT", "相同请求处理中")
		}
	}
	now := time.Now().UTC()
	row := modelfinance.ModelCache{
		RunID: tok.RunID, TaskID: tok.TaskID, RequestID: requestID, OwnerID: run.OwnerID, Purpose: tok.Purpose,
		BodyHash: bodyHash, Status: "pending", CreatedAt: now, UpdatedAt: now,
	}
	if err := p.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "run_id"}, {Name: "task_id"}, {Name: "request_id"}},
		DoNothing: true,
	}).Create(&row).Error; err != nil {
		return nil, err
	}
	res, err := p.Budget.Reserve(ctx, BudgetRequest{
		RequestID: requestID, RunID: tok.RunID, TaskID: tok.TaskID, Purpose: tok.Purpose, Kind: "model",
		BodyHash: bodyHash, ReservedInput: 8000, ReservedOutput: 1200, OwnerID: run.OwnerID,
	})
	if err != nil {
		_ = p.DB.Model(&modelfinance.ModelCache{}).Where("run_id = ? AND task_id = ? AND request_id = ?", tok.RunID, tok.TaskID, requestID).Update("status", "failed")
		return nil, err
	}
	raw, _ := json.Marshal(fixtureModelResponse(gotProto))
	_ = p.Budget.Reconcile(ctx, res.ID, ObservedUsage{Unknown: false, ActualInput: intPtr(10), ActualOutput: intPtr(5)})
	_ = p.DB.Model(&modelfinance.ModelCache{}).Where("run_id = ? AND task_id = ? AND request_id = ?", tok.RunID, tok.TaskID, requestID).Updates(map[string]any{
		"status": "succeeded", "response": datatypes.JSON(raw), "updated_at": time.Now().UTC(),
	})
	return raw, nil
}

func intPtr(v int) *int { return &v }

func ReadBody(r io.ReadCloser) ([]byte, error) {
	defer r.Close()
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	return buf.Bytes(), err
}

func NormalizeJSONHash(b []byte) string {
	var v any
	if json.Unmarshal(b, &v) != nil {
		return SHA256Text(string(b))
	}
	h, _ := HashCanonical(v)
	return h
}

func HeaderTokenHash(h string) string {
	return SHA256Text(strings.TrimSpace(h))
}

func runProtocol(run modelfinance.ResearchRun) (string, error) {
	var cfg map[string]string
	_ = json.Unmarshal(run.ConfigVersions, &cfg)
	raw := ""
	if cfg != nil {
		raw = cfg["protocol"]
	}
	return NormalizeProtocol(raw)
}

func fixtureModelResponse(protocol string) map[string]any {
	if protocol == ProtocolResponses {
		return map[string]any{
			"id":      "resp_" + uuid.NewString(),
			"object":  "response",
			"status":  "completed",
			"model":   ModelAlias(),
			"output":  []any{map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": `{"ok":true}`}}}},
			"usage":   map[string]any{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15},
		}
	}
	return map[string]any{
		"id":      "chatcmpl_" + uuid.NewString(),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   ModelAlias(),
		"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": `{"ok":true}`}, "finish_reason": "stop"}},
		"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
	}
}
