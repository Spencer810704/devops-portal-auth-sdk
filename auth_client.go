// Package auth 提供統一的身份驗證客戶端 SDK
// 供所有微服務使用，實現動態權限檢查、用戶狀態驗證等 Phase 1 功能
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// AuthClient 統一身份驗證客戶端介面
type AuthClient interface {
	// JWT 驗證
	ValidateToken(tokenString string) (*Claims, error)
	
	// 動態權限與安全檢查
	ValidateTokenWithDynamicAuth(ctx context.Context, tokenString string) (*AuthResult, error)
	CheckUserStatus(ctx context.Context, userID string) (bool, error)
	CheckForceLogout(ctx context.Context, userID string, tokenIssuedAt int64) (bool, error)
	GetUserDynamicPermissions(ctx context.Context, userID string) ([]string, error)
	
	// 管理功能
	SetUserStatus(ctx context.Context, userID string, isActive bool) error
	SetForceLogout(ctx context.Context, userID string) error
}

// Claims JWT 聲明結構
type Claims struct {
	UserID      string   `json:"user_id"`
	Username    string   `json:"username"`
	Email       string   `json:"email"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
	TokenType   string   `json:"token_type"`
	jwt.RegisteredClaims
}

// AuthResult 身份驗證結果
type AuthResult struct {
	Claims              *Claims  `json:"claims"`
	DynamicPermissions  []string `json:"dynamic_permissions"`
	IsActive            bool     `json:"is_active"`
	ShouldForceLogout   bool     `json:"should_force_logout"`
}

// UserStatus 用戶狀態結構
type UserStatus struct {
	IsActive  bool      `json:"is_active"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Config 客戶端配置
type Config struct {
	PublicKeyPath string        // JWT 公鑰路徑
	Issuer        string        // JWT 發行者
	RedisAddr     string        // Redis 地址
	RedisPassword string        // Redis 密碼
	RedisDB       int           // Redis 資料庫
	AuthServiceURL string       // Auth 服務 URL（備用）
	Logger        *zap.Logger   // 日誌記錄器
}

// Client 身份驗證客戶端實作
type Client struct {
	config     *Config
	publicKey  interface{}
	redisClient *redis.Client
	httpClient  *http.Client
	logger     *zap.Logger
}

// NewClient 建立新的身份驗證客戶端
func NewClient(config *Config) (*Client, error) {
	// 載入 JWT 公鑰
	publicKey, err := loadPublicKey(config.PublicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load public key: %w", err)
	}

	// 初始化 Redis 客戶端
	redisClient := redis.NewClient(&redis.Options{
		Addr:     config.RedisAddr,
		Password: config.RedisPassword,
		DB:       config.RedisDB,
	})

	// 測試 Redis 連接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if err := redisClient.Ping(ctx).Err(); err != nil {
		config.Logger.Warn("Redis connection failed, will use fallback methods", zap.Error(err))
	}

	return &Client{
		config:      config,
		publicKey:   publicKey,
		redisClient: redisClient,
		httpClient:  &http.Client{Timeout: 5 * time.Second},
		logger:      config.Logger,
	}, nil
}

// ValidateToken 驗證 JWT Token
func (c *Client) ValidateToken(tokenString string) (*Claims, error) {
	// 移除 Bearer 前綴
	tokenString = strings.TrimPrefix(tokenString, "Bearer ")

	// 解析 Token
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// 驗證簽名方法
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return c.publicKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	// 驗證 Token 有效性
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	// 驗證發行者
	if claims.Issuer != c.config.Issuer {
		return nil, fmt.Errorf("invalid token issuer")
	}

	return claims, nil
}

