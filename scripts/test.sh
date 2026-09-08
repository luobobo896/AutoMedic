#!/usr/bin/env bash
# ============================================================
# AutoMedic 测试：go vet + go test（PostgreSQL）
# 用法：./scripts/test.sh [任意 go test 参数，如 -run TestE2E -v]
# 端到端测试用「假 dsh」替代真实 Harness，不调用任何大模型服务。
# 默认拉起本机 docker 容器 automedic-pg-test（55432）。
# 也可自行提供 AUTOMEDIC_TEST_PG_DSN。
# ============================================================
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT/server"

ensure_test_pg() {
  if [[ -n "${AUTOMEDIC_TEST_PG_DSN:-}" ]]; then
    echo "==> 使用 AUTOMEDIC_TEST_PG_DSN"
    return 0
  fi
  command -v docker >/dev/null || { echo "缺少 docker，且未设置 AUTOMEDIC_TEST_PG_DSN" >&2; exit 1; }
  local cid=automedic-pg-test
  if ! docker inspect -f '{{.State.Running}}' "$cid" 2>/dev/null | grep -q true; then
    echo "==> 启动测试 PostgreSQL 容器 $cid"
    docker rm -f "$cid" >/dev/null 2>&1 || true
    docker run -d --name "$cid" \
      -e POSTGRES_USER=automedic \
      -e POSTGRES_PASSWORD=automedic \
      -e POSTGRES_DB=automedic_test \
      -p 55432:5432 \
      pgvector/pgvector:pg17 >/dev/null
  else
    echo "==> 复用测试 PostgreSQL 容器 $cid"
  fi
  local i
  for i in $(seq 1 40); do
    if docker exec "$cid" pg_isready -U automedic -d automedic_test >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done
  docker exec "$cid" pg_isready -U automedic -d automedic_test >/dev/null
  export AUTOMEDIC_TEST_PG_DSN='host=127.0.0.1 user=automedic password=automedic dbname=automedic_test port=55432 sslmode=disable TimeZone=Asia/Shanghai'
}

ensure_test_pg

echo "==> go vet"
go vet ./...

echo "==> go test"
go test ./... "$@"

echo "==> 完成"
