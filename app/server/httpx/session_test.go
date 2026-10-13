package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"zhigu/server/httpx"
	model "zhigu/server/model/finance"
	"zhigu/server/testdb"
)

func TestMain(m *testing.M) { code := m.Run(); testdb.StopIfStarted(); os.Exit(code) }

func TestDurableSessionCSRFExpiryAndRevocation(t *testing.T) {
	db := testdb.Start(t)
	user := model.User{Username: "owner", PasswordHash: "hash", Role: "admin"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	httpx.ConfigureAuth(httpx.AuthConfig{DB: db, Production: true, PublicOrigin: "https://zhigu.example"})
	t.Cleanup(func() { httpx.ConfigureAuth(httpx.AuthConfig{}) })
	token, err := httpx.CreateSession(db, user)
	if err != nil {
		t.Fatal(err)
	}
	var stored model.Session
	if err := db.First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.TokenDigest == token || len(stored.TokenDigest) != 64 || stored.JTI == "" {
		t.Fatalf("session stored raw secret: %+v", stored)
	}
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/csrf", httpx.AuthRequired(), httpx.SessionCSRF)
	engine.POST("/write", httpx.AuthRequired(), func(c *gin.Context) { c.Status(204) })
	engine.POST("/logout", httpx.AuthRequired(), httpx.Logout)
	request := func(method, path, csrf, origin, referer string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		r.AddCookie(&http.Cookie{Name: httpx.SessionCookieName, Value: token})
		r.Header.Set("X-CSRF-Token", csrf)
		r.Header.Set("Origin", origin)
		r.Header.Set("Referer", referer)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)
		return w
	}
	csrfResponse := request("GET", "/csrf", "", "", "")
	if csrfResponse.Code != 200 {
		t.Fatalf("csrf issuance: %d %s", csrfResponse.Code, csrfResponse.Body)
	}
	csrf := csrfResponse.Header().Get("X-CSRF-Token")
	if csrf == "" {
		t.Fatal("missing csrf token")
	}
	if err := db.First(&stored, "jti = ?", stored.JTI).Error; err != nil {
		t.Fatal(err)
	}
	if stored.CSRFDigest == nil || *stored.CSRFDigest == csrf || len(*stored.CSRFDigest) != 64 {
		t.Fatal("CSRF not stored as digest")
	}
	for _, tc := range []struct {
		csrf, origin, referer string
		want                  int
	}{
		{"", "https://zhigu.example", "", 403}, {csrf, "https://evil.example", "", 403},
		{csrf, "", "", 403}, {csrf, "https://zhigu.example/", "", 403},
		{csrf, "https://zhigu.example", "", 204}, {csrf, "", "https://zhigu.example/page", 204},
		{csrf, "https://evil.example", "https://zhigu.example/page", 403},
	} {
		if w := request("POST", "/write", tc.csrf, tc.origin, tc.referer); w.Code != tc.want {
			t.Fatalf("csrf/origin %+v: %d %s", tc, w.Code, w.Body)
		}
	}
	rotated := request("GET", "/csrf", "", "", "").Header().Get("X-CSRF-Token")
	if rotated == csrf || rotated == "" {
		t.Fatal("csrf was not rotated")
	}
	if w := request("POST", "/write", csrf, "https://zhigu.example", ""); w.Code != 403 {
		t.Fatal("old csrf accepted after rotation")
	}
	csrf = rotated
	if w := request("POST", "/logout", csrf, "https://zhigu.example", ""); w.Code != 200 {
		t.Fatalf("logout: %d %s", w.Code, w.Body)
	}
	if w := request("GET", "/csrf", "", "", ""); w.Code != 401 {
		t.Fatalf("revoked accepted: %d", w.Code)
	}
	token, err = httpx.CreateSession(db, user)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.Session{}).Where("user_id = ?", user.ID).Updates(map[string]any{"created_at": time.Now().Add(-time.Hour), "expires_at": time.Now().Add(-time.Second)}).Error; err != nil {
		t.Fatal(err)
	}
	if w := request("GET", "/csrf", "", "", ""); w.Code != 401 {
		t.Fatalf("expired accepted: %d", w.Code)
	}
}

func TestProductionRejectsBearerAndDevAccepts(t *testing.T) {
	token, err := httpx.SignToken(42, "owner", "admin")
	if err != nil {
		t.Fatal(err)
	}
	for _, production := range []bool{false, true} {
		httpx.ConfigureAuth(httpx.AuthConfig{Production: production})
		engine := gin.New()
		engine.GET("/private", httpx.AuthRequired(), func(c *gin.Context) { c.Status(204) })
		for _, header := range []string{"Authorization", "x-token"} {
			r := httptest.NewRequest("GET", "/private", nil)
			if header == "Authorization" {
				r.Header.Set(header, "Bearer "+token)
			} else {
				r.Header.Set(header, token)
			}
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, r)
			want := 204
			if production {
				want = 401
			}
			if w.Code != want {
				t.Fatalf("production=%v header=%s: %d", production, header, w.Code)
			}
		}
	}
	httpx.ConfigureAuth(httpx.AuthConfig{})
}

