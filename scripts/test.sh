#!/usr/bin/env bash
# ============================================================
# AutoMedic 测试：go vet + go test
# 用法：./scripts/test.sh [任意 go test 参数，如 -run TestE2E -v]
# 端到端测试用「假 dsh」替代真实 Harness，不调用任何大模型服务。
# ============================================================
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT/server"

echo "==> go vet"
go vet ./...

echo "==> go test"
go test ./... "$@"

echo "==> 完成"
