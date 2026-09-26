package finance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	api "zhigu/server/api/v1/finance"
	model "zhigu/server/model/finance"
	f "zhigu/server/service/finance"
	"zhigu/server/testdb"
)

func TestResearchViewpointCrossProcessFixture(t *testing.T) {
	db := testdb.Start(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("probe-password"), bcrypt.MinCost)
	if err := db.Create(&model.User{ID: 1001, Username: "rv-probe", PasswordHash: string(hash), Role: "user", CreatedAt: time.Now(), UpdatedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	pyPort := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	pyURL := fmt.Sprintf("http://127.0.0.1:%d", pyPort)
	budget := f.NewDBBudget(db)
	svc := f.NewService(db, &f.HTTPResearchClient{Base: pyURL, Token: "rv-internal", Client: &http.Client{Timeout: 3 * time.Second}}, budget, f.NewFixtureConfig())
	t.Setenv("ZHIGU_INTERNAL_TOKEN", "rv-internal")
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())
	api.Register(engine, svc, f.NewModelProxy(db, budget))
	server := httptest.NewServer(engine)
	defer server.Close()
	t.Setenv("ZHIGU_RESEARCH_DOCUMENT_DIR", t.TempDir())
	root := repoRoot(t)
	pyRoot := filepath.Join(root, "app/research-service")
	pyBin := filepath.Join(pyRoot, ".venv/bin/python")
	if _, err := os.Stat(pyBin); err != nil {
		t.Fatalf("research-service venv missing: %v", err)
	}
	proc := exec.Command(pyBin, "-m", "uvicorn", "app.main:app", "--host", "127.0.0.1", "--port", fmt.Sprint(pyPort), "--log-level", "warning")
	proc.Dir = pyRoot
	proc.Env = append(os.Environ(), "ZHIGU_INTERNAL_TOKEN=rv-internal", "ZHIGU_RESEARCH_EXECUTOR=fixture", "ZHIGU_GO_INTERNAL_URL="+server.URL, "ZHIGU_RESEARCH_SQLITE="+filepath.Join(t.TempDir(), "jobs.sqlite"), "PYTHONDONTWRITEBYTECODE=1")
	var logs bytes.Buffer
	proc.Stdout = &logs
	proc.Stderr = &logs
	if err := proc.Start(); err != nil {
		t.Fatal(err)
	}
	stopped := false
	defer func() {
		if !stopped {
			proc.Process.Kill()
			proc.Wait()
		}
	}()
	ready := false
	for i := 0; i < 100; i++ {
		res, err := http.Get(pyURL + "/healthz")
		if err == nil {
			res.Body.Close()
			ready = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		t.Fatalf("python not ready: %s", logs.String())
	}

	requestJSON := func(method, path, token, key string, payload any) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(payload)
		req, _ := http.NewRequest(method, server.URL+path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		if res.StatusCode >= 300 {
			t.Fatalf("%s %s: %d %s", method, path, res.StatusCode, body)
		}
		var envelope map[string]any
		_ = json.Unmarshal(body, &envelope)
		data, _ := envelope["data"].(map[string]any)
		return data
	}

	login := requestJSON("POST", "/api/finance/auth/login", "", "", map[string]any{"username": "rv-probe", "password": "probe-password"})
	token, _ := login["token"].(string)

	var upload bytes.Buffer
	writer := multipart.NewWriter(&upload)
	file, _ := writer.CreateFormFile("file", "report.txt")
	_, _ = file.Write([]byte("研报认为公司收入将继续增长。"))
	_ = writer.Close()
	uploadReq, _ := http.NewRequest("POST", server.URL+"/api/finance/research-documents", &upload)
	uploadReq.Header.Set("Authorization", "Bearer "+token)
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
	uploadRes, err := http.DefaultClient.Do(uploadReq)
	if err != nil {
		t.Fatal(err)
	}
	uploadBody, _ := io.ReadAll(uploadRes.Body)
	uploadRes.Body.Close()
	if uploadRes.StatusCode >= 300 {
		t.Fatalf("upload: %d %s", uploadRes.StatusCode, uploadBody)
	}
	var uploadEnvelope struct {
		Data struct {
			DocumentID string `json:"document_id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(uploadBody, &uploadEnvelope)
	if uploadEnvelope.Data.DocumentID == "" {
		t.Fatalf("missing document id: %s", uploadBody)
	}
	docID := uploadEnvelope.Data.DocumentID

	draft := requestJSON("POST", "/api/finance/claims/parse", token, "", map[string]any{"document_id": docID, "focus_text": "现金流"})
	if draft["input_mode"] != "report_only" {
		t.Fatalf("input mode=%v", draft)
	}
	start, end := "2026-01-01", "2026-12-31"
	patched := requestJSON("PATCH", "/api/finance/claims/"+draft["draft_id"].(string), token, "", map[string]any{
		"revision": draft["revision"], "instrument_id": "DEMO:COMPANY", "horizon_start": start, "horizon_end": end,
	})
	run := requestJSON("POST", "/api/finance/research", token, "rv-chain", map[string]any{"draft_id": patched["draft_id"], "revision": patched["revision"]})
	runID, _ := run["run_id"].(string)
	svc.Tick(context.Background(), false)
	view := requestJSON("GET", "/api/finance/research/"+runID, token, "", nil)
	if view["status"] != "completed" {
		t.Fatalf("chain did not complete: %v", view)
	}
	report, ok := view["report"].(map[string]any)
	if !ok || report["schema_version"] != "research-report.v2" {
		t.Fatalf("missing v2 report: %v", view)
	}
	if report["document_id"] == nil && view["document_id"] != docID {
		t.Fatalf("document not attached: %v", view)
	}
	factChecks, _ := report["fact_checks"].([]any)
	if len(factChecks) == 0 {
		t.Fatalf("missing fact checks: %v", report)
	}
	evidenceIndex, _ := report["evidence_index"].([]any)
	if len(evidenceIndex) == 0 {
		t.Fatalf("missing evidence index: %v", report)
	}
	exportReq, _ := http.NewRequest("GET", server.URL+"/api/finance/research/"+runID+"/export?format=html", nil)
	exportReq.Header.Set("Authorization", "Bearer "+token)
	exportRes, err := http.DefaultClient.Do(exportReq)
	if err != nil {
		t.Fatal(err)
	}
	exportBody, _ := io.ReadAll(exportRes.Body)
	exportRes.Body.Close()
	html := string(exportBody)
	if exportRes.StatusCode != http.StatusOK || !strings.Contains(html, "本报告仅供研究参考，不构成投资建议") || strings.Contains(strings.ToLower(html), "<script") {
		t.Fatalf("export invalid: status=%d body=%s", exportRes.StatusCode, html)
	}
	proc.Process.Kill()
	proc.Wait()
	stopped = true
	t.Log("Vue contract fixtures + Go/PostgreSQL/Python/document evidence/report export chain PASS")
}
