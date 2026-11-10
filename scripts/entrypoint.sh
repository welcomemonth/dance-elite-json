#!/bin/sh

set -eu

# 默认端口和数据目录（可用环境变量覆盖）
PORT="${PORT:-8090}"
DATA_DIR="${DATA_DIR:-/opt/dance-elite}"

# 确保 SQLite 数据目录存在，并赋予 nonroot 用户读写权限
mkdir -p "${DATA_DIR}"
chown -R nonroot:nonroot "${DATA_DIR}"

# 降权到 nonroot 运行应用
exec su-exec nonroot "$@"
