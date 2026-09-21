package testdb

import (
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"zhigu/server/initialize"
)

var (
	once   sync.Once
	shared *gorm.DB
	startErr error
	ep     *embeddedpostgres.EmbeddedPostgres
)

func Start(t *testing.T) *gorm.DB {
	t.Helper()
	once.Do(func() {
		runtimeDir, err := os.MkdirTemp("", "zhigu-pg-*")
		if err != nil {
			startErr = err
			return
		}
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			startErr = err
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
		ep = embeddedpostgres.NewDatabase(cfg)
		if err := ep.Start(); err != nil {
			startErr = fmt.Errorf("embedded postgres start: %w", err)
			return
		}
		dsn := fmt.Sprintf("host=127.0.0.1 port=%d user=postgres password=postgres dbname=postgres sslmode=disable TimeZone=UTC", port)
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
		if err != nil {
			startErr = err
			return
		}
		if err := initialize.Migrate(db); err != nil {
			startErr = fmt.Errorf("apply migration: %w", err)
			return
		}
		shared = db
	})
	if startErr != nil {
		t.Fatalf("%v", startErr)
	}
	reset(t, shared)
	return shared
}

func reset(t *testing.T, db *gorm.DB) {
	t.Helper()
	stmts := []string{
		"TRUNCATE TABLE finance_backtest_results, finance_backtest_equity, finance_backtest_fills, finance_backtest_orders, finance_backtest_runs, finance_strategy_versions, finance_strategies, finance_strategy_generations, finance_strategy_drafts, finance_strategy_workspace, finance_strategy_idempotency, finance_market_bars, finance_market_snapshots, finance_market_actions, finance_market_instrument_aliases, finance_market_instruments, finance_market_calendars, finance_market_rules, finance_market_catalog_releases, finance_market_pointers, finance_report_checks, finance_questions, finance_reports, finance_calculations, finance_provider_records, finance_data_records, finance_task_evidence, finance_evidence, finance_tool_grants, finance_usage_ledger, finance_model_cache, finance_internal_tokens, finance_research_tasks, finance_research_runs, finance_claim_drafts, finance_config_versions, finance_users RESTART IDENTITY CASCADE",
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("reset: %v", err)
		}
	}
}
