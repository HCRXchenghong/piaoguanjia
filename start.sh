#!/usr/bin/env bash
# 票管家 · Linux / macOS 启动脚本
set -euo pipefail

cd "$(dirname "$0")"

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
esac

BIN="server/bin/invoice-server-linux-${ARCH}"
if [ ! -f "$BIN" ]; then
  if [ -x "server/bin/invoice-server.exe" ]; then
    echo "[!] 没有找到 Linux 版可执行文件，请先运行 deploy/build-linux.sh 编译"
  else
    echo "[!] 缺少可执行文件：$BIN"
  fi
  exit 1
fi

DATA_DIR="${INVOICE_DATA_DIR:-server/data}"
PORT="${INVOICE_PORT:-8686}"
HOST="${INVOICE_ADDR:-:${PORT}}"

# 检查 MySQL 是否在监听
if command -v ss >/dev/null 2>&1; then
  if ! ss -ltn 2>/dev/null | grep -q ':3306 '; then
    echo "[!] 3306 端口没有监听，请先启动 MySQL：sudo systemctl start mysql"
    exit 1
  fi
fi

if [ ! -f "web/dist/index.html" ]; then
  echo "[!] web/dist 不存在，请先运行 deploy/build-linux.sh 构建前端"
fi

echo "  数据目录：$DATA_DIR"
echo "  监听地址：$HOST"
echo "  访问地址：http://127.0.0.1:${PORT}"
echo

exec "$BIN" -addr "$HOST" -web web/dist -data "$DATA_DIR"
