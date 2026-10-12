package initialize

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	modelfinance "zhigu/server/model/finance"
	"zhigu/server/runtime"
)

func TestOwnerStartupChecks(t *testing.T) {
	db := testDB(t)
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	cfg := runtime.Config{State: runtime.State{DeploymentEnvironment: runtime.Production}}
	if err := EnsureOwner(db, cfg); err == nil {
		t.Fatal("production without owner/bootstrap accepted")
	}
	path := filepath.Join(t.TempDir(), "owner.json")
	password := "my-unique-owner-password-2026!"
	if err := os.WriteFile(path, []byte(`{"username":"research-owner","password":"`+password+`"}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapOwnerFile = path
	if err := EnsureOwner(db, cfg); err != nil {
		t.Fatal(err)
	}
	var owner modelfinance.User
	if err := db.First(&owner).Error; err != nil {
		t.Fatal(err)
	}
	if owner.Username != "research-owner" || owner.Role != "admin" || !strings.HasPrefix(owner.PasswordHash, "$argon2id$v=19$m=19456,t=2,p=1$") || strings.Contains(owner.PasswordHash, password) {
		t.Fatal("owner credentials not safely stored")
	}
	if err := EnsureOwner(db, cfg); err == nil {
		t.Fatal("repeated bootstrap accepted")
	}
	cfg.BootstrapOwnerFile = ""
	if err := EnsureOwner(db, cfg); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.Model(&modelfinance.User{}).Count(&count)
	if count != 1 {
		t.Fatalf("owner count %d", count)
	}
}

func TestOwnerInvalidBootstrapFailsClosed(t *testing.T) {
	for _, body := range []string{`{}`, `{"username":"admin","password":"long-enough-owner-password"}`, `{"username":"owner","password":"Passw0rd!"}`, `{"username":"owner","password":"long-enough-owner-password","extra":true}`, `{"username":"owner","password":"long-enough-owner-password"} {}`} {
		db := testDB(t)
		if err := Migrate(db); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "owner")
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if err := EnsureOwner(db, runtime.Config{State: runtime.State{DeploymentEnvironment: runtime.Production}, BootstrapOwnerFile: path}); err == nil || strings.Contains(err.Error(), body) || strings.Contains(err.Error(), path) {
			t.Fatalf("unsafe bootstrap result: %v", err)
		}
		var count int64
		db.Model(&modelfinance.User{}).Count(&count)
		if count != 0 {
			t.Fatal("invalid bootstrap wrote user")
		}
	}
}

func TestProductionRejectsExistingDefaultUsersAndSeed(t *testing.T) {
	db := testDB(t)
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&modelfinance.User{Username: "admin", PasswordHash: "legacy", Role: "admin"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := EnsureOwner(db, runtime.Config{State: runtime.State{DeploymentEnvironment: runtime.Production}}); err == nil {
		t.Fatal("existing demo account accepted")
	}
	t.Setenv("ZHIGU_ENV", "production")
	if err := Seed(db); err == nil {
		t.Fatal("production demo seed accepted")
	}
}

func TestProductionRejectsOwnerWithInvalidPasswordHash(t *testing.T) {
	db := testDB(t)
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&modelfinance.User{Username: "research-owner", PasswordHash: "legacy-weak-hash", Role: "admin"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := EnsureOwner(db, runtime.Config{State: runtime.State{DeploymentEnvironment: runtime.Production}}); err == nil {
		t.Fatal("owner with invalid password hash accepted")
	}
}
