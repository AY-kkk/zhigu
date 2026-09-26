package finance_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	api "zhigu/server/api/v1/finance"
	model "zhigu/server/model/finance"
	f "zhigu/server/service/finance"
	"zhigu/server/testdb"
)

func TestStageBHTTPContractAndAsOf(t *testing.T) {
	db := testdb.Start(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("Passw0rd!"), bcrypt.MinCost)
	if err := db.Create(&model.User{ID: 1001, Username: "invitee", PasswordHash: string(hash), Role: "user"}).Error; err != nil {
		t.Fatal(err)
	}
	svc := f.NewService(db, f.NewFakeResearchClient(), f.NewMemoryBudget(), f.NewFixtureConfig())
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	api.Register(engine, svc, f.NewModelProxy(db, f.NewMemoryBudget()))
	server := httptest.NewServer(engine)
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL+"/internal/finance/data-source-profile", nil)
	req.Header.Set("Authorization", "Bearer zhigu-internal-dev")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 409 {
		t.Fatalf("missing contract want 409 got %d", res.StatusCode)
	}

	req2, _ := http.NewRequest("GET", server.URL+"/internal/finance/data-source-profile", nil)
	req2.Header.Set("Authorization", "Bearer zhigu-internal-dev")
	req2.Header.Set(f.ContractVersionHeader, f.ContractVersion)
	res2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != 200 {
		t.Fatalf("profile want 200 got %d", res2.StatusCode)
	}

	login, err := http.Post(server.URL+"/api/finance/auth/login", "application/json", bytes.NewReader([]byte(`{"username":"invitee","password":"Passw0rd!"}`)))
	if err != nil {
		t.Fatal(err)
	}
	defer login.Body.Close()
	var env struct {
		Data map[string]any `json:"data"`
	}
	_ = json.NewDecoder(login.Body).Decode(&env)
	token, _ := env.Data["token"].(string)
	draftBody := []byte(`{"text":"演示公司的收入增长能否支持未来一年股价上涨？这是一条超过二十个字的测试观点。"}`)
	preq, _ := http.NewRequest("POST", server.URL+"/api/finance/claims/parse", bytes.NewReader(draftBody))
	preq.Header.Set("Authorization", "Bearer "+token)
	preq.Header.Set("Content-Type", "application/json")
	pres, err := http.DefaultClient.Do(preq)
	if err != nil {
		t.Fatal(err)
	}
	defer pres.Body.Close()
	var parsed struct {
		Data map[string]any `json:"data"`
	}
	_ = json.NewDecoder(pres.Body).Decode(&parsed)
	createBody, _ := json.Marshal(map[string]any{
		"draft_id": parsed.Data["draft_id"], "revision": parsed.Data["revision"], "as_of": "2026-09-18T00:00:00Z",
	})
	creq, _ := http.NewRequest("POST", server.URL+"/api/finance/research", bytes.NewReader(createBody))
	creq.Header.Set("Authorization", "Bearer "+token)
	creq.Header.Set("Content-Type", "application/json")
	creq.Header.Set("Idempotency-Key", "asof-http")
	cres, err := http.DefaultClient.Do(creq)
	if err != nil {
		t.Fatal(err)
	}
	defer cres.Body.Close()
	if cres.StatusCode != 400 {
		t.Fatalf("client as_of want 400 got %d", cres.StatusCode)
	}

	ireq, _ := http.NewRequest("GET", server.URL+"/api/finance/instruments?q="+url.QueryEscape("演示"), nil)
	ireq.Header.Set("Authorization", "Bearer "+token)
	ires, err := http.DefaultClient.Do(ireq)
	if err != nil {
		t.Fatal(err)
	}
	defer ires.Body.Close()
	if ires.StatusCode != 200 {
		t.Fatalf("instruments want 200 got %d", ires.StatusCode)
	}
	var ienv struct {
		Data struct {
			Items []map[string]any `json:"items"`
			Mode  string           `json:"mode"`
		} `json:"data"`
	}
	if err := json.NewDecoder(ires.Body).Decode(&ienv); err != nil {
		t.Fatal(err)
	}
	if ienv.Data.Mode != f.ModeFixture || len(ienv.Data.Items) != 1 {
		t.Fatalf("fixture catalog %+v", ienv.Data)
	}
	if ienv.Data.Items[0]["instrument_id"] != "DEMO:COMPANY" {
		t.Fatalf("want DEMO got %+v", ienv.Data.Items[0])
	}
}

