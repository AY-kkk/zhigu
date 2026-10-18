// Package runtime owns startup configuration. Secret values never form part of
// its JSON representation or the public startup summary.
package runtime

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"golang.org/x/sys/unix"
)

type DeploymentEnvironment string
type OperationMode string
type DataProvenance string
type Health string

const (
	Test        DeploymentEnvironment = "test"
	Demo        DeploymentEnvironment = "demo"
	Production  DeploymentEnvironment = "production"
	Off         OperationMode         = "off"
	ReadOnly    OperationMode         = "read_only"
	Live        OperationMode         = "live"
	Fixture     DataProvenance        = "fixture"
	Real        DataProvenance        = "real"
	Ready       Health                = "ready"
	Degraded    Health                = "degraded"
	Unavailable Health                = "unavailable"
)

type State struct {
	DeploymentEnvironment DeploymentEnvironment `json:"deployment_environment"`
	OperationMode         OperationMode         `json:"operation_mode"`
	DataProvenance        DataProvenance        `json:"data_provenance"`
	Health                Health                `json:"health"`
	Reason                string                `json:"reason,omitempty"`
}

type Module struct {
	State
	Enabled bool `json:"enabled"`
}

type Config struct {
	State
	HTTPAddr              string            `json:"http_addr"`
	PublicOrigin          string            `json:"public_origin"`
	Research              Module            `json:"research"`
	Futures               Module            `json:"futures"`
	Intel                 Module            `json:"intel"`
	DatabaseDSN           string            `json:"-"`
	InternalToken         string            `json:"-"`
	FuturesToken          string            `json:"-"`
	SessionKeys           []string          `json:"-"`
	IntelCookieSecret     string            `json:"-"`
	BootstrapOwnerFile    string            `json:"-"`
	SeedDemoUsers         bool              `json:"-"`
	Revision              string            `json:"revision"`
	BuildTime             string            `json:"build_time"`
	MigrationVersion      string            `json:"migration_version"`
	PersistentDirectories []string          `json:"persistent_directories"`
	PersistentMounts      []string          `json:"persistent_mounts"`
	Providers             []string          `json:"providers"`
	ModelVersions         map[string]string `json:"model_versions"`
	resolvedSecrets       map[string]string
}

func LoadEnv() (Config, error) { return Load(os.Getenv) }

func Load(getenv func(string) string) (Config, error) {
	return load(getenv, func() ([]byte, error) { return os.ReadFile("/proc/self/mountinfo") })
}

