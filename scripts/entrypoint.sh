#!/bin/sh

set -eu

# 默认端口和数据目录（可用环境变量覆盖）
PORT="${PORT:-8090}"
DSN="${DSN:-/opt/dance-elite}"

# 确保数据目录及静态资源目录存在，并赋予 nonroot 用户读写权限
mkdir -p "${DSN}" "${DSN}/uploads"
chown -R nonroot:nonroot "${DSN}"

# 降权到 nonroot 运行应用
exec su-exec nonroot "$@" --port "${PORT}" --dsn "${DSN}"
