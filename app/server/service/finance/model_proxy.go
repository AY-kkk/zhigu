package finance

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/shopspring/decimal"
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
		p.failCache(tok.RunID, tok.TaskID, requestID)
		return nil, err
	}
	raw, usage, err := p.forward(ctx, run, gotProto, body)
	if err != nil {
		p.failCache(tok.RunID, tok.TaskID, requestID)
		_ = p.Budget.Reconcile(ctx, res.ID, ObservedUsage{Unknown: true})
		return nil, err
	}
	_ = p.Budget.Reconcile(ctx, res.ID, usage)
	_ = p.DB.Model(&modelfinance.ModelCache{}).Where("run_id = ? AND task_id = ? AND request_id = ?", tok.RunID, tok.TaskID, requestID).Updates(map[string]any{
		"status": "succeeded", "response": datatypes.JSON(raw), "updated_at": time.Now().UTC(),
	})
	return raw, nil
}

func (p *ModelProxy) failCache(runID, taskID, requestID string) {
	_ = p.DB.Model(&modelfinance.ModelCache{}).Where("run_id = ? AND task_id = ? AND request_id = ?", runID, taskID, requestID).Update("status", "failed")
}

func (p *ModelProxy) forward(ctx context.Context, run modelfinance.ResearchRun, protocol string, body []byte) ([]byte, ObservedUsage, error) {
	if err := ctx.Err(); err != nil {
		return nil, ObservedUsage{}, NewError(409, "conflict", "RUN_CLOSED", "请求已取消")
	}
	if err := requireModelFeeCap(); err != nil {
		return nil, ObservedUsage{}, err
	}
	baseURL, apiKey, modelName, cfgProto, err := p.frozenModel(run)
	if err != nil {
		return nil, ObservedUsage{}, err
	}
	if cfgProto != protocol {
		return nil, ObservedUsage{}, NewError(409, "conflict", "MODEL_PROTOCOL_MISMATCH", "冻结配置与入口协议不符")
	}
	if err := ValidateUpstreamURL(baseURL); err != nil {
		return nil, ObservedUsage{}, err
	}
	payload := rewriteModel(body, modelName)
	var raw []byte
	var code int
	switch protocol {
	case ProtocolResponses:
		raw, code, err = (&ResponsesAdapter{Client: p.Client}).Forward(baseURL, apiKey, payload)
	default:
		raw, code, err = (&ChatCompletionsAdapter{Client: p.Client}).Forward(baseURL, apiKey, payload)
	}
	if err != nil {
		return nil, ObservedUsage{}, err
	}
	if code < 200 || code >= 300 {
		return nil, ObservedUsage{}, NewError(503, "unavailable", "UPSTREAM_ERROR", "上游模型返回失败")
	}
	return raw, usageFromModelBody(raw), nil
}

func requireModelFeeCap() error {
	raw := strings.TrimSpace(os.Getenv("ZHIGU_MODEL_FEE_CAP"))
	if raw == "" {
		return NewError(409, "budget", "FEE_CAP_REQUIRED", "未配置模型费用上限，拒绝出站")
	}
	d, err := decimal.NewFromString(raw)
	if err != nil || !d.IsPositive() {
		return NewError(409, "budget", "FEE_CAP_REQUIRED", "模型费用上限无效")
	}
	return nil
}

func (p *ModelProxy) frozenModel(run modelfinance.ResearchRun) (baseURL, apiKey, modelName, protocol string, err error) {
	var cfg map[string]string
	_ = json.Unmarshal(run.ConfigVersions, &cfg)
	id := ""
	if cfg != nil {
		id = strings.TrimSpace(cfg["model_config_id"])
	}
	if id == "" {
		return "", "", "", "", NewError(409, "conflict", "MODEL_NOT_CONFIGURED", "研究未冻结可用模型配置")
	}
	var row modelfinance.ConfigVersion
	if err := p.DB.Where("id = ? AND kind = ?", id, "model").Take(&row).Error; err != nil {
		return "", "", "", "", NewError(409, "conflict", "MODEL_NOT_CONFIGURED", "冻结的模型配置不存在")
	}
	var pub map[string]any
	_ = json.Unmarshal(row.PublicConfig, &pub)
	baseURL, _ = pub["base_url"].(string)
	modelName, _ = pub["model"].(string)
	rawProto, _ := pub["protocol"].(string)
	protocol, err = NormalizeProtocol(rawProto)
	if err != nil {
		return "", "", "", "", err
	}
	apiKey, err = DecryptSecret(row.SecretCiphertext)
	if err != nil || strings.TrimSpace(apiKey) == "" {
		return "", "", "", "", NewError(409, "conflict", "MODEL_NOT_CONFIGURED", "模型配置没有可用密钥")
	}
	if strings.TrimSpace(baseURL) == "" {
		return "", "", "", "", NewError(409, "conflict", "MODEL_NOT_CONFIGURED", "模型配置没有上游地址")
	}
	return baseURL, apiKey, modelName, protocol, nil
}

func rewriteModel(body []byte, modelName string) []byte {
	if strings.TrimSpace(modelName) == "" {
		return body
	}
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return body
	}
	payload["model"] = modelName
	raw, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return raw
}

func usageFromModelBody(raw []byte) ObservedUsage {
	var doc struct {
		ID    string `json:"id"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			InputTokens      int `json:"input_tokens"`
			OutputTokens     int `json:"output_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return ObservedUsage{Unknown: true, UpstreamRequestID: ""}
	}
	in, out := doc.Usage.PromptTokens, doc.Usage.CompletionTokens
	if doc.Usage.InputTokens > 0 || doc.Usage.OutputTokens > 0 {
		in, out = doc.Usage.InputTokens, doc.Usage.OutputTokens
	}
	if in == 0 && out == 0 {
		return ObservedUsage{Unknown: true, UpstreamRequestID: doc.ID}
	}
	return ObservedUsage{ActualInput: intPtr(in), ActualOutput: intPtr(out), UpstreamRequestID: doc.ID}
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
