package finance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	api "zhigu/server/api/v1/finance"
	model "zhigu/server/model/finance"
	f "zhigu/server/service/finance"
	"zhigu/server/testdb"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../../.."))
}

func TestR2CrossProcessFixture(t *testing.T) {
	db := testdb.Start(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("probe-password"), bcrypt.MinCost)
	for _, id := range []uint{1001, 1002} {
		if e := db.Create(&model.User{ID: id, Username: fmt.Sprintf("probe-%d", id), PasswordHash: string(hash), Role: "user", CreatedAt: time.Now(), UpdatedAt: time.Now()}).Error; e != nil {
			t.Fatal(e)
		}
	}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	pyURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	budget := f.NewDBBudget(db)
	svc := f.NewService(db, &f.HTTPResearchClient{Base: pyURL, Token: "r2-internal", Client: &http.Client{Timeout: 2 * time.Second}}, budget, f.NewFixtureConfig())
	t.Setenv("ZHIGU_INTERNAL_TOKEN", "r2-internal")
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())
	api.Register(engine, svc, f.NewModelProxy(db, budget))
	server := httptest.NewServer(engine)
	defer server.Close()
	root := repoRoot(t)
	pyRoot := filepath.Join(root, "app/research-service")
	pyBin := filepath.Join(pyRoot, ".venv/bin/python")
	if _, err := os.Stat(pyBin); err != nil {
		t.Fatalf("research-service venv missing: %v", err)
	}
	proc := exec.Command(pyBin, "-m", "uvicorn", "app.main:app", "--host", "127.0.0.1", "--port", fmt.Sprint(port), "--log-level", "warning")
	proc.Dir = pyRoot
	proc.Env = append(os.Environ(), "ZHIGU_INTERNAL_TOKEN=r2-internal", "ZHIGU_RESEARCH_EXECUTOR=fixture", "ZHIGU_GO_INTERNAL_URL="+server.URL, "ZHIGU_RESEARCH_SQLITE="+filepath.Join(t.TempDir(), "jobs.sqlite"), "PYTHONDONTWRITEBYTECODE=1")
	var logs bytes.Buffer
	proc.Stdout = &logs
	proc.Stderr = &logs
	if e = proc.Start(); e != nil {
		t.Fatal(e)
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
		res, e := http.Get(pyURL + "/healthz")
		if e == nil {
			res.Body.Close()
			ready = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		proc.Process.Kill()
		proc.Wait()
		stopped = true
		t.Fatalf("python not ready: %s", logs.String())
	}
	request := func(method, path, token, key string, payload any) map[string]any {
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
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		if res.StatusCode >= 300 {
			t.Fatalf("%s %s: %d %s", method, path, res.StatusCode, b)
		}
		var envelope map[string]any
		json.Unmarshal(b, &envelope)
		return envelope["data"].(map[string]any)
	}
	create := func(id uint) (string, string) {
		token := request("POST", "/api/finance/auth/login", "", "", map[string]any{"username": fmt.Sprintf("probe-%d", id), "password": "probe-password"})["token"].(string)
		draft := request("POST", "/api/finance/claims/parse", token, "", map[string]any{"text": "演示公司的收入增长能否支持未来一年股价上涨？这是一条超过二十个字的测试观点。"})
		out := request("POST", "/api/finance/research", token, fmt.Sprintf("r2-http-%d", id), map[string]any{"draft_id": draft["draft_id"], "revision": draft["revision"], "instrument_id": "DEMO:COMPANY", "horizon": draft["suggested_horizon"], "as_of": "2026-09-18T00:00:00Z", "claim_text": "演示公司的收入增长能否支持未来一年股价上涨？这是一条超过二十个字的测试观点。"})
		return token, out["run_id"].(string)
	}
	token, id := create(1001)
	svc.Tick(context.Background(), false)
	view := request("GET", "/api/finance/research/"+id, token, "", nil)
	if view["status"] != "completed" {
		t.Fatalf("fixture full path not complete: %v", view)
	}
	report, ok := view["report"].(map[string]any)
	if !ok {
		t.Fatal("missing report")
	}
	ids, _ := report["evidence_ids"].([]any)
	if len(ids) != 2 {
		t.Fatalf("want two registered evidence records, got %v", report)
	}
	for _, eid := range ids {
		ev := request("GET", "/api/finance/evidence/"+eid.(string), token, "", nil)
		if ev["text"] == "" {
			t.Fatal("empty evidence")
		}
	}
	var grants, records, links int64
	db.Model(&model.ToolGrant{}).Count(&grants)
	db.Model(&model.DataRecord{}).Count(&records)
	db.Model(&model.TaskEvidence{}).Count(&links)
	t.Logf("HTTP login→parse→run→Eino→Python→Go tools→report PASS: grants=%d data_records=%d task_evidence=%d", grants, records, links)
	proc.Process.Kill()
	proc.Wait()
	stopped = true
	token, id = create(1002)
	svc.Tick(context.Background(), false)
	view = request("GET", "/api/finance/research/"+id, token, "", nil)
	if view["status"] != "failed" || view["report"] != nil {
		t.Fatalf("Python outage was masked: %v", view)
	}
	t.Log("Python stopped → new research failed without report PASS")
}
