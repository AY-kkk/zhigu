package initialize

import (
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"zhigu/server/migrations"
	modelfinance "zhigu/server/model/finance"
)

func OpenDB() (*gorm.DB, error) {
	dsn := os.Getenv("ZHIGU_POSTGRES_DSN")
	if dsn == "" {
		dsn = "host=127.0.0.1 user=zhigu password=zhigu dbname=zhigu port=5432 sslmode=disable TimeZone=UTC"
	}
	return gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Warn)})
}

func Migrate(db *gorm.DB) error {
	files, err := fs.Glob(migrations.FS, "finance/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, name := range files {
		sqlBytes, err := migrations.FS.ReadFile(name)
		if err != nil {
			return err
		}
		for _, stmt := range splitSQL(string(sqlBytes)) {
			if err := db.Exec(stmt).Error; err != nil {
				lower := strings.ToLower(err.Error())
				if strings.Contains(lower, "already exists") ||
					strings.Contains(lower, "duplicate key") ||
					strings.Contains(lower, "sqlstate 23505") ||
					strings.Contains(lower, "multiple primary keys") {
					continue
				}
				return fmt.Errorf("migrate %s: %w", name, err)
			}
		}
	}
	return nil
}

func Seed(db *gorm.DB) error {
	hash, err := bcrypt.GenerateFromPassword([]byte("Passw0rd!"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	users := []modelfinance.User{
		{ID: 1, Username: "admin", PasswordHash: string(hash), Role: "admin"},
		{ID: 1001, Username: "invitee", PasswordHash: string(hash), Role: "user"},
		{ID: 1002, Username: "invitee-b", PasswordHash: string(hash), Role: "user"},
	}
	for _, u := range users {
		var count int64
		if err := db.Model(&modelfinance.User{}).Where("id = ?", u.ID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			if err := db.Create(&u).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func splitSQL(s string) []string {
	var cleaned []string
	for _, line := range strings.Split(s, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "--") {
			continue
		}
		cleaned = append(cleaned, line)
	}
	parts := strings.Split(strings.Join(cleaned, "\n"), ";")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}
