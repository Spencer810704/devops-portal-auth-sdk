package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Recovery 統一的恢復中間件
// 提供統一的 panic 處理和結構化日誌記錄
func Recovery(logger *zap.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		// 記錄 panic 信息
		logger.Error("Panic recovered",
			zap.Any("error", recovered),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("client_ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
			zap.String("stack", string(debug.Stack())),
		)

		// Add request ID if available
		if requestID := c.GetString("request_id"); requestID != "" {
			logger.Error("Panic context", zap.String("request_id", requestID))
		}

		// 返回統一错誤響應
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":     "Internal server error",
			"message":   "An unexpected error occurred",
			"timestamp": gin.H{},
		})
	})
}
