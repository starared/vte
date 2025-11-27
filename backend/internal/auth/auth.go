package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"vte/internal/database"
	"vte/internal/models"
)

var secretKey string

func SetSecretKey(key string) {
	secretKey = key
}

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func GenerateAPIKey() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// fallback: 使用时间戳生成
		return hex.EncodeToString([]byte(time.Now().String()))[:64]
	}
	return hex.EncodeToString(b)
}

func GenerateToken(username string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": username,
		"exp": time.Now().Add(7 * 24 * time.Hour).Unix(),
	})
	return token.SignedString([]byte(secretKey))
}

func ParseToken(tokenString string) (string, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return []byte(secretKey), nil
	})
	if err != nil {
		return "", err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		if sub, ok := claims["sub"].(string); ok {
			return sub, nil
		}
	}
	return "", errors.New("invalid token")
}

func GetUserByUsername(username string) (*models.User, error) {
	db := database.DB()
	row := db.QueryRow(
		"SELECT id, username, hashed_password, api_key, is_admin, is_active FROM users WHERE username = ?",
		username,
	)

	var user models.User
	var isAdmin, isActive int
	err := row.Scan(&user.ID, &user.Username, &user.HashedPassword, &user.APIKey, &isAdmin, &isActive)
	if err != nil {
		return nil, err
	}
	user.IsAdmin = isAdmin == 1
	user.IsActive = isActive == 1
	return &user, nil
}

func GetUserByAPIKey(apiKey string) (*models.User, error) {
	db := database.DB()
	row := db.QueryRow(
		"SELECT id, username, hashed_password, api_key, is_admin, is_active FROM users WHERE api_key = ? AND is_active = 1",
		apiKey,
	)

	var user models.User
	var isAdmin, isActive int
	err := row.Scan(&user.ID, &user.Username, &user.HashedPassword, &user.APIKey, &isAdmin, &isActive)
	if err != nil {
		return nil, err
	}
	user.IsAdmin = isAdmin == 1
	user.IsActive = isActive == 1
	return &user, nil
}

// Middleware: JWT 认证（用于前端）
func JWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(401, gin.H{"detail": "缺少认证凭据"})
			c.Abort()
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		username, err := ParseToken(token)
		if err != nil {
			c.JSON(401, gin.H{"detail": "无效的认证凭据"})
			c.Abort()
			return
		}

		user, err := GetUserByUsername(username)
		if err != nil || !user.IsActive {
			c.JSON(401, gin.H{"detail": "用户不存在或已禁用"})
			c.Abort()
			return
		}

		c.Set("user", user)
		c.Next()
	}
}

// Middleware: API Key 认证（用于 OpenAI 兼容接口）
func APIKeyAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(401, gin.H{"detail": "缺少 API Key"})
			c.Abort()
			return
		}

		apiKey := strings.TrimPrefix(authHeader, "Bearer ")
		user, err := GetUserByAPIKey(apiKey)
		if err != nil {
			c.JSON(401, gin.H{"detail": "无效的 API Key"})
			c.Abort()
			return
		}

		c.Set("user", user)
		c.Next()
	}
}

// Middleware: 要求管理员权限
func AdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := c.Get("user")
		if !exists {
			c.JSON(401, gin.H{"detail": "未认证"})
			c.Abort()
			return
		}

		if u, ok := user.(*models.User); !ok || !u.IsAdmin {
			c.JSON(403, gin.H{"detail": "需要管理员权限"})
			c.Abort()
			return
		}

		c.Next()
	}
}
