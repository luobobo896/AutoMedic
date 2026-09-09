#!/usr/bin/env bash
# ============================================================
# AutoMedic 本地开发模式
#   后端: :8080（托管已构建的 web/dist；未构建则只用 API）
#   前端: :5173（vite dev server，代理 /api 与 /ws 到 8080）
# 用法：./scripts/dev.sh [ back | web | all ]
# ============================================================
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

MODE="${1:-all}"

start_back() {
  echo "==> 启动后端 :8080"
  (cd server && mkdir -p bin && go build -o bin/automedic-server ./cmd/server)
  # 优先使用本地私密配置（git 忽略）；不存在则退回开发模板 config.yaml
  local cfg="configs/config.yaml"
  if [ -f server/configs/config.local.yaml ]; then
    cfg="configs/config.local.yaml"
    echo "==> 使用本地配置: server/configs/config.local.yaml"
  fi
  (cd server && ./bin/automedic-server -config "$cfg")
}

start_web() {
  echo "==> 启动前端 :5173"
  if [ ! -d web/node_modules ]; then
    (cd web && npm install --no-audit --no-fund)
  fi
  (cd web && npm run dev)
}

case "$MODE" in
  back) start_back ;;
  web)  start_web ;;
  all)
    start_back &
    BACK_PID=$!
    trap 'kill $BACK_PID 2>/dev/null || true' EXIT
    sleep 2
    start_web
    ;;
  *) echo "用法: $0 [back|web|all]"; exit 1 ;;
esac
