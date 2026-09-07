# AutoMedic

> 事件驱动的 **DeepSeek Harness（dsh）** 全自动 / 半自动 Bug 修复平台。
> 生产告警进来了，AutoMedic 自动判定是不是代码问题、自动在隔离工作区里让 dsh 改代码、给出补丁，你只需要点一下「确认」。

---

## 它解决什么

| 痛点 | AutoMedic 的做法 |
| --- | --- |
| 告警多、噪音多，业务拒绝/第三方故障也来打扰开发 | 规则引擎：级别/来源/关键字/正则 + **排除词**（第三方限流、上游超时、权限不足…）+ 频次阈值 + 冷却窗口 |
| 半夜出故障没人处理 | 事件入站即触发修复，dsh headless 全自动跑完 |
| 不敢让 AI 直接推代码 | 隔离 git 工作区 + `workspace-write` 权限模式 + **半自动确认**（默认），确认后才 commit/push |
| 多项目多仓库，凭证散落 | 项目→多仓库结构，凭证中心（SSH/账号密码/Token）AES 加密、跨项目复用 |
| 模型上下文一刀切 | 每个模型独立配置**输入/输出上下文**，通过 dsh `--patch` 层注入 |
| 修了什么、修没修好说不清 | 全程终端日志 + 结果摘要（根因/改动文件/diff/验证方式/置信度）+ 多维统计报表 |

---

## 核心链路

```
告警源 ──X-AM-Token──► 事件入站 ──► 规则过滤 ──► 生成任务
                                                   │
                                    ┌──────────────┴──────────────┐
                                    ▼                             ▼
                            准备隔离工作区                    dropped/ignored
                            (clone/fetch + fix 分支)          （非代码问题）
                                    │
                                    ▼
                       dsh --profile headless
                       （模型/上下文经 --patch 注入，仅工作区可写）
                                    │
                        流式日志 ──► WebSocket ──► 浏览器终端
                                    │
                                    ▼
                        解析 .automedic/result.json
                                    │
                    ┌───────────────┴────────────────┐
                    ▼                                ▼
              no_code_change                    有代码改动
              → ignored（附诊断）        ┌─────────┴─────────┐
                                        ▼                   ▼
                                   半自动(semi)         全自动(auto)
                                   confirming           commit → push
                                   人工确认/驳回          → release_hook
```

---

## 快速开始

### Docker（推荐）

```bash
cp .env.example .env
# 生成并填入 AUTOMEDIC_ADMIN_TOKEN；AUTOMEDIC_SECRET_KEY 建议留空，容器会自动生成
openssl rand -hex 24

docker compose up -d
open http://localhost:8080
```

### 本地开发

```bash
# 后端 :8080 + 前端 :5173（热更新）
./scripts/dev.sh

# 或只跑后端（需先构建前端）
./scripts/build.sh
cd server && ./bin/automedic-server -config configs/config.yaml
```

前置依赖：**Go ≥ 1.23**、**Node ≥ 20**、**dsh**（`npm i -g @deepseek-ai/dsh`）、git。

---

## 目录结构

```
automedic/
├── server/                      # Go 后端
│   ├── cmd/server/main.go       # 入口
│   ├── configs/                 # config.yaml（本地）/ config.docker.yaml（容器）
│   ├── migrations/              # MySQL 建表 + 种子数据 SQL
│   ├── scripts/smoke.sh         # 端到端冒烟脚本
│   └── internal/
│       ├── api/                 # Gin 路由与 Handler
│       ├── config/              # 配置加载（AUTOMEDIC_ 环境变量覆盖）
│       ├── crypto/              # AES-256-GCM 加密 / SHA256
│       ├── dsh/                 # ★ dsh headless 调用封装 + 任务文本生成
│       ├── execx/               # 流式命令执行（进程组、超时、逐行回调）
│       ├── git/                 # 隔离工作区（clone/commit/push/凭证注入）
│       ├── model/               # GORM 模型
│       ├── service/             # 规则引擎 / 事件入站 / 任务执行器 / 日志写入
│       ├── store/               # 数据库（SQLite/MySQL）、迁移、默认数据
│       └── ws/                  # WebSocket Hub（任务终端广播）
├── web/                         # Vue3 + Element Plus 前端
│   └── src/views/               # 13 个页面：概览/项目/仓库/规则/凭证/模型配置/
│                                #   令牌/事件/任务/任务详情/统计/设置
├── deploy/                      # systemd 单元、Nginx 反代、可选 ingest sidecar
├── scripts/                     # build.sh / dev.sh / test.sh
├── docs/                        # 部署 / 接口 / dsh 集成 / 采集接入（Sentry、Loki）
├── Dockerfile                   # 三段式：前端 → 后端 → 运行时(node+git)
└── docker-compose.yml
```

