package handlers

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"vte/internal/auth"
	"vte/internal/database"
	"vte/internal/logger"
	"vte/internal/models"
)

// minPasswordLength 修改密码时新密码的最短长度
const minPasswordLength = 8

func Login(c *gin.Context) {
	if !allowLogin(c.ClientIP()) {
		c.Header("Retry-After", "60")
		c.JSON(429, gin.H{"detail": "登录尝试过多，请一分钟后重试"})
		return
	}
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"detail": "无效的请求"})
		return
	}

	user, err := auth.GetUserByUsername(req.Username)
	if err != nil || !auth.CheckPassword(req.Password, user.HashedPassword) {
		logger.Warn(fmt.Sprintf("%s | 登录失败 | %s", c.ClientIP(), req.Username))
		c.JSON(401, gin.H{"detail": "用户名或密码错误"})
		return
	}

	if !user.IsActive {
		logger.Warn(fmt.Sprintf("%s | 账户禁用 | %s", c.ClientIP(), req.Username))
		c.JSON(403, gin.H{"detail": "账户已禁用"})
		return
	}

	token, err := auth.GenerateToken(user)
	if err != nil {
		c.JSON(500, gin.H{"detail": "生成令牌失败"})
		return
	}

	logger.Info(fmt.Sprintf("%s | 登录成功 | %s", c.ClientIP(), req.Username))
	c.JSON(200, models.TokenResponse{
		AccessToken: token,
		TokenType:   "bearer",
	})
}

func GetMe(c *gin.Context) {
	user := c.MustGet("user").(*models.User)
	c.JSON(200, gin.H{
		"id":       user.ID,
		"username": user.Username,
		"api_key":  user.APIKey,
		"is_admin": user.IsAdmin,
	})
}

func ChangePassword(c *gin.Context) {
	var req models.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"detail": "无效的请求"})
		return
	}

	if utf8.RuneCountInString(req.NewPassword) < minPasswordLength {
		c.JSON(400, gin.H{"detail": fmt.Sprintf("新密码至少需要 %d 位", minPasswordLength)})
		return
	}

	user := c.MustGet("user").(*models.User)

	if !auth.CheckPassword(req.OldPassword, user.HashedPassword) {
		logger.Warn(fmt.Sprintf("%s | 修改密码失败 | %s", c.ClientIP(), user.Username))
		c.JSON(400, gin.H{"detail": "原密码错误"})
		return
	}

	hashed, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		c.JSON(500, gin.H{"detail": "密码加密失败"})
		return
	}

	// 记录修改时间：早于该时间签发的令牌（包括其他设备上的登录）全部失效
	changedAt := time.Now().Unix()
	db := database.DB()
	_, err = db.Exec("UPDATE users SET hashed_password = ?, password_changed_at = ? WHERE id = ?", hashed, changedAt, user.ID)
	if err != nil {
		c.JSON(500, gin.H{"detail": "更新失败"})
		return
	}
	user.PasswordChangedAt = changedAt

	// 给当前会话签发新令牌，避免修改密码后自己也被登出
	token, err := auth.GenerateToken(user)
	if err != nil {
		c.JSON(500, gin.H{"detail": "生成令牌失败"})
		return
	}

	logger.Info(fmt.Sprintf("%s | 修改密码 | %s", c.ClientIP(), user.Username))
	c.JSON(200, gin.H{"message": "密码修改成功", "access_token": token, "token_type": "bearer"})
}

func ChangeUsername(c *gin.Context) {
	var req models.ChangeUsernameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"detail": "无效的请求"})
		return
	}

	req.NewUsername = strings.TrimSpace(req.NewUsername)
	if req.NewUsername == "" {
		c.JSON(400, gin.H{"detail": "用户名不能为空"})
		return
	}

	user := c.MustGet("user").(*models.User)
	db := database.DB()

	// 检查用户名是否已存在
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM users WHERE username = ? AND id != ?", req.NewUsername, user.ID).Scan(&count); err != nil {
		c.JSON(500, gin.H{"detail": "查询失败"})
		return
	}
	if count > 0 {
		c.JSON(400, gin.H{"detail": "用户名已存在"})
		return
	}

	oldUsername := user.Username
	_, err := db.Exec("UPDATE users SET username = ? WHERE id = ?", req.NewUsername, user.ID)
	if err != nil {
		c.JSON(500, gin.H{"detail": "更新失败"})
		return
	}

	logger.Info(fmt.Sprintf("%s | 修改用户名 | %s -> %s", c.ClientIP(), oldUsername, req.NewUsername))
	c.JSON(200, gin.H{"message": "用户名修改成功"})
}

func RegenerateAPIKey(c *gin.Context) {
	user := c.MustGet("user").(*models.User)
	db := database.DB()

	newKey, err := auth.GenerateAPIKey()
	if err != nil {
		c.JSON(500, gin.H{"detail": "生成密钥失败"})
		return
	}
	_, err = db.Exec("UPDATE users SET api_key = ? WHERE id = ?", newKey, user.ID)
	if err != nil {
		c.JSON(500, gin.H{"detail": "更新失败"})
		return
	}

	logger.Info(fmt.Sprintf("%s | 重新生成API Key | %s", c.ClientIP(), user.Username))
	c.JSON(200, gin.H{"api_key": newKey})
}

var loginAttempts = struct {
	sync.Mutex
	times map[string][]time.Time
}{times: map[string][]time.Time{}}

func allowLogin(ip string) bool {
	loginAttempts.Lock()
	defer loginAttempts.Unlock()
	now := time.Now()
	cutoff := now.Add(-time.Minute)
	for key, times := range loginAttempts.times {
		if len(times) == 0 || times[len(times)-1].Before(cutoff) {
			delete(loginAttempts.times, key)
		}
	}
	kept := loginAttempts.times[ip][:0]
	for _, at := range loginAttempts.times[ip] {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	loginAttempts.times[ip] = kept
	if len(kept) >= 10 {
		return false
	}
	loginAttempts.times[ip] = append(kept, now)
	return true
}
