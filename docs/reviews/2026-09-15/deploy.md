# AutoMedic 代码审查（第 4 路：部署 / 脚本 / 文档一致性）

- 审查时间：2026-09-15
- 基线：`3c10e9d`（`fix(web): 统一 token 与响应式，消除死类名和图表色漂移`），工作区无未提交改动
- 范围：`Dockerfile`、`docker-compose.yml`、`.env.example`、`.dockerignore`、`.gitignore`、`deploy/**`、`scripts/**`、`server/configs/**`、`server/migrations/**`、`docs/**`、`README.md`；交叉核对 `server/internal/api/router.go` ↔ `docs/接口文档.md`、`server/internal/store` ↔ `server/migrations/README.md`、`server/internal/config` ↔ 文档配置项
- 方法：源码/脚本/文档通读 + 三组自动交叉核对（配置字段、路由清单、env 覆盖面）+ 规定命令验证（`bash -n`、`docker compose config`）；**只读审查**，未修改任何产品代码或配置，未部署、未接触任何线上环境

---

## 结论（先看这个）

容器与主机安全基线大体成型：镜像已非 root、dsh 版本在容器路径钉死、密钥文件落盘 `0600`、健康检查与重启策略齐备、应用日志已按天轮转并带 `retain`。本轮最严重的问题不在代码，而在**部署文档与实现脱节到"照文档做会失败或不安全"的程度**：

1. **全新 Docker 部署 100% 起不来。** `release` 模式拒绝默认引导口令，而 compose 没有注入 `AUTOMEDIC_AUTH_BOOTSTRAP_PASSWORD`，`.env.example` 与部署文档的快速开始也都没提这个变量 → 容器 `exit 1` + `restart: unless-stopped` 死循环（P0-1）。
2. **裸机部署文档给出的配方等于"公开密钥 + 公开口令"。** 文档让运维直接复制开发模板 `config.yaml`（`mode: debug`、`jwt_secret: CHANGE_ME_...`、`password: CHANGE_ME`），而默认口令守卫只比对 `admin123`/`change-me` 两个字面量，大小写不同即放行（P0-2）。
3. **迁移脚本路径漂移**：文档与 Dockerfile 都指向 `server/migrations/*.sql`，该目录只有 `README.md`，SQL 实际在 `docs/database/`（P1-1）。
4. **文档仍以 `X-Admin-Token` 为操作主线**，但后端已无任何中间件读取它：部署文档、README、接口文档 §13、`server/scripts/smoke.sh` 照抄必 401（P1-2）。
5. 可复现构建只做了一半：dsh 已钉版本、`web/package-lock.json` 已入库，但镜像构建与 HK 部署链都不使用 lockfile，HK 还在以 root 拉取 latest 的 Go/npm 工具链（P1-3、P1-4）；compose 默认库口令仍是 `automedic`（P1-5）。

建议本周只做四件事：修 P0-1/P0-2 的文档与变量注入（含 compose 补 `AUTOMEDIC_AUTH_BOOTSTRAP_PASSWORD`）、把 SQL 迁到 `server/migrations/`、把文档里的 admin_token 操作链全部换成账号密码登录、镜像构建改 `npm ci` + COPY lockfile。

---

## P0 — 必须先修

### P0-1 全新 Docker 部署必失败：release 拒绝默认引导口令，compose 未注入、快速开始未提示该变量

- 位置：`docker-compose.yml:43-54`、`server/configs/config.docker.yaml`（无 `auth:` 段）、`server/internal/config/config.go:157-162`、`server/internal/store/seed_rbac.go:51-56`、`server/cmd/server/main.go:72-75`、`docs/部署文档.md:60-68`、`README.md:56-63`
- 证据链：
  - `docker-compose.yml:50` 固定 `AUTOMEDIC_SERVER_MODE: release`；`docker-compose.yml:43-54` 的 `environment:` 白名单里没有 `AUTOMEDIC_AUTH_BOOTSTRAP_PASSWORD`（实测 `docker compose config` 输出同样确认缺失）。
  - 容器配置源 `server/configs/config.docker.yaml` 没有 `auth:` 段（`Dockerfile:54` 把它复制为 `/app/configs/config.yaml`），于是 `config.Default()` 生效：`Password: "admin123"`（`config.go:157-162`）。
  - `seed_rbac.go:51-56`：用户表为空且 `Mode == "release"` 且口令为 `admin123`/`change-me` 时 `return errors.New("生产模式拒绝使用默认引导口令…")`；`main.go:72-75` 对 `SeedRBAC` 失败直接 `os.Exit(1)`。
  - 该行为已有单测锁定：`server/internal/store/seed_rbac_test.go:78-85`（`TestSeedRBACRejectsDefaultPasswordInRelease`）。
  - 文档侧：`README.md:56-63` 与 `docs/部署文档.md:60-68` 的"准备"步骤只要求生成 `AUTOMEDIC_ADMIN_TOKEN`；`.env.example` 里完全没有 `AUTOMEDIC_AUTH_BOOTSTRAP_PASSWORD`，部署文档仅在 §10.1 生产基线（`:343-344`）提到它，快速开始路径没有任何提示。
