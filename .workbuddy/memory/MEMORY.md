# AutoMedic 项目长期记忆

## 项目定位
事件驱动的 **DeepSeek Harness（dsh）** 全自动/半自动 Bug 修复平台。
后端 Go 1.23（Gin + GORM + SQLite/MySQL），前端 Vue3 + Element Plus + ECharts。

## 硬性约束（不可违反）
1. **只调用官方 CLI `dsh --profile headless`**，不 import dsh 内部包、不内嵌 dsh Web UI。
2. **headless 只接受一个 task 文本参数** —— 模型 / 输入上下文 / 输出上下文必须经 cordis `--patch` 层注入（`agent-default-model` 的 provider / model / inputContextTokens / outputContextTokens），禁止硬编码到命令行。
3. 修复过程必须在隔离 git 工作区内进行，`dsh.permission_mode` 保持 `workspace-write`，不得改为 `danger-full-access`。
4. 修复过程以终端日志呈现（execx 逐行 → WS Hub → 批量落库 task_logs），完成后展示结果摘要。

## 关键约定
- 配置：`server/configs/config.yaml`（本地）/ `config.docker.yaml`（容器）。`AUTOMEDIC_` 前缀环境变量覆盖，优先级 环境变量 > 配置文件 > 默认值。
- `security.secret_key`（AES-256-GCM 主密钥）一旦上线不可随意更换，否则已存凭证/API Key 无法解密。
- 模型输入/输出上下文**逐模型独立配置**，渲染进 `--patch` 传给 dsh。
- 平台写入工作区的 `AUTOMEDIC.md` / `AGENTS.md` 需记录到 `.automedic/managed-files.json`，提交前精准清理，否则被误判为代码改动。
- `no_code_change` 判定优先级：result.json 字段 > git status 是否有改动。命中则任务置 `ignored`，不提交不推送。
- 任务状态机：pending → running → confirming(半自动) → success / failed / ignored / cancelled / rejected。

## 环境注意
- 前端 `web/node_modules` 是到 `/tmp/am-build/node_modules` 的软链（broker 沙箱拒绝在项目目录 install）。`/tmp` 清空后需重新安装。
- 构建前端：`node node_modules/vite/bin/vite.js build`。
- 本机 dsh 由 fnm 的 node v25.9.0 全局安装，路径 `~/.local/share/fnm/node-versions/v25.9.0/installation/bin/dsh`，需在 `dsh.env` 的 PATH 中。

## 主要文档
- `docs/部署文档.md` — Docker / 裸机 / MySQL 部署、配置全解、排障
- `docs/接口文档.md` — 全部 REST + WS
- `docs/dsh集成说明.md` — dsh 调用形态、patch 注入、日志采集、结果解析
