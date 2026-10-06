package futures

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ExecuteInternalModel is the only model boundary for futures. It reads a
// separately configured public endpoint and never reuses finance stock-run grants.
func (d *Domain) ExecuteInternalModel(ctx context.Context, spec GrantSpec, arguments map[string]any) (map[string]any, error) {
	if err := d.AuthorizeExecution(ctx, Identity{OwnerID: spec.OwnerID, Mode: spec.Mode}); err != nil {
		return nil, err
	}
	endpoint := os.Getenv("FUTURES_MODEL_URL")
	if endpoint == "" {
		return nil, ErrUnavailable
	}
	apiKey := os.Getenv("FUTURES_MODEL_API_KEY")
	modelName := os.Getenv("FUTURES_MODEL_NAME")
	price, err := decimal.NewFromString(os.Getenv("FUTURES_MODEL_PRICE_CNY_PER_1K"))
	if modelName == "" || err != nil || !price.IsPositive() {
		return nil, ErrUnavailable
	}
	tokens := estimateMessageTokens(arguments["messages"]) + 4096
	budget := NewDBBudget(d.DB)
	day := beijingDay(time.Now().UTC())
	if err := budget.SeedAccount(ctx, UserAccount, spec.OwnerID, day, decimal.RequireFromString("5"), 0, 0, 0); err != nil {
		return nil, err
	}
	if err := budget.SeedAccount(ctx, ModuleAccount, 0, day, decimal.RequireFromString("50"), 0, 0, 0); err != nil {
		return nil, err
	}
	attemptID := "model_" + uuid.NewString()
	maxModel, maxTools, maxTokens := executionBudgetLimits(ctx, d.DB, spec.OwnerID, spec.Mode, spec.RunID)
	maximumCost := price.Mul(decimal.NewFromInt(int64(tokens))).Div(decimal.NewFromInt(1000))
	reservation, err := budget.Reserve(ctx, Reservation{AttemptID: attemptID, OwnerID: spec.OwnerID, RunID: spec.RunID, BudgetDay: day, AmountCNY: maximumCost, Tokens: tokens, Kind: "model", MaxModelCalls: maxModel, MaxToolCalls: maxTools, MaxTokens: maxTokens})
	if err != nil {
		return nil, err
	}
	body := map[string]any{"model": modelName, "messages": arguments["messages"], "max_output_tokens": 4096}
	raw, err := json.Marshal(body)
	if err != nil {
		_ = budget.SettleUnknown(ctx, spec.OwnerID, attemptID, reservation.AmountCNY, reservation.Tokens)
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		_ = budget.SettleUnknown(ctx, spec.OwnerID, attemptID, reservation.AmountCNY, reservation.Tokens)
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		_ = budget.SettleUnknown(ctx, spec.OwnerID, attemptID, reservation.AmountCNY, reservation.Tokens)
		return nil, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_ = budget.SettleUnknown(ctx, spec.OwnerID, attemptID, reservation.AmountCNY, reservation.Tokens)
		return nil, ErrUnavailable
	}
	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		_ = budget.SettleUnknown(ctx, spec.OwnerID, attemptID, reservation.AmountCNY, reservation.Tokens)
		return nil, ErrUnavailable
	}
	content := payload["content"]
	if content == nil {
		if choices, ok := payload["choices"].([]any); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]any); ok {
				if message, ok := choice["message"].(map[string]any); ok {
					content = message["content"]
				} else {
					content = choice["message"]
				}
			}
		}
	}
	if content == nil {
		_ = budget.SettleUnknown(ctx, spec.OwnerID, attemptID, reservation.AmountCNY, reservation.Tokens)
		return nil, ErrUnavailable
	}
	if inputTokens, outputTokens, ok := modelUsage(payload); ok {
		actualTokens := inputTokens + outputTokens
		actualCost := price.Mul(decimal.NewFromInt(int64(actualTokens))).Div(decimal.NewFromInt(1000))
		if err := budget.SettleKnown(ctx, spec.OwnerID, attemptID, actualCost, actualTokens); err != nil {
			return nil, err
		}
	} else if err := budget.SettleUnknown(ctx, spec.OwnerID, attemptID, reservation.AmountCNY, reservation.Tokens); err != nil {
		return nil, err
	}
	return map[string]any{"content": content}, nil
}

func estimateMessageTokens(value any) int {
	raw, _ := json.Marshal(value)
	return len(raw)/4 + 1
}

func modelUsage(payload map[string]any) (int, int, bool) {
	usage, ok := payload["usage"].(map[string]any)
	if !ok {
		return 0, 0, false
	}
	input, ok1 := usage["prompt_tokens"].(float64)
	output, ok2 := usage["completion_tokens"].(float64)
	return int(input), int(output), ok1 && ok2
}
