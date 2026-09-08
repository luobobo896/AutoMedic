#!/usr/bin/env bash
# ============================================================
# AutoMedic 一键发布到香港 HK 测试服务器
#
# 服务器从 GitHub 拉取已推送的 commit，再在远端构建。
# 本机未提交 / 未推送的改动不会上 HK。
#
# 用法：
#   ./scripts/deploy-hk.sh
#   ./scripts/deploy-hk.sh --skip-deps    # 依赖已就绪时只 git pull 并重建
#
# 目标：ssh hk（hk.hsddns.com）
# 访问：https://hk.hsddns.com/automedic/
# ============================================================
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SSH_HOST="${AUTOMEDIC_SSH_HOST:-hk}"
REMOTE_SRC="${AUTOMEDIC_SRC:-/opt/automedic/src}"
GIT_URL="${AUTOMEDIC_GIT_URL:-https://github.com/luobobo896/AutoMedic.git}"
GIT_REF="${AUTOMEDIC_GIT_REF:-main}"
SKIP_DEPS=0

for arg in "$@"; do
  case "$arg" in
    --skip-deps) SKIP_DEPS=1 ;;
    -h|--help)
      sed -n '2,16p' "$0"
      exit 0
      ;;
    *) echo "未知参数: $arg" >&2; exit 1 ;;
  esac
done

log() { printf '[%s] %s\n' "$(date '+%F %T')" "$*"; }
die() { printf '[%s] ERROR %s\n' "$(date '+%F %T')" "$*" >&2; exit 1; }

command -v ssh >/dev/null || die "本机缺少 ssh"
command -v git >/dev/null || die "本机缺少 git"

cd "$ROOT"
git rev-parse --is-inside-work-tree >/dev/null 2>&1 || die "当前目录不是 git 仓库"
if git status --porcelain | grep -q .; then
  die "工作区有未提交改动。发布只拉 GitHub 上的 commit，请先提交并 git push"
fi

log "检查 SSH ${SSH_HOST}"
ssh -o BatchMode=yes -o ConnectTimeout=10 "$SSH_HOST" 'echo ok' >/dev/null \
  || die "无法免密登录 ${SSH_HOST}，请先确认 ~/.ssh/config 的 Host hk"

log "核对 origin/${GIT_REF} 已包含当前 HEAD"
git fetch origin "$GIT_REF"
HEAD_SHA="$(git rev-parse HEAD)"
REMOTE_SHA="$(git rev-parse "origin/${GIT_REF}")"
if [[ "$HEAD_SHA" != "$REMOTE_SHA" ]]; then
  die "本地 HEAD ${HEAD_SHA:0:8} 与 origin/${GIT_REF} ${REMOTE_SHA:0:8} 不一致，请先 git push"
fi

RELEASE_ID="$(printf 'commit=%s dirty=no date=%s\n' "$(git rev-parse --short HEAD)" "$(date -u '+%F_%T')Z")"
log "发布标识: $RELEASE_ID"
log "HK 将 git fetch ${GIT_URL} @ ${HEAD_SHA:0:12}"

log "远端 git 同步并构建"
ssh -o BatchMode=yes "$SSH_HOST" \
  "AUTOMEDIC_SKIP_DEPS=${SKIP_DEPS} AUTOMEDIC_RELEASE_ID='${RELEASE_ID}' AUTOMEDIC_SRC='${REMOTE_SRC}' AUTOMEDIC_GIT_URL='${GIT_URL}' AUTOMEDIC_GIT_COMMIT='${HEAD_SHA}' AUTOMEDIC_GIT_REF='${GIT_REF}' bash -s" <<'REMOTE'
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive LANG=C LC_ALL=C
SRC="${AUTOMEDIC_SRC:-/opt/automedic/src}"
URL="${AUTOMEDIC_GIT_URL:?}"
COMMIT="${AUTOMEDIC_GIT_COMMIT:?}"
REF="${AUTOMEDIC_GIT_REF:-main}"
command -v git >/dev/null || { apt-get update -qq && apt-get install -y -qq git; }
mkdir -p /opt/automedic
if [[ ! -d "$SRC/.git" ]]; then
  echo "[git] $SRC 不是 git 仓库，clone $URL"
  rm -rf "$SRC"
  git clone --branch "$REF" "$URL" "$SRC"
fi
git -C "$SRC" remote get-url origin >/dev/null 2>&1 || git -C "$SRC" remote add origin "$URL"
git -C "$SRC" remote set-url origin "$URL"
git -C "$SRC" fetch --prune origin
git -C "$SRC" cat-file -t "$COMMIT" >/dev/null 2>&1 || git -C "$SRC" fetch origin "$COMMIT"
git -C "$SRC" checkout -f "$COMMIT"
git -C "$SRC" reset --hard "$COMMIT"
git -C "$SRC" clean -fd -e web/node_modules -e server/bin
echo "[git] 工作区 $(git -C "$SRC" rev-parse --short HEAD) $(git -C "$SRC" log -1 --format=%s)"
bash "$SRC/deploy/hk/remote.sh"
REMOTE

log "本机验收 https://hk.hsddns.com/automedic/healthz"
curl -fsS --max-time 15 "https://hk.hsddns.com/automedic/healthz"
echo
log "完成。Web: https://hk.hsddns.com/automedic/"
log "首次部署的管理令牌只在远端日志中打印一次（FIRST_ADMIN_TOKEN），之后保存在 /opt/automedic/.env"
