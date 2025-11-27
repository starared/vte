package handlers

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"vte/internal/auth"
	"vte/internal/database"
	"vte/internal/logger"
	"vte/internal/models"
)

func Login(c *gin.Context) {
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

	token, err := auth.GenerateToken(user.Username)
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

	db := database.DB()
	_, err = db.Exec("UPDATE users SET hashed_password = ? WHERE id = ?", hashed, user.ID)
	if err != nil {
		c.JSON(500, gin.H{"detail": "更新失败"})
		return
	}

	logger.Info(fmt.Sprintf("%s | 修改密码 | %s", c.ClientIP(), user.Username))
	c.JSON(200, gin.H{"message": "密码修改成功"})
}

func ChangeUsername(c *gin.Context) {
	var req models.ChangeUsernameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"detail": "无效的请求"})
		return
	}

	user := c.MustGet("user").(*models.User)
	db := database.DB()

	// 检查用户名是否已存在
	var count int
	db.QueryRow("SELECT COUNT(*) FROM users WHERE username = ? AND id != ?", req.NewUsername, user.ID).Scan(&count)
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

	newKey := auth.GenerateAPIKey()
	_, err := db.Exec("UPDATE users SET api_key = ? WHERE id = ?", newKey, user.ID)
	if err != nil {
		c.JSON(500, gin.H{"detail": "更新失败"})
		return
	}

	logger.Info(fmt.Sprintf("%s | 重新生成API Key | %s", c.ClientIP(), user.Username))
	c.JSON(200, gin.H{"api_key": newKey})
}
