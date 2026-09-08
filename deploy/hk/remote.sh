#!/usr/bin/env bash
# ============================================================
# AutoMedic HK 远端部署（由 scripts/deploy-hk.sh 调用）
# 在 hk 上以 root 执行：源码构建 + 依赖最新化 + systemd + nginx
# ============================================================
set -euo pipefail

SRC="${AUTOMEDIC_SRC:-/opt/automedic/src}"
ROOT="${AUTOMEDIC_ROOT:-/opt/automedic}"
LISTEN="${AUTOMEDIC_LISTEN:-127.0.0.1:8087}"
PUBLIC_HOST="${AUTOMEDIC_PUBLIC_HOST:-hk.hsddns.com}"
PUBLIC_PATH="${AUTOMEDIC_PUBLIC_PATH:-/automedic}"
SKIP_DEPS="${AUTOMEDIC_SKIP_DEPS:-0}"
NGINX_SITE="${AUTOMEDIC_NGINX_SITE:-/etc/nginx/sites-enabled/whatsapp-ai-hk}"

export DEBIAN_FRONTEND=noninteractive
export LANG=C LC_ALL=C
export PATH="/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

log()  { printf '[%s] %s\n' "$(date '+%F %T')" "$*"; }
die()  { printf '[%s] ERROR %s\n' "$(date '+%F %T')" "$*" >&2; exit 1; }
skip() { log "SKIP  $1（已是最新：$2）"; }

[[ "$(id -u)" -eq 0 ]] || die "必须以 root 运行"
[[ -d "$SRC" ]] || die "源码目录不存在: $SRC"

APT_UPDATED=0
apt_update_once() {
  if [[ "$APT_UPDATED" -eq 0 ]]; then
    log "apt-get update"
    apt-get update -qq
    APT_UPDATED=1
  fi
}

ensure_apt_pkg() {
  local pkg="$1"
  apt_update_once
  local installed candidate
  installed="$(dpkg-query -W -f='${Version}' "$pkg" 2>/dev/null || true)"
  candidate="$(apt-cache policy "$pkg" | awk '/^[[:space:]]*Candidate:/ {print $2; exit}')"
  [[ -n "$candidate" && "$candidate" != "(none)" ]] || die "找不到 apt 包 $pkg"
  if [[ -n "$installed" && "$installed" == "$candidate" ]]; then
    skip "apt:$pkg" "$installed"
    return 0
  fi
  log "INSTALL apt:$pkg（${installed:-未安装} -> $candidate）"
  apt-get install -y -qq "$pkg"
}

semver_ge() {
  # $1 >= $2 ?  仅用于 go1.23 这种前缀比较的兜底
  local a="${1#go}" b="${2#go}"
  [[ "$(printf '%s\n%s\n' "$b" "$a" | sort -V | tail -n1)" == "$a" ]]
}

ensure_go_latest() {
  local current latest url tmp
  current="$(go version 2>/dev/null | awk '{print $3}' || true)"
  latest="$(curl -fsSL --max-time 20 'https://go.dev/VERSION?m=text' 2>/dev/null | head -n1 || true)"
  if [[ -z "$latest" ]]; then
    latest="$(curl -fsSL --max-time 20 'https://golang.google.cn/VERSION?m=text' 2>/dev/null | head -n1 || true)"
  fi
  if [[ -z "$latest" ]]; then
    if [[ -n "$current" ]] && semver_ge "$current" "go1.23"; then
      log "WARN 无法查询最新 Go，保留已满足构建要求的 $current"
      return 0
    fi
    die "无法查询最新 Go，且本机 Go 不满足 >=1.23"
  fi
  if [[ "$current" == "$latest" ]]; then
    skip "Go" "$current"
    return 0
  fi
  log "INSTALL Go（${current:-未安装} -> $latest）"
  tmp="$(mktemp -d)"
  url="https://go.dev/dl/${latest}.linux-amd64.tar.gz"
  if ! curl -fL --max-time 180 -o "$tmp/go.tgz" "$url"; then
    url="https://mirrors.aliyun.com/golang/${latest}.linux-amd64.tar.gz"
    curl -fL --max-time 180 -o "$tmp/go.tgz" "$url" || die "下载 Go $latest 失败"
  fi
  rm -rf /usr/local/go
  tar -C /usr/local -xzf "$tmp/go.tgz"
  rm -rf "$tmp"
  hash -r
  go version | grep -q "$latest" || die "Go 安装后版本不是 $latest"
}

ensure_npm_global() {
  local pkg="$1"
  local latest current
  latest="$(npm view "$pkg" version)"
  current="$(
    set +o pipefail
    npm ls -g --depth=0 --json 2>/dev/null | python3 -c "
import json, sys
try:
    data = json.load(sys.stdin)
except Exception:
    data = {}
deps = data.get('dependencies') or {}
print((deps.get('$pkg') or {}).get('version') or '')
"
  )"
  if [[ -n "$current" && "$current" == "$latest" ]]; then
    skip "npm:$pkg" "$current"
    return 0
  fi
  log "INSTALL npm:$pkg（${current:-未安装} -> $latest）"
  npm install -g "${pkg}@${latest}" --no-audit --no-fund --ignore-scripts=false
}