func TestStageBIdentityAndRoutes(t *testing.T) {
	db := testdb.Start(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("Passw0rd!"), bcrypt.MinCost)
	if err := db.Create(&model.User{ID: 1001, Username: "invitee", PasswordHash: string(hash), Role: "user"}).Error; err != nil {
		t.Fatal(err)
	}
	svc := f.NewService(db, f.NewFakeResearchClient(), f.NewMemoryBudget(), f.NewFixtureConfig())
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	api.Register(engine, svc, f.NewModelProxy(db, f.NewMemoryBudget()))
	api.RegisterAdmin(engine, f.NewConfigService(db), svc)
	server := httptest.NewServer(engine)
	defer server.Close()

	login, err := http.Post(server.URL+"/api/finance/auth/login", "application/json", bytes.NewReader([]byte(`{"username":"invitee","password":"Passw0rd!"}`)))
	if err != nil {
		t.Fatal(err)
	}
	defer login.Body.Close()
	if login.StatusCode != 200 {
		t.Fatalf("login want 200 got %d", login.StatusCode)
	}
	var env struct {
		Data map[string]any `json:"data"`
	}
	_ = json.NewDecoder(login.Body).Decode(&env)
	token, _ := env.Data["token"].(string)
	if token == "" {
		t.Fatal("missing login token")
	}

	preq, _ := http.NewRequest("POST", server.URL+"/api/finance/claims/parse", bytes.NewReader([]byte(`{"text":"演示公司的收入增长能否支持未来一年股价上涨？这是一条超过二十个字的测试观点。"}`)))
	preq.Header.Set("Authorization", "Bearer "+token)
	preq.Header.Set("Content-Type", "application/json")
	pres, err := http.DefaultClient.Do(preq)
	if err != nil {
		t.Fatal(err)
	}
	pres.Body.Close()
	if pres.StatusCode != 200 {
		t.Fatalf("authed parse want 200 got %d", pres.StatusCode)
	}

	unauth, err := http.Post(server.URL+"/api/finance/claims/parse", "application/json", bytes.NewReader([]byte(`{"text":"xxxxxxxxxxxxxxxxxxxx"}`)))
	if err != nil {
		t.Fatal(err)
	}
	unauth.Body.Close()
	if unauth.StatusCode != 401 {
		t.Fatalf("public parse want 401 got %d", unauth.StatusCode)
	}

	areq, _ := http.NewRequest("GET", server.URL+"/api/finance/admin/model-configs", nil)
	areq.Header.Set("Authorization", "Bearer "+token)
	ares, err := http.DefaultClient.Do(areq)
	if err != nil {
		t.Fatal(err)
	}
	ares.Body.Close()
	if ares.StatusCode != 403 {
		t.Fatalf("user admin want 403 got %d", ares.StatusCode)
	}

	ireq, _ := http.NewRequest("GET", server.URL+"/internal/finance/data-source-profile", nil)
	ires, err := http.DefaultClient.Do(ireq)
	if err != nil {
		t.Fatal(err)
	}
	ires.Body.Close()
	if ires.StatusCode != 401 {
		t.Fatalf("internal no token want 401 got %d", ires.StatusCode)
	}

	ireq2, _ := http.NewRequest("GET", server.URL+"/internal/finance/data-source-profile", nil)
	ireq2.Header.Set("Authorization", "Bearer "+token)
	ires2, err := http.DefaultClient.Do(ireq2)
	if err != nil {
		t.Fatal(err)
	}
	ires2.Body.Close()
	if ires2.StatusCode != 401 {
		t.Fatalf("user token is not internal token, want 401 got %d", ires2.StatusCode)
	}

	ireq3, _ := http.NewRequest("GET", server.URL+"/internal/finance/data-source-profile", nil)
	ireq3.Header.Set("Authorization", "Bearer zhigu-internal-dev")
	ires3, err := http.DefaultClient.Do(ireq3)
	if err != nil {
		t.Fatal(err)
	}
	ires3.Body.Close()
	if ires3.StatusCode != 409 {
		t.Fatalf("internal missing contract want 409 got %d", ires3.StatusCode)
	}
}

func TestStageBGoSaaSIdentityAndRoutes(t *testing.T) {
	TestStageBIdentityAndRoutes(t)
}
