package httpx

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	model "zhigu/server/model/finance"
)

const SessionCookieName = "__Host-zhigu_session"
const SessionAbsoluteTTL = 12 * time.Hour
const SessionIdleTTL = 30 * time.Minute
const maxPasswordBytes = 1024

type AuthConfig struct {
	DB           *gorm.DB
	Production   bool
	PublicOrigin string
}

var authState struct {
	sync.RWMutex
	config AuthConfig
}

// ConfigureAuth is called once at startup, after validated runtime configuration.
func ConfigureAuth(config AuthConfig) {
	authState.Lock()
	authState.config = config
	authState.Unlock()
}
func currentAuthConfig() AuthConfig {
	authState.RLock()
	config := authState.config
	authState.RUnlock()
	return config
}
func ProductionAuth() bool {
	return currentAuthConfig().Production || os.Getenv("ZHIGU_ENV") == "production" || os.Getenv("ZHIGU_DEPLOYMENT_ENVIRONMENT") == "production"
}

func secretToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", errors.New("secure random unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
func digest(raw string) string { sum := sha256.Sum256([]byte(raw)); return hex.EncodeToString(sum[:]) }

func credentialDigest(user model.User) string {
	v2 := ""
	if user.PasswordHashV2 != nil {
		v2 = *user.PasswordHashV2
	}
	return digest(user.PasswordHash + "\x00" + v2)
}

func CreateSession(db *gorm.DB, user model.User) (string, error) {
	if db == nil || user.DisabledAt != nil {
		return "", errors.New("session unavailable")
	}
	token, err := secretToken()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	session := model.Session{JTI: uuid.NewString(), UserID: user.ID, TokenDigest: digest(token), PasswordDigest: credentialDigest(user), CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(SessionAbsoluteTTL)}
	// Serialize issuance with host password/disable updates, whose trigger
	// revokes sessions. Credentials verified before the transaction may be stale.
	if err := db.Transaction(func(tx *gorm.DB) error {
		var current model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", user.ID).Take(&current).Error; err != nil {
			return err
		}
		if current.DisabledAt != nil || subtle.ConstantTimeCompare([]byte(credentialDigest(current)), []byte(session.PasswordDigest)) != 1 {
			return errors.New("credentials changed")
		}
		return tx.Create(&session).Error
	}); err != nil {
		return "", errors.New("session could not be persisted")
	}
	return token, nil
}

func SetSessionCookie(c *gin.Context, token string) {
	http.SetCookie(c.Writer, &http.Cookie{Name: SessionCookieName, Value: token, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: int(SessionAbsoluteTTL.Seconds()), Expires: time.Now().Add(SessionAbsoluteTTL)})
	c.Header("Cache-Control", "no-store")
}

func authenticateSession(c *gin.Context) bool {
	config := currentAuthConfig()
	if config.DB == nil {
		return false
	}
	token, err := c.Cookie(SessionCookieName)
	if err != nil || len(token) != 43 {
		return false
	}
	now := time.Now().UTC()
	var session model.Session
	if err := config.DB.WithContext(c.Request.Context()).Where("token_digest = ? AND revoked_at IS NULL AND expires_at > ? AND last_seen_at > ?", digest(token), now, now.Add(-SessionIdleTTL)).Take(&session).Error; err != nil {
		return false
	}
	var user model.User
	if err := config.DB.WithContext(c.Request.Context()).Where("id = ?", session.UserID).Take(&user).Error; err != nil {
		return false
	}
	if user.DisabledAt != nil {
		_ = RevokeUserSessions(config.DB, user.ID)
		return false
	}
	if subtle.ConstantTimeCompare([]byte(session.PasswordDigest), []byte(credentialDigest(user))) != 1 {
		_ = RevokeUserSessions(config.DB, user.ID)
		return false
	}
	result := config.DB.WithContext(c.Request.Context()).Model(&model.Session{}).Where("jti = ? AND revoked_at IS NULL AND expires_at > ? AND last_seen_at > ?", session.JTI, now, now.Add(-SessionIdleTTL)).Update("last_seen_at", now)
	if result.Error != nil || result.RowsAffected != 1 {
		return false
	}
	c.Set("user_id", user.ID)
	c.Set("username", user.Username)
	c.Set("role", user.Role)
	c.Set("browser_session", session)
	return true
}

func validRequestOrigin(c *gin.Context, public string) bool {
	if public == "" {
		return false
	}
	if origin := c.GetHeader("Origin"); origin != "" {
		return origin == public
	}
	referer, err := url.Parse(c.GetHeader("Referer"))
	if err != nil || referer.Scheme == "" || referer.Host == "" || referer.User != nil {
		return false
	}
	return referer.Scheme+"://"+referer.Host == public
}

