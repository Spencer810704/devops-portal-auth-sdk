package middleware

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger 統一的日誌中間件
// 提供結構化日誌記錄，包含請求ID、用戶信息等
func Logger(logger *zap.Logger) gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		fields := []zapcore.Field{
			zap.String("method", param.Method),
			zap.String("path", param.Path),
			zap.String("protocol", param.Request.Proto),
			zap.Int("status", param.StatusCode),
			zap.Duration("latency", param.Latency),
			zap.String("client_ip", param.ClientIP),
			zap.String("user_agent", param.Request.UserAgent()),
			zap.Time("timestamp", param.TimeStamp),
		}

		if param.ErrorMessage != "" {
			fields = append(fields, zap.String("error", param.ErrorMessage))
		}

		// Add request ID if available
		if requestID := param.Request.Header.Get("X-Request-ID"); requestID != "" {
			fields = append(fields, zap.String("request_id", requestID))
		}

		// Add user information if available
		if userID := param.Request.Header.Get("X-User-ID"); userID != "" {
			fields = append(fields, zap.String("user_id", userID))
		}

		// Log based on status code
		if param.StatusCode >= 500 {
			logger.Error("HTTP Request", fields...)
		} else if param.StatusCode >= 400 {
			logger.Warn("HTTP Request", fields...)
		} else {
			logger.Info("HTTP Request", fields...)
		}

		return ""
	})
}
