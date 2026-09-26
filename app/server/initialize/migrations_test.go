package initialize

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	pgOnce  sync.Once
	pgInst  *embeddedpostgres.EmbeddedPostgres
	pgPort  uint32
	pgErr   error
	testSeq int32
)

// TestMain stops the embedded server so test runs do not leak shared memory.
func TestMain(m *testing.M) {
	code := m.Run()
	if pgInst != nil {
		_ = pgInst.Stop()
	}
	os.Exit(code)
}

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	pgOnce.Do(func() {
		runtimeDir, err := os.MkdirTemp("", "zhigu-migrate-pg-*")
		if err != nil {
			pgErr = err
			return
		}
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			pgErr = err
			return
		}
		port := uint32(ln.Addr().(*net.TCPAddr).Port)
		_ = ln.Close()
		cfg := embeddedpostgres.DefaultConfig().
			Username("postgres").
			Password("postgres").
			Database("postgres").
			Version(embeddedpostgres.V16).
			Port(port).
			RuntimePath(runtimeDir).
			Logger(io.Discard).
			StartTimeout(90 * time.Second)
		pgInst = embeddedpostgres.NewDatabase(cfg)
		if err := pgInst.Start(); err != nil {
			pgErr = fmt.Errorf("embedded postgres: %w", err)
			return
		}
		pgPort = port
	})
	if pgErr != nil {
		t.Fatalf("%v", pgErr)
	}
	name := fmt.Sprintf("zhigu_mig_%d", atomic.AddInt32(&testSeq, 1))
	admin := openTestDB(t, "postgres")
	if err := admin.Exec("CREATE DATABASE " + name).Error; err != nil {
		t.Fatalf("create database %s: %v", name, err)
	}
	return openTestDB(t, name)
}

func openTestDB(t *testing.T, dbname string) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("host=127.0.0.1 port=%d user=postgres password=postgres dbname=%s sslmode=disable TimeZone=UTC", pgPort, dbname)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open %s: %v", dbname, err)
	}
	return db
}

func realFiles(t *testing.T) []Migration {
	t.Helper()
	ms, err := LoadEmbeddedMigrations()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	if len(ms) < 5 {
		t.Fatalf("want at least 5 migration files, got %d", len(ms))
	}
	return ms
}

func mustMigration(version, sql string) Migration {
	sum := sha256.Sum256([]byte(sql))
	return Migration{Version: version, SQL: sql, Checksum: hex.EncodeToString(sum[:])}
}

// applyRawSQL simulates a legacy database built before the ledger existed.
func applyRawSQL(t *testing.T, db *gorm.DB, sql string) {
	t.Helper()
	for _, stmt := range splitStatements(sql) {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("apply raw statement: %v\n%s", err, stmt)
		}
	}
}

func ledgerCount(t *testing.T, db *gorm.DB) int {
	t.Helper()
	if !tableExists(t, db, "finance_schema_migrations") {
		return 0
	}
	var n int
	if err := db.Raw(`SELECT count(*) FROM finance_schema_migrations`).Scan(&n).Error; err != nil {
		t.Fatalf("ledger count: %v", err)
	}
	return n
}

func tableExists(t *testing.T, db *gorm.DB, table string) bool {
	t.Helper()
	var n int
	if err := db.Raw(`SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name = ?`, table).Scan(&n).Error; err != nil {
		t.Fatalf("table exists: %v", err)
	}
	return n > 0
}

func columnExists(t *testing.T, db *gorm.DB, table, column string) bool {
	t.Helper()
	var n int
	if err := db.Raw(`SELECT count(*) FROM information_schema.columns WHERE table_schema = 'public' AND table_name = ? AND column_name = ?`, table, column).Scan(&n).Error; err != nil {
		t.Fatalf("column exists: %v", err)
	}
	return n > 0
}

func execExpectError(t *testing.T, db *gorm.DB, want string, sql string) {
	t.Helper()
	err := db.Exec(sql).Error
	if err == nil {
		t.Fatalf("expected error for %q", sql)
	}
	if want != "" && !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err.Error(), want)
	}
}