// LoginOriginAllowed prevents login CSRF before there is a session-bound token.
func LoginOriginAllowed(c *gin.Context) bool {
	return validRequestOrigin(c, currentAuthConfig().PublicOrigin)
}

func validateCSRF(c *gin.Context) bool {
	config := currentAuthConfig()
	value, exists := c.Get("browser_session")
	if !exists {
		return false
	}
	session, ok := value.(model.Session)
	if !ok {
		return false
	}
	raw := c.GetHeader("X-CSRF-Token")
	return len(raw) == 43 && session.CSRFDigest != nil && validRequestOrigin(c, config.PublicOrigin) && subtle.ConstantTimeCompare([]byte(*session.CSRFDigest), []byte(digest(raw))) == 1
}

func SessionCSRF(c *gin.Context) {
	value, exists := c.Get("browser_session")
	session, ok := value.(model.Session)
	if !exists || !ok {
		Fail(c, 401, "UNAUTHENTICATED", "未登录")
		return
	}
	raw, err := secretToken()
	if err != nil {
		Fail(c, 500, "INTERNAL", "会话暂不可用")
		return
	}
	config := currentAuthConfig()
	result := config.DB.WithContext(c.Request.Context()).Model(&model.Session{}).Where("jti = ? AND revoked_at IS NULL AND expires_at > ?", session.JTI, time.Now()).Update("csrf_digest", digest(raw))
	if result.Error != nil || result.RowsAffected != 1 {
		Fail(c, 401, "UNAUTHENTICATED", "会话已失效")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("X-CSRF-Token", raw)
	OK(c, 200, gin.H{"csrf_token": raw})
}

func Logout(c *gin.Context) {
	if value, ok := c.Get("browser_session"); ok {
		session := value.(model.Session)
		config := currentAuthConfig()
		if err := config.DB.WithContext(c.Request.Context()).Model(&model.Session{}).Where("jti = ?", session.JTI).Update("revoked_at", time.Now().UTC()).Error; err != nil {
			Fail(c, 500, "INTERNAL", "会话暂不可用")
			return
		}
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: SessionCookieName, Value: "", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0)})
	c.Header("Cache-Control", "no-store")
	OK(c, 200, gin.H{"logged_out": true})
}

func RevokeUserSessions(db *gorm.DB, userID uint) error {
	if db == nil {
		return errors.New("session unavailable")
	}
	if err := db.Model(&model.Session{}).Where("user_id = ? AND revoked_at IS NULL", userID).Update("revoked_at", time.Now().UTC()).Error; err != nil {
		return errors.New("session revocation failed")
	}
	return nil
}

// HashPassword never creates bcrypt hashes. Password length is measured in bytes.
func HashPassword(password string) (string, error) {
	if len(password) < 16 || len(password) > maxPasswordBytes {
		return "", errors.New("password length must be 16 to 1024 bytes")
	}
	return RehashPassword(password)
}

// RehashPassword is for verified legacy credentials only, never new owners.
func RehashPassword(password string) (string, error) {
	if len(password) == 0 || len(password) > maxPasswordBytes {
		return "", errors.New("invalid password length")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", errors.New("password hashing unavailable")
	}
	key := argon2.IDKey([]byte(password), salt, 2, 19*1024, 1, 32)
	return "$argon2id$v=19$m=19456,t=2,p=1$" + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(key), nil
}

// VerifyPassword accepts legacy bcrypt for development compatibility and asks
// the caller to replace it before issuing a session. Production callers must
// additionally require canonical bcrypt cost 12.
func VerifyPassword(hash, password string) (bool, bool) {
	if len(password) == 0 || len(password) > maxPasswordBytes {
		return false, false
	}
	if strings.HasPrefix(hash, "$2") {
		cost, err := bcrypt.Cost([]byte(hash))
		if err != nil || cost > 16 || len(password) > 72 {
			return false, false
		}
		return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil, true
	}
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false, false
	}
	var memory, iterations, parallelism uint32
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil || memory < 8 || memory > 64*1024 || iterations < 1 || iterations > 4 || parallelism < 1 || parallelism > 4 || memory < 8*parallelism {
		return false, false
	}
	if parts[3] != fmt.Sprintf("m=%d,t=%d,p=%d", memory, iterations, parallelism) {
		return false, false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 16 || len(salt) > 64 {
		return false, false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) != 32 {
		return false, false
	}
	key := argon2.IDKey([]byte(password), salt, iterations, memory, uint8(parallelism), 32)
	return subtle.ConstantTimeCompare(key, expected) == 1, memory < 19*1024 || iterations < 2
}

// Fixed dummy avoids a first-request hash-generation penalty. It is not a
// credential: verification against it can never authenticate an account.
const dummyPasswordHash = "$2a$12$XajjQvNhvvRt5GSeFk1xFeyqRrsxkhBkUiQeg0dt.wU1qD4aFDcga"

