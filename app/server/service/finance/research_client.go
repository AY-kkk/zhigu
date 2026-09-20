package finance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type HTTPResearchClient struct {
	Base   string
	Token  string
	Client *http.Client
}

func NewHTTPResearchClient() (ResearchClient, error) {
	mode := os.Getenv("ZHIGU_RESEARCH_MODE")
	if mode == "unit-test" {
		return NewFakeResearchClient(), nil
	}
	base := os.Getenv("ZHIGU_RESEARCH_URL")
	if base == "" {
		return nil, fmt.Errorf("ZHIGU_RESEARCH_URL is required unless ZHIGU_RESEARCH_MODE=unit-test")
	}
	token := os.Getenv("ZHIGU_INTERNAL_TOKEN")
	if token == "" {
		token = "zhigu-internal-dev"
	}
	return &HTTPResearchClient{
		Base:  base,
		Token: token,
		Client: &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func MustHTTPResearchClient() ResearchClient {
	c, err := NewHTTPResearchClient()
	if err != nil {
		panic(err)
	}
	return c
}

func (c *HTTPResearchClient) Submit(ctx context.Context, task ResearchTask) (TaskReceipt, error) {
	raw, _ := json.Marshal(task)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+"/internal/research/tasks", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	if task.TaskToken != "" {
		req.Header.Set("X-Zhigu-Task-Token", task.TaskToken)
	}
	res, err := c.Client.Do(req)
	if err != nil {
		return TaskReceipt{}, NewError(503, "transport", "PYTHON_UNAVAILABLE", err.Error())
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode == 409 {
		return TaskReceipt{}, NewError(409, "conflict", "TASK_PAYLOAD_CONFLICT", "任务幂等冲突")
	}
	if res.StatusCode >= 300 {
		return TaskReceipt{}, NewError(res.StatusCode, "transport", "PYTHON_ERROR", string(body))
	}
	var out TaskReceipt
	_ = json.Unmarshal(body, &out)
	return out, nil
}

func (c *HTTPResearchClient) Get(ctx context.Context, taskID string) (TaskSnapshot, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/internal/research/tasks/%s", c.Base, taskID), nil)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	res, err := c.do(req)
	if err != nil {
		return TaskSnapshot{}, err
	}
	defer res.Body.Close()
	out, err := decodeTaskSnapshot(res)
	if err != nil {
		return TaskSnapshot{}, err
	}
	if isTerminalTaskStatus(out.Status) {
		_ = c.ack(ctx, taskID)
	}
	return out, nil
}

func (c *HTTPResearchClient) Cancel(ctx context.Context, taskID string) (TaskSnapshot, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/internal/research/tasks/%s/cancel", c.Base, taskID), nil)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	res, err := c.do(req)
	if err != nil {
		return TaskSnapshot{}, err
	}
	defer res.Body.Close()
	out, err := decodeTaskSnapshot(res)
	if err != nil {
		return TaskSnapshot{}, err
	}
	if !isTerminalTaskStatus(out.Status) {
		return TaskSnapshot{}, NewError(502, "transport", "CANCEL_NOT_CONFIRMED", "执行器未确认取消")
	}
	return out, nil
}

func (c *HTTPResearchClient) Purge(ctx context.Context, taskID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/internal/research/tasks/%s/purge", c.Base, taskID), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	res, err := c.Client.Do(req)
	if err != nil {
		return NewError(503, "transport", "PYTHON_UNAVAILABLE", err.Error())
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return nil
	}
	if res.StatusCode >= 300 {
		body, _ := io.ReadAll(res.Body)
		return NewError(res.StatusCode, "transport", "PYTHON_ERROR", string(body))
	}
	return nil
}

func (c *HTTPResearchClient) do(req *http.Request) (*http.Response, error) {
	cli := c.Client
	if cli == nil {
		cli = http.DefaultClient
	}
	res, err := cli.Do(req)
	if err != nil {
		return nil, NewError(503, "transport", "PYTHON_UNAVAILABLE", err.Error())
	}
	return res, nil
}

func (c *HTTPResearchClient) ack(ctx context.Context, taskID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/internal/research/tasks/%s/ack", c.Base, taskID), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	res, err := c.do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return nil
	}
	if res.StatusCode >= 300 {
		body, _ := io.ReadAll(res.Body)
		return NewError(res.StatusCode, "transport", "PYTHON_ERROR", string(body))
	}
	return nil
}

func decodeTaskSnapshot(res *http.Response) (TaskSnapshot, error) {
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return TaskSnapshot{}, NewError(res.StatusCode, "transport", "PYTHON_ERROR", string(body))
	}
	var out TaskSnapshot
	if err := json.Unmarshal(body, &out); err != nil {
		return TaskSnapshot{}, NewError(502, "transport", "PYTHON_INVALID_RESPONSE", "无法解析执行器响应")
	}
	return out, nil
}

func isTerminalTaskStatus(status string) bool {
	switch status {
	case "succeeded", "insufficient", "failed", "canceled":
		return true
	default:
		return false
	}
}
