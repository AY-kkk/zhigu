package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func productionEnv() map[string]string {
	return map[string]string{
		"ZHIGU_ENV": "production", "ZHIGU_PUBLIC_ORIGIN": "https://research.example.org",
		"POSTGRES_USER": "owner", "POSTGRES_DB": "research", "POSTGRES_PASSWORD": "secure-database-password-2026-unique",
		"ZHIGU_SESSION_KEYS":    `["current-session-key-2026-random-value-unique","previous-session-key-2025-random-value-unique"]`,
		"ZHIGU_INTERNAL_TOKEN":  "research-service-token-2026-random-unique",
		"FUTURES_SERVICE_TOKEN": "futures-service-token-2026-random-unique",
		"ZHIGU_MASTER_KEY":      "encryption-master-key-2026-random-unique",
	}
}

func loadMap(env map[string]string) (Config, error) {
	return Load(func(k string) string { return env[k] })
}

func persistentTestDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(".", ".runtime-storage-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Remove(dir); err != nil {
			t.Error(err)
		}
	})
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func TestSecretFilePrecedesEnvironment(t *testing.T) {
	env := productionEnv()
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte("file-research-token-2026-random-unique\n"), 0600); err != nil {
		t.Fatal(err)
	}
	env["ZHIGU_INTERNAL_TOKEN_FILE"] = path
	env["ZHIGU_INTERNAL_TOKEN"] = "unsafe"
	cfg, err := loadMap(env)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.InternalToken != "file-research-token-2026-random-unique" {
		t.Fatalf("file secret not used")
	}
	env["ZHIGU_INTERNAL_TOKEN_FILE"] = path + ".missing"
	if _, err := loadMap(env); err == nil || strings.Contains(err.Error(), path) {
		t.Fatalf("missing secret file must fail safely: %v", err)
	}
}

func TestProductionNegativeStartup(t *testing.T) {
	cases := []struct{ key, value string }{
		{"ZHIGU_ENV", "staging"}, {"ZHIGU_DEPLOYMENT_ENVIRONMENT", "test"},
		{"ZHIGU_OPERATION_MODE", "fixture"}, {"ZHIGU_OPERATION_MODE", "demo"}, {"ZHIGU_OPERATION_MODE", "unavailable"},
		{"ZHIGU_DATA_PROVENANCE", "fixture"}, {"ZHIGU_HEALTH", "live"},
		{"ZHIGU_INTERNAL_TOKEN", "zhigu-internal-dev"}, {"ZHIGU_INTERNAL_TOKEN", "change-me-to-a-long-secret-value-12345"},
		{"ZHIGU_INTERNAL_TOKEN", ""}, {"POSTGRES_PASSWORD", "zhigu"},
		{"ZHIGU_MASTER_KEY", ""}, {"ZHIGU_MASTER_KEY", "zhigu-dev-only-master-key"},
		{"ZHIGU_SESSION_KEYS", `["short","short"]`}, {"ZHIGU_SESSION_KEYS", `["current-session-key-2026-random-value-unique"]`},
		{"FUTURES_SERVICE_TOKEN", "research-service-token-2026-random-unique"},
		{"ZHIGU_INTERNAL_TOKEN", "current-session-key-2026-random-value-unique"},
		{"ZHIGU_ALLOW_INSECURE_DEV", "1"}, {"ZHIGU_RESEARCH_EXECUTOR", "fixture"}, {"FUTURES_EXECUTOR", "fixture"},
		{"ZHIGU_RESEARCH_MODE", "unit-test"}, {"ZHIGU_DATA_MODE", "fixture"}, {"ZHIGU_INTEL_MODE", "demo"},
		{"ZHIGU_PUBLIC_ORIGIN", "http://research.example.org"}, {"ZHIGU_PUBLIC_ORIGIN", "https://localhost"},
		{"ZHIGU_PUBLIC_ORIGIN", "https://127.0.0.1"}, {"ZHIGU_PUBLIC_ORIGIN", "https://10.0.0.2"},
		{"ZHIGU_PUBLIC_ORIGIN", "https://research.example.org/path"},
		{"ZHIGU_INTERNAL_TOKEN", "your-long-internal-token-placeholder-123456"},
		{"ZHIGU_INTEL_FIXTURE_DIR", "/some/fixtures"},
	}
	for _, tc := range cases {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			env := productionEnv()
			env[tc.key] = tc.value
			if _, err := loadMap(env); err == nil {
				t.Fatal("unsafe production configuration accepted")
			}
		})
	}
	for _, module := range []string{"FUTURES", "INTEL", "RESEARCH"} {
		env := productionEnv()
		env["ZHIGU_"+module+"_MODE"] = "live"
		env["ZHIGU_"+module+"_ENABLED"] = "true"
		if _, err := loadMap(env); err == nil {
			t.Fatalf("%s live without provider/model accepted", module)
		}
	}
}