// CanonicalProductionBcrypt bounds production legacy work to one cost class.
// Other costs require an offline reset/migration before production login.
func CanonicalProductionBcrypt(hash string) bool {
	if len(hash) != 60 || (!strings.HasPrefix(hash, "$2a$12$") && !strings.HasPrefix(hash, "$2b$12$") && !strings.HasPrefix(hash, "$2y$12$")) {
		return false
	}
	for _, character := range hash[7:] {
		if !strings.ContainsRune("./ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789", character) {
			return false
		}
	}
	// bcrypt's unpadded base64 has unused low bits in the last salt/hash
	// characters. Reject aliases with nonzero padding bits.
	return strings.ContainsRune(".Oeu", rune(hash[28])) && strings.ContainsRune(".CGKOSWaeimquy26", rune(hash[59]))
}

// CanonicalProductionArgon2id recognizes the bounded hashes written by the
// parent release. It permits only exact parameters and canonical base64.
func CanonicalProductionArgon2id(hash string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" {
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
	if err != nil || len(salt) < 16 || len(salt) > 64 || base64.RawStdEncoding.EncodeToString(salt) != parts[4] {
		return false
	}
	key, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	return err == nil && len(key) == 32 && base64.RawStdEncoding.EncodeToString(key) == parts[5]
}

func VerifyLoginPassword(user *model.User, password string, production bool) (bool, bool) {
	return verifyLoginPassword(user, password, production, VerifyPassword)
}

func verifyLoginPassword(user *model.User, password string, production bool, verify func(string, string) (bool, bool)) (bool, bool) {
	hash, eligible := dummyPasswordHash, user != nil
	if user != nil {
		if user.PasswordHashV2 != nil && *user.PasswordHashV2 != "" {
			hash = *user.PasswordHashV2
			if !strings.HasPrefix(hash, "$argon2id$") {
				hash, eligible = dummyPasswordHash, false
			}
		} else {
			hash = user.PasswordHash
			if production && !CanonicalProductionBcrypt(hash) && !CanonicalProductionArgon2id(hash) {
				hash, eligible = dummyPasswordHash, false
			}
		}
	}
	// bcrypt accepts at most 72 bytes. Overlong legacy input must still pay
	// the dummy cost, while never authenticating via truncation.
	if strings.HasPrefix(hash, "$2") && len(password) > 72 {
		hash, password, eligible = dummyPasswordHash, password[:72], false
	}
	valid, rehash := verify(hash, password)
	return eligible && valid, eligible && rehash
}

func BurnPasswordVerification(password string) {
	_, _ = VerifyLoginPassword(nil, password, true)
}

func NormalizeAccount(account string) string { return strings.ToLower(strings.TrimSpace(account)) }

type loginBucket struct {
	attempts          int
	started, timeNext time.Time
}
type LoginLimiter struct {
	sync.Mutex
	buckets map[string]loginBucket
}

func NewLoginLimiter() *LoginLimiter { return &LoginLimiter{buckets: make(map[string]loginBucket)} }

// Allow bounds both the memory footprint and retry work; entries expire in 15m.
func (limiter *LoginLimiter) Allow(ip, account string, now time.Time) time.Duration {
	limiter.Lock()
	defer limiter.Unlock()
	key := digest(ip + "\x00" + NormalizeAccount(account))
	bucket, exists := limiter.buckets[key]
	if !exists && len(limiter.buckets) >= 4096 {
		for key, b := range limiter.buckets {
			if now.Sub(b.started) >= 15*time.Minute {
				delete(limiter.buckets, key)
			}
		}
		if len(limiter.buckets) >= 4096 {
			return time.Minute
		}
	}
	if !exists || now.Sub(bucket.started) >= 15*time.Minute {
		bucket = loginBucket{started: now}
	}
	if now.Before(bucket.timeNext) {
		return bucket.timeNext.Sub(now)
	}
	bucket.attempts++
	if bucket.attempts > 5 {
		exponent := bucket.attempts - 5
		if exponent > 6 {
			exponent = 6
		}
		delay := time.Duration(1<<exponent) * time.Second
		bucket.timeNext = now.Add(delay)
		limiter.buckets[key] = bucket
		if bucket.attempts == 6 {
			return delay
		}
		return 0
	}
	limiter.buckets[key] = bucket
	return 0
}

func LoginRetryAfter(c *gin.Context, delay time.Duration) {
	seconds := int(delay.Seconds()) + 1
	c.Header("Retry-After", strconv.Itoa(seconds))
	Fail(c, 429, "RATE_LIMITED", "登录尝试过于频繁，请稍后重试")
}
