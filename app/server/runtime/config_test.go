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
	var declared []string
	_ = json.Unmarshal([]byte(env["ZHIGU_PERSISTENT_MOUNTS"]), &declared)
	mountinfo := "1 0 0:1 / / rw - overlay overlay rw\n"
	for _, dir := range declared {
		mountinfo += mountLine(dir, "ext4", "rw")
	}
	return load(func(k string) string { return env[k] }, func() ([]byte, error) { return []byte(mountinfo), nil })
}

func mountLine(path, filesystem, options string) string {
	escaped := strings.NewReplacer("\\", "\\134", " ", "\\040", "\t", "\\011", "\n", "\\012").Replace(path)
	return "2 1 8:1 / " + escaped + " " + options + " - " + filesystem + " /dev/sda1 " + options + "\n"
}

func declareStorage(env map[string]string, key, dir string) {
	env[key] = dir
	env["ZHIGU_PERSISTENT_MOUNTS"] = stringMustJSON([]string{dir})
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

func TestProductionValidatesEffectiveDSNPassword(t *testing.T) {
	for _, dsn := range []string{"postgres://owner:zhigu@postgres/research", "host=postgres user=owner password=zhigu dbname=research"} {
		env := productionEnv()
		env["ZHIGU_POSTGRES_DSN"] = dsn
		if _, err := loadMap(env); err == nil {
			t.Fatal("weak DSN masked by strong independent password")
		}
	}
	env := productionEnv()
	env["ZHIGU_POSTGRES_DSN"] = "postgres://owner:secure-dsn-password-2026-unique@postgres/research"
	if _, err := loadMap(env); err == nil {
		t.Fatal("ambiguous DSN and independent PostgreSQL configuration accepted")
	}
	for _, name := range []string{"POSTGRES_USER", "POSTGRES_DB", "POSTGRES_PASSWORD"} {
		delete(env, name)
	}
	if _, err := loadMap(env); err != nil {
		t.Fatalf("unambiguous strong DSN rejected: %v", err)
	}
	env["ZHIGU_POSTGRES_DSN"] = "postgres://owner:secure-dsn-password-2026-unique@postgres/research?password=zhigu"
	if _, err := loadMap(env); err == nil {
		t.Fatal("effective query password ignored")
	}
}

func intelLiveEnv() map[string]string {
	return map[string]string{
		"ZHIGU_INTEL_MODEL_BASE_URL": "https://model.example.org/v1", "ZHIGU_INTEL_MODEL": "research-model",
		"ZHIGU_INTEL_MODEL_CONFIG_ID": "model-config-v1", "ZHIGU_INTEL_MODEL_CONFIG_DIGEST": "model-config-digest-v1",
		"ZHIGU_INTEL_MODEL_VERSION": "v1", "ZHIGU_INTEL_MODEL_PRICE_CNY_PER_1K": "0.1", "ZHIGU_INTEL_MODEL_API_KEY": "unique-intel-model-key-long-enough-2026",
		"ZHIGU_INTEL_PROVIDERS": "ifind", "IFIND_MCP_URL": "https://ifind.example.org/mcp", "IFIND_AUTHORIZATION": "Bearer unique-ifind-authorization-2026",
	}
}

func TestIntelLiveUsesConsumedConfiguration(t *testing.T) {
	t.Run("consumed configuration", func(t *testing.T) {
		env := intelLiveEnv()
		if err := validateLive(func(k string) string { return env[k] }, "INTEL"); err != nil {
			t.Fatalf("actual Intel configuration rejected: %v", err)
		}
	})
	for _, missing := range []string{"ZHIGU_INTEL_MODEL_BASE_URL", "ZHIGU_INTEL_MODEL", "ZHIGU_INTEL_MODEL_CONFIG_ID", "ZHIGU_INTEL_MODEL_CONFIG_DIGEST", "ZHIGU_INTEL_PROVIDERS", "IFIND_MCP_URL", "IFIND_AUTHORIZATION"} {
		t.Run(missing, func(t *testing.T) {
			env := intelLiveEnv()
			delete(env, missing)
			env["ZHIGU_INTEL_MODEL_URL"] = "https://unused.example.org"
			env["ZHIGU_INTEL_MODEL_NAME"] = "unused-model"
			env["ZHIGU_INTEL_PROVIDER_URL"] = "https://unused-provider.example.org"
			if err := validateLive(func(k string) string { return env[k] }, "INTEL"); err == nil {
				t.Fatalf("unused variables masked missing %s", missing)
			}
		})
	}
}

func TestProductionRequiresDeclaredPersistentMount(t *testing.T) {
	dir := persistentTestDir(t)
	for _, declarations := range []string{"", `["/"]`, stringMustJSON([]string{dir})} {
		env := productionEnv()
		env["ZHIGU_FUTURES_ENABLED"] = "true"
		env["ZHIGU_FUTURES_STORAGE_DIR"] = dir
		env["ZHIGU_PERSISTENT_MOUNTS"] = declarations
		if _, err := Load(func(k string) string { return env[k] }); err == nil {
			t.Fatal("writable container directory accepted without a proven persistent mount")
		}
	}
}

func TestPersistentMountEvidence(t *testing.T) {
	dir := persistentTestDir(t)
	root := "1 0 0:1 / / rw - overlay overlay rw\n"
	for _, tc := range []struct {
		name, declared, mountinfo string
		allowed                   bool
	}{
		{"declared disk mount", stringMustJSON([]string{dir}), root + mountLine(dir, "ext4", "rw"), true},
		{"undeclared disk mount", "", root + mountLine(dir, "ext4", "rw"), false},
		{"declaration without mount", stringMustJSON([]string{dir}), root, false},
		{"ephemeral overlay", stringMustJSON([]string{dir}), root + mountLine(dir, "overlay", "rw"), false},
		{"ephemeral tmpfs", stringMustJSON([]string{dir}), root + mountLine(dir, "tmpfs", "rw"), false},
		{"ambiguous overmount", stringMustJSON([]string{dir}), root + mountLine(dir, "ext4", "rw") + mountLine(dir, "tmpfs", "rw"), false},
		{"read-only disk mount", stringMustJSON([]string{dir}), root + mountLine(dir, "ext4", "ro"), false},
		{"malformed declaration", dir, root + mountLine(dir, "ext4", "rw"), false},
		{"malformed mountinfo", stringMustJSON([]string{dir}), "not mount information", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := productionEnv()
			env["ZHIGU_FUTURES_ENABLED"] = "true"
			env["ZHIGU_FUTURES_STORAGE_DIR"] = dir
			env["ZHIGU_PERSISTENT_MOUNTS"] = tc.declared
			_, err := load(func(k string) string { return env[k] }, func() ([]byte, error) { return []byte(tc.mountinfo), nil })
			if (err == nil) != tc.allowed {
				t.Fatalf("mount evidence validation allowed=%v: %v", tc.allowed, err)
			}
		})
	}
}