func load(getenv func(string) string, readMountInfo func() ([]byte, error)) (Config, error) {
	c := Config{HTTPAddr: getenv("ZHIGU_HTTP_ADDR"), PublicOrigin: getenv("ZHIGU_PUBLIC_ORIGIN"), BootstrapOwnerFile: getenv("ZHIGU_BOOTSTRAP_OWNER_FILE"), Revision: getenv("ZHIGU_VERSION_SHA"), BuildTime: getenv("ZHIGU_BUILD_TIME"), MigrationVersion: getenv("ZHIGU_MIGRATION_VERSION"), Providers: []string{}, PersistentDirectories: []string{}, ModelVersions: map[string]string{}, resolvedSecrets: map[string]string{}}
	env, alias := getenv("ZHIGU_ENV"), getenv("ZHIGU_DEPLOYMENT_ENVIRONMENT")
	if env != "" && alias != "" && env != alias {
		return c, errors.New("deployment environment variables conflict")
	}
	if env == "" {
		env = alias
	}
	if env == "" {
		env = string(Test)
	}
	c.DeploymentEnvironment = DeploymentEnvironment(env)
	production := c.DeploymentEnvironment == Production
	if c.HTTPAddr == "" {
		c.HTTPAddr = ":8080"
	}
	c.OperationMode = OperationMode(defaultValue(getenv("ZHIGU_OPERATION_MODE"), string(ReadOnly)))
	c.DataProvenance = DataProvenance(defaultValue(getenv("ZHIGU_DATA_PROVENANCE"), string(Fixture)))
	c.Health = Health(defaultValue(getenv("ZHIGU_HEALTH"), string(Ready)))
	if production {
		if getenv("ZHIGU_INTEL_FIXTURE_DIR") != "" {
			return c, errors.New("intel fixture directory is forbidden in production")
		}
		c.DataProvenance = DataProvenance(defaultValue(getenv("ZHIGU_DATA_PROVENANCE"), string(Real)))
		c.Health = Health(defaultValue(getenv("ZHIGU_HEALTH"), string(Degraded)))
	}
	if err := c.State.Validate(); err != nil {
		return c, err
	}
	if getenv("ZHIGU_ALLOW_INSECURE_DEV") != "" && getenv("ZHIGU_ALLOW_INSECURE_DEV") != "0" {
		host, _, err := net.SplitHostPort(c.HTTPAddr)
		ip := net.ParseIP(host)
		if getenv("ZHIGU_ALLOW_INSECURE_DEV") != "1" || c.DeploymentEnvironment != Test || err != nil || (host != "localhost" && (ip == nil || !ip.IsLoopback())) {
			return c, errors.New("insecure development requires explicit test environment and loopback bind")
		}
		c.SeedDemoUsers = true
	}
	for _, name := range []string{"ZHIGU_INTERNAL_TOKEN", "FUTURES_SERVICE_TOKEN", "ZHIGU_SESSION_KEYS", "POSTGRES_PASSWORD", "ZHIGU_POSTGRES_DSN", "ZHIGU_JWT_SECRET", "ZHIGU_INTEL_COOKIE_SECRET", "ZHIGU_MASTER_KEY", "FUTURES_MODEL_API_KEY", "ZHIGU_RESEARCH_MODEL_API_KEY", "ZHIGU_INTEL_MODEL_API_KEY", "FUYAO_API_KEY", "IFIND_AUTHORIZATION"} {
		value, err := secret(getenv, name)
		if err != nil {
			return c, err
		}
		c.resolvedSecrets[name] = value
	}
	c.InternalToken = c.resolvedSecrets["ZHIGU_INTERNAL_TOKEN"]
	c.FuturesToken = c.resolvedSecrets["FUTURES_SERVICE_TOKEN"]
	c.IntelCookieSecret = c.resolvedSecrets["ZHIGU_INTEL_COOKIE_SECRET"]
	if raw := c.resolvedSecrets["ZHIGU_SESSION_KEYS"]; raw != "" {
		if strings.HasPrefix(raw, "[") {
			if err := json.Unmarshal([]byte(raw), &c.SessionKeys); err != nil {
				return c, errors.New("ZHIGU_SESSION_KEYS must be a key array")
			}
		} else {
			c.SessionKeys = strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' })
		}
	}
	var err error
	for _, entry := range []struct {
		name   string
		target *Module
	}{{"RESEARCH", &c.Research}, {"FUTURES", &c.Futures}, {"INTEL", &c.Intel}} {
		entry.target.State = c.State
		enabled := getenv("ZHIGU_" + entry.name + "_ENABLED")
		entry.target.Enabled = enabled == "true" || enabled == "1" || (entry.name == "RESEARCH" && enabled == "" && !production)
		if enabled != "" && enabled != "true" && enabled != "false" && enabled != "1" && enabled != "0" {
			return c, fmt.Errorf("ZHIGU_%s_ENABLED must be boolean", entry.name)
		}
		mode := getenv("ZHIGU_" + entry.name + "_MODE")
		// Legacy research client selection and demo intel remain compatible only
		// outside production; they are never operation modes in the typed state.
		if !production && (mode == "demo" || mode == "unit-test" || mode == "http") {
			mode = ""
		}
		if mode == "" {
			if entry.target.Enabled {
				mode = string(ReadOnly)
			} else {
				mode = string(Off)
			}
		}
		entry.target.OperationMode = OperationMode(mode)
		if err = entry.target.State.Validate(); err != nil {
			return c, fmt.Errorf("%s: %w", entry.name, err)
		}
		if !entry.target.Enabled {
			entry.target.Health = Unavailable
			entry.target.Reason = "MODULE_DISABLED"
		}
		if production && entry.target.OperationMode == Live {
			if !entry.target.Enabled {
				return c, fmt.Errorf("%s live requires enabled module", entry.name)
			}
			if err := validateLive(getenv, entry.name); err != nil {
				return c, err
			}
		}
	}
	c.DatabaseDSN = c.resolvedSecrets["ZHIGU_POSTGRES_DSN"]
	if production && c.DatabaseDSN != "" {
		for _, name := range []string{"POSTGRES_USER", "POSTGRES_DB", "POSTGRES_HOST", "POSTGRES_PORT", "POSTGRES_SSLMODE", "POSTGRES_PASSWORD", "POSTGRES_PASSWORD_FILE"} {
			if getenv(name) != "" {
				return c, errors.New("ZHIGU_POSTGRES_DSN cannot be combined with independent PostgreSQL configuration")
			}
		}
	}
	if c.DatabaseDSN == "" && production {
		user, database, password := getenv("POSTGRES_USER"), getenv("POSTGRES_DB"), c.resolvedSecrets["POSTGRES_PASSWORD"]
		if user == "" || database == "" {
			return c, errors.New("POSTGRES_USER and POSTGRES_DB are required")
		}
		dbURL := url.URL{Scheme: "postgres", Host: net.JoinHostPort(defaultValue(getenv("POSTGRES_HOST"), "postgres"), defaultValue(getenv("POSTGRES_PORT"), "5432")), Path: "/" + database, User: url.UserPassword(user, password)}
		query := url.Values{"sslmode": {defaultValue(getenv("POSTGRES_SSLMODE"), "disable")}, "TimeZone": {"UTC"}}
		dbURL.RawQuery = query.Encode()
		c.DatabaseDSN = dbURL.String()
	}
	if production && c.Futures.Enabled && getenv("ZHIGU_FUTURES_STORAGE_DIR") == "" {
		return c, errors.New("ZHIGU_FUTURES_STORAGE_DIR is required for enabled production futures")
	}
	if production && c.Research.Enabled && getenv("ZHIGU_RESEARCH_DOCUMENT_DIR") == "" {
		return c, errors.New("ZHIGU_RESEARCH_DOCUMENT_DIR is required for enabled production research")
	}
	for _, name := range []string{"ZHIGU_FUTURES_STORAGE_DIR", "ZHIGU_RESEARCH_STORAGE_DIR", "ZHIGU_RESEARCH_DOCUMENT_DIR"} {
		if path := getenv(name); path != "" {
			if production {
				if err := persistentDirectory(path, getenv("ZHIGU_PERSISTENT_MOUNTS"), readMountInfo); err != nil {
					return c, fmt.Errorf("%s must be writable persistent storage", name)
				}
			}
			c.PersistentDirectories = append(c.PersistentDirectories, path)
		}
	}
	if production && len(c.PersistentDirectories) > 0 {
		c.PersistentMounts, _ = declaredMounts(getenv("ZHIGU_PERSISTENT_MOUNTS"))
	}
	if providers := getenv("ZHIGU_INTEL_PROVIDERS"); providers != "" {
		c.Providers = strings.Split(providers, ",")
	}
	for _, name := range []string{"FUTURES_MODEL_VERSION", "ZHIGU_INTEL_MODEL_VERSION", "ZHIGU_RESEARCH_MODEL_VERSION"} {
		if value := getenv(name); value != "" {
			c.ModelVersions[name] = value
		}
	}
	if production {
		if err := validateOrigin(c.PublicOrigin); err != nil {
			return c, err
		}
		if c.DataProvenance != Real {
			return c, errors.New("production requires real data provenance")
		}
		for _, name := range []string{"ZHIGU_RESEARCH_EXECUTOR", "FUTURES_EXECUTOR", "FUTURES_RESEARCH_EXECUTOR", "ZHIGU_FUTURES_EXECUTOR", "ZHIGU_MARKET_MODE", "ZHIGU_DATA_MODE", "ZHIGU_INTEL_MODE"} {
			value := strings.ToLower(getenv(name))
			if value == "fixture" || value == "demo" || value == "fake" || value == "unit-test" {
				return c, fmt.Errorf("%s cannot select fixtures in production", name)
			}
		}
		for _, item := range []struct {
			name, value string
			min         int
		}{{"ZHIGU_INTERNAL_TOKEN", c.InternalToken, 32}, {"FUTURES_SERVICE_TOKEN", c.FuturesToken, 32}, {"ZHIGU_MASTER_KEY", c.resolvedSecrets["ZHIGU_MASTER_KEY"], 32}} {
			if err := validSecret(item.name, item.value, item.min); err != nil {
				return c, err
			}
		}
		if len(c.SessionKeys) != 2 {
			return c, errors.New("ZHIGU_SESSION_KEYS requires current and previous keys")
		}
		for _, key := range c.SessionKeys {
			if err := validSecret("ZHIGU_SESSION_KEYS", key, 32); err != nil {
				return c, err
			}
			if key == c.InternalToken || key == c.FuturesToken {
				return c, errors.New("service and session credentials must be distinct")
			}
		}
		if c.SessionKeys[0] == c.SessionKeys[1] || c.InternalToken == c.FuturesToken {
			return c, errors.New("credentials must be distinct")
		}
		password, err := dsnPassword(c.DatabaseDSN)
		if err != nil {
			return c, err
		}
		if err := validSecret("database password", password, 16); err != nil {
			return c, err
		}
		if c.Intel.Enabled {
			if err := validSecret("ZHIGU_INTEL_COOKIE_SECRET", c.IntelCookieSecret, 32); err != nil {
				return c, err
			}
		}
		for name, value := range c.resolvedSecrets {
			if value != "" && name != "ZHIGU_SESSION_KEYS" && name != "ZHIGU_POSTGRES_DSN" {
				if err := validSecret(name, value, 16); err != nil {
					return c, err
				}
			}
		}
		if c.OperationMode == Live {
			return c, errors.New("global live requires module-specific release gates")
		}
	}
	return c, nil
}