func TestSplitStatements(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		want []string
	}{
		{"simple", "SELECT 1; SELECT 2;", []string{"SELECT 1", "SELECT 2"}},
		{"string with semicolon", "SELECT 'a;b'; SELECT 2;", []string{"SELECT 'a;b'", "SELECT 2"}},
		{"line comment", "SELECT 1; -- keep; me\nSELECT 2;", []string{"SELECT 1", "-- keep; me\nSELECT 2"}},
		{"block comment", "SELECT /* x;y */ 1;", []string{"SELECT /* x;y */ 1"}},
		{"dollar body", "CREATE FUNCTION f() RETURNS trigger AS $$\nBEGIN\n  RAISE EXCEPTION 'a;b';\nEND;\n$$ LANGUAGE plpgsql;", []string{"CREATE FUNCTION f() RETURNS trigger AS $$\nBEGIN\n  RAISE EXCEPTION 'a;b';\nEND;\n$$ LANGUAGE plpgsql"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitStatements(tc.sql)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d statements %q, want %d", len(got), got, len(tc.want))
			}
			for i := range got {
				if strings.TrimSpace(got[i]) != strings.TrimSpace(tc.want[i]) {
					t.Fatalf("statement %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestParseSchemaObjects(t *testing.T) {
	exp, err := parseSchemaObjects(`
CREATE TABLE sample_a (
  id TEXT PRIMARY KEY,
  owner_id INTEGER NOT NULL REFERENCES finance_users(id),
  slug TEXT NOT NULL UNIQUE,
  payload JSONB NOT NULL DEFAULT '{}',
  status TEXT NOT NULL,
  CONSTRAINT sample_a_status_chk CHECK (status IN ('a', 'b')),
  CONSTRAINT sample_a_pair_uniq UNIQUE (owner_id, slug)
);
CREATE INDEX sample_a_owner ON sample_a (owner_id, status);
ALTER TABLE sample_a ADD COLUMN extra TEXT;
ALTER TABLE sample_a DROP CONSTRAINT IF EXISTS gone_constraint;
`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	exp = exp.dedup()
	if len(exp.tables) != 1 || exp.tables[0] != "sample_a" {
		t.Fatalf("tables = %v", exp.tables)
	}
	if len(exp.columns) != 6 {
		t.Fatalf("columns = %v", exp.columns)
	}
	if len(exp.indexes) != 1 || exp.indexes[0] != "sample_a_owner" {
		t.Fatalf("indexes = %v", exp.indexes)
	}
	if len(exp.named) != 2 {
		t.Fatalf("named = %v", exp.named)
	}
	if len(exp.pks) != 1 || keyDesc(exp.pks[0]) != "sample_a(id)" {
		t.Fatalf("pks = %v", exp.pks)
	}
	if len(exp.uniques) != 2 {
		t.Fatalf("uniques = %v", exp.uniques)
	}
	if len(exp.fks) != 1 || exp.fks[0].ref != "finance_users" {
		t.Fatalf("fks = %v", exp.fks)
	}
	if exp.checks["sample_a"] != 1 {
		t.Fatalf("checks = %v", exp.checks)
	}
}

func TestMigrateFreshDatabaseAndRepeat(t *testing.T) {
	files := realFiles(t)
	db := testDB(t)
	if err := MigrateMigrations(db, files); err != nil {
		t.Fatalf("migrate fresh: %v", err)
	}
	if got := ledgerCount(t, db); got != len(files) {
		t.Fatalf("ledger rows = %d, want %d", got, len(files))
	}
	for _, table := range []string{
		"finance_strategy_market_items", "finance_strategy_market_versions",
		"finance_strategy_market_evidence", "finance_strategy_market_audit",
		"finance_intel_namespaces", "finance_intel_demo_sessions",
		"finance_intel_instruments", "finance_intel_watchlist",
		"finance_intel_source_revisions", "finance_intel_events",
		"finance_intel_evidence", "finance_intel_snapshots",
		"finance_intel_changes", "finance_intel_timeline_nodes",
		"finance_intel_conflicts", "finance_intel_outbox",
		"finance_intel_notifications", "finance_intel_jobs",
		"finance_intel_review_items", "finance_intel_idempotency",
		"finance_intel_mutes", "finance_intel_extraction_runs",
		"finance_intel_event_bindings", "finance_intel_evidence_memberships",
		"finance_intel_provider_cursors", "finance_intel_usage",
		"finance_intel_audit",
	} {
		if !tableExists(t, db, table) {
			t.Fatalf("missing table %s", table)
		}
	}
	if !columnExists(t, db, "finance_intel_watchlist", "principal_key") ||
		!columnExists(t, db, "finance_intel_watchlist", "revision") ||
		!columnExists(t, db, "finance_intel_events", "review_status") ||
		!columnExists(t, db, "finance_intel_evidence", "authoritative") ||
		!columnExists(t, db, "finance_intel_evidence", "subject_unique") ||
		!columnExists(t, db, "finance_intel_evidence", "modality_explicit") ||
		!columnExists(t, db, "finance_intel_jobs", "generation") {
		t.Fatal("missing Intel scope/audit fields required by SPEC")
	}
	if !columnExists(t, db, "finance_strategy_drafts", "origin_market_item_id") ||
		!columnExists(t, db, "finance_strategy_versions", "editor_state") ||
		!columnExists(t, db, "finance_strategy_generations", "explanation") ||
		!columnExists(t, db, "finance_strategy_idempotency", "response_snapshot") {
		t.Fatal("missing incremental columns on 004 tables")
	}
	// Repeat start is a no-op and never re-executes or re-registers.
	if err := MigrateMigrations(db, files); err != nil {
		t.Fatalf("migrate repeat: %v", err)
	}
	if got := ledgerCount(t, db); got != len(files) {
		t.Fatalf("ledger rows after repeat = %d, want %d", got, len(files))
	}

	t.Run("market constraints enforce contract", func(t *testing.T) {
		db.Exec(`INSERT INTO finance_users (id, username, password_hash, role) VALUES (1001, 'u', 'x', 'user')`)
		db.Exec(`INSERT INTO finance_strategy_market_items (id, slug, status, revision, created_by) VALUES ('smi_a', 'slug-a', 'draft', 1, 1001)`)
		execExpectError(t, db, "", `INSERT INTO finance_strategy_market_items (id, slug, status, revision, created_by) VALUES ('smi_b', 'slug-a', 'draft', 1, 1001)`)
		execExpectError(t, db, "", `INSERT INTO finance_strategy_market_items (id, slug, status, revision, created_by) VALUES ('smi_b', 'slug-b', 'published', 1, 1001)`)
		execExpectError(t, db, "", `INSERT INTO finance_strategy_market_audit (id, item_id, market_version_id, actor_id, action, request_id, before_revision, after_revision, payload_hash) VALUES ('sma_x', 'smi_a', 'smv_missing', 1001, 'publish', 'req', 1, 2, 'h')`)
	})

	t.Run("audit is append-only", func(t *testing.T) {
		db.Exec(`INSERT INTO finance_users (id, username, password_hash, role) VALUES (1002, 'u2', 'x', 'user')`)
		db.Exec(`INSERT INTO finance_strategy_market_items (id, slug, status, revision, created_by) VALUES ('smi_c', 'slug-c', 'draft', 1, 1002)`)
		if err := db.Exec(`INSERT INTO finance_strategy_market_audit (id, item_id, actor_id, action, request_id, before_revision, after_revision, payload_hash) VALUES ('sma_a', 'smi_c', 1002, 'create_version', 'req1', 1, 2, 'h')`).Error; err != nil {
			t.Fatalf("insert audit: %v", err)
		}
		execExpectError(t, db, "append-only", `UPDATE finance_strategy_market_audit SET reason = 'tamper' WHERE id = 'sma_a'`)
		execExpectError(t, db, "append-only", `DELETE FROM finance_strategy_market_audit WHERE id = 'sma_a'`)
	})
}

func TestMigrateLegacyBaseline(t *testing.T) {
	files := realFiles(t)
	db := testDB(t)
	for _, m := range files[:4] {
		applyRawSQL(t, db, m.SQL)
	}
	if err := MigrateMigrations(db, files); err != nil {
		t.Fatalf("migrate legacy: %v", err)
	}
	if got := ledgerCount(t, db); got != len(files) {
		t.Fatalf("ledger rows = %d, want %d", got, len(files))
	}
	if !tableExists(t, db, "finance_strategy_market_items") {
		t.Fatal("005 not applied on legacy database")
	}
	if err := MigrateMigrations(db, files); err != nil {
		t.Fatalf("migrate legacy repeat: %v", err)
	}
}

func TestMigratePartialSchemaRefused(t *testing.T) {
	files := realFiles(t)
	db := testDB(t)
	for _, m := range files[:4] {
		applyRawSQL(t, db, m.SQL)
	}
	db.Exec(`CREATE TABLE finance_strategy_market_items (id TEXT PRIMARY KEY)`)
	err := MigrateMigrations(db, files)
	if err == nil || !strings.Contains(err.Error(), "partially applied") {
		t.Fatalf("want partially-applied error, got %v", err)
	}
	if got := ledgerCount(t, db); got != 0 {
		t.Fatalf("ledger rows = %d, want 0", got)
	}
}

func TestMigrateSchemaDriftRefused(t *testing.T) {
	files := realFiles(t)
	db := testDB(t)
	for _, m := range files[:4] {
		applyRawSQL(t, db, m.SQL)
	}
	if err := db.Exec(`ALTER TABLE finance_users DROP CONSTRAINT finance_users_role_check`).Error; err != nil {
		t.Fatalf("drop check: %v", err)
	}
	err := MigrateMigrations(db, files)
	if err == nil || !strings.Contains(err.Error(), "finance_users") {
		t.Fatalf("want schema mismatch error naming finance_users, got %v", err)
	}
	if got := ledgerCount(t, db); got != 0 {
		t.Fatalf("ledger rows = %d, want 0", got)
	}
}

func TestMigrateChecksumDriftStops(t *testing.T) {
	db := testDB(t)
	v1 := mustMigration("001_x", "CREATE TABLE drift_t (id TEXT PRIMARY KEY);")
	if err := MigrateMigrations(db, []Migration{v1}); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	v1Changed := mustMigration("001_x", "CREATE TABLE drift_t (id TEXT PRIMARY KEY, extra TEXT);")
	err := MigrateMigrations(db, []Migration{v1Changed})
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("want checksum error, got %v", err)
	}
}

func TestMigrateFailedRunRollsBack(t *testing.T) {
	db := testDB(t)
	good := mustMigration("001_ok", "CREATE TABLE rollback_ok (id TEXT PRIMARY KEY);")
	bad := mustMigration("002_bad", "CREATE TABLE rollback_bad (id TEXT PRIMARY KEY); SELECT zhigu_no_such_function();")
	later := mustMigration("003_later", "CREATE TABLE rollback_later (id TEXT PRIMARY KEY);")
	err := MigrateMigrations(db, []Migration{good, bad, later})
	if err == nil {
		t.Fatal("want migration failure")
	}
	for _, table := range []string{"rollback_ok", "rollback_bad", "rollback_later"} {
		if tableExists(t, db, table) {
			t.Fatalf("table %s must not exist after rolled-back run", table)
		}
	}
	if got := ledgerCount(t, db); got != 0 {
		t.Fatalf("ledger rows = %d, want 0", got)
	}
}

func TestMigrateConcurrentStarts(t *testing.T) {
	files := realFiles(t)
	db := testDB(t)
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- MigrateMigrations(db, files)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent migrate: %v", err)
		}
	}
	if got := ledgerCount(t, db); got != len(files) {
		t.Fatalf("ledger rows = %d, want %d", got, len(files))
	}
}