---

## 功能模块

| 模块 | 说明 |
| --- | --- |
| **事件监听与任务管理** | 令牌鉴权的 HTTP 投递接口；8 种任务状态；全自动/半自动双模式；worker 池 + 崩溃恢复 |
| **修复策略与过滤** | 优先级规则匹配；排除关键字拦截业务拒绝与第三方故障；频次阈值抑制抖动；指纹冷却 |
| **多仓库项目管理** | 项目 → 多仓库；仓库可绑定凭证与模型；项目详情可浏览远端目录树；`repo_hint` 缩小定位范围 |
| **凭证中心** | SSH 私钥 / 账号密码 / HTTP Token；AES-256-GCM 加密；跨项目复用；使用记录可追溯 |
| **大模型配置中心** | 9 个内置厂家 + 自定义；每个模型独立配置输入/输出上下文、温度、最大轮次、额外参数 |
| **项目令牌** | `am_` 前缀随机令牌，只存 SHA256；支持过期时间与来源 IP CIDR 白名单 |
| **修复记录与统计** | 完整终端日志、补丁 diff、根因摘要；趋势图与项目/仓库/状态/规则/来源/模型多维分组统计 |

---

## 自动化测试

```bash
./scripts/test.sh            # go vet + go test ./...
./scripts/test.sh -run TestE2E -v
```

端到端测试用「假 dsh」替换真实 Harness（不调用任何大模型），覆盖：

| 用例 | 验证内容 |
| --- | --- |
| `TestE2EAutoFixCommitPushRelease` | 全自动：事件入站 → 规则命中 → 工作区 → 提交 → 推送 → 发布钩子；远端分支内容正确；平台指令文件未被误提交 |
| `TestE2ESemiConfirmThenPush` | 半自动：先进入 `confirming`，确认前不推送，确认后提交 + 推送 + 触发钩子 |
| `TestE2EReject` | 驳回后不产生 commit |
| `TestE2ENoCodeChangeIgnored` | dsh 判定无代码变更 → `ignored`，不触发发布钩子 |
| `TestE2EWorkspaceCleanupAfterReuse` | 工作区复用时基线正确、无上次修复残留 |
| `TestRunOutPassesEnv`（git 包） | 回归：git 子进程必须拿到 `GIT_SSH_COMMAND` 等凭证环境变量 |
| `TestAuthEnvSSHKey` / `TestAuthEnvHTTPToken` | SSH 私钥与 HTTP Token 两种凭证注入形态、私钥 `0600` 与清理 |
| `TestCommitUsesAuthorIdentity` | 提交者身份来自配置，而非本机 git config |
| `TestListRemoteTreeAndShowFile` | 远端目录树浅取、过滤 `node_modules`、文本预览与路径穿越拒绝 |

---

## 文档

| 文档 | 内容 |
| --- | --- |
| [docs/部署文档.md](docs/部署文档.md) | Docker / 裸机 / MySQL 三种部署方式、配置项全解、初始化清单、运维排障、安全基线 |
| [docs/接口文档.md](docs/接口文档.md) | 全部 REST 接口与 WebSocket 协议、请求/响应示例、状态机、端到端 curl 示例 |
| [docs/dsh集成说明.md](docs/dsh集成说明.md) | dsh headless 调用形态、`--patch` 模型上下文注入、日志采集链路、结果解析约定 |
| [docs/采集接入.md](docs/采集接入.md) | Sentry / Loki 字段映射、Grafana webhook 模板、可选 sidecar |

---

## 安全提示

- `security.secret_key` 用于加密凭证与 API Key，**上线后不可随意更改**（更换会导致已存数据无法解密）。
- `dsh.permission_mode` 默认 `workspace-write`，dsh 只能在隔离工作区内写文件。生产环境不要改为 `danger-full-access`。
- 管理令牌 `server.admin_token` 与投递令牌请分别配置，投递令牌建议绑定来源 IP 与过期时间。

---

## 许可

内部项目，按需调整。
