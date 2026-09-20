package initialize

import "gorm.io/gorm"

// RegisterFinanceModels is the GoSaaS gorm_biz.go drop-in.
// Production finance tables use explicit SQL, not AutoMigrate.
func RegisterFinanceModels(_ *gorm.DB) error {
	return nil
}
