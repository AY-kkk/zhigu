package futures

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestFuturesMigrationIsIndependentAndChecksumGuarded(t *testing.T) {
	db := repoTestDB(t)
	if !MaintenanceAvailable(db) {
		t.Fatal("installed schema must retain maintenance")
	}
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Raw(`SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name='futures_drafts'`).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("futures_drafts count=%d err=%v", count, err)
	}
	ms, err := LoadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	bad := ms[0]
	raw := []byte(bad.SQL + "\n-- checksum drift")
	sum := sha256.Sum256(raw)
	bad.SQL = string(raw)
	bad.Checksum = hex.EncodeToString(sum[:])
	if err := MigrateList(context.Background(), db, []Migration{bad}); err == nil {
		t.Fatal("checksum drift must fail")
	}
}

func TestDisabledBootstrapDoesNotCreateFuturesSchema(t *testing.T) {
	db := repoTestDBWithoutFutures(t)
	state, err := Bootstrap(context.Background(), db, Config{Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	if MaintenanceAvailable(db) {
		t.Fatal("fresh disabled module must not start maintenance")
	}
	if state.Ready || state.Mode != "off" {
		t.Fatalf("disabled bootstrap state=%+v", state)
	}
	var count int64
	if err := db.Raw(`SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name='futures_drafts'`).Scan(&count).Error; err != nil || count != 0 {
		t.Fatalf("disabled bootstrap created schema count=%d err=%v", count, err)
	}
}