func (s State) Validate() error {
	if s.DeploymentEnvironment != Test && s.DeploymentEnvironment != Demo && s.DeploymentEnvironment != Production {
		return errors.New("invalid deployment_environment")
	}
	if s.OperationMode != Off && s.OperationMode != ReadOnly && s.OperationMode != Live {
		return errors.New("invalid operation_mode")
	}
	if s.DataProvenance != Fixture && s.DataProvenance != Real {
		return errors.New("invalid data_provenance")
	}
	if s.Health != Ready && s.Health != Degraded && s.Health != Unavailable {
		return errors.New("invalid health")
	}
	return nil
}

func secret(getenv func(string) string, name string) (string, error) {
	if path := getenv(name + "_FILE"); path != "" {
		f, err := os.Open(path)
		if err != nil {
			return "", fmt.Errorf("%s_FILE cannot be read", name)
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() > 65536 {
			return "", fmt.Errorf("%s_FILE must be a small regular file", name)
		}
		body, err := io.ReadAll(io.LimitReader(f, 65537))
		if err != nil || len(body) > 65536 {
			return "", fmt.Errorf("%s_FILE cannot be read", name)
		}
		value := strings.TrimSpace(string(body))
		if value == "" {
			return "", fmt.Errorf("%s_FILE is empty", name)
		}
		return value, nil
	}
	return strings.TrimSpace(getenv(name)), nil
}

func validSecret(name, value string, minimum int) error {
	decoded := []byte(value)
	if b, err := base64.StdEncoding.DecodeString(value); err == nil {
		decoded = b
	}
	lower := strings.ToLower(value)
	placeholder := lower == "zhigu" || lower == "postgres" || lower == "password" || lower == "passw0rd!"
	for _, marker := range []string{"change-me", "changeme", "replace-me", "placeholder", "example-secret", "zhigu-internal-dev", "zhigu-dev", "test-secret", "your-secret", "your_api_key"} {
		placeholder = placeholder || strings.Contains(lower, marker)
	}
	if len(decoded) < minimum || placeholder {
		return fmt.Errorf("%s is missing, weak, or a placeholder", name)
	}
	return nil
}

func validateOrigin(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("ZHIGU_PUBLIC_ORIGIN must be an HTTPS origin without credentials or path")
	}
	host := strings.ToLower(u.Hostname())
	ip := net.ParseIP(host)
	if host == "localhost" || !strings.Contains(host, ".") || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") || (ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsMulticast())) {
		return errors.New("ZHIGU_PUBLIC_ORIGIN must be a public origin")
	}
	return nil
}

