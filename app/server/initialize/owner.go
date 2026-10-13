package initialize

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
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
				if httpx.CanonicalProductionBcrypt(user.PasswordHash) {
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
					if !httpx.CanonicalProductionBcrypt(user.PasswordHash) || (user.PasswordHashV2 != nil && (!strings.HasPrefix(*user.PasswordHashV2, "$argon2id$") || !validOwnerHash(*user.PasswordHashV2))) {
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
	if strings.HasPrefix(hash, "$2") {
		return httpx.CanonicalProductionBcrypt(hash)
	}
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	var memory, iterations, parallelism uint32
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil || memory < 19*1024 || memory > 64*1024 || iterations < 2 || iterations > 4 || parallelism < 1 || parallelism > 4 {
		return false
	}
	if parts[3] != fmt.Sprintf("m=%d,t=%d,p=%d", memory, iterations, parallelism) {
		return false
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil || len(salt) < 16 || len(salt) > 64 {
		return false
	}
	key, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	return err == nil && len(key) == 32
}

func hashOwnerPassword(password string) (string, error) {
	return httpx.HashPassword(password)
}
