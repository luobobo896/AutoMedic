#!/bin/sh
# AutoMedic 容器启动脚本
#  1) 未提供 AUTOMEDIC_SECURITY_SECRET_KEY 时，在数据卷生成并复用持久化密钥
#  2) exec 交由 automedic-server 处理信号
set -e

KEY_FILE="${AUTOMEDIC_SECURITY_SECRET_KEY_FILE:-/app/data/.secret_key}"

if [ -z "${AUTOMEDIC_SECURITY_SECRET_KEY:-}" ]; then
  mkdir -p "$(dirname "$KEY_FILE")"
  if [ ! -s "$KEY_FILE" ]; then
    # 48 字节随机 -> base64，取前 32 字节作为 AES-256 主密钥
    if command -v openssl >/dev/null 2>&1; then
      openssl rand -base64 48 | tr -d '\n' > "$KEY_FILE"
    else
      head -c 48 /dev/urandom | base64 | tr -d '\n' > "$KEY_FILE"
    fi
    chmod 600 "$KEY_FILE"
    echo "[entrypoint] 已生成持久化密钥: $KEY_FILE"
  fi
  export AUTOMEDIC_SECURITY_SECRET_KEY_FILE="$KEY_FILE"
fi

exec /app/automedic-server "$@"