- 影响：全新环境按推荐路径部署必然失败（容器反复重启，`/healthz` 永不通过）；运维最常见"绕过"是把 `AUTOMEDIC_SERVER_MODE` 改成 `debug`，把一个启动阻断变成生产环境弱口令 + debug 日志。
- 建议：`docker-compose.yml` 增加 `AUTOMEDIC_AUTH_BOOTSTRAP_PASSWORD: ${AUTOMEDIC_AUTH_BOOTSTRAP_PASSWORD:?...}`（或整体改用 `env_file: .env`），`.env.example` 与 `docs/部署文档.md` §3.1 同步加该变量与生成命令；`.env.example` 顺手删掉已死的 `AUTOMEDIC_ADMIN_TOKEN` 主线。

### P0-2 裸机部署文档的配方 = 公开 JWT 签名密钥 + 公开引导口令

- 位置：`docs/部署文档.md:139`、`docs/部署文档.md:146-152`、`server/configs/config.yaml:9`、`server/configs/config.yaml:33`、`server/configs/config.yaml:39`、`server/internal/config/config.go:295-311`、`server/internal/store/seed_rbac.go:54`
- 证据：
  - `docs/部署文档.md:139` 让运维 `sudo cp server/configs/config.yaml /opt/automedic/configs/`；该模板 `mode: "debug"`（`config.yaml:9`）、`jwt_secret: "CHANGE_ME_USE_AUTOMEDIC_AUTH_JWT_SECRET"`（`:33`）、`bootstrap_admin.password: "CHANGE_ME"`（`:39`）。
  - `docs/部署文档.md:146-152` 的 `/opt/automedic/.env` 只有 `AUTOMEDIC_SERVER_ADMIN_TOKEN`、`AUTOMEDIC_SECURITY_SECRET_KEY`、`AUTOMEDIC_SERVER_WEB_DIR`、`AUTOMEDIC_DB_DRIVER`、`AUTOMEDIC_DB_DSN`、`AUTOMEDIC_DSH_BIN`、`AUTOMEDIC_GIT_WORKSPACE_ROOT` 七项，**没有** `AUTOMEDIC_SERVER_MODE`、`AUTOMEDIC_AUTH_JWT_SECRET`、`AUTOMEDIC_AUTH_BOOTSTRAP_PASSWORD`——与同文档 §10.1（`:343-344`）"生产必须注入"自相矛盾。
  - `ResolveJWTSecret()`（`config.go:295-297`）对非空 `auth.jwt_secret` 直接 `padOrTrim` 后使用；`padOrTrim` 取前 32 字符（`config.go:330-340`），即签名密钥恒等于仓库内公开字符串 `CHANGE_ME_USE_AUTOMEDIC_AUTH_JWT`。任何读过本仓库的人可自签超管 JWT。
  - `seed_rbac.go:54` 的守卫只匹配 `"admin123"` 与 `"change-me"`，**大小写敏感**，模板里的 `CHANGE_ME` 在 `release` 下同样放行。
- 影响：按文档裸机部署 = 可用公开常量伪造任意身份 + 引导口令是公开占位串 + debug 模式；等于绕开 09-09 已做的两轮鉴权加固。
- 建议：① 文档 §4.2 的 `.env` 模板补齐 `AUTOMEDIC_SERVER_MODE=release`、`AUTOMEDIC_AUTH_JWT_SECRET`、`AUTOMEDIC_AUTH_BOOTSTRAP_PASSWORD`；② `config.yaml` 的 `jwt_secret` 占位值改为空串（空则走 `secret_key_file`/随机兜底，`config.go:302-311`）；③ 守卫改为大小写不敏感并覆盖 `CHANGE_ME*`/占位 `jwt_secret`，在 release 下发现占位密钥直接拒绝启动。

