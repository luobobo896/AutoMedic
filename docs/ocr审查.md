# 仓库审查（Open Code Review）

> AutoMedic **只把 OCR 当审查器**，不替代 `dsh` 修 bug。
> 入口：项目详情 / 仓库列表 → **审查**。人勾选意见后再走平台半自动修复。

官方 CLI：[alibaba/open-code-review](https://github.com/alibaba/open-code-review/blob/main/README.zh-CN.md)

---

## 1. 边界

| 做 | 不做 |
| --- | --- |
| 人点「审查」→ 服务端浅取仓库 → `ocr review` / `ocr scan --path` | 不把 OCR 塞进 dsh 主链路 |
| 抽屉展示 findings，勾选后创建修复任务 | 不根据 findings 自动 Enqueue |
| 修复任务 `source=ocr`，**始终 semi** | 不整仓默认 `ocr scan`、不自动 push |

审查用的 LLM **来自平台「大模型配置中心」**，启动时注入 `OCR_LLM_URL` / `OCR_LLM_TOKEN` / `OCR_LLM_MODEL`，不要求再跑 `ocr config provider`。

模型选择（前一项非空即停）：

1. 仓库 `review_model_id`
2. 项目 `default_review_model_id`
3. 仓库修复模型 `model_id`
4. 项目 `default_model_id`
5. 全局默认模型

厂家未配 API Key、或自定义厂家未填 Base URL 时任务失败并回显原因。

`ocr review --from/--to` 只审相对基线的 diff。**已合入当前分支的预埋问题不会出现在 diff 里**，必须用 `ocr scan --path`。平台拒绝 `from` 与当前分支相同（否则会空跑成功、0 条意见）。diff 审查会取完整 from/to 历史以便 `git merge-base`；本机 Git ≥ 2.41。失败时 `error_msg` 只保留人能看懂的原因（超时、merge-base、缺密钥），不贴 `▶ code_search` 一类工具流水。超时但已经写出意见时按成功回收。列表展示短摘要，点「详情」看完整问题。运行中 `progress` 为当前步骤，`logs` 为过程摘要；OCR 进度来自 session jsonl。

---

## 2. 安装

```bash
npm install -g @alibaba-group/open-code-review
```

要求 Git ≥ 2.41。模型与密钥在 Web「大模型配置中心」配置，与 dsh 同一套；项目/仓库可另选审查模型。平台进程配置：

| 项 | 默认 | 环境变量 |
| --- | --- | --- |
| `ocr.bin` | `ocr` | `AUTOMEDIC_OCR_BIN` |
| `ocr.timeout_sec` | `600` | — |
| `ocr.env` | 空 | 传给 ocr 进程的额外 `KEY=VALUE`（不要用来配第二套 API Key） |

Web「系统设置 → Open Code Review」可热改。

---

## 3. 流程

```text
人选择范围（扫描已合入路径，或 diff from→当前分支；from 不能等于当前分支）
  → POST /api/v1/repos/:id/review   立即返回 job（pending）
  → 服务端准备工作区（scan 浅取；diff 审查取完整 from/to 历史以便 merge-base）+ ocr --format json --output
  → GET /api/v1/reviews/:id         轮询至 success/failed（running 时带 progress / logs，不是空转圈）
  → 勾选 keys
  → POST /api/v1/reviews/:id/fix    为当前仓库创建 semi 任务并入队
```

修复任务跳过规则 / 频次 / 冷却。dsh prompt 仍用现有事件字段：标题、`path:line`、意见正文。

整仓 `scan_all=true` 仅 API 提供，Web 默认不开放。