ensure_user() {
  if id automedic >/dev/null 2>&1; then
    skip "user:automedic" "$(id -u automedic)"
    return 0
  fi
  log "创建系统用户 automedic"
  useradd -r -d "$ROOT" -s /usr/sbin/nologin automedic
}

ensure_dirs() {
  mkdir -p "$ROOT"/{configs,data/workspaces,data/logs,data/.dsh,web,migrations} "$SRC"
  chown automedic:automedic "$ROOT"
  chown -R automedic:automedic "$ROOT/data"
  chmod 755 "$ROOT"
  chmod 750 "$ROOT/data"
}

ensure_secrets() {
  local envf="$ROOT/.env" keyf="$ROOT/data/.secret_key"
  if [[ ! -s "$keyf" ]]; then
    log "生成加密主密钥 $keyf"
    openssl rand -base64 48 | tr -d '\n' > "$keyf"
    chmod 600 "$keyf"
  else
    skip "secret_key_file" "已存在"
  fi
  if [[ ! -f "$envf" ]]; then
    local token
    token="$(openssl rand -hex 24)"
    cat > "$envf" <<EOF
AUTOMEDIC_SERVER_ADDR=${LISTEN}
AUTOMEDIC_SERVER_MODE=release
AUTOMEDIC_SERVER_WEB_DIR=${ROOT}/web/dist
AUTOMEDIC_SERVER_ADMIN_TOKEN=${token}
AUTOMEDIC_DB_DRIVER=sqlite
AUTOMEDIC_DB_DSN=${ROOT}/data/automedic.db
AUTOMEDIC_SECURITY_SECRET_KEY_FILE=${keyf}
AUTOMEDIC_DSH_HOME=${ROOT}/data/.dsh
AUTOMEDIC_GIT_WORKSPACE_ROOT=${ROOT}/data/workspaces
EOF
    chmod 600 "$envf"
    log "已写入首次管理令牌到 $envf（AUTOMEDIC_SERVER_ADMIN_TOKEN）"
    log "FIRST_ADMIN_TOKEN=${token}"
  else
    skip ".env" "已存在，不覆盖密钥"
  fi
}

patch_runtime_config() {
  local dsh_bin ocr_bin node_dir
  dsh_bin="$(command -v dsh || true)"
  ocr_bin="$(command -v ocr || true)"
  node_dir="$(dirname "$(command -v node)")"
  [[ -n "$dsh_bin" ]] || die "dsh 不在 PATH"
  [[ -n "$ocr_bin" ]] || die "ocr 不在 PATH"
  install -m 644 "$SRC/deploy/hk/config.yaml" "$ROOT/configs/config.yaml"
  python3 - "$ROOT/configs/config.yaml" "$dsh_bin" "$ocr_bin" "$node_dir" <<'PY'
import pathlib, sys
path, dsh, ocr, node_dir = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
text = pathlib.Path(path).read_text()
text = text.replace('bin: "/usr/bin/dsh"', f'bin: "{dsh}"', 1)
text = text.replace('bin: "/usr/bin/ocr"', f'bin: "{ocr}"', 1)
old = '    - "PATH=/usr/local/bin:/usr/bin:/bin"'
new = f'    - "PATH={node_dir}:/usr/local/bin:/usr/bin:/bin"'
if old not in text:
    raise SystemExit("config.yaml PATH 行未找到，拒绝静默跳过")
text = text.replace(old, new, 1)
pathlib.Path(path).write_text(text)
PY
}

build_from_source() {
  [[ -x /usr/local/go/bin/go ]] || die "未找到 /usr/local/go/bin/go"
  local gover
  gover="$(go version | awk '{print $3}')"
  semver_ge "$gover" "go1.23" || die "Go 版本过低: $gover"
  log "构建后端"
  mkdir -p "$SRC/server/bin"
  (cd "$SRC/server" && GOPROXY="${GOPROXY:-https://goproxy.cn,https://proxy.golang.org,direct}" \
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o bin/automedic-server ./cmd/server)
  log "构建前端（base=${PUBLIC_PATH}/）"
  (cd "$SRC/web" && npm install --no-audit --no-fund)
  (cd "$SRC/web" && \
    VITE_BASE="${PUBLIC_PATH}/" \
    VITE_API_BASE="${PUBLIC_PATH}/api" \
    VITE_WS_BASE="wss://${PUBLIC_HOST}${PUBLIC_PATH}" \
    npm run build)
  [[ -x "$SRC/server/bin/automedic-server" ]] || die "后端产物缺失"
  [[ -f "$SRC/web/dist/index.html" ]] || die "前端产物缺失"
}

