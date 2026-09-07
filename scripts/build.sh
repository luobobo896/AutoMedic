#!/usr/bin/env bash
# ============================================================
# AutoMedic 一键构建：后端 Go 二进制 + 前端静态产物
# 用法：./scripts/build.sh [--skip-web] [--skip-server]
# ============================================================
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

SKIP_WEB=0
SKIP_SERVER=0
for arg in "$@"; do
  case "$arg" in
    --skip-web)    SKIP_WEB=1 ;;
    --skip-server) SKIP_SERVER=1 ;;
    *) echo "未知参数: $arg"; exit 1 ;;
  esac
done

echo "==> 项目根目录: $ROOT"

# ---------- 后端 ----------
if [ "$SKIP_SERVER" -eq 0 ]; then
  echo "==> 构建后端 (Go)"
  mkdir -p server/bin
  (cd server && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o bin/automedic-server ./cmd/server)
  echo "    产物: server/bin/automedic-server"
fi

# ---------- 前端 ----------
if [ "$SKIP_WEB" -eq 0 ]; then
  echo "==> 构建前端 (Vite)"
  if [ ! -d web/node_modules ]; then
    echo "    未找到 web/node_modules，先执行安装…"
    (cd web && npm install --no-audit --no-fund)
  fi
  (cd web && npm run build)
  echo "    产物: web/dist"
fi

echo "==> 构建完成"
[ "$SKIP_SERVER" -eq 0 ] && ls -lh server/bin/automedic-server || true
[ "$SKIP_WEB" -eq 0 ]    && du -sh web/dist                     || true
