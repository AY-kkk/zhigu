package httpx

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

func JWTSecret() []byte {
	s := os.Getenv("ZHIGU_JWT_SECRET")
	if s == "" {
		s = "zhigu-dev-only-not-for-production"
	}
	return []byte(s)
}

func SignToken(userID uint, username, role string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:   userID,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(12 * time.Hour)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(JWTSecret())
}

func TokenFromRequest(c *gin.Context) string {
	if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	if t := c.GetHeader("x-token"); t != "" {
		return t
	}
	return ""
}

func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := TokenFromRequest(c)
		if raw == "" {
			Fail(c, http.StatusUnauthorized, "UNAUTHENTICATED", "未登录")
			c.Abort()
			return
		}
		claims := &Claims{}
		tok, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
			return JWTSecret(), nil
		})
		if err != nil || !tok.Valid {
			Fail(c, http.StatusUnauthorized, "UNAUTHENTICATED", "未登录或令牌无效")
			c.Abort()
			return
		}
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Next()
	}
}

func AdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get("role")
		if role != "admin" {
			Fail(c, http.StatusForbidden, "FORBIDDEN", "无管理员权限")
			c.Abort()
			return
		}
		c.Next()
	}
}

func CurrentUserID(c *gin.Context) uint {
	v, _ := c.Get("user_id")
	id, _ := v.(uint)
	return id
}