---

## P1 — 高风险 / 缺闭环

| # | 位置 | 问题与证据 | 影响与建议 |
|---|---|---|---|
| P1-1 | `docs/部署文档.md:138`、`:362`、`Dockerfile:55`、`server/migrations/README.md:1-3` | 文档让 `sudo cp server/migrations/*.sql /opt/automedic/migrations/`，并承诺"破坏性变更会附手工迁移脚本（`server/migrations/`）"，但该目录只有 `README.md`；SQL 全在 `docs/database/001…006`。`Dockerfile:55` 同样把只有 README 的目录拷进 `/app/migrations` | 裸机安装该步直接报 cp 失败；"手工迁移脚本"承诺无处落地。建议把 SQL 移到 `server/migrations/`（或同步改文档 + `Dockerfile` 用 `docs/database`），并让 §11 指向真实路径 |
| P1-2 | `docs/部署文档.md:85`、`:205`、`:255`、`:300`、`:334`、`README.md:58`、`:166`、`docs/接口文档.md:663-693`、`server/scripts/smoke.sh:8` | `X-Admin-Token` 已无任何中间件读取（`rtk rg` 仅命中 CORS 白名单 `router.go:49` 与配置字段），接口文档 §1.2（`:52-57`）自己也这么说；但部署文档仍叫用户"在设置里填 AUTOMEDIC_ADMIN_TOKEN""用 AUTOMEDIC_ADMIN_TOKEN 登录 Web""curl -H X-Admin-Token"，接口文档 §13 端到端示例与 `smoke.sh:8` 全用它 | 照文档操作必然 401，排障人员会误判为服务异常；`smoke.sh` 作为唯一端到端脚本已不可用。建议：文档改用 `POST /api/v1/auth/login` + Bearer，`smoke.sh` 增加登录取 token 的步骤（或删除该脚本引用） |
| P1-3 | `Dockerfile:14-15`、`scripts/build.sh:35`、`deploy/hk/remote.sh:273` | 镜像构建只 `COPY web/package.json` 后 `npm install`；`web/package-lock.json`（93.5 KB）已入库但从未参与构建或部署 | 前端依赖按 `package.json` 范围重新解析，同 commit 在不同时间构建出不同产物。建议 `COPY web/package.json web/package-lock.json ./` + `npm ci`，脚本侧同样改 `npm ci` |
| P1-4 | `deploy/hk/remote.sh:58-88`、`:90-111`、`:390-391`、`docs/部署文档.md:381` | HK 部署以 root 执行：`ensure_go_latest` 每次查 go.dev 最新版并 `rm -rf /usr/local/go` 后重装；`ensure_npm_global` 用 `npm view <pkg> version` 装 latest 的 `@deepseek-ai/dsh` / `@alibaba-group/open-code-review`；文档明示"升级到候选最新版" | 同名 commit 不可复现、上游发新版本即静默换工具链、root 安装未钉版本包属供应链面且无回滚。建议改为钉死版本（与 `Dockerfile:41` 保持一致），或至少支持 `AUTOMEDIC_DSH_VERSION` 覆盖并默认钉死 |
| P1-5 | `docker-compose.yml:20`、`:52`、`.env.example:29-30`、`Dockerfile:69` | DB 口令默认值仍是 `automedic`（compose 进程口令、应用 DSN、镜像 ENV 三处），`.env.example` 直接给出可用默认口令；09-09 报告 P1-10 的"compose 默认口令"项未修 | 生产照抄 `.env` 即为弱口令库；建议 compose 改 `${AUTOMEDIC_DB_PASSWORD:?}` 强制显式设置，示例留空 + 生成命令 |
| P1-6 | `deploy/sidecar/ingest-sidecar.py:164-194`、`:197-202`、`docs/采集接入.md:117-135` | sidecar `do_POST` 完全不做入站鉴权，默认监听 `0.0.0.0:8091`；文档指导"Sentry/Grafana 只打 sidecar"，即它必须对外可达。`_read_json`（`:143-148`）按 `Content-Length` 全量读入内存且无上限 | 任何可达者可伪造任意告警 → 命中规则 → 生成自动/半自动修复任务，事件文本还会进入 dsh 任务文本（外部可控提示内容 + 资源消耗）。建议默认绑定 `127.0.0.1`、加共享密钥头校验与 body 上限，文档补"不要直接暴露到公网" |
| P1-7 | `deploy/automedic.service:20`、`deploy/hk/automedic.service:13`、`server/cmd/server/main.go:83` | 单元文件声明 `ExecReload=/bin/kill -HUP $MAINPID`，但程序只 `NotifyContext(SIGINT, SIGTERM)`，未处理 SIGHUP——Go 进程对 SIGHUP 的默认动作是终止 | `systemctl reload automedic` 实际是"无优雅退出的重启"，会中断运行中的 dsh 任务与工作区；建议删除 `ExecReload`，或实现 SIGHUP 重载配置后再声明 |

