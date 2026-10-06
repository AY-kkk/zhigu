package futures

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"zhigu/server/migrations"
)

type Migration struct {
	Version  int64
	SQL      string
	Checksum string
}

type RuntimeState struct {
	Enabled bool
	Ready   bool
	Mode    string
	Reason  string
}

type Config struct {
	Enabled bool
	Mode    string
}

func LoadMigrations() ([]Migration, error) {
	names, err := fs.Glob(migrations.FS, "futures/*.sql")
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
		base := name[strings.LastIndexByte(name, '/')+1:]
		version, err := strconv.ParseInt(strings.SplitN(base, "_", 2)[0], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("futures migration %s: %w", name, err)
		}
		sum := sha256.Sum256(raw)
		out = append(out, Migration{Version: version, SQL: string(raw), Checksum: hex.EncodeToString(sum[:])})
	}
	return out, nil
}

// Bootstrap performs futures-local setup only. Disabled deployments do not
// install the schema; failures are returned to the caller and never stop finance.
func Bootstrap(ctx context.Context, db *gorm.DB, cfg Config) (RuntimeState, error) {
	if !cfg.Enabled {
		mode := cfg.Mode
		if mode == "" {
			mode = "off"
		}
		return RuntimeState{Enabled: false, Ready: false, Mode: "off", Reason: "FUTURES_DISABLED"}, nil
	}
	ms, err := LoadMigrations()
	if err != nil {
		return RuntimeState{Enabled: true, Ready: false, Mode: "off", Reason: "FUTURES_MIGRATION_UNAVAILABLE"}, err
	}
	if err := MigrateList(ctx, db, ms); err != nil {
		return RuntimeState{Enabled: true, Ready: false, Mode: "off", Reason: "FUTURES_MIGRATION_FAILED"}, err
	}
	mode := cfg.Mode
	if mode == "live" {
		mode = "read_only"
	} // M1 has no verified data/model pipeline.
	return RuntimeState{Enabled: true, Ready: false, Mode: mode, Reason: "FUTURES_NOT_IMPLEMENTED"}, nil
}

func Migrate(ctx context.Context, db *gorm.DB) error {
	ms, err := LoadMigrations()
	if err != nil {
		return err
	}
	return MigrateList(ctx, db, ms)
}

func MigrateList(ctx context.Context, db *gorm.DB, ms []Migration) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(7250146202644)`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`CREATE TABLE IF NOT EXISTS futures_schema_migrations (
  version BIGINT PRIMARY KEY,
  checksum TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`).Error; err != nil {
			return err
		}
		for _, m := range ms {
			var checksum string
			err := tx.Raw(`SELECT checksum FROM futures_schema_migrations WHERE version=?`, m.Version).Scan(&checksum).Error
			if err != nil && err != gorm.ErrRecordNotFound {
				return err
			}
			if checksum != "" {
				if checksum != m.Checksum {
					return fmt.Errorf("futures migration %d checksum drift", m.Version)
				}
				continue
			}
			for _, stmt := range splitMigrationSQL(m.SQL) {
				if err := tx.Exec(stmt).Error; err != nil {
					return fmt.Errorf("futures migration %d: %w", m.Version, err)
				}
			}
			if err := tx.Exec(`INSERT INTO futures_schema_migrations(version,checksum) VALUES(?,?)`, m.Version, m.Checksum).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func splitMigrationSQL(sql string) []string {
	var out []string
	var b strings.Builder
	for i := 0; i < len(sql); i++ {
		c := sql[i]
		if c == '\'' {
			j := i + 1
			for j < len(sql) {
				if sql[j] == '\'' {
					if j+1 < len(sql) && sql[j+1] == '\'' {
						j += 2
						continue
					}
					j++
					break
				}
				j++
			}
			b.WriteString(sql[i:j])
			i = j - 1
			continue
		}
		if c == '-' && i+1 < len(sql) && sql[i+1] == '-' {
			j := strings.IndexByte(sql[i:], '\n')
			if j < 0 {
				i = len(sql)
				continue
			}
			b.WriteString(sql[i : i+j])
			i += j
			continue
		}
		if c == '/' && i+1 < len(sql) && sql[i+1] == '*' {
			j := strings.Index(sql[i+2:], "*/")
			if j < 0 {
				i = len(sql)
				continue
			}
			j += i + 4
			b.WriteString(sql[i:j])
			i = j - 1
			continue
		}
		if c == ';' {
			if s := strings.TrimSpace(b.String()); s != "" {
				out = append(out, s)
			}
			b.Reset()
			continue
		}
		b.WriteByte(c)
	}
	if s := strings.TrimSpace(b.String()); s != "" {
		out = append(out, s)
	}
	return out
}