func TestPersistentMountCanonicalPathsAndChildVolumes(t *testing.T) {
	root := persistentTestDir(t)
	child := filepath.Join(root, "nested storage")
	alias := filepath.Join(root, "declared-link")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(child) })
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(alias) })
	env := productionEnv()
	env["ZHIGU_FUTURES_ENABLED"] = "true"
	env["ZHIGU_FUTURES_STORAGE_DIR"] = child
	env["ZHIGU_PERSISTENT_MOUNTS"] = stringMustJSON([]string{alias})
	loadEvidence := func(body string) (Config, error) {
		return load(func(k string) string { return env[k] }, func() ([]byte, error) { return []byte(body), nil })
	}
	cfg, err := loadEvidence(mountLine(root, "ext4", "rw"))
	if err != nil || len(cfg.PersistentMounts) != 1 || cfg.PersistentMounts[0] != root {
		t.Fatalf("canonical declared parent volume rejected: %v", err)
	}
	if _, err := loadEvidence(mountLine(root, "ext4", "rw") + mountLine(child, "tmpfs", "rw")); err == nil {
		t.Fatal("nested tmpfs bypassed persistent parent check")
	}
	env["ZHIGU_PERSISTENT_MOUNTS"] = stringMustJSON([]string{child})
	if _, err := loadEvidence(mountLine(child, "ext4", "rw")); err != nil {
		t.Fatalf("escaped mount path rejected: %v", err)
	}
	_, err = load(func(k string) string { return env[k] }, func() ([]byte, error) { return nil, os.ErrPermission })
	if err == nil || strings.Contains(err.Error(), child) {
		t.Fatalf("missing mount evidence did not fail safely: %v", err)
	}
}

func TestIntelLiveAcceptsConfiguredRealProviders(t *testing.T) {
	for _, providers := range []string{"ifind", "fuyao", "cninfo", "ifind,fuyao"} {
		t.Run(providers, func(t *testing.T) {
			env := productionEnv()
			for k, v := range intelLiveEnv() {
				env[k] = v
			}
			env["ZHIGU_INTEL_ENABLED"] = "true"
			env["ZHIGU_INTEL_MODE"] = "live"
			env["ZHIGU_INTEL_COOKIE_SECRET"] = "unique-intel-cookie-key-2026-long-enough"
			env["ZHIGU_INTEL_PROVIDERS"] = providers
			env["FUYAO_API_KEY"] = "unique-fuyao-api-key-long-enough-2026"
			env["ZHIGU_CNINFO_LIVE_VERIFIED"] = "1"
			cfg, err := loadMap(env)
			if err != nil {
				t.Fatalf("actual production provider config rejected: %v", err)
			}
			if cfg.Intel.OperationMode != Live {
				t.Fatal("live configuration not retained")
			}
		})
	}
	for _, providers := range []string{"fixture", "unknown", "ifind,fixture"} {
		env := intelLiveEnv()
		env["ZHIGU_INTEL_PROVIDERS"] = providers
		if err := validateLive(func(k string) string { return env[k] }, "INTEL"); err == nil {
			t.Fatalf("non-real provider accepted: %s", providers)
		}
	}
}

func stringMustJSON(value any) string { body, _ := json.Marshal(value); return string(body) }

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
	declareStorage(env, "ZHIGU_RESEARCH_DOCUMENT_DIR", persistentTestDir(t))
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
	declareStorage(env, "ZHIGU_FUTURES_STORAGE_DIR", persistentTestDir(t))
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