func validateLive(getenv func(string) string, module string) error {
	prefix := "ZHIGU_" + module + "_"
	if module == "FUTURES" {
		prefix = "FUTURES_"
	}
	required := []string{prefix + "MODEL_URL", prefix + "MODEL_NAME", prefix + "MODEL_VERSION", prefix + "MODEL_PRICE_CNY_PER_1K", prefix + "PROVIDER_URL"}
	if module == "INTEL" {
		required = []string{"ZHIGU_INTEL_MODEL_BASE_URL", "ZHIGU_INTEL_MODEL", "ZHIGU_INTEL_MODEL_CONFIG_ID", "ZHIGU_INTEL_MODEL_CONFIG_DIGEST", "ZHIGU_INTEL_MODEL_VERSION", "ZHIGU_INTEL_MODEL_PRICE_CNY_PER_1K"}
	}
	for _, name := range required {
		if strings.TrimSpace(getenv(name)) == "" {
			return fmt.Errorf("%s live requires %s", module, name)
		}
	}
	price, err := strconv.ParseFloat(getenv(prefix+"MODEL_PRICE_CNY_PER_1K"), 64)
	if err != nil || price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return fmt.Errorf("%s live requires positive model price", module)
	}
	key, err := secret(getenv, prefix+"MODEL_API_KEY")
	if err != nil {
		return err
	}
	if err := validSecret(prefix+"MODEL_API_KEY", key, 16); err != nil {
		return err
	}
	if module == "INTEL" {
		return validateIntelLiveProviders(getenv)
	}
	return nil
}