---

## P2 — 改进项

1. **compose 变量白名单过窄 + `.env.example` 承诺失实**：`docker-compose.yml:43-54` 只注入 9 个变量，`config.go:249-268` 支持 20 个 env 键；`.env.example:4` 却写"所有 `AUTOMEDIC_` 前缀变量均会覆盖 config.yaml"。Docker 路径下无法注入的有 8 个：`AUTH_BOOTSTRAP_PASSWORD`、`AUTH_BOOTSTRAP_USERNAME`、`DSH_HOME`、`DSH_PERMISSION_MODE`、`GIT_BIN`、`LOG_LEVEL`、`OCR_BIN`、`SERVER_ALLOW_ORIGINS`——其中包括前后端分离必须的 CORS 配置与日志级别。建议 compose 直接用 `env_file: .env`。
2. **接口文档与路由清单漂移**：`router.go` 共 84 条路由，接口文档只覆盖 45 条路径，**18 个 method+path 未记录**（路径级 13 条，清单见文末附录）：`users/roles/tenants` 全部 CRUD、`/auth/profile|logout|password`、`/auth/refresh`、`/permissions`、`/repos/:id/reviews`，RBAC/权限码/`X-Tenant-ID` 切换模型完全无文档，而 `README.md:155` 称该文档为"全部 REST 接口"。同时 `docs/接口文档.md:54`、`:654`、`:657` 称 `/ws/tasks/:id`"无鉴权（内网使用）"，实际 `router.go:203-208` 已挂 `Auth.Middleware()` + `RequirePerm(task:read)` 且 handler 内校验归属——安全姿态描述错误。建议以 `router.go` 为准生成清单并加一条比对脚本。
3. **备份与回滚缺口**：`docs/部署文档.md:316-326` 只有一句 `pg_dump -Fc automedic`，无恢复演练、无保留期、无迁移前备份；`:351-362` 的升级步骤 `docker compose pull` 与本仓库 compose 语义不符（`docker-compose.yml:33-38` 是本地 `build:` + `image: latest`），也**没有任何回滚步骤**（`deploy/hk/remote.sh:291-293` 写了 `RELEASE` 文件但全仓无使用方）。建议补"迁移前备份 + 记录制品摘要 + 回退到上一 commit 的完整命令"。
4. **破坏性命令无前置校验/无回滚**：`deploy/hk/remote.sh:83`（下载成功即 `rm -rf /usr/local/go`，解包失败即丢工具链）、`:285`（`rm -rf "$ROOT/web/dist"` 后 `cp -a`，中途失败站点即 500）、`scripts/deploy-hk.sh:76`（源目录不是 git 仓库时 `rm -rf "$SRC"`，`SRC` 来自环境变量且未校验 ≠ `AUTOMEDIC_ROOT`；同脚本 `:84-85` 还有 `reset --hard` + `clean -fd`）。建议加 `[[ "$SRC" != "$ROOT" ]]` 校验、先备份旧产物、失败保留上一版本。
5. **日志轮转与停机窗口**：应用侧已实现按天写文件 + `retain` 清理（`server/internal/logging/logging.go:14-16`、`:53-59`）✅；但 `docker-compose.yml:32-65` 未配置日志驱动上限，json-file 会无限增长；也未设 `stop_grace_period`（默认 10s < `main.go:108` 的 15s 关闭窗口，容器可能在优雅关闭前被 SIGKILL）。
6. **镜像版本未受控**：`Dockerfile:12`、`:20`、`:31`（`node:22-alpine`、`golang:1.23-alpine`、`node:22-bookworm-slim`）与 `docker-compose.yml:15`、`scripts/test.sh:28`（`pgvector/pgvector:pg17`）都只钉主版本/浮动 tag。建议按 digest 或 minor 钉死。
7. **迁移双轨 + 静默降级**：`server/internal/store/db.go:65-83` 在 `AutoMigrate` 后补 6 条 `CREATE INDEX IF NOT EXISTS`，失败仅 `slog.Debug("skip index")`；另有 `docs/database/*.sql` 手工脚本。建议索引失败至少 `Warn`，并在 `server/migrations/README.md` 写清两份脚本的关系、适用场景与校验方法。
8. **`.dockerignore` 未排除本地私密配置**：`.dockerignore:1-16` 没有 `server/configs/config.local.yaml`，而 `.gitignore:21` 正是为"本地真实 DSN"准备的文件——构建上下文会把生产凭据传给 docker daemon。建议补 `server/configs/config.local.yaml`。
9. **部署文档 §6 表格细节**：`:206` 写 `read/write_timeout` 默认 60/300，与内置默认 120（`config.go:143-145`）不一致；`:211` 标 `secret_key` "必填"与 `:70-75`"容器可留空自动生成"矛盾；`:198` 宣称"所有字段可用 `AUTOMEDIC_` 前缀环境变量覆盖"而表内大量字段标"—"；`:39-52` 的引用块/代码块夹在表格中间，导致 git / openssh-client / PostgreSQL 三行脱离表格渲染。
10. **容器内无迁移脚本落地物**：`Dockerfile:55` 只拷进 README，§11 的"手工迁移脚本"在容器场景无对应文件（与 P1-1 同源）。
11. **其他文档陈旧**：`README.md:88` 与 `docs/接口文档.md:696` 指向 `server/scripts/smoke.sh`（该脚本用已废弃的 `X-Admin-Token`）；`README.md:154` 称"MySQL 三种部署方式"与 `docs/部署文档.md:191`"SQLite/MySQL 已移除"矛盾；`README.md:101` 称 13 个页面，实际 `web/src/views` 有 21 个 `.vue`（含 User/Role/Tenant/Dict/Forbidden）。
12. **两个 systemd 单元权限不一致**：`deploy/hk/automedic.service:27` 的 `ReadWritePaths=/opt/automedic /opt/automedic/data` 比 `deploy/automedic.service:34` 的 `/opt/automedic/data` 更宽。建议统一为 `data`（`RELEASE` 文件由部署脚本写，不需要服务写权限）。
13. **测试库端口对外发布**：`scripts/test.sh:23-28` 用 `-p 55432:5432` + `automedic/automedic` 弱口令，本机共享环境可被写入。建议 `-p 127.0.0.1:55432:5432`。
14. **entrypoint 注释与实现不符**：`deploy/docker-entrypoint.sh:12-14` 注释"取前 32 字节作为 AES-256 主密钥"，实际写入的是完整 base64 字符串（64 字符），最终由 `config.go:330-340` 截前 32 字符使用（≈192 bit 熵）。建议改为写 32 字节随机值并同步注释。

