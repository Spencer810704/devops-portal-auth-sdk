// Package auth 提供 Gin 框架的身份驗證中介軟體
// 統一處理動態權限檢查、用戶狀態驗證等功能
package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// GinMiddleware Gin 框架的身份驗證中介軟體
type GinMiddleware struct {
	authClient AuthClient
	logger     *zap.Logger
}

// NewGinMiddleware 建立新的 Gin 中介軟體
func NewGinMiddleware(authClient AuthClient, logger *zap.Logger) *GinMiddleware {
	return &GinMiddleware{
		authClient: authClient,
		logger:     logger,
	}
}

// ErrorResponse 統一錯誤回應格式
type ErrorResponse struct {
	Success bool   `json:"success"`
	Code    int    `json:"code"`
	Message string `json:"message"`
	Error   string `json:"error"`
}

// Authenticate 身份驗證中介軟體（使用動態權限檢查）
func (m *GinMiddleware) Authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 獲取 Authorization 標頭
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			m.respondUnauthorized(c, "Missing authorization header")
			c.Abort()
			return
		}

		// 2. 檢查 Bearer 格式
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			m.respondUnauthorized(c, "Invalid authorization header format")
			c.Abort()
			return
		}

		// 3. 執行完整的動態身份驗證
		authResult, err := m.authClient.ValidateTokenWithDynamicAuth(c.Request.Context(), tokenString)
		if err != nil {
			m.logger.Debug("Token validation failed", 
				zap.Error(err),
				zap.String("token_prefix", tokenString[:min(len(tokenString), 20)]))
			m.respondUnauthorized(c, "Invalid or expired token")
			c.Abort()
			return
		}

		// 4. 檢查用戶是否啟用
		if !authResult.IsActive {
			m.respondForbidden(c, "User account is disabled")
			c.Abort()
			return
		}

		// 5. 檢查是否需要強制登出
		if authResult.ShouldForceLogout {
			m.respondUnauthorized(c, "Please login again")
			c.Abort()
			return
		}

		// 6. 設置用戶上下文（使用動態權限）
		claims := authResult.Claims
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("email", claims.Email)
		c.Set("roles", claims.Roles)
		c.Set("permissions", authResult.DynamicPermissions) // 使用動態權限
		c.Set("token_id", claims.ID)

		// 7. 記錄成功驗證
		m.logger.Debug("User authenticated successfully",
			zap.String("user_id", claims.UserID),
			zap.String("username", claims.Username),
			zap.Int("permission_count", len(authResult.DynamicPermissions)))

		c.Next()
	}
}

// RequirePermission 需要特定權限的中介軟體
func (m *GinMiddleware) RequirePermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		permissions, exists := c.Get("permissions")
		if !exists {
			m.respondForbidden(c, "No permissions found")
			c.Abort()
			return
		}

		userPermissions, ok := permissions.([]string)
		if !ok {
			m.respondForbidden(c, "Invalid permissions format")
			c.Abort()
			return
		}

		// 檢查權限
		hasPermission := m.checkPermission(userPermissions, permission)
		if !hasPermission {
			m.logger.Info("Permission denied",
				zap.String("user_id", m.getUserID(c)),
				zap.String("required_permission", permission),
				zap.Strings("user_permissions", userPermissions))
			
			m.respondForbidden(c, "Insufficient permissions: required '"+permission+"'")
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireAnyPermission 需要任一權限的中介軟體
func (m *GinMiddleware) RequireAnyPermission(permissions ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userPermissions, exists := c.Get("permissions")
		if !exists {
			m.respondForbidden(c, "No permissions found")
			c.Abort()
			return
		}

		userPerms, ok := userPermissions.([]string)
		if !ok {
			m.respondForbidden(c, "Invalid permissions format")
			c.Abort()
			return
		}

		// 檢查是否有任一權限
		hasPermission := false
		for _, requiredPerm := range permissions {
			if m.checkPermission(userPerms, requiredPerm) {
				hasPermission = true
				break
			}
		}

		if !hasPermission {
			m.logger.Info("Permission denied",
				zap.String("user_id", m.getUserID(c)),
				zap.Strings("required_permissions", permissions),
				zap.Strings("user_permissions", userPerms))
			
			m.respondForbidden(c, "Insufficient permissions: required one of ["+strings.Join(permissions, ", ")+"]")
			c.Abort()
			return
		}

		c.Next()
	}
}

// OptionalAuth 可選身份驗證（如果有 token 則驗證，但不強制要求）
func (m *GinMiddleware) OptionalAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			// 沒有提供 token，繼續處理
			c.Next()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			// token 格式無效，繼續處理但不設置用戶上下文
			c.Next()
			return
		}

		// 嘗試驗證 token
		authResult, err := m.authClient.ValidateTokenWithDynamicAuth(c.Request.Context(), tokenString)
		if err == nil && authResult.IsActive && !authResult.ShouldForceLogout {
			// token 有效且用戶啟用，設置用戶上下文
			claims := authResult.Claims
			c.Set("user_id", claims.UserID)
			c.Set("username", claims.Username)
			c.Set("email", claims.Email)
			c.Set("roles", claims.Roles)
			c.Set("permissions", authResult.DynamicPermissions)
			c.Set("token_id", claims.ID)
		}

		c.Next()
	}
}

// checkPermission 檢查用戶是否擁有指定權限
func (m *GinMiddleware) checkPermission(userPermissions []string, requiredPermission string) bool {
	for _, perm := range userPermissions {
		// 檢查完全匹配
		if perm == requiredPermission {
			return true
		}
		
		// 檢查萬用字元權限
		if perm == "*" || perm == "*:*:*" {
			return true
		}
		
		// 檢查部分萬用字元匹配
		if m.matchesWildcardPermission(perm, requiredPermission) {
			return true
		}
	}
	return false
}

// matchesWildcardPermission 檢查萬用字元權限匹配
func (m *GinMiddleware) matchesWildcardPermission(userPerm, requiredPerm string) bool {
	if !strings.Contains(userPerm, "*") {
		return false
	}

	userParts := strings.Split(userPerm, ":")
	requiredParts := strings.Split(requiredPerm, ":")

	if len(userParts) != len(requiredParts) {
		return false
	}

	for i, userPart := range userParts {
		if userPart != "*" && userPart != requiredParts[i] {
			return false
		}
	}

	return true
}

// 響應方法
func (m *GinMiddleware) respondUnauthorized(c *gin.Context, message string) {
	c.JSON(http.StatusUnauthorized, ErrorResponse{
		Success: false,
		Code:    http.StatusUnauthorized,
		Message: message,
		Error:   "UNAUTHORIZED",
	})
}

func (m *GinMiddleware) respondForbidden(c *gin.Context, message string) {
	c.JSON(http.StatusForbidden, ErrorResponse{
		Success: false,
		Code:    http.StatusForbidden,
		Message: message,
		Error:   "FORBIDDEN",
	})
}

// 輔助方法
func (m *GinMiddleware) getUserID(c *gin.Context) string {
	if userID, exists := c.Get("user_id"); exists {
		if userIDStr, ok := userID.(string); ok {
			return userIDStr
		}
	}
	return "unknown"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}