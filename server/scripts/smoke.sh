#!/usr/bin/env bash
# AutoMedic 冒烟测试：项目 → 仓库 → 规则 → 令牌 → 投递事件 → 任务
set -euo pipefail

BASE="${BASE:-http://127.0.0.1:8080}"
TOKEN="${ADMIN_TOKEN:-change-me}"
export no_proxy="*" NO_PROXY="*"
H=(-H "Content-Type: application/json" -H "X-Admin-Token: $TOKEN")

say() { printf '\n\033[36m== %s ==\033[0m\n' "$1"; }

say "健康检查"
curl -s "$BASE/healthz"; echo

say "默认厂家与模型"
curl -s "${H[@]}" "$BASE/api/v1/providers" | head -c 400; echo

say "创建项目"
PROJ=$(curl -s "${H[@]}" -X POST "$BASE/api/v1/projects" -d '{
  "name":"示例电商后端","key":"shop-api","fix_mode":"semi",
  "description":"订单、支付、库存主流程",
  "context":"Go + Gin + GORM，主要业务：下单、支付回调、库存扣减"
}')
echo "$PROJ"
PID=$(echo "$PROJ" | sed -n 's/.*"id":\([0-9]*\).*/\1/p')
echo "project_id=$PID"

say "创建仓库"
REPO=$(curl -s "${H[@]}" -X POST "$BASE/api/v1/repos" -d "{
  \"project_id\":$PID,\"name\":\"shop-api\",\"url\":\"$REPO_URL\",\"branch\":\"main\",
  \"language\":\"Go\",\"code_paths\":\"internal/order,internal/pay\"
}")
echo "$REPO"
RID=$(echo "$REPO" | sed -n 's/.*"id":\([0-9]*\).*/\1/p')
echo "repo_id=$RID"

say "创建修复规则（排除业务拒绝与第三方故障）"
RULE=$(curl -s "${H[@]}" -X POST "$BASE/api/v1/rules" -d "{
  \"project_id\":$PID,\"name\":\"主流程致命错误\",\"enabled\":true,\"priority\":10,
  \"levels\":\"fatal,error\",
  \"keywords\":\"panic,nil pointer,nullpointer,index out of range,timeout,5xx,内部错误\",
  \"exclude_keywords\":\"余额不足,权限不足,参数校验失败,第三方,上游超时,限流,用户取消\",
  \"exclude_sources\":\"biz-reject\",
  \"min_count\":1,\"window_sec\":300,\"cooldown_sec\":600,
  \"action\":\"fix\",\"max_retries\":2,
  \"description\":\"仅修复影响主流程的内部代码缺陷\"
}")
echo "$RULE"

say "创建投递令牌"
TOK=$(curl -s "${H[@]}" -X POST "$BASE/api/v1/tokens" -d "{\"project_id\":$PID,\"name\":\"loki-collector\"}")
echo "$TOK"
AM=$(echo "$TOK" | sed -n 's/.*"plain_token":"\([^"]*\)".*/\1/p')
echo "ingest_token=$AM"

say "投递一个真实缺陷事件（空指针）"
curl -s -H "Content-Type: application/json" -H "X-AM-Token: $AM" \
  -X POST "$BASE/api/v1/ingest/events" -d '{
    "source":"sentry","level":"fatal","title":"panic: runtime error: invalid memory address or nil pointer dereference",
    "message":"order service panic when building payment context","stack":"panic: runtime error: invalid memory address or nil pointer dereference\n  at internal/order/service.go:128\n  at internal/order/handler.go:45",
    "fingerprint":"order-nil-ctx-001",
    "payload":{"env":"prod","service":"order"}
  }'; echo

say "投递一个应被忽略的事件（业务拒绝）"
curl -s -H "Content-Type: application/json" -H "X-AM-Token: $AM" \
  -X POST "$BASE/api/v1/ingest/events" -d '{
    "source":"biz-reject","level":"error","title":"下单失败：余额不足",
    "message":"用户余额不足导致下单被拒绝","fingerprint":"balance-001"
  }'; echo

say "任务列表"
sleep 2
curl -s "${H[@]}" "$BASE/api/v1/tasks" | head -c 900; echo

say "事件列表"
curl -s "${H[@]}" "$BASE/api/v1/events" | head -c 600; echo

say "统计概览"
curl -s "${H[@]}" "$BASE/api/v1/stats/overview?days=7"; echo
curl -s "${H[@]}" "$BASE/api/v1/stats/group?group=status&days=7"; echo

say "完成"