func TestPasswordPolicyAndLegacyUpgrade(t *testing.T) {
	hash, err := httpx.HashPassword("valid-password-more-than-16")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatal(hash)
	}
	if ok, rehash := httpx.VerifyPassword(hash, "valid-password-more-than-16"); !ok || rehash {
		t.Fatalf("argon verify: %v %v", ok, rehash)
	}
	if ok, _ := httpx.VerifyPassword(hash, "wrong"); ok {
		t.Fatal("wrong password accepted")
	}
	if ok, _ := httpx.VerifyPassword(strings.Replace(hash, "p=1$", "p=1,trailing=1$", 1), "valid-password-more-than-16"); ok {
		t.Fatal("malformed Argon2 parameters accepted")
	}
	for _, cost := range []int{10, 12} {
		legacy, _ := bcrypt.GenerateFromPassword([]byte("valid-password-more-than-16"), cost)
		if ok, rehash := httpx.VerifyPassword(string(legacy), "valid-password-more-than-16"); !ok || !rehash {
			t.Fatalf("bcrypt cost %d: %v %v", cost, ok, rehash)
		}
		if ok, _ := httpx.VerifyPassword(string(legacy), strings.Repeat("x", 73)); ok {
			t.Fatal("bcrypt overlong accepted")
		}
	}
	for _, password := range []string{"short", strings.Repeat("x", 1025)} {
		if _, err := httpx.HashPassword(password); err == nil {
			t.Fatal("invalid new password accepted")
		}
	}
	for _, hash := range []string{"$argon2id$v=19$m=4294967295,t=2,p=1$a$b", "$argon2id$v=19$m=19456,t=2,p=0$a$b", "malformed"} {
		if ok, _ := httpx.VerifyPassword(hash, "password"); ok {
			t.Fatal("malformed hash accepted")
		}
	}
}

func TestLoginLimiterNormalizesAndBounds(t *testing.T) {
	limiter := httpx.NewLoginLimiter()
	now := time.Now()
	for i := 0; i < 5; i++ {
		if delay := limiter.Allow("192.0.2.1", " Owner ", now); delay != 0 {
			t.Fatalf("early block: %v", delay)
		}
	}
	if delay := limiter.Allow("192.0.2.1", "owner", now); delay <= 0 {
		t.Fatal("normalized account bypass")
	}
	if delay := limiter.Allow("192.0.2.2", "owner", now); delay != 0 {
		t.Fatal("IP dimensions merged")
	}
	if delay := limiter.Allow("192.0.2.1", "another", now); delay != 0 {
		t.Fatal("account dimensions merged")
	}
	if delay := limiter.Allow("192.0.2.1", "owner", now.Add(3*time.Second)); delay != 0 {
		t.Fatalf("retry rejected after backoff elapsed: %v", delay)
	}
	if delay := limiter.Allow("192.0.2.1", "owner", now.Add(3*time.Second)); delay <= 2*time.Second {
		t.Fatalf("backoff did not increase: %v", delay)
	}
	if delay := limiter.Allow("192.0.2.1", "owner", now.Add(20*time.Minute)); delay != 0 {
		t.Fatal("window did not reset")
	}
}

func TestSessionSurvivesProcessConfigResetButNotIdleDisableOrPasswordChange(t *testing.T) {
	db := testdb.Start(t)
	user := model.User{Username: "owner", PasswordHash: "hash", Role: "admin"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	config := httpx.AuthConfig{DB: db, Production: true, PublicOrigin: "https://zhigu.example"}
	httpx.ConfigureAuth(config)
	t.Cleanup(func() { httpx.ConfigureAuth(httpx.AuthConfig{}) })
	engine := gin.New()
	engine.GET("/private", httpx.AuthRequired(), func(c *gin.Context) { c.Status(204) })
	request := func(token string) int {
		r := httptest.NewRequest("GET", "/private", nil)
		r.AddCookie(&http.Cookie{Name: httpx.SessionCookieName, Value: token})
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)
		return w.Code
	}
	for _, mutation := range []string{"restart", "idle", "disable", "password", "password-v2"} {
		token, err := httpx.CreateSession(db, user)
		if err != nil {
			t.Fatal(err)
		}
		if mutation == "restart" {
			httpx.ConfigureAuth(httpx.AuthConfig{})
			httpx.ConfigureAuth(config)
			if code := request(token); code != 204 {
				t.Fatalf("durability: %d", code)
			}
			continue
		}
		switch mutation {
		case "idle":
			db.Model(&model.Session{}).Where("user_id = ?", user.ID).Update("last_seen_at", time.Now().Add(-31*time.Minute))
		case "disable":
			db.Model(&model.User{}).Where("id = ?", user.ID).Update("disabled_at", time.Now())
		case "password":
			db.Model(&model.User{}).Where("id = ?", user.ID).Update("password_hash", "changed")
		case "password-v2":
			db.Model(&model.User{}).Where("id = ?", user.ID).Update("password_hash_v2", "changed-v2")
		}
		if code := request(token); code != 401 {
			t.Fatalf("%s session accepted: %d", mutation, code)
		}
		db.Model(&model.User{}).Where("id = ?", user.ID).Updates(map[string]any{"disabled_at": nil, "password_hash": "hash", "password_hash_v2": nil})
		if mutation == "disable" && request(token) != 401 {
			t.Fatal("re-enabling owner revived previously disabled session")
		}
	}
}

func BenchmarkArgon2idPasswordVerification(b *testing.B) {
	hash, err := httpx.HashPassword("benchmark-owner-password-2026")
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if ok, _ := httpx.VerifyPassword(hash, "benchmark-owner-password-2026"); !ok {
			b.Fatal("verify failed")
		}
	}
}

func TestSessionCreationRejectsCredentialsChangedAfterVerification(t *testing.T) {
	db := testdb.Start(t)
	user := model.User{Username: "owner", PasswordHash: "verified-hash", Role: "admin"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.User{}).Where("id = ?", user.ID).Update("password_hash", "new-password-hash").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := httpx.CreateSession(db, user); err == nil {
		t.Fatal("session issued using stale verified credentials")
	}
	if err := db.Model(&model.User{}).Where("id = ?", user.ID).Updates(map[string]any{"password_hash": user.PasswordHash, "disabled_at": time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := httpx.CreateSession(db, user); err == nil {
		t.Fatal("session issued for owner disabled after verification")
	}
}
