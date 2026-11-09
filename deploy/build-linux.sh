#!/usr/bin/env bash
# 在 Linux 上编译后端并构建前端
set -euo pipefail

cd "$(dirname "$0")/.."
ROOT="$(pwd)"
GO="${GO:-go}"

echo "==> 编译后端（linux/amd64）"
cd "$ROOT/server"
if [ ! -d bin ]; then mkdir -p bin; fi
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 "$GO" build -o bin/invoice-server-linux-amd64 ./cmd/server
echo "    -> server/bin/invoice-server-linux-amd64"

echo "==> 构建前端"
cd "$ROOT/web"
if [ ! -d node_modules ]; then
  npm install --registry=https://registry.npmmirror.com --no-audit --no-fund
fi
npm run build
echo "    -> web/dist"

echo
echo "完成。运行 ./start.sh 启动服务。"
