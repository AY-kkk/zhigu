package initialize

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"gorm.io/gorm"

	"zhigu/server/migrations"
)

// migrationLockKey serializes schema migration across concurrent process starts.
const migrationLockKey int64 = 7250146202633

// Migration is one versioned schema change: the complete file content plus its
// checksum as recorded in finance_schema_migrations.
type Migration struct {
	Version  string
	SQL      string
	Checksum string
}

type ledgerRow struct {
	Version  string
	Checksum string
}

// LoadEmbeddedMigrations returns the embedded migration files ordered by version.
func LoadEmbeddedMigrations() ([]Migration, error) {
	names, err := fs.Glob(migrations.FS, "finance/*.sql")
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	out := make([]Migration, 0, len(names))
	for _, name := range names {
		raw, err := migrations.FS.ReadFile(name)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(raw)
		base := name
		if i := strings.LastIndexByte(base, '/'); i >= 0 {
			base = base[i+1:]
		}
		out = append(out, Migration{
			Version:  strings.TrimSuffix(base, ".sql"),
			SQL:      string(raw),
			Checksum: hex.EncodeToString(sum[:]),
		})
	}
	return out, nil
}

// Migrate applies the embedded migrations with a versioned ledger:
// each file executes completely before its checksum is registered, failed runs
// roll back whole and block startup, and an existing database is only adopted as
// historical baseline after its schema is verified against the file expectations.
func Migrate(db *gorm.DB) error {
	ms, err := LoadEmbeddedMigrations()
	if err != nil {
		return err
	}
	return MigrateMigrations(db, ms)
}

// MigrateMigrations is Migrate with an explicit file list (used by tests).
func MigrateMigrations(db *gorm.DB, ms []Migration) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(fmt.Sprintf("SELECT pg_advisory_xact_lock(%d)", migrationLockKey)).Error; err != nil {
			return fmt.Errorf("migration lock: %w", err)
		}
		if err := tx.Exec(`CREATE TABLE IF NOT EXISTS finance_schema_migrations (
  version TEXT PRIMARY KEY,
  checksum TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`).Error; err != nil {
			return fmt.Errorf("create migration ledger: %w", err)
		}
		var rows []ledgerRow
		if err := tx.Raw(`SELECT version, checksum FROM finance_schema_migrations ORDER BY version`).Scan(&rows).Error; err != nil {
			return fmt.Errorf("read migration ledger: %w", err)
		}
		registered := make(map[string]string, len(rows))
		for _, r := range rows {
			registered[r.Version] = r.Checksum
		}

		cat, err := loadCatalog(tx)
		if err != nil {
			return fmt.Errorf("inspect schema: %w", err)
		}

		var baseline, pending []Migration
		var baselineExps []schemaExpect
		seenPending := false
		for _, m := range ms {
			if have, ok := registered[m.Version]; ok {
				if have != m.Checksum {
					return fmt.Errorf("migration %s: registered checksum %s but file now hashes to %s; refusing to start",
						m.Version, have, m.Checksum)
				}
				continue
			}
			exp, err := parseSchemaObjects(m.SQL)
			if err != nil {
				return fmt.Errorf("migration %s: %w", m.Version, err)
			}
			exp = exp.dedup()
			total := cat.objectCount(exp)
			diffs := cat.missing(exp)
			switch {
			case total > 0 && len(diffs) == 0:
				// Every object this file declares already exists: legacy database
				// without ledger. Verified below before being registered.
				if seenPending {
					return fmt.Errorf("migration %s is already applied but an earlier migration is missing; out-of-order schema state", m.Version)
				}
				baseline = append(baseline, m)
				baselineExps = append(baselineExps, exp)
			case len(diffs) == total:
				seenPending = true
				pending = append(pending, m)
			default:
				return fmt.Errorf("migration %s is partially applied; repair the schema before start:\n  %s",
					m.Version, strings.Join(diffs, "\n  "))
			}
		}

		if len(baseline) > 0 {
			merged := schemaExpect{checks: map[string]int{}}
			for _, exp := range baselineExps {
				merged.merge(exp)
			}
			if diffs := cat.missing(merged.dedup()); len(diffs) > 0 {
				return fmt.Errorf("existing schema does not match migration expectations; repair before start:\n  %s",
					strings.Join(diffs, "\n  "))
			}
			for _, m := range baseline {
				if err := insertLedger(tx, m); err != nil {
					return err
				}
			}
		}

		for _, m := range pending {
			for _, stmt := range splitStatements(m.SQL) {
				if err := tx.Exec(stmt).Error; err != nil {
					return fmt.Errorf("migration %s: %w", m.Version, err)
				}
			}
			if err := insertLedger(tx, m); err != nil {
				return err
			}
		}
		return nil
	})
}

func insertLedger(tx *gorm.DB, m Migration) error {
	if err := tx.Exec(`INSERT INTO finance_schema_migrations (version, checksum) VALUES (?, ?)`, m.Version, m.Checksum).Error; err != nil {
		return fmt.Errorf("migration %s: register: %w", m.Version, err)
	}
	return nil
}