// ValidateTokenWithDynamicAuth 驗證 Token 並執行動態權限檢查
func (c *Client) ValidateTokenWithDynamicAuth(ctx context.Context, tokenString string) (*AuthResult, error) {
	// 1. 驗證 JWT Token
	claims, err := c.ValidateToken(tokenString)
	if err != nil {
		return nil, fmt.Errorf("token validation failed: %w", err)
	}

	result := &AuthResult{
		Claims: claims,
	}

	// 2. 檢查用戶狀態
	isActive, err := c.CheckUserStatus(ctx, claims.UserID)
	if err != nil {
		c.logger.Warn("Failed to check user status, defaulting to active", 
			zap.String("user_id", claims.UserID), zap.Error(err))
		isActive = true // 容錯：預設為啟用
	}
	result.IsActive = isActive

	if !isActive {
		return result, nil // 用戶已停用，不需要檢查其他項目
	}

	// 3. 檢查強制登出
	shouldForceLogout, err := c.CheckForceLogout(ctx, claims.UserID, claims.IssuedAt.Unix())
	if err != nil {
		c.logger.Warn("Failed to check force logout, defaulting to false", 
			zap.String("user_id", claims.UserID), zap.Error(err))
		shouldForceLogout = false // 容錯：預設不強制登出
	}
	result.ShouldForceLogout = shouldForceLogout

	// 4. 獲取動態權限
	dynamicPermissions, err := c.GetUserDynamicPermissions(ctx, claims.UserID)
	if err != nil {
		c.logger.Warn("Failed to get dynamic permissions, using JWT permissions", 
			zap.String("user_id", claims.UserID), zap.Error(err))
		dynamicPermissions = claims.Permissions // 容錯：使用 JWT 中的權限
	}
	result.DynamicPermissions = dynamicPermissions

	return result, nil
}

// CheckUserStatus 檢查用戶狀態
func (c *Client) CheckUserStatus(ctx context.Context, userID string) (bool, error) {
	key := fmt.Sprintf("user:status:%s", userID)
	
	val, err := c.redisClient.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return true, nil // 緩存不存在，預設為啟用
		}
		return true, err // 容錯：查詢失敗時允許通過
	}

	var status UserStatus
	if err := json.Unmarshal([]byte(val), &status); err != nil {
		return true, fmt.Errorf("failed to parse user status: %w", err)
	}

	return status.IsActive, nil
}

// CheckForceLogout 檢查強制登出標記
func (c *Client) CheckForceLogout(ctx context.Context, userID string, tokenIssuedAt int64) (bool, error) {
	key := fmt.Sprintf("user:force_logout:%s", userID)
	
	val, err := c.redisClient.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return false, nil // 沒有強制登出標記
		}
		return false, err
	}

	// 解析時間戳
	forceLogoutTimestamp, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return false, fmt.Errorf("failed to parse force logout timestamp: %w", err)
	}

	// 如果強制登出時間晚於 token 簽發時間，則需要重新登入
	return forceLogoutTimestamp > tokenIssuedAt, nil
}

// GetUserDynamicPermissions 獲取用戶的動態權限
func (c *Client) GetUserDynamicPermissions(ctx context.Context, userID string) ([]string, error) {
	key := fmt.Sprintf("user:dynamic_permissions:%s", userID)
	
	val, err := c.redisClient.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // 緩存不存在
		}
		return nil, err
	}

	var cacheData map[string]interface{}
	if err := json.Unmarshal([]byte(val), &cacheData); err != nil {
		return nil, fmt.Errorf("failed to parse cached permissions: %w", err)
	}

	// 解析權限列表
	permissionsData, ok := cacheData["permissions"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid permissions format in cache")
	}

	permissions := make([]string, len(permissionsData))
	for i, perm := range permissionsData {
		if permStr, ok := perm.(string); ok {
			permissions[i] = permStr
		}
	}

	return permissions, nil
}

// SetUserStatus 設置用戶狀態
func (c *Client) SetUserStatus(ctx context.Context, userID string, isActive bool) error {
	key := fmt.Sprintf("user:status:%s", userID)
	status := UserStatus{
		IsActive:  isActive,
		UpdatedAt: time.Now(),
	}

	data, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("failed to marshal user status: %w", err)
	}

	err = c.redisClient.Set(ctx, key, string(data), 10*time.Minute).Err()
	if err != nil {
		return fmt.Errorf("failed to set user status: %w", err)
	}

	return nil
}

// SetForceLogout 設置強制登出標記
func (c *Client) SetForceLogout(ctx context.Context, userID string) error {
	key := fmt.Sprintf("user:force_logout:%s", userID)
	timestamp := time.Now().Unix()

	err := c.redisClient.Set(ctx, key, timestamp, 24*time.Hour).Err()
	if err != nil {
		return fmt.Errorf("failed to set force logout: %w", err)
	}

	return nil
}

// Close 關閉客戶端連接
func (c *Client) Close() error {
	if c.redisClient != nil {
		return c.redisClient.Close()
	}
	return nil
}

// loadPublicKey 載入 RSA 公鑰
func loadPublicKey(path string) (interface{}, error) {
	// 這裡需要根據實際的公鑰格式實作
	// 為了簡化，這裡返回一個假的實作
	return nil, fmt.Errorf("loadPublicKey not implemented - please implement based on your key format")
}