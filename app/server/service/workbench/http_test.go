package workbench_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	api "zhigu/server/api/v1/finance"
	model "zhigu/server/model/finance"
	f "zhigu/server/service/finance"
	"zhigu/server/service/market"
	"zhigu/server/service/workbench"
	"zhigu/server/testdb"
)

func TestStrategyHTTPSearchGenerateAndIsolation(t *testing.T) {
	db := testdb.Start(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("Passw0rd!"), bcrypt.MinCost)
	for _, u := range []model.User{
		{ID: 1001, Username: "invitee", PasswordHash: string(hash), Role: "user"},
		{ID: 1002, Username: "invitee-b", PasswordHash: string(hash), Role: "user"},
	} {
		if err := db.Create(&u).Error; err != nil {
			t.Fatal(err)
		}
	}
	svc := f.NewService(db, f.NewFakeResearchClient(), f.NewMemoryBudget(), f.NewFixtureConfig())
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	api.Register(engine, svc, f.NewModelProxy(db, f.NewMemoryBudget()))
	api.RegisterStrategy(engine, workbench.NewHub(db, market.NewService(db)))
	server := httptest.NewServer(engine)
	defer server.Close()

	unauth, err := http.Get(server.URL + "/api/finance/market/instruments?q=茅台")
	if err != nil {
		t.Fatal(err)
	}
	unauth.Body.Close()
	if unauth.StatusCode != 401 {
		t.Fatalf("want 401 got %d", unauth.StatusCode)
	}

	tokenA := login(t, server.URL, "invitee")
	tokenB := login(t, server.URL, "invitee-b")
	req, _ := http.NewRequest("GET", server.URL+"/api/finance/market/instruments?q=510300.SH", nil)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("search %d", res.StatusCode)
	}
	var env struct {
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	_ = json.NewDecoder(res.Body).Decode(&env)
	if len(env.Data.Items) == 0 || env.Data.Items[0]["asset_type"] != "fund" {
		t.Fatalf("510300 %+v", env.Data.Items)
	}

	body := []byte(`{"text":"MACD金叉买入，MACD死叉卖出","instrument_id":"00700.HK"}`)
	gen, _ := http.NewRequest("POST", server.URL+"/api/finance/strategy-drafts/generate", bytes.NewReader(body))
	gen.Header.Set("Authorization", "Bearer "+tokenA)
	gen.Header.Set("Content-Type", "application/json")
	gen.Header.Set("Idempotency-Key", "k1")
	gres, err := http.DefaultClient.Do(gen)
	if err != nil {
		t.Fatal(err)
	}
	defer gres.Body.Close()
	if gres.StatusCode != 202 {
		t.Fatalf("generate %d", gres.StatusCode)
	}
	var genv struct {
		Data map[string]any `json:"data"`
	}
	_ = json.NewDecoder(gres.Body).Decode(&genv)
	gid, _ := genv.Data["generation_id"].(string)
	var status string
	for i := 0; i < 40; i++ {
		time.Sleep(25 * time.Millisecond)
		q, _ := http.NewRequest("GET", server.URL+"/api/finance/strategy-generations/"+gid, nil)
		q.Header.Set("Authorization", "Bearer "+tokenA)
		qr, err := http.DefaultClient.Do(q)
		if err != nil {
			t.Fatal(err)
		}
		var qenv struct {
			Data map[string]any `json:"data"`
		}
		_ = json.NewDecoder(qr.Body).Decode(&qenv)
		qr.Body.Close()
		status, _ = qenv.Data["status"].(string)
		if status == "ready" || status == "failed" || status == "unsupported" || status == "needs_clarification" {
			break
		}
	}
	if status != "ready" {
		t.Fatalf("gen status %s", status)
	}
	other, _ := http.NewRequest("GET", server.URL+"/api/finance/strategy-generations/"+gid, nil)
	other.Header.Set("Authorization", "Bearer "+tokenB)
	ores, _ := http.DefaultClient.Do(other)
	ores.Body.Close()
	if ores.StatusCode != 404 {
		t.Fatalf("cross-user want 404 got %d", ores.StatusCode)
	}

	det, _ := http.NewRequest("GET", server.URL+"/api/finance/market/instruments/00700.HK", nil)
	det.Header.Set("Authorization", "Bearer "+tokenA)
	dres, err := http.DefaultClient.Do(det)
	if err != nil {
		t.Fatal(err)
	}
	dres.Body.Close()
	if dres.StatusCode != 200 {
		t.Fatalf("instrument %d", dres.StatusCode)
	}

	draftReq, _ := http.NewRequest("GET", server.URL+"/api/finance/strategy-generations/"+gid, nil)
	draftReq.Header.Set("Authorization", "Bearer "+tokenA)
	dfr, err := http.DefaultClient.Do(draftReq)
	if err != nil {
		t.Fatal(err)
	}
	var dfe struct {
		Data map[string]any `json:"data"`
	}
	_ = json.NewDecoder(dfr.Body).Decode(&dfe)
	dfr.Body.Close()
	draftID, _ := dfe.Data["draft_id"].(string)
	rev := 0
	switch v := dfe.Data["draft_revision"].(type) {
	case float64:
		rev = int(v)
	}
	saveBody, _ := json.Marshal(map[string]any{"draft_id": draftID, "revision": rev})
	save, _ := http.NewRequest("POST", server.URL+"/api/finance/strategies", bytes.NewReader(saveBody))
	save.Header.Set("Authorization", "Bearer "+tokenA)
	save.Header.Set("Content-Type", "application/json")
	save.Header.Set("Idempotency-Key", "save-1")
	sres, err := http.DefaultClient.Do(save)
	if err != nil {
		t.Fatal(err)
	}
	if sres.StatusCode != 200 {
		t.Fatalf("save %d", sres.StatusCode)
	}
	var senv struct {
		Data map[string]any `json:"data"`
	}
	_ = json.NewDecoder(sres.Body).Decode(&senv)
	sres.Body.Close()
	vid, _ := senv.Data["version_id"].(string)
	if vid == "" {
		t.Fatalf("save body %+v", senv.Data)
	}

	btBody, _ := json.Marshal(map[string]any{
		"strategy_version_id":  vid,
		"instrument_id":        "00700.HK",
		"start":                "2025-01-02",
		"end":                  "2026-09-18",
		"initial_cash":         "100000",
		"currency":             "HKD",
		"fee_schedule_id":      "product_default_assumption",
		"commission_config":    "0.00025",
		"slippage_bps":         "10",
		"participation_cap":    "0.01",
		"benchmark":            "buy_hold",
		"execution_profile_id": "next_session_open",
	})
	bt, _ := http.NewRequest("POST", server.URL+"/api/finance/backtests", bytes.NewReader(btBody))
	bt.Header.Set("Authorization", "Bearer "+tokenA)
	bt.Header.Set("Content-Type", "application/json")
	bt.Header.Set("Idempotency-Key", "bt-1")
	bres, err := http.DefaultClient.Do(bt)
	if err != nil {
		t.Fatal(err)
	}
	if bres.StatusCode != 202 {
		t.Fatalf("backtest %d", bres.StatusCode)
	}
	var benv struct {
		Data map[string]any `json:"data"`
	}
	_ = json.NewDecoder(bres.Body).Decode(&benv)
	bres.Body.Close()
	rid, _ := benv.Data["run_id"].(string)
	var btStatus string
	for i := 0; i < 80; i++ {
		time.Sleep(40 * time.Millisecond)
		q, _ := http.NewRequest("GET", server.URL+"/api/finance/backtests/"+rid, nil)
		q.Header.Set("Authorization", "Bearer "+tokenA)
		qr, err := http.DefaultClient.Do(q)
		if err != nil {
			t.Fatal(err)
		}
		var qenv struct {
			Data map[string]any `json:"data"`
		}
		_ = json.NewDecoder(qr.Body).Decode(&qenv)
		qr.Body.Close()
		btStatus, _ = qenv.Data["status"].(string)
		if btStatus == "succeeded" || btStatus == "failed" || btStatus == "insufficient_data" || btStatus == "canceled" {
			break
		}
	}
	if btStatus != "succeeded" {
		t.Fatalf("backtest status %s", btStatus)
	}
	resReq, _ := http.NewRequest("GET", server.URL+"/api/finance/backtests/"+rid+"/results", nil)
	resReq.Header.Set("Authorization", "Bearer "+tokenA)
	rr, err := http.DefaultClient.Do(resReq)
	if err != nil {
		t.Fatal(err)
	}
	if rr.StatusCode != 200 {
		t.Fatalf("results %d", rr.StatusCode)
	}
	rr.Body.Close()
	otherBT, _ := http.NewRequest("GET", server.URL+"/api/finance/backtests/"+rid+"/results", nil)
	otherBT.Header.Set("Authorization", "Bearer "+tokenB)
	obr, _ := http.DefaultClient.Do(otherBT)
	obr.Body.Close()
	if obr.StatusCode != 404 {
		t.Fatalf("cross-user results want 404 got %d", obr.StatusCode)
	}
}

func login(t *testing.T, base, user string) string {
	t.Helper()
	login, err := http.Post(base+"/api/finance/auth/login", "application/json", bytes.NewReader([]byte(`{"username":"`+user+`","password":"Passw0rd!"}`)))
	if err != nil {
		t.Fatal(err)
	}
	defer login.Body.Close()
	var env struct {
		Data map[string]any `json:"data"`
	}
	_ = json.NewDecoder(login.Body).Decode(&env)
	tok, _ := env.Data["token"].(string)
	if tok == "" {
		t.Fatal("no token")
	}
	return tok
}
