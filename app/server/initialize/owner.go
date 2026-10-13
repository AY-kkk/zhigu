package initialize

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"zhigu/server/httpx"
	modelfinance "zhigu/server/model/finance"
	"zhigu/server/runtime"
)

// EnsureOwner serializes bootstrap with an exclusive table lock. The credential
// file is read only if no owner exists; an existing owner with a mounted file is
// an operator error, never a request to reset a password.
func EnsureOwner(db *gorm.DB, cfg runtime.Config) error {
	if cfg.DeploymentEnvironment != runtime.Production && cfg.BootstrapOwnerFile == "" {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("LOCK TABLE finance_users IN EXCLUSIVE MODE").Error; err != nil {
			return errors.New("owner startup database check failed")
		}
		var users []modelfinance.User
		if err := tx.Find(&users).Error; err != nil {
			return errors.New("owner startup database check failed")
		}
		owners := 0
		for _, user := range users {
			if cfg.DeploymentEnvironment == runtime.Production {
				if defaultUsername(user.Username) {
					return errors.New("production database contains a default account or password")
				}
				defaultPassword := false
				if validOwnerHash(user.PasswordHash) {
					defaultPassword, _ = httpx.VerifyPassword(user.PasswordHash, "Passw0rd!")
				}
				if user.PasswordHashV2 != nil && strings.HasPrefix(*user.PasswordHashV2, "$argon2id$") && validOwnerHash(*user.PasswordHashV2) {
					v2Default, _ := httpx.VerifyPassword(*user.PasswordHashV2, "Passw0rd!")
					defaultPassword = defaultPassword || v2Default
				}
				if defaultPassword {
					return errors.New("production database contains a default account or password")
				}
			}
			if user.Role == "admin" {
				if cfg.DeploymentEnvironment == runtime.Production {
					legacyValid := httpx.CanonicalProductionBcrypt(user.PasswordHash)
					// Task 1 and the first session implementation wrote Argon2id
					// into the primary column. The first successful login normalizes
					// this exact, canonical NULL-v2 transition into dual hashes.
					primaryTransition := user.PasswordHashV2 == nil && httpx.CanonicalProductionArgon2id(user.PasswordHash)
					if (!legacyValid && !primaryTransition) || (user.PasswordHashV2 != nil && !httpx.CanonicalProductionArgon2id(*user.PasswordHashV2)) {
						return errors.New("owner password hash does not meet production policy")
					}
				}
				owners++
			}
		}
		if owners > 1 {
			return errors.New("production requires exactly one owner")
		}
		if owners == 1 {
			if cfg.BootstrapOwnerFile != "" {
				return errors.New("owner already exists; remove bootstrap credential")
			}
			return nil
		}
		if cfg.BootstrapOwnerFile == "" {
			return errors.New("production requires an owner or one-time bootstrap credential")
		}
		credentials, err := readOwnerCredential(cfg.BootstrapOwnerFile)
		if err != nil {
			return err
		}
		hash, err := hashOwnerPassword(credentials.Password)
		if err != nil {
			return err
		}
		legacy, err := bcrypt.GenerateFromPassword([]byte(credentials.Password), 12)
		if err != nil {
			return errors.New("owner compatibility password hashing failed")
		}
		now := time.Now().UTC()
		owner := modelfinance.User{Username: credentials.Username, PasswordHash: string(legacy), PasswordHashV2: &hash, Role: "admin", CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&owner).Error; err != nil {
			return errors.New("owner bootstrap could not be persisted")
		}
		return nil
	})
}

type ownerCredential struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func readOwnerCredential(path string) (ownerCredential, error) {
	var credential ownerCredential
	f, err := os.Open(path)
	if err != nil {
		return credential, errors.New("owner bootstrap credential cannot be read")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16384 {
		return credential, errors.New("invalid owner bootstrap credential")
	}
	decoder := json.NewDecoder(io.LimitReader(f, 16385))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&credential); err != nil {
		return credential, errors.New("invalid owner bootstrap credential")
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return credential, errors.New("invalid owner bootstrap credential")
	}
	credential.Username = strings.TrimSpace(credential.Username)
	if len(credential.Username) < 3 || len(credential.Username) > 128 || defaultUsername(credential.Username) || len(credential.Password) < 16 || len(credential.Password) > 72 || strings.Contains(strings.ToLower(credential.Password), "passw0rd") || strings.Contains(strings.ToLower(credential.Password), "change-me") {
		return credential, errors.New("owner bootstrap requires a non-default account and strong password")
	}
	return credential, nil
}

func defaultUsername(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "admin", "invitee", "invitee-b":
		return true
	}
	return false
}

func validOwnerHash(hash string) bool {
	return httpx.CanonicalProductionBcrypt(hash) || httpx.CanonicalProductionArgon2id(hash)
}

func hashOwnerPassword(password string) (string, error) {
	return httpx.HashPassword(password)
}