func validateIntelLiveProviders(getenv func(string) string) error {
	protocol := defaultValue(strings.TrimSpace(getenv("ZHIGU_INTEL_MODEL_PROTOCOL")), "openai_chat_completions")
	if protocol != "openai_chat_completions" && protocol != "openai_responses" {
		return errors.New("INTEL live requires a supported model protocol")
	}
	if err := publicHTTPSURL(getenv("ZHIGU_INTEL_MODEL_BASE_URL")); err != nil {
		return errors.New("INTEL live requires a public HTTPS model base URL")
	}
	providers := strings.Split(getenv("ZHIGU_INTEL_PROVIDERS"), ",")
	for _, raw := range providers {
		provider := strings.TrimSpace(raw)
		switch provider {
		case "ifind":
			if err := publicHTTPSURL(getenv("IFIND_MCP_URL")); err != nil {
				return errors.New("INTEL ifind requires a public HTTPS IFIND_MCP_URL")
			}
			key, err := secret(getenv, "IFIND_AUTHORIZATION")
			if err != nil {
				return err
			}
			if err := validSecret("IFIND_AUTHORIZATION", key, 16); err != nil {
				return err
			}
		case "fuyao":
			key, err := secret(getenv, "FUYAO_API_KEY")
			if err != nil {
				return err
			}
			if err := validSecret("FUYAO_API_KEY", key, 16); err != nil {
				return err
			}
		case "cninfo":
			if getenv("ZHIGU_CNINFO_LIVE_VERIFIED") != "1" {
				return errors.New("INTEL cninfo requires explicit live verification")
			}
		default:
			return errors.New("INTEL live requires configured real providers; fixtures and unknown providers are forbidden")
		}
	}
	return nil
}

func publicHTTPSURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return err
	}
	u.Path = ""
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return validateOrigin(u.String())
}

func dsnPassword(dsn string) (string, error) {
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return "", errors.New("invalid production database DSN")
	}
	return config.Password, nil
}

func persistentDirectory(path, declarations string, readMountInfo func() ([]byte, error)) error {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(resolved) || resolved == "/" || resolved == "/tmp" || strings.HasPrefix(resolved, "/tmp/") || strings.HasPrefix(resolved, "/private/tmp/") {
		return errors.New("temporary storage")
	}
	if temporary, err := filepath.EvalSymlinks(os.TempDir()); err == nil && (resolved == temporary || strings.HasPrefix(resolved, temporary+string(os.PathSeparator))) {
		return errors.New("temporary storage")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return errors.New("missing directory")
	}
	if err := unix.Access(resolved, unix.W_OK); err != nil {
		return err
	}
	declared, err := declaredMounts(declarations)
	if err != nil {
		return err
	}
	body, err := readMountInfo()
	if err != nil {
		return errors.New("cannot prove persistent mount")
	}
	actual, err := coveringMount(resolved, string(body))
	if err != nil {
		return err
	}
	for _, mount := range declared {
		if actual.path == mount && actual.writable && persistentFilesystem(actual.filesystem) {
			return nil
		}
	}
	return errors.New("storage must be on a declared writable persistent mount")
}

func declaredMounts(raw string) ([]string, error) {
	var paths []string
	if err := json.Unmarshal([]byte(raw), &paths); err != nil || len(paths) == 0 {
		return nil, errors.New("ZHIGU_PERSISTENT_MOUNTS must be a nonempty JSON array")
	}
	for i, path := range paths {
		if !filepath.IsAbs(path) {
			return nil, errors.New("persistent mount declaration must be absolute")
		}
		canonical, err := filepath.EvalSymlinks(path)
		if err != nil || canonical == "/" {
			return nil, errors.New("persistent mount declaration is invalid")
		}
		paths[i] = canonical
	}
	return paths, nil
}

