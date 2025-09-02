package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestID 統一的請求ID中間件
// 為每個請求生成唯一標識符，方便追蹤和除錯
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}
		
		c.Header("X-Request-ID", requestID)
		c.Set("request_id", requestID)
		c.Next()
	}
}

// generateRequestID 生成唯一請求ID
// 使用時間戳 + 隨機字符串的方式生成
func generateRequestID() string {
	timestamp := time.Now().Format("20060102150405")
	randomBytes := make([]byte, 4)
	rand.Read(randomBytes)
	randomHex := hex.EncodeToString(randomBytes)
	return timestamp + "-" + randomHex
}
