#!/bin/bash
# 構建 Shared Auth SDK Base Image

set -e

# 顏色定義
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

SDK_VERSION=${1:-"1.0.0"}
IMAGE_NAME="devops-portal/auth-sdk-base"
FULL_IMAGE_NAME="${IMAGE_NAME}:${SDK_VERSION}"
LATEST_IMAGE_NAME="${IMAGE_NAME}:latest"

echo -e "${BLUE}🏗️  Building Shared Auth SDK Base Image${NC}"
echo "Version: $SDK_VERSION"
echo "Image: $FULL_IMAGE_NAME"
echo ""

# 構建 Base Image
echo -e "${BLUE}Building base image...${NC}"
docker build -t "$FULL_IMAGE_NAME" -t "$LATEST_IMAGE_NAME" .

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✅ SDK Base Image built successfully!${NC}"
    echo ""
    echo "Images created:"
    echo "  - $FULL_IMAGE_NAME"
    echo "  - $LATEST_IMAGE_NAME" 
    echo ""
    
    # 顯示鏡像信息
    echo -e "${BLUE}Image details:${NC}"
    docker images | grep "devops-portal/auth-sdk-base"
    echo ""
    
    # 測試 image
    echo -e "${BLUE}Testing image...${NC}"
    if docker run --rm "$LATEST_IMAGE_NAME" ls -la /shared/auth-sdk/ > /dev/null 2>&1; then
        echo -e "${GREEN}✅ Image test passed - SDK files are accessible${NC}"
    else
        echo -e "${YELLOW}⚠️  Image test warning - please verify manually${NC}"
    fi
    
    echo ""
    echo -e "${GREEN}🎉 SDK Base Image is ready!${NC}"
    echo "Other microservices can now use: FROM $LATEST_IMAGE_NAME"
    
else
    echo -e "${RED}❌ Failed to build SDK Base Image${NC}"
    exit 1
fi