package initialize

import (
	"os"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	modelfinance "zhigu/server/model/finance"
)

func OpenDB() (*gorm.DB, error) {
	dsn := os.Getenv("ZHIGU_POSTGRES_DSN")
	if dsn == "" {
		dsn = "host=127.0.0.1 user=zhigu password=zhigu dbname=zhigu port=5432 sslmode=disable TimeZone=UTC"
	}
	return gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Warn)})
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
