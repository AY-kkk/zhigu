package initialize

import (
	"golang.org/x/crypto/bcrypt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"zhigu/server/httpx"

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
	if owner.Username != "research-owner" || owner.Role != "admin" || bcrypt.CompareHashAndPassword([]byte(owner.PasswordHash), []byte(password)) != nil || strings.Contains(owner.PasswordHash, password) {
		t.Fatal("owner credentials not safely stored")
	}
	var v2 string
	if err := db.Raw("SELECT password_hash_v2 FROM finance_users WHERE id = ?", owner.ID).Scan(&v2).Error; err != nil {
		t.Fatal(err)
	}
	if ok, rehash := httpx.VerifyPassword(v2, password); !ok || rehash {
		t.Fatal("new owner lacks compatible Argon2id v2 credentials")
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
	for _, body := range []string{`{}`, `{"username":"admin","password":"long-enough-owner-password"}`, `{"username":"owner","password":"Passw0rd!"}`, `{"username":"owner","password":"long-enough-owner-password","extra":true}`, `{"username":"owner","password":"long-enough-owner-password"} {}`, `{"username":"owner","password":"` + strings.Repeat("x", 73) + `"}`} {
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

func TestOwnerExistingLegacyBcryptCanReachLoginUpgrade(t *testing.T) {
	db := testDB(t)
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("legacy-owner-credential"), 12)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&modelfinance.User{Username: "research-owner", PasswordHash: string(hash), Role: "admin"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := EnsureOwner(db, runtime.Config{State: runtime.State{DeploymentEnvironment: runtime.Production}}); err != nil {
		t.Fatalf("legacy owner cannot reach rehash login: %v", err)
	}
	newHash, err := hashOwnerPassword("my-unique-owner-password-2026!")
	if err != nil {
		t.Fatal(err)
	}
	if ok, rehash := httpx.VerifyPassword(newHash, "my-unique-owner-password-2026!"); !ok || rehash {
		t.Fatalf("bootstrap password incompatible: %v %v", ok, rehash)
	}
}

func TestProductionRejectsWeakLegacyBcryptOwner(t *testing.T) {
	db := testDB(t)
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("legacy-owner-credential"), 10)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&modelfinance.User{Username: "research-owner", PasswordHash: string(hash), Role: "admin"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := EnsureOwner(db, runtime.Config{State: runtime.State{DeploymentEnvironment: runtime.Production}}); err == nil {
		t.Fatal("weak bcrypt owner accepted in production")
	}
}

func TestProductionRejectsDefaultPasswordRehashedToArgon2id(t *testing.T) {
	db := testDB(t)
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	hash, err := httpx.RehashPassword("Passw0rd!")
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := bcrypt.GenerateFromPassword([]byte("valid-owner-password"), 12)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&modelfinance.User{Username: "research-owner", PasswordHash: string(legacy), PasswordHashV2: &hash, Role: "admin"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := EnsureOwner(db, runtime.Config{State: runtime.State{DeploymentEnvironment: runtime.Production}}); err == nil {
		t.Fatal("Argon2id default password accepted in production")
	}
}

func TestProductionRejectsMalformedArgon2idOwnerHash(t *testing.T) {
	db := testDB(t)
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	hash, err := httpx.HashPassword("actual-valid-owner-password")
	if err != nil {
		t.Fatal(err)
	}
	hash = strings.Replace(hash, "p=1$", "p=1,trailing=1$", 1)
	legacy, err := bcrypt.GenerateFromPassword([]byte("actual-valid-owner-password"), 12)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&modelfinance.User{Username: "research-owner", PasswordHash: string(legacy), PasswordHashV2: &hash, Role: "admin"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := EnsureOwner(db, runtime.Config{State: runtime.State{DeploymentEnvironment: runtime.Production}}); err == nil {
		t.Fatal("malformed Argon2id owner accepted")
	}
}

func TestProductionRejectsLegacyDefaultPasswordEvenWithValidV2(t *testing.T) {
	db := testDB(t)
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	legacy, err := bcrypt.GenerateFromPassword([]byte("Passw0rd!"), 12)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := httpx.HashPassword("actual-valid-owner-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&modelfinance.User{Username: "research-owner", PasswordHash: string(legacy), PasswordHashV2: &v2, Role: "admin"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := EnsureOwner(db, runtime.Config{State: runtime.State{DeploymentEnvironment: runtime.Production}}); err == nil {
		t.Fatal("default legacy credential accepted despite valid v2")
	}
}

func TestProductionRejectsCost13LegacyOwnerBeforeExpensiveVerification(t *testing.T) {
	db := testDB(t)
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	legacy := "$2a$13$XajjQvNhvvRt5GSeFk1xFeyqRrsxkhBkUiQeg0dt.wU1qD4aFDcga"
	if err := db.Create(&modelfinance.User{Username: "research-owner", PasswordHash: legacy, Role: "admin"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := EnsureOwner(db, runtime.Config{State: runtime.State{DeploymentEnvironment: runtime.Production}}); err == nil {
		t.Fatal("cost13 legacy owner accepted in production")
	}
}

func TestProductionRejectsDefaultOrMalformedPrimaryArgonTransition(t *testing.T) {
	defaultHash, err := httpx.RehashPassword("Passw0rd!")
	if err != nil {
		t.Fatal(err)
	}
	validHash, err := httpx.HashPassword("parent-version-owner-password")
	if err != nil {
		t.Fatal(err)
	}
	for _, hash := range []string{defaultHash, strings.Replace(validHash, "p=1$", "p=1,trailing=1$", 1)} {
		db := testDB(t)
		if err := Migrate(db); err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&modelfinance.User{Username: "research-owner", PasswordHash: hash, Role: "admin"}).Error; err != nil {
			t.Fatal(err)
		}
		if err := EnsureOwner(db, runtime.Config{State: runtime.State{DeploymentEnvironment: runtime.Production}}); err == nil {
			t.Fatal("default or malformed primary Argon2id transition accepted")
		}
	}
}
