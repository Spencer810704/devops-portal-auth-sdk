# Shared Auth SDK 整合指南

## 🔗 如何在其他微服務中引用 shared-auth-sdk

### 方案一：Go Modules 本地路徑（開發推薦）

#### 1. 在微服務中添加依賴

```bash
# 在各微服務目錄下執行（如 cdn-mgmt-svc）
cd cdn-mgmt-svc
go mod edit -require=shared-auth-sdk@v0.0.0-00010101000000-000000000000
go mod edit -replace=shared-auth-sdk=../shared-auth-sdk
go mod tidy
```

#### 2. 更新 go.mod 文件
```go
// cdn-mgmt-svc/go.mod
module cdn-mgmt-svc

go 1.21

require (
    shared-auth-sdk v0.0.0-00010101000000-000000000000
    // ... 其他依賴
)

replace shared-auth-sdk => ../shared-auth-sdk
```

#### 3. 在代碼中使用
```go
import (
    "shared-auth-sdk"
    "github.com/gin-gonic/gin"
    "go.uber.org/zap"
)

func main() {
    // 初始化 auth 客戶端
    authConfig := &auth.Config{
        PublicKeyPath:  "keys/public_key.pem",
        Issuer:         "auth-service", 
        RedisAddr:      "localhost:6379",
        RedisPassword:  "",
        RedisDB:        0,
        Logger:         logger,
    }
    
    authClient, err := auth.NewClient(authConfig)
    if err != nil {
        panic(err)
    }
    defer authClient.Close()
    
    // 使用 Gin 中介軟體
    authMiddleware := auth.NewGinMiddleware(authClient, logger)
    
    r := gin.New()
    r.Use(authMiddleware.Authenticate())
    
    // ... 設置路由
}
```

---

### 方案二：Git Submodule

#### 1. 將 shared-auth-sdk 作為 git submodule

```bash
# 在項目根目錄
cd devops-portal-microservices
git submodule add https://github.com/your-org/shared-auth-sdk.git shared-auth-sdk

# 在各微服務中引用
cd cdn-mgmt-svc  
go mod edit -require=shared-auth-sdk@v0.0.0-00010101000000-000000000000
go mod edit -replace=shared-auth-sdk=../shared-auth-sdk
```

#### 2. 其他開發者獲取代碼
```bash
git clone --recursive https://github.com/your-org/devops-portal-microservices.git
# 或
git clone https://github.com/your-org/devops-portal-microservices.git
git submodule init
git submodule update
```

---

### 方案三：內部 Go Module Registry

#### 1. 推送 SDK 到內部 Git 倉庫
```bash
cd shared-auth-sdk
git init
git add .
git commit -m "Initial shared auth SDK"
git remote add origin https://github.com/your-org/shared-auth-sdk.git
git push -u origin main
```

#### 2. 在微服務中引用
```bash
cd cdn-mgmt-svc
go get github.com/your-org/shared-auth-sdk@latest
```

```go
// cdn-mgmt-svc/go.mod
require (
    github.com/your-org/shared-auth-sdk v1.0.0
)
```

---

### 方案四：本地 Vendor 方式

#### 1. 複製 SDK 到各微服務
```bash
cd cdn-mgmt-svc
mkdir -p vendor/shared-auth-sdk  
cp -r ../shared-auth-sdk/* vendor/shared-auth-sdk/

# 修改 import 路徑
import "cdn-mgmt-svc/vendor/shared-auth-sdk"
```

---

## 🚀 **推薦實作步驟（方案一）**

### Step 1: 完善 shared-auth-sdk

```bash
cd shared-auth-sdk

# 確保 go.mod 正確
cat > go.mod << 'EOF'
module shared-auth-sdk

go 1.21

require (
    github.com/gin-gonic/gin v1.9.1
    github.com/golang-jwt/jwt/v5 v5.2.0  
    github.com/redis/go-redis/v9 v9.3.0
    go.uber.org/zap v1.26.0
)
EOF

# 初始化依賴
go mod tidy
```

### Step 2: 修復 loadPublicKey 函數

