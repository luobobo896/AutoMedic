#!/usr/bin/env bash
# ============================================================
# AutoMedic 一键发布到香港 HK 测试服务器（源码部署）
#
# 用法：
#   ./scripts/deploy-hk.sh
#   ./scripts/deploy-hk.sh --skip-deps    # 依赖已就绪时只同步源码并重建
#
# 目标：ssh hk（hk.hsddns.com）
# 访问：https://hk.hsddns.com/automedic/
# ============================================================
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SSH_HOST="${AUTOMEDIC_SSH_HOST:-hk}"
REMOTE_SRC="${AUTOMEDIC_SRC:-/opt/automedic/src}"
SKIP_DEPS=0

for arg in "$@"; do
  case "$arg" in
    --skip-deps) SKIP_DEPS=1 ;;
    -h|--help)
      sed -n '2,14p' "$0"
      exit 0
      ;;
    *) echo "未知参数: $arg" >&2; exit 1 ;;
  esac
done

log() { printf '[%s] %s\n' "$(date '+%F %T')" "$*"; }
die() { printf '[%s] ERROR %s\n' "$(date '+%F %T')" "$*" >&2; exit 1; }

command -v ssh >/dev/null || die "本机缺少 ssh"
command -v rsync >/dev/null || die "本机缺少 rsync"

log "检查 SSH ${SSH_HOST}"
ssh -o BatchMode=yes -o ConnectTimeout=10 "$SSH_HOST" 'echo ok' >/dev/null \
  || die "无法免密登录 ${SSH_HOST}，请先确认 ~/.ssh/config 的 Host hk"

RELEASE_ID="$(
  cd "$ROOT"
  if git rev-parse --short HEAD >/dev/null 2>&1; then
    printf 'commit=%s dirty=%s date=%s\n' \
      "$(git rev-parse --short HEAD)" \
      "$(git status --porcelain | awk 'END{print (NR>0)?"yes":"no"}')" \
      "$(date -u '+%F_%T')Z"
  else
    printf 'date=%s\n' "$(date -u '+%F_%T')Z"
  fi
)"
log "发布标识: $RELEASE_ID"
if git -C "$ROOT" status --porcelain | grep -q .; then
  log "WARN 工作区有未提交改动，将按当前工作区源码同步（不含 .env / node_modules / dist）"
fi

log "远端准备目录并确保 rsync"
ssh -o BatchMode=yes "$SSH_HOST" "bash -s" <<EOF
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive LANG=C LC_ALL=C
mkdir -p '${REMOTE_SRC}' /opt/automedic
if ! command -v rsync >/dev/null 2>&1; then
  apt-get update -qq
  apt-get install -y -qq rsync
fi
command -v rsync >/dev/null
EOF

log "同步源码 -> ${SSH_HOST}:${REMOTE_SRC}"
rsync -az --delete \
  --exclude '.git/' \
  --exclude 'web/node_modules/' \
  --exclude 'web/dist/' \
  --exclude 'server/bin/' \
  --exclude 'server/data/' \
  --exclude '.env' \
  --exclude '*.db' \
  --exclude '*.db-shm' \
  --exclude '*.db-wal' \
  --exclude '.DS_Store' \
  --exclude '.idea/' \
  --exclude '.vscode/' \
  "$ROOT"/ "${SSH_HOST}:${REMOTE_SRC}/"

log "远端构建与发布"
ssh -o BatchMode=yes "$SSH_HOST" \
  "AUTOMEDIC_SKIP_DEPS=${SKIP_DEPS} AUTOMEDIC_RELEASE_ID='${RELEASE_ID}' bash '${REMOTE_SRC}/deploy/hk/remote.sh'"

log "本机验收 https://hk.hsddns.com/automedic/healthz"
curl -fsS --max-time 15 "https://hk.hsddns.com/automedic/healthz"
echo
log "完成。Web: https://hk.hsddns.com/automedic/"
log "首次部署的管理令牌只在远端日志中打印一次（FIRST_ADMIN_TOKEN），之后保存在 /opt/automedic/.env"
