package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strconv"
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

func GenerateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// 不能退回到时间戳之类可预测的值
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// TokenClaims 登录令牌中解析出的信息
type TokenClaims struct {
	UserID   int    // 新令牌使用用户 ID，修改用户名后令牌依然有效
	Username string // 旧版本签发的令牌只有用户名
	IssuedAt int64  // 签发时间（Unix 秒），0 表示旧令牌没有该字段
}

func GenerateToken(user *models.User) (string, error) {
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": strconv.Itoa(user.ID),
		"uid": user.ID,
		"iat": now.Unix(),
		"exp": now.Add(7 * 24 * time.Hour).Unix(),
	})
	return token.SignedString([]byte(secretKey))
}

func ParseToken(tokenString string) (*TokenClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return []byte(secretKey), nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	result := &TokenClaims{}
	if iat, ok := claims["iat"].(float64); ok {
		result.IssuedAt = int64(iat)
	}
	if uid, ok := claims["uid"].(float64); ok && uid > 0 {
		result.UserID = int(uid)
		return result, nil
	}
	// 兼容旧版本令牌：sub 为用户名
	if sub, ok := claims["sub"].(string); ok && sub != "" {
		result.Username = sub
		return result, nil
	}
	return nil, errors.New("invalid token")
}

const userColumns = "id, username, hashed_password, api_key, is_admin, is_active, COALESCE(password_changed_at, 0)"

func scanUser(row *sql.Row) (*models.User, error) {
	var user models.User
	var isAdmin, isActive int
	err := row.Scan(&user.ID, &user.Username, &user.HashedPassword, &user.APIKey, &isAdmin, &isActive, &user.PasswordChangedAt)
	if err != nil {
		return nil, err
	}
	user.IsAdmin = isAdmin == 1
	user.IsActive = isActive == 1
	return &user, nil
}

func GetUserByID(id int) (*models.User, error) {
	return scanUser(database.DB().QueryRow("SELECT "+userColumns+" FROM users WHERE id = ?", id))
}

func GetUserByUsername(username string) (*models.User, error) {
	return scanUser(database.DB().QueryRow("SELECT "+userColumns+" FROM users WHERE username = ?", username))
}

func GetUserByAPIKey(apiKey string) (*models.User, error) {
	return scanUser(database.DB().QueryRow("SELECT "+userColumns+" FROM users WHERE api_key = ? AND is_active = 1", apiKey))
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
		claims, err := ParseToken(token)
		if err != nil {
			c.JSON(401, gin.H{"detail": "无效的认证凭据"})
			c.Abort()
			return
		}

		var user *models.User
		if claims.UserID > 0 {
			user, err = GetUserByID(claims.UserID)
		} else {
			user, err = GetUserByUsername(claims.Username)
		}
		if err != nil || !user.IsActive {
			c.JSON(401, gin.H{"detail": "用户不存在或已禁用"})
			c.Abort()
			return
		}

		// 修改密码后，之前签发的令牌全部失效
		if user.PasswordChangedAt > 0 && claims.IssuedAt < user.PasswordChangedAt {
			c.JSON(401, gin.H{"detail": "密码已修改，请重新登录"})
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
			c.JSON(401, gin.H{"error": gin.H{"message": "缺少 API Key", "type": "authentication_error"}, "detail": "缺少 API Key"})
			c.Abort()
			return
		}

		apiKey := strings.TrimPrefix(authHeader, "Bearer ")
		if user, err := GetUserByAPIKey(apiKey); err == nil {
			c.Set("user", user)
			c.Next()
			return
		}

		// 尝试临时 API Key
		if tempKey, err := database.GetTempAPIKeyByToken(apiKey); err == nil {
			if !tempKey.IsActive {
				c.JSON(401, gin.H{"error": gin.H{"message": "API Key 已禁用", "type": "authentication_error"}, "detail": "API Key 已禁用"})
				c.Abort()
				return
			}
			if tempKey.ExpiresAt != nil && time.Now().After(*tempKey.ExpiresAt) {
				c.JSON(401, gin.H{"error": gin.H{"message": "API Key 已过期", "type": "authentication_error"}, "detail": "API Key 已过期"})
				c.Abort()
				return
			}
			if tempKey.MaxRequests > 0 && tempKey.UsedRequests >= tempKey.MaxRequests {
				c.JSON(429, gin.H{"error": gin.H{"message": "API Key 请求次数已用尽", "type": "authentication_error"}, "detail": "API Key 请求次数已用尽"})
				c.Abort()
				return
			}

			c.Set("temp_api_key", tempKey)
			c.Next()
			return
		}

		c.JSON(401, gin.H{"error": gin.H{"message": "无效的 API Key", "type": "authentication_error"}, "detail": "无效的 API Key"})
		c.Abort()
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
