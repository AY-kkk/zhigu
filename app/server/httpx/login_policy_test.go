package httpx

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
	model "zhigu/server/model/finance"
)

func TestProductionLegacyVerificationUsesOnlyCost12Work(t *testing.T) {
	for _, tc := range []struct {
		name string
		user *model.User
	}{
		{"unknown", nil},
		{"wrong cost12", &model.User{PasswordHash: dummyPasswordHash}},
		{"cost13", &model.User{PasswordHash: strings.Replace(dummyPasswordHash, "$12$", "$13$", 1)}},
		{"cost10", &model.User{PasswordHash: strings.Replace(dummyPasswordHash, "$12$", "$10$", 1)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			valid, _ := verifyLoginPassword(tc.user, "wrong-password", true, func(hash, password string) (bool, bool) {
				calls++
				if cost, err := bcrypt.Cost([]byte(hash)); err != nil || cost != 12 {
					t.Fatalf("wrong work class: cost=%d err=%v", cost, err)
				}
				if tc.name == "cost13" && hash == tc.user.PasswordHash {
					t.Fatal("cost13 reached expensive comparison")
				}
				return false, false
			})
			if calls != 1 || valid {
				t.Fatalf("wrong verification result: calls=%d valid=%v", calls, valid)
			}
		})
	}
	// Even a dummy match can never authenticate an unknown/unsupported account.
	for _, user := range []*model.User{nil, {PasswordHash: strings.Replace(dummyPasswordHash, "$12$", "$13$", 1)}} {
		if valid, _ := verifyLoginPassword(user, "password", true, func(string, string) (bool, bool) { return true, true }); valid {
			t.Fatal("dummy match authenticated ineligible credentials")
		}
	}
}

func TestProductionV2NeverFallsBackToLegacyPassword(t *testing.T) {
	password := "argon-owner-password"
	v2, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	user := model.User{PasswordHash: dummyPasswordHash, PasswordHashV2: &v2}
	if valid, _ := VerifyLoginPassword(&user, password, true); !valid {
		t.Fatal("v2 Argon2id was not preferred")
	}
	malformed := "malformed-v2"
	user.PasswordHashV2 = &malformed
	if valid, _ := verifyLoginPassword(&user, password, true, func(hash, password string) (bool, bool) {
		if hash == user.PasswordHash && hash != dummyPasswordHash {
			t.Fatal("malformed v2 fell back to legacy hash")
		}
		return true, true
	}); valid {
		t.Fatal("malformed v2 permitted fallback authentication")
	}
}

func TestProductionBcryptRejectsNonCanonicalEncoding(t *testing.T) {
	if !CanonicalProductionBcrypt(dummyPasswordHash) {
		t.Fatal("canonical bcrypt12 rejected")
	}
	for _, hash := range []string{
		dummyPasswordHash[:28] + "f" + dummyPasswordHash[29:],
		dummyPasswordHash[:59] + "b",
		dummyPasswordHash + "trailing",
	} {
		if CanonicalProductionBcrypt(hash) {
			t.Fatal("noncanonical bcrypt accepted")
		}
	}
}

func TestProductionPrimaryArgonRequiresCanonicalTransitionHash(t *testing.T) {
	password := "parent-version-owner-password"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	user := model.User{PasswordHash: hash}
	if valid, _ := VerifyLoginPassword(&user, password, true); !valid {
		t.Fatal("canonical parent-version primary Argon2id rejected")
	}
	parts := strings.Split(hash, "$")
	noncanonicalSalt := strings.Replace(hash, parts[4], parts[4]+"\n", 1)
	for _, bad := range []string{
		strings.Replace(hash, "m=19456", "m=19456,trailing=1", 1),
		strings.Replace(hash, "t=2", "t=1", 1),
		noncanonicalSalt,
	} {
		user.PasswordHash = bad
		if valid, _ := verifyLoginPassword(&user, password, true, func(string, string) (bool, bool) { return true, false }); valid {
			t.Fatal("noncanonical primary Argon2id reached authentication")
		}
	}
}
