#!/bin/sh

set -eu

VERSION="${1:-}"

# 阿里云镜像仓库配置
# 公网地址（默认）
# 如果在阿里云 ECS/VPC 内网环境，可以设置环境变量使用 VPC 地址：
#   ALIYUN_REGISTRY=crpi-npw0av7ozhtwtzou-vpc.cn-hangzhou.personal.cr.aliyuncs.com ./push-aliyun.sh 1.0.0
REGISTRY="${ALIYUN_REGISTRY:-crpi-npw0av7ozhtwtzou.cn-hangzhou.personal.cr.aliyuncs.com}"
NAMESPACE="memos_meetmonth"
REPO="dance-elite-json"
FULL_IMAGE="${REGISTRY}/${NAMESPACE}/${REPO}"

if [ -z "$VERSION" ]; then
  echo "Usage: $0 <version>"
  echo "Example: $0 1.0.0"
  echo ""
  echo "Environment variables:"
  echo "  ALIYUN_REGISTRY  可选，默认使用公网地址。"
  echo "                   在阿里云 ECS/VPC 环境中可设为 VPC 内网地址以节省公网流量:"
  echo "                   export ALIYUN_REGISTRY=crpi-npw0av7ozhtwtzou-vpc.cn-hangzhou.personal.cr.aliyuncs.com"
  exit 1
fi

# 切换到项目根目录（脚本位于 scripts/ 下）
cd "$(dirname "$0")/../"

# 检查 Dockerfile 是否存在
if [ ! -f "scripts/Dockerfile" ]; then
  echo "Error: scripts/Dockerfile not found"
  exit 1
fi

echo "======================================"
echo "Version: ${VERSION}"
echo "Target:  ${FULL_IMAGE}:${VERSION}"
echo "======================================"

echo ""
echo "=== [1/3] Building Docker image ==="

# 构建镜像，同时打上本地标签
docker build \
  -f scripts/Dockerfile \
  -t "${REPO}:${VERSION}" \
  -t "${REPO}:latest" \
  .

echo ""
echo "=== [2/3] Tagging for Aliyun Registry ==="

# 给镜像打上阿里云仓库的完整标签
docker tag "${REPO}:${VERSION}" "${FULL_IMAGE}:${VERSION}"
docker tag "${REPO}:${VERSION}" "${FULL_IMAGE}:latest"

echo ""
echo "=== [3/3] Pushing to Aliyun Registry ==="

# 推送到阿里云（假设已经完成 docker login）
docker push "${FULL_IMAGE}:${VERSION}"
docker push "${FULL_IMAGE}:latest"

echo ""
echo "======================================"
echo "Done! Image pushed successfully:"
echo "  ${FULL_IMAGE}:${VERSION}"
echo "  ${FULL_IMAGE}:latest"
echo "======================================"