```go
// shared-auth-sdk/auth_client.go
// 替換 loadPublicKey 函數

import (
    "crypto/rsa"
    "crypto/x509"
    "encoding/pem"
    "fmt"
    "os"
)

func loadPublicKey(path string) (*rsa.PublicKey, error) {
    keyData, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("failed to read public key file: %w", err)
    }

    block, _ := pem.Decode(keyData)
    if block == nil {
        return nil, fmt.Errorf("failed to parse PEM block")
    }

    pub, err := x509.ParsePKIXPublicKey(block.Bytes)
    if err != nil {
        return nil, fmt.Errorf("failed to parse public key: %w", err)
    }

    publicKey, ok := pub.(*rsa.PublicKey)
    if !ok {
        return nil, fmt.Errorf("not an RSA public key")
    }

    return publicKey, nil
}
```

### Step 3: 升級 cdn-mgmt-svc

```bash
cd cdn-mgmt-svc

# 添加依賴
go mod edit -require=shared-auth-sdk@v0.0.0-00010101000000-000000000000
go mod edit -replace=shared-auth-sdk=../shared-auth-sdk

# 下載依賴
go mod tidy
```

### Step 4: 修改 cdn-mgmt-svc 代碼

```go
// cmd/api/main.go
import (
    // ... 現有 imports
    sharedAuth "shared-auth-sdk"
)

func main() {
    // ... 現有初始化代碼

    // 替換原有的 auth middleware
    authConfig := &sharedAuth.Config{
        PublicKeyPath:  cfg.JWT.PublicKeyPath,
        Issuer:         cfg.JWT.Issuer,
        RedisAddr:      fmt.Sprintf("%s:%s", cfg.Redis.Host, cfg.Redis.Port),
        RedisPassword:  cfg.Redis.Password,
        RedisDB:        cfg.Redis.DB,
        Logger:         logging.GetLogger(),
    }

    authClient, err := sharedAuth.NewClient(authConfig)
    if err != nil {
        logging.Fatal("Failed to initialize auth client", zap.Error(err))
    }
    defer authClient.Close()

    authMiddleware := sharedAuth.NewGinMiddleware(authClient, logging.GetLogger())

    // 替換路由中的 middleware
    r := routes.NewRouter(cfg, cdnHandler, operationHandler, authMiddleware)
}
```

### Step 5: 測試整合

```bash
cd cdn-mgmt-svc

# 檢查編譯
go build ./cmd/api

# 運行服務
go run ./cmd/api/main.go
```

---

## 🔧 **故障排除**

### 常見錯誤 1: "module not found"
```bash
# 確保路徑正確
ls -la ../shared-auth-sdk

# 重新整理依賴
go mod tidy
go clean -modcache
```

### 常見錯誤 2: "ambiguous import"
```go
// 使用別名避免衝突
import sharedAuth "shared-auth-sdk"
```

### 常見錯誤 3: "undefined: loadPublicKey"
確保 `shared-auth-sdk/auth_client.go` 中的 `loadPublicKey` 函數已正確實作。

---

## 🏗️ **目錄結構**

最終的項目結構應該是：
```
devops-portal-microservices/
├── shared-auth-sdk/          # 共享 SDK
│   ├── go.mod
│   ├── auth_client.go
│   ├── gin_middleware.go
│   └── README.md
├── cdn-mgmt-svc/            # CDN 微服務
│   ├── go.mod               # 引用 shared-auth-sdk
│   └── cmd/api/main.go      # 使用 sharedAuth
├── domain-mgmt-svc/         # 域名微服務
│   ├── go.mod               # 引用 shared-auth-sdk  
│   └── ...
└── auth-mgmt-svc/           # 認證服務
    └── ...
```

---

## ✅ **驗證步驟**

1. **編譯測試**：
```bash
cd cdn-mgmt-svc && go build ./cmd/api
cd domain-mgmt-svc && go build ./cmd/api
```

2. **運行測試**：
```bash
go run ./cmd/api/main.go
curl http://localhost:8083/api/v1/health
```

3. **功能測試**：
使用之前的測試腳本驗證動態權限是否正常工作。

---

這樣您就可以在所有微服務中使用統一的身份驗證 SDK 了！需要我幫您實際執行某個步驟嗎？