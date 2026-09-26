package finance

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"gorm.io/gorm"

	modelfinance "zhigu/server/model/finance"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func activateTestModel(t *testing.T, db *gorm.DB) {
	t.Helper()
	cfg, err := NewConfigService(db).SaveModel(context.Background(), map[string]any{
		"protocol": ProtocolChatCompletions,
		"base_url": "https://api.openai.com/v1",
		"model":    "finance-research",
	}, "sk-test")
	if err != nil {
		t.Fatal(err)
	}
	var row modelfinance.ConfigVersion
	if err := db.Where("id = ?", cfg.ID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	if err := NewConfigService(db).MarkTested(context.Background(), cfg.ID, row.ConfigDigest); err != nil {
		t.Fatal(err)
	}
	if err := NewConfigService(db).Activate(context.Background(), cfg.ID); err != nil {
		t.Fatal(err)
	}
}

func stubModelClient(body string) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
}

func TestModelProxyFailClosedWithoutConfigOrFeeCap(t *testing.T) {
	s := setup(t)
	r := createMinimalRun(t, s, 1001, "proxy-closed")
	task := reviewTaskID(t, s, r.RunID)
	token, err := s.IssueTaskToken(r.RunID, task, "research")
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"model":"finance-research","messages":[{"role":"user","content":"hello"}]}`)
	proxy := NewModelProxy(s.DB, NewDBBudget(s.DB))
	hits := 0
	proxy.Client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		hits++
		return nil, io.EOF
	})}
	t.Setenv("ZHIGU_MODEL_FEE_CAP", "1")
	_, err = proxy.Complete(context.Background(), "proxy-closed-1", NormalizeJSONHash(body), HeaderTokenHash(token), body, ProtocolChatCompletions)
	if ErrorCode(err) != "MODEL_NOT_CONFIGURED" {
		t.Fatalf("missing config: %v", err)
	}
	if hits != 0 {
		t.Fatal("missing config still dialed upstream")
	}
	_ = s.DB.Model(&modelfinance.ResearchRun{}).Where("id = ?", r.RunID).Update("status", StatusCanceled)

	activateTestModel(t, s.DB)
	t.Setenv("ZHIGU_MODEL_FEE_CAP", "")
	r2 := createMinimalRun(t, s, 1001, "proxy-fee")
	token2, err := s.IssueTaskToken(r2.RunID, reviewTaskID(t, s, r2.RunID), "research")
	if err != nil {
		t.Fatal(err)
	}
	_, err = proxy.Complete(context.Background(), "proxy-fee-1", NormalizeJSONHash(body), HeaderTokenHash(token2), body, ProtocolChatCompletions)
	if ErrorCode(err) != "FEE_CAP_REQUIRED" {
		t.Fatalf("missing fee cap: %v", err)
	}
	if hits != 0 {
		t.Fatal("missing fee cap still dialed upstream")
	}
}

func TestModelProxyForwardsChatCompletions(t *testing.T) {
	s := setup(t)
	activateTestModel(t, s.DB)
	t.Setenv("ZHIGU_MODEL_FEE_CAP", "2.50")
	r := createMinimalRun(t, s, 1001, "proxy-forward")
	token, err := s.IssueTaskToken(r.RunID, reviewTaskID(t, s, r.RunID), "research")
	if err != nil {
		t.Fatal(err)
	}
	var seen []byte
	proxy := NewModelProxy(s.DB, NewDBBudget(s.DB))
	proxy.Client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.HasSuffix(req.URL.Path, "/chat/completions") {
			t.Errorf("path %s", req.URL.Path)
		}
		seen, _ = io.ReadAll(req.Body)
		raw := `{"id":"chatcmpl_live","choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":4,"completion_tokens":2}}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(raw)), Header: make(http.Header)}, nil
	})}
	body := []byte(`{"model":"finance-research","messages":[{"role":"user","content":"hello"}]}`)
	out, err := proxy.Complete(context.Background(), "proxy-forward-1", NormalizeJSONHash(body), HeaderTokenHash(token), body, ProtocolChatCompletions)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out, []byte("chatcmpl_live")) {
		t.Fatalf("upstream body missing: %s", out)
	}
	if !bytes.Contains(seen, []byte("研究口径（服务端冻结）")) {
		t.Fatalf("scope prefix missing: %s", seen)
	}
}

func TestGrantRejectsChangedArgs(t *testing.T) {
	s := setup(t)
	r := createMinimalRun(t, s, 1001, "grant-args")
	task := reviewTaskID(t, s, r.RunID)
	first := map[string]any{"metrics": []string{"revenue"}}
	g, err := s.CreateGrant(context.Background(), r.RunID, task, GrantIn{RequestID: "grant-args-1", ToolName: "get_financials", ArgsHash: mustArgsHash(t, first)})
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.CreateGrant(context.Background(), r.RunID, task, GrantIn{RequestID: "grant-args-1", ToolName: "get_financials", ArgsHash: mustArgsHash(t, first)})
	if err != nil || again.ID != g.ID {
		t.Fatalf("same grant replay: %v %+v", err, again)
	}
	changed := map[string]any{"metrics": []string{"net_income"}}
	if _, err := s.CreateGrant(context.Background(), r.RunID, task, GrantIn{RequestID: "grant-args-1", ToolName: "get_financials", ArgsHash: mustArgsHash(t, changed)}); ErrorCode(err) != "GRANT_PAYLOAD_CONFLICT" {
		t.Fatalf("changed args: %v", err)
	}
}