func TestProductionRequestBoundary(t *testing.T) {
	cfg, err := loadMap(productionEnv())
	if err != nil {
		t.Fatal(err)
	}
	for _, req := range []struct{ method, path, mode string }{
		{"POST", "/api/finance/claims/parse", ""}, {"POST", "/internal/llm/v1/responses", ""},
		{"POST", "/api/finance/intel/v1/demo-sessions", "demo"}, {"GET", "/api/finance/intel/v1/events", "demo"},
	} {
		if cfg.RequestAllowed(req.method, req.path, req.mode) {
			t.Fatalf("unsafe request allowed: %s %s", req.method, req.path)
		}
	}
	if !cfg.RequestAllowed("POST", "/api/finance/auth/login", "") || !cfg.RequestAllowed("GET", "/healthz", "") {
		t.Fatal("startup/login blocked")
	}
	env := productionEnv()
	env["ZHIGU_RESEARCH_ENABLED"] = "true"
	env["ZHIGU_RESEARCH_MODE"] = "read_only"
	env["ZHIGU_RESEARCH_DOCUMENT_DIR"] = persistentTestDir(t)
	cfg, err = loadMap(env)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.RequestAllowed("GET", "/api/finance/research", "") || cfg.RequestAllowed("POST", "/api/finance/research", "") {
		t.Fatal("read_only boundary incorrect")
	}
}

func TestIndependentStateAndRedactedSummary(t *testing.T) {
	env := productionEnv()
	env["ZHIGU_FUTURES_ENABLED"] = "true"
	env["ZHIGU_FUTURES_MODE"] = "read_only"
	env["ZHIGU_FUTURES_STORAGE_DIR"] = persistentTestDir(t)
	cfg, err := loadMap(env)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DeploymentEnvironment != Production || cfg.OperationMode != ReadOnly || cfg.DataProvenance != Real || cfg.Health != Degraded {
		t.Fatalf("wrong runtime state: %s", cfg.SummaryJSON())
	}
	if cfg.Futures.OperationMode != ReadOnly || cfg.Futures.DataProvenance != Real {
		t.Fatal("module state conflated")
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{cfg.InternalToken, cfg.FuturesToken, cfg.SessionKeys[0], cfg.DatabaseDSN} {
		if strings.Contains(cfg.SummaryJSON(), secret) || strings.Contains(string(raw), secret) {
			t.Fatal("configuration leaks secret")
		}
	}
	for _, key := range []string{"deployment_environment", "operation_mode", "data_provenance", "health", "revision", "build_time", "migration_version", "persistent_directories", "providers", "model_versions"} {
		if !strings.Contains(cfg.SummaryJSON(), `"`+key+`"`) {
			t.Fatalf("summary missing %s", key)
		}
	}
}

func TestProductionStorageFailsClosed(t *testing.T) {
	for _, value := range []string{"", t.TempDir(), "/missing/production-volume"} {
		env := productionEnv()
		env["ZHIGU_FUTURES_ENABLED"] = "true"
		env["ZHIGU_FUTURES_STORAGE_DIR"] = value
		if _, err := loadMap(env); err == nil {
			t.Fatalf("non-persistent storage accepted: %s", value)
		}
	}
}

func TestLivePriceMustBeFinite(t *testing.T) {
	for _, price := range []string{"NaN", "+Inf", "-Inf", "0", "-1"} {
		env := map[string]string{"FUTURES_MODEL_URL": "https://model.example.org", "FUTURES_MODEL_NAME": "research", "FUTURES_MODEL_VERSION": "v1", "FUTURES_MODEL_PRICE_CNY_PER_1K": price, "FUTURES_PROVIDER_URL": "https://provider.example.org", "FUTURES_MODEL_API_KEY": "unique-provider-key-long-enough-2026"}
		if err := validateLive(func(k string) string { return env[k] }, "FUTURES"); err == nil {
			t.Fatalf("unsafe model price accepted: %s", price)
		}
	}
}

func TestNonProductionCompatibilityAndInsecureLoopback(t *testing.T) {
	cfg, err := loadMap(map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DeploymentEnvironment != Test {
		t.Fatal("default development environment changed")
	}
	for _, env := range []map[string]string{{"ZHIGU_ENV": "demo", "ZHIGU_ALLOW_INSECURE_DEV": "1"}, {"ZHIGU_ENV": "test", "ZHIGU_ALLOW_INSECURE_DEV": "1", "ZHIGU_HTTP_ADDR": ":8080"}} {
		if _, err := loadMap(env); err == nil {
			t.Fatal("insecure non-loopback accepted")
		}
	}
	cfg, err = loadMap(map[string]string{"ZHIGU_ENV": "test", "ZHIGU_ALLOW_INSECURE_DEV": "1", "ZHIGU_HTTP_ADDR": "127.0.0.1:8080"})
	if err != nil || !cfg.SeedDemoUsers {
		t.Fatalf("explicit local test rejected: %v", err)
	}
}
