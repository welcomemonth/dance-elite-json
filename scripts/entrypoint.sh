#!/bin/sh

set -eu

# 密钥通过容器运行时环境注入（仓库根目录 .env.example 为模板），
# 镜像与仓库中不存放任何真实密钥。后端启动自检会在缺失关键项时 fail-fast，
# 并一次性列出缺失的环境变量名。
#
# 默认端口和数据目录（可用环境变量覆盖）
PORT="${PORT:-8090}"
DATA_DIR="${DATA_DIR:-/opt/dance-elite}"
export PORT DATA_DIR

# 确保 SQLite 数据目录存在，并赋予 nonroot 用户读写权限
mkdir -p "${DATA_DIR}"
chown -R nonroot:nonroot "${DATA_DIR}"

# 降权到 nonroot 运行应用
exec su-exec nonroot "$@"
