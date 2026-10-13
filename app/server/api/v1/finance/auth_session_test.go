package finance

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"zhigu/server/httpx"
	model "zhigu/server/model/finance"
	service "zhigu/server/service/finance"
	"zhigu/server/testdb"
)

func TestProductionLoginUsesCookieAndRehashesLegacyPassword(t *testing.T) {
	t.Setenv("ZHIGU_ENV", "production")
	t.Setenv("ZHIGU_PUBLIC_ORIGIN", "https://zhigu.example")
	db := testdb.Start(t)
	httpx.ConfigureAuth(httpx.AuthConfig{DB: db, Production: true, PublicOrigin: "https://zhigu.example"})
	t.Cleanup(func() { httpx.ConfigureAuth(httpx.AuthConfig{}) })
	hash, err := bcrypt.GenerateFromPassword([]byte("a-valid-long-password"), 12)
	if err != nil {
		t.Fatal(err)
	}
	user := model.User{Username: "owner", PasswordHash: string(hash), Role: "admin"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	Register(engine, service.NewService(db, service.NewFakeResearchClient(), service.NewMemoryBudget(), service.NewFixtureConfig()), nil)
	req := httptest.NewRequest(http.MethodPost, "/api/finance/auth/login", bytes.NewBufferString(`{"username":"owner","password":"a-valid-long-password"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://zhigu.example")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &envelope)
	if _, exists := envelope.Data["token"]; exists {
		t.Fatal("production login exposes token in JSON")
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("want session cookie, got %v", cookies)
	}
	cookie := cookies[0]
	if cookie.Name != "__Host-zhigu_session" || !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || cookie.Domain != "" {
		t.Fatalf("unsafe cookie: %+v", cookie)
	}
	if err := db.First(&user, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(user.PasswordHash, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("legacy password not rehashed: %s", user.PasswordHash)
	}
}

func TestWeakLegacyBcryptRejectedOnlyInProduction(t *testing.T) {
	for _, production := range []bool{true, false} {
		t.Run(fmt.Sprintf("production=%v", production), func(t *testing.T) {
			db := testdb.Start(t)
			hash, err := bcrypt.GenerateFromPassword([]byte("legacy-valid-owner-password"), 10)
			if err != nil {
				t.Fatal(err)
			}
			user := model.User{Username: "owner", PasswordHash: string(hash), Role: "admin"}
			if err := db.Create(&user).Error; err != nil {
				t.Fatal(err)
			}
			httpx.ConfigureAuth(httpx.AuthConfig{DB: db, Production: production, PublicOrigin: "https://zhigu.example"})
			t.Cleanup(func() { httpx.ConfigureAuth(httpx.AuthConfig{}) })
			engine := gin.New()
			a := &API{Svc: service.NewService(db, service.NewFakeResearchClient(), service.NewMemoryBudget(), service.NewFixtureConfig()), LoginLimiter: httpx.NewLoginLimiter()}
			engine.POST("/login", a.Login)
			r := httptest.NewRequest("POST", "/login", bytes.NewBufferString(`{"username":"owner","password":"legacy-valid-owner-password"}`))
			r.Header.Set("Origin", "https://zhigu.example")
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, r)
			if err := db.First(&user, user.ID).Error; err != nil {
				t.Fatal(err)
			}
			if production {
				var sessions int64
				if err := db.Model(&model.Session{}).Count(&sessions).Error; err != nil {
					t.Fatal(err)
				}
				if w.Code != 401 || sessions != 0 || user.PasswordHash != string(hash) || len(w.Result().Cookies()) != 0 {
					t.Fatalf("weak production credential accepted: status=%d sessions=%d", w.Code, sessions)
				}
			} else if w.Code != 200 || !strings.HasPrefix(user.PasswordHash, "$argon2id$") {
				t.Fatalf("development legacy upgrade rejected: %d %s", w.Code, w.Body)
			}
		})
	}
}

func TestProductionLoginRejectsForeignOrAbsentOrigin(t *testing.T) {
	httpx.ConfigureAuth(httpx.AuthConfig{Production: true, PublicOrigin: "https://zhigu.example"})
	t.Cleanup(func() { httpx.ConfigureAuth(httpx.AuthConfig{}) })
	engine := gin.New()
	a := &API{}
	engine.POST("/login", a.Login)
	for _, origin := range []string{"", "https://evil.example", "https://zhigu.example/"} {
		r := httptest.NewRequest("POST", "/login", bytes.NewBufferString(`{"username":"owner","password":"valid-password"}`))
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("origin %q accepted: %d %s", origin, w.Code, w.Body)
		}
	}
}

func TestLoginUnknownWrongDisabledAndLimits(t *testing.T) {
	db := testdb.Start(t)
	hash, err := httpx.HashPassword("actual-valid-owner-password")
	if err != nil {
		t.Fatal(err)
	}
	user := model.User{Username: "owner", PasswordHash: hash, Role: "admin"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	a := &API{Svc: service.NewService(db, service.NewFakeResearchClient(), service.NewMemoryBudget(), service.NewFixtureConfig()), LoginLimiter: httpx.NewLoginLimiter()}
	engine := gin.New()
	engine.POST("/login", a.Login)
	request := func(account, password, ip string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"username": account, "password": password})
		r := httptest.NewRequest("POST", "/login", bytes.NewReader(body))
		r.RemoteAddr = ip + ":1234"
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)
		return w
	}
	var message string
	for _, account := range []string{"owner", "missing-owner"} {
		w := request(account, "wrong", "192.0.2.10")
		if w.Code != 401 {
			t.Fatalf("wrong credential: %d", w.Code)
		}
		var envelope struct {
			Error struct{ Code, Message string }
		}
		_ = json.Unmarshal(w.Body.Bytes(), &envelope)
		if message == "" {
			message = envelope.Error.Message
		} else if message != envelope.Error.Message {
			t.Fatal("account existence leaked")
		}
	}
	if err := db.Model(&model.User{}).Where("id = ?", user.ID).Update("disabled_at", time.Now()).Error; err != nil {
		t.Fatal(err)
	}
	if w := request("owner", "actual-valid-owner-password", "192.0.2.9"); w.Code != 401 || !strings.Contains(w.Body.String(), message) {
		t.Fatalf("disabled account exposed: %d %s", w.Code, w.Body)
	}
	if err := db.Model(&model.User{}).Where("id = ?", user.ID).Update("disabled_at", nil).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		w := request(" Another ", "wrong", "192.0.2.2")
		if w.Code != 401 {
			t.Fatalf("early limit %d: %d", i, w.Code)
		}
	}
	w := request("another", "wrong", "192.0.2.2")
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatalf("normalized limit: %d %s", w.Code, w.Body)
	}
	if w := request("another", "wrong", "192.0.2.3"); w.Code != 401 {
		t.Fatalf("different IP rejected: %d", w.Code)
	}
	if w := request(strings.Repeat("x", 129), "wrong", "192.0.2.4"); w.Code != 401 {
		t.Fatalf("oversized account: %d", w.Code)
	}
	if w := request("owner", strings.Repeat("x", 1025), "192.0.2.4"); w.Code != 401 {
		t.Fatalf("oversized password: %d", w.Code)
	}
}

func TestProductionLoginDatabaseFailureIsRedacted(t *testing.T) {
	db := testdb.Start(t)
	hash, err := httpx.HashPassword("actual-valid-owner-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.User{Username: "owner", PasswordHash: hash, Role: "admin"}).Error; err != nil {
		t.Fatal(err)
	}
	httpx.ConfigureAuth(httpx.AuthConfig{DB: db, Production: true, PublicOrigin: "https://zhigu.example"})
	t.Cleanup(func() { httpx.ConfigureAuth(httpx.AuthConfig{}) })
	rollback := errors.New("rollback test outage")
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("ALTER TABLE finance_browser_sessions RENAME TO session_outage").Error; err != nil {
			t.Fatal(err)
		}
		engine := gin.New()
		a := &API{Svc: service.NewService(tx, service.NewFakeResearchClient(), service.NewMemoryBudget(), service.NewFixtureConfig()), LoginLimiter: httpx.NewLoginLimiter()}
		engine.POST("/login", a.Login)
		r := httptest.NewRequest("POST", "/login", bytes.NewBufferString(`{"username":"owner","password":"actual-valid-owner-password"}`))
		r.Header.Set("Origin", "https://zhigu.example")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)
		if w.Code != 500 || strings.Contains(w.Body.String(), "finance_browser_sessions") || strings.Contains(w.Body.String(), "SQLSTATE") || strings.Contains(w.Body.String(), "actual-valid-owner-password") {
			t.Fatalf("unsafe DB error: %d %s", w.Code, w.Body)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
}
