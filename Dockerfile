# syntax=docker/dockerfile:1
# ------------------------------------------------------------
# AutoMedic 镜像
#   stage 1: 构建前端 (node)
#   stage 2: 构建后端 (golang)
#   stage 3: 运行时 (node slim + git + openssh)
#     - 运行时必须包含 node：dsh 是 node CLI
#     - 必须包含 git/openssh：隔离工作区 clone/push
# ------------------------------------------------------------

# ---------- Stage 1: web ----------
FROM node:22-alpine AS webbuild
WORKDIR /src/web
COPY web/package.json ./
RUN npm install --no-audit --no-fund
COPY web/ ./
RUN npm run build

# ---------- Stage 2: server ----------
FROM golang:1.23-alpine AS serverbuild
RUN apk add --no-cache git ca-certificates && \
    go env -w GOPROXY=https://goproxy.cn,https://proxy.golang.org,direct
WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" \
    -o /out/automedic-server ./cmd/server

# ---------- Stage 3: runtime ----------
FROM node:22-bookworm-slim
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
        git openssh-client ca-certificates tzdata curl \
    && rm -rf /var/lib/apt/lists/* \
    && ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime

# 全局安装 dsh（DeepSeek Harness）。官方 npm 包名：@deepseek-ai/dsh
# 不要改为 @deepseek/dsh 或 dsh —— 这两个不是官方包。
ARG DSH_VERSION=latest
RUN npm install -g "@deepseek-ai/dsh@${DSH_VERSION}" --no-audit --no-fund && \
    (dsh --version || true)
ENV PATH="/usr/local/bin:${PATH}"

# 启动脚本：未提供 AUTOMEDIC_SECURITY_SECRET_KEY 时，在数据卷中生成并复用一个持久化密钥，
# 避免容器重建后已加密的凭证 / API Key 无法解密。
COPY deploy/docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh

WORKDIR /app
COPY --from=serverbuild /out/automedic-server /app/automedic-server
COPY --from=webbuild    /src/web/dist          /app/web/dist
COPY server/configs/config.docker.yaml        /app/configs/config.yaml
COPY server/migrations                        /app/migrations

RUN mkdir -p /app/data/workspaces /app/data/logs
VOLUME ["/app/data"]

EXPOSE 8080
ENV AUTOMEDIC_SERVER_ADDR=:8080 \
    AUTOMEDIC_SERVER_WEB_DIR=/app/web/dist \
    AUTOMEDIC_DB_DRIVER=postgres \
    AUTOMEDIC_DB_DSN="host=postgres user=automedic password=automedic dbname=automedic port=5432 sslmode=disable TimeZone=Asia/Shanghai" \
    AUTOMEDIC_DSH_BIN=/usr/local/bin/dsh \
    AUTOMEDIC_GIT_WORKSPACE_ROOT=/app/data/workspaces \
    AUTOMEDIC_SECURITY_SECRET_KEY_FILE=/app/data/.secret_key

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD curl -fsS http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["-config", "/app/configs/config.yaml"]
