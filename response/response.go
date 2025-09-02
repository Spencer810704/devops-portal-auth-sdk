package response

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// APIResponse 統一的 API 響應格式
type APIResponse struct {
	Success   bool        `json:"success"`
	Data      interface{} `json:"data,omitempty"`
	Error     *ErrorInfo  `json:"error,omitempty"`
	Message   string      `json:"message,omitempty"`
	Timestamp int64       `json:"timestamp"`
	RequestID string      `json:"request_id,omitempty"`
}

// ErrorInfo 错誤信息結構
type ErrorInfo struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

// Success 返回成功響應
func Success(c *gin.Context, data interface{}, message ...string) {
	response := APIResponse{
		Success:   true,
		Data:      data,
		Timestamp: time.Now().Unix(),
		RequestID: getRequestID(c),
	}

	if len(message) > 0 {
		response.Message = message[0]
	}

	c.JSON(http.StatusOK, response)
}

// Error 返回错誤響應
func Error(c *gin.Context, statusCode int, code, message string, details ...interface{}) {
	errorInfo := &ErrorInfo{
		Code:    code,
		Message: message,
	}

	if len(details) > 0 {
		errorInfo.Details = details[0]
	}

	response := APIResponse{
		Success:   false,
		Error:     errorInfo,
		Timestamp: time.Now().Unix(),
		RequestID: getRequestID(c),
	}

	c.JSON(statusCode, response)
}

// BadRequest 返回 400 错誤
func BadRequest(c *gin.Context, message string, details ...interface{}) {
	Error(c, http.StatusBadRequest, "BAD_REQUEST", message, details...)
}

// Unauthorized 返回 401 错誤
func Unauthorized(c *gin.Context, message string, details ...interface{}) {
	Error(c, http.StatusUnauthorized, "UNAUTHORIZED", message, details...)
}

// Forbidden 返回 403 错誤
func Forbidden(c *gin.Context, message string, details ...interface{}) {
	Error(c, http.StatusForbidden, "FORBIDDEN", message, details...)
}

// NotFound 返回 404 错誤
func NotFound(c *gin.Context, message string, details ...interface{}) {
	Error(c, http.StatusNotFound, "NOT_FOUND", message, details...)
}

// InternalServerError 返回 500 错誤
func InternalServerError(c *gin.Context, message string, details ...interface{}) {
	Error(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", message, details...)
}

// getRequestID 獲取請求ID
func getRequestID(c *gin.Context) string {
	if requestID, exists := c.Get("request_id"); exists {
		if id, ok := requestID.(string); ok {
			return id
		}
	}
	return c.GetHeader("X-Request-ID")
}