type mountRecord struct {
	path, filesystem string
	writable         bool
}

func coveringMount(path, body string) (mountRecord, error) {
	var chosen mountRecord
	scanner := bufio.NewScanner(strings.NewReader(body))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		separator := -1
		for i, field := range fields {
			if field == "-" {
				separator = i
				break
			}
		}
		if separator < 6 || separator+3 >= len(fields) {
			return chosen, errors.New("invalid mount information")
		}
		mountpoint := strings.NewReplacer("\\040", " ", "\\011", "\t", "\\012", "\n", "\\134", "\\").Replace(fields[4])
		if !filepath.IsAbs(mountpoint) {
			return chosen, errors.New("invalid mount information")
		}
		if path != mountpoint && !strings.HasPrefix(path, strings.TrimSuffix(mountpoint, "/")+"/") {
			continue
		}
		if mountpoint == chosen.path {
			return chosen, errors.New("ambiguous storage mount information")
		}
		if len(mountpoint) > len(chosen.path) {
			chosen = mountRecord{path: mountpoint, filesystem: fields[separator+1], writable: mountOption(fields[5], "rw") && mountOption(fields[separator+3], "rw")}
		}
	}
	if scanner.Err() != nil || chosen.path == "" {
		return chosen, errors.New("cannot prove persistent mount")
	}
	return chosen, nil
}

func mountOption(options, wanted string) bool {
	for _, value := range strings.Split(options, ",") {
		if value == wanted {
			return true
		}
	}
	return false
}

func persistentFilesystem(filesystem string) bool {
	switch filesystem {
	case "ext2", "ext3", "ext4", "xfs", "btrfs", "zfs", "nfs", "nfs4", "cifs", "smb3", "ceph", "fuse.ceph", "fuse.glusterfs", "virtiofs", "9p":
		return true
	}
	return false
}

func defaultValue(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func (c Config) SummaryJSON() string { body, _ := json.Marshal(c); return string(body) }
func (c Config) String() string      { return c.SummaryJSON() }

// RequestAllowed closes the legacy research/demo API boundaries until their
// module is enabled. Login and owner configuration remain usable during setup.
func (c Config) RequestAllowed(method, path, intelMode string) bool {
	if c.DeploymentEnvironment != Production {
		return true
	}
	if strings.HasPrefix(path, "/api/finance/auth/") || strings.HasPrefix(path, "/api/v1/session/") {
		return true
	}
	if strings.HasPrefix(path, "/api/finance/intel/") {
		if intelMode != "" && intelMode != "live" || strings.Contains(path, "/demo-sessions") || strings.Contains(path, "/replay/") {
			return false
		}
		return c.Intel.Enabled && c.Intel.OperationMode != Off && (method == "GET" || method == "HEAD" || method == "OPTIONS" || c.Intel.OperationMode == Live)
	}
	if strings.HasPrefix(path, "/api/finance/admin") {
		return !strings.HasSuffix(path, "/test") || c.Research.OperationMode == Live
	}
	if strings.HasPrefix(path, "/api/finance/") || strings.HasPrefix(path, "/internal/finance/") || strings.HasPrefix(path, "/internal/llm/") {
		return c.Research.Enabled && c.Research.OperationMode != Off && (method == "GET" || method == "HEAD" || method == "OPTIONS" || c.Research.OperationMode == Live)
	}
	return true
}

// ApplyEnvironment bridges legacy services which still read credentials from
// environment variables. It is called once, before constructing any services.
func (c Config) ApplyEnvironment() error {
	for name, value := range c.resolvedSecrets {
		if value != "" {
			if err := os.Setenv(name, value); err != nil {
				return fmt.Errorf("cannot configure %s", name)
			}
		}
	}
	if c.DeploymentEnvironment == Production {
		for name, value := range map[string]string{"ZHIGU_ENV": string(Production), "ZHIGU_DEPLOYMENT_ENVIRONMENT": string(Production), "ZHIGU_DATA_MODE": "live", "ZHIGU_MARKET_MODE": "live", "ZHIGU_JWT_SECRET": c.SessionKeys[0], "ZHIGU_INTEL_PUBLIC_ORIGIN": c.PublicOrigin} {
			if err := os.Setenv(name, value); err != nil {
				return fmt.Errorf("cannot configure %s", name)
			}
		}
	}
	return nil
}