---

## 上轮已修项复核（含 P1-10 三项）

| 上轮项 | 来源 | 本轮结论 | 证据（file:line） |
|---|---|---|---|
| P1-10a dsh 版本钉死 | 09-09 | **部分修复**：容器路径已钉 `0.1.2-rc.1`；HK 部署链与文档仍 latest | 已钉：`Dockerfile:41-43`、`docker-compose.yml:37`。未钉：`deploy/hk/remote.sh:90-111`、`:390-391`、`docs/部署文档.md:37`、`:42` |
| P1-10b 前端 lockfile | 09-09 | **部分修复**：`web/package-lock.json` 已入库；但镜像构建与部署脚本都不使用它（`npm install`，且不 COPY lock） | 已入库：`rtk git ls-files web/package-lock.json`。未使用：`Dockerfile:14-15`、`scripts/build.sh:35`、`deploy/hk/remote.sh:273` |
| P1-10c compose 默认 DB 口令 | 09-09 | **未修复** | `docker-compose.yml:20`、`:52`、`.env.example:29-30`、`Dockerfile:69` |
| 容器非 root（09-08 P1-14） | 09-08 | 已修复 | `Dockerfile:59-62`（`useradd -u 10001` + `chown -R` + `USER appuser`） |
| JWT 硬编码兜底（09-08 P1-10） | 09-08 | 已修复（无硬编码常量，随机兜底 + 失败 panic） | `server/internal/config/config.go:295-322` |
| release 拒绝默认引导口令（09-09 P1-6） | 09-09 | 已修复，但**缺少变量注入与文档同步**，导致全新 Docker 部署阻断、裸机按文档走则绕过 | `server/internal/store/seed_rbac.go:51-56`、`seed_rbac_test.go:78-85`；缺口见 P0-1 / P0-2 |
| 密钥文件权限 0600 | 09-08/09-09 | 已修复 | `deploy/docker-entrypoint.sh:18`；`deploy/hk/remote.sh:187`、`:232`、`:299` |
| 日志脱敏 | 09-08 P0-7 | 已修复（命令串/环境变量值掩码 + URL 凭据擦除） | `server/internal/dsh/runner.go:361-362`、`server/internal/execx/execx.go:121,194-196` |
| 日志轮转 | 本轮核对 | 部分达标：应用内按天 + `retain`；容器层无上限（见 P2-5） | `server/internal/logging/logging.go:14-16`、`:53-59`；缺口 `docker-compose.yml:32-65` |
| 健康检查与重启策略 | 本轮核对 | 达标 | `Dockerfile:74-75`、`docker-compose.yml:25-30`、`:40`、`:57-65`（`depends_on: service_healthy` + `restart: unless-stopped`） |
| HK 以 root 拉 latest 工具链（09-09 P2） | 09-09 | 未修复（本轮升级为 P1-4） | `deploy/hk/remote.sh:58-111`、`docs/部署文档.md:381` |
| 示例 Nginx 仅 HTTP（09-09 P2） | 09-09 | 未修复：HTTPS/HSTS 仍全注释，默认 `listen 80`；compose 又把 8080 发布到所有网卡 | `deploy/nginx.conf:16`、`:29-37`、`docker-compose.yml:41-42`、`docs/部署文档.md:338` |
| Postgres `sslmode=disable`（09-09 P2） | 09-09 | 未修复 | `docker-compose.yml:52`、`.env.example:30`、`docs/部署文档.md:150`、`:182`、`:188` |
| 迁移双轨（09-08 P2） | 09-08 | 未修复，并新增路径漂移 | `server/internal/store/db.go:65-83`、`docs/database/*.sql`、`server/migrations/README.md:1-3`、缺口见 P1-1 |
| 脚本 `set -euo pipefail` | 本轮核对 | 达标（`scripts/**` 与 `remote.sh` 均具备）；`deploy/docker-entrypoint.sh:5` 仅 `set -e`（POSIX sh，可接受） | `scripts/build.sh:6`、`scripts/dev.sh:8`、`scripts/test.sh:9`、`scripts/check-ui-style.sh:4`、`scripts/deploy-hk.sh:15`、`deploy/hk/remote.sh:7`、`server/scripts/smoke.sh:3` |
| 幂等性（重复执行部署脚本） | 本轮核对 | 基本达标：apt/npm/用户/角色/库/密钥均有 skip 分支；`ensure_go_latest` 例外（每次都向最新版漂移） | `deploy/hk/remote.sh:37-50`、`:114-121`、`:134-160`、`:209-238` |