install_release() {
  install -m 755 "$SRC/server/bin/automedic-server" "$ROOT/automedic-server"
  rm -rf "$ROOT/web/dist"
  mkdir -p "$ROOT/web"
  cp -a "$SRC/web/dist" "$ROOT/web/dist"
  mkdir -p "$ROOT/migrations"
  cp -a "$SRC/server/migrations/." "$ROOT/migrations/"
  install -m 644 "$SRC/deploy/hk/automedic.service" /etc/systemd/system/automedic.service
  if [[ -n "${AUTOMEDIC_RELEASE_ID:-}" ]]; then
    printf '%s\n' "$AUTOMEDIC_RELEASE_ID" > "$ROOT/RELEASE"
  fi
  chown automedic:automedic "$ROOT"
  chown -R automedic:automedic "$ROOT/data" "$ROOT/web" "$ROOT/configs" "$ROOT/migrations"
  chown automedic:automedic "$ROOT/automedic-server" "$ROOT/.env" "$ROOT/configs/config.yaml"
  chmod 755 "$ROOT"
  chmod 750 "$ROOT/data" "$ROOT/automedic-server"
  chmod 600 "$ROOT/.env" "$ROOT/data/.secret_key"
}

patch_nginx() {
  [[ -f "$NGINX_SITE" ]] || die "nginx 站点文件不存在: $NGINX_SITE"
  python3 - "$NGINX_SITE" "$LISTEN" <<'PY'
import datetime, pathlib, re, shutil, sys
site, listen = sys.argv[1], sys.argv[2]
p = pathlib.Path(site)
text = p.read_text()
block = f"""    location = /automedic {{
        return 301 /automedic/;
    }}
    location /automedic/ {{
        proxy_pass http://{listen}/;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
        client_max_body_size 10m;
    }}
"""
pat = re.compile(
    r'(?:    location = /automedic \{\n        return 301 /automedic/;\n    \}\n)?'
    r'    location /automedic/ \{.*?\n    \}\n',
    re.S,
)
if pat.search(text):
    text = pat.sub(block, text, count=1)
else:
    needle = "    location = / {"
    if needle not in text:
        raise SystemExit("未找到 nginx 插入点 location = /")
    text = text.replace(needle, block + "\n" + needle, 1)
bak = p.parent / f"{p.name}.bak-automedic-{datetime.datetime.now().strftime('%Y%m%d%H%M%S')}"
shutil.copy2(p, bak)
p.write_text(text)
print(str(bak))
PY
  nginx -t
  systemctl reload nginx
  log "nginx 已重载，/automedic/ -> http://${LISTEN}/"
}

start_and_verify() {
  systemctl daemon-reload
  systemctl reset-failed automedic || true
  systemctl enable automedic
  systemctl restart automedic
  local i
  for i in $(seq 1 30); do
    if curl -fsS --max-time 2 "http://${LISTEN}/healthz" >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done
  local body
  body="$(curl -fsS --max-time 5 "http://${LISTEN}/healthz")" || {
    journalctl -u automedic -n 80 --no-pager >&2 || true
    die "本机 healthz 失败: http://${LISTEN}/healthz"
  }
  log "本机 healthz: $body"
  curl -fsS --max-time 8 -o /dev/null "https://${PUBLIC_HOST}${PUBLIC_PATH}/healthz" \
    || die "公网 healthz 失败: https://${PUBLIC_HOST}${PUBLIC_PATH}/healthz"
  curl -fsS --max-time 8 -o /dev/null "https://${PUBLIC_HOST}${PUBLIC_PATH}/" \
    || die "公网首页失败: https://${PUBLIC_HOST}${PUBLIC_PATH}/"
  log "公网验证通过: https://${PUBLIC_HOST}${PUBLIC_PATH}/"
  systemctl --no-pager --full status automedic | sed -n '1,12p'
}

main() {
  log "开始 HK 部署  src=$SRC root=$ROOT listen=$LISTEN"
  ensure_user
  ensure_dirs
  if [[ "$SKIP_DEPS" != "1" ]]; then
    ensure_apt_pkg ca-certificates
    ensure_apt_pkg curl
    ensure_apt_pkg git
    ensure_apt_pkg openssh-client
    ensure_apt_pkg openssl
    ensure_apt_pkg nginx
    ensure_apt_pkg sqlite3
    ensure_apt_pkg rsync
    ensure_apt_pkg nodejs
    ensure_go_latest
    ensure_npm_global "@deepseek-ai/dsh"
    ensure_npm_global "@alibaba-group/open-code-review"
    command -v dsh >/dev/null || die "dsh 安装后仍不可执行"
    command -v ocr >/dev/null || die "ocr 安装后仍不可执行"
    log "dsh: $(command -v dsh) $(dsh --version 2>/dev/null || true)"
    log "ocr: $(command -v ocr) $(ocr --version 2>/dev/null || true)"
    log "node: $(node -v)  npm: $(npm -v)  go: $(go version)  git: $(git --version)"
  else
    log "SKIP 依赖安装（--skip-deps）"
  fi
  ensure_secrets
  patch_runtime_config
  build_from_source
  install_release
  patch_nginx
  start_and_verify
  log "部署完成"
}

main "$@"