---

## 验证输出摘要（本轮实际执行的命令与结果）

| 检查项 | 命令 | 结果 |
|---|---|---|
| 基线确认 | `rtk git log -1 --format='%H %s'` | `3c10e9d3d1b02e55f313367cf3e2aa4f4148935f fix(web): 统一 token 与响应式，消除死类名和图表色漂移` ✅ 与任务基线一致 |
| Shell 语法 | `rtk bash -n scripts/build.sh scripts/dev.sh scripts/test.sh scripts/deploy-hk.sh scripts/check-ui-style.sh deploy/docker-entrypoint.sh deploy/hk/remote.sh server/scripts/smoke.sh` | 全部通过（8 个脚本，无输出）✅ |
| POSIX 语法 | `rtk sh -n deploy/docker-entrypoint.sh` | 通过 ✅ |
| Compose 校验 | `rtk docker compose config` | 通过（Docker 29.4.0 / Compose v5.1.2）。输出确认：`AUTOMEDIC_SERVER_MODE: release`；**无** `AUTOMEDIC_AUTH_BOOTSTRAP_PASSWORD`；`POSTGRES_PASSWORD: automedic`；`ports: published 8080`（0.0.0.0）；`restart: unless-stopped`；`depends_on: service_healthy` ✅ 语法有效，同时复现 P0-1 / P1-5 |
| 配置字段 ↔ 结构体 | 自写只读 Python 核对（`server/internal/config/config.go` 解析 `yaml` tag 后与文档/YAML 比对） | `docs/部署文档.md` §6 表格字段全部命中结构体（`read/write_timeout`、`author_name/email` 为合并写法，两字段均存在）；`docs/接口文档.md` §11 PUT 请求体 20 个字段全部命中；三份 YAML（`server/configs/config.yaml` 42 键、`config.docker.yaml` 40 键、`deploy/hk/config.yaml` 39 键）**0 个未匹配键** ✅ |
| env 覆盖面 | 同上（解析 `applyEnv` 白名单 ↔ 文档/示例/脚本中出现的 `AUTOMEDIC_*`） | `applyEnv` 覆盖 20 个键；文档无"凭空变量"；但 52 个配置字段中仅 20 个可由环境变量覆盖，`.env.example:4` 的"所有"表述不成立；Docker 路径实际可注入 12 个（见 P2-1） |
| 路由清单 | 同上（解析 `router.go` 路由 ↔ `docs/接口文档.md` 路径） | `router.go` 84 条路由 vs 文档 45 条路径，18 个 method+path 未记录（清单见 P2-2 与附录） |
| 迁移说明 | `rtk cat server/migrations/README.md`、`rtk ls -la server/migrations docs/database` | `server/migrations/` 仅 `README.md`（129 B）；SQL 在 `docs/database/001…006`；`README.md` 自述"表结构由 GORM AutoMigrate 幂等创建"，与 `store/db.go:65-83` 一致，但与 `docs/部署文档.md:138`/`Dockerfile:55` 的路径引用不一致（P1-1） |

> 说明：本机存在 docker CLI 与 compose 插件，因此按要求执行了 `docker compose config`（纯静态解析，未启动任何容器、未拉取镜像、未改动任何环境）；未执行部署、未连接任何线上主机。

---

## 附：核对明细（供交叉验证）

- **接口清单**：`router.go` 有而 `docs/接口文档.md` 未记录的 18 个 method+path —— `GET /api/v1/auth/profile`、`POST /api/v1/auth/logout`、`POST /api/v1/auth/refresh`、`PUT /api/v1/auth/password`、`GET /api/v1/permissions`、`GET /api/v1/repos/:id/reviews`、`GET|POST|PUT|DELETE /api/v1/users`（含 `/:id`）、`GET|POST|PUT|DELETE /api/v1/roles`（含 `/:id`）、`GET|POST|PUT|DELETE /api/v1/tenants`（含 `/:id`）。（`POST /api/v1/auth/login` 只在 §1.2 文字中出现，表格与方法清单均无。）
- **配置字段**：`docs/部署文档.md` §6 未覆盖的结构体字段 —— `auth.jwt_secret`、`auth.access_token_ttl`、`auth.refresh_token_ttl`、`auth.bootstrap_admin.*`、`server.allow_origins`、`server.trusted_proxies`、`ocr.model_id`、`dsh.system_guard`、`git.bin`（`ocr.model_id` 另见 `docs/ocr审查.md:45-47`）。
- **env 键全量（20）**：`AUTOMEDIC_SERVER_ADDR/MODE/WEB_DIR/ADMIN_TOKEN/ALLOW_ORIGINS/TRUSTED_PROXIES`、`AUTOMEDIC_DB_DRIVER/DSN`、`AUTOMEDIC_AUTH_JWT_SECRET`、`AUTOMEDIC_AUTH_BOOTSTRAP_USERNAME/PASSWORD`、`AUTOMEDIC_SECURITY_SECRET_KEY`、`AUTOMEDIC_SECURITY_SECRET_KEY_FILE`、`AUTOMEDIC_DSH_BIN/HOME/PERMISSION_MODE`、`AUTOMEDIC_OCR_BIN`、`AUTOMEDIC_GIT_WORKSPACE_ROOT/BIN`、`AUTOMEDIC_LOG_LEVEL`。
