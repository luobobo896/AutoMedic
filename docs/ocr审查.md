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

OCR 有自己的 LLM 配置（`ocr config provider`），与平台「大模型配置中心」无关。未安装或未配置时，审查任务失败并回显错误，不会假装成功。

---

## 2. 安装

```bash
npm install -g @alibaba-group/open-code-review
ocr config provider
ocr config model
```

要求 Git ≥ 2.41。平台配置：

| 项 | 默认 | 环境变量 |
| --- | --- | --- |
| `ocr.bin` | `ocr` | `AUTOMEDIC_OCR_BIN` |
| `ocr.timeout_sec` | `600` | — |
| `ocr.env` | 空 | 传给 ocr 进程的 `KEY=VALUE` |

Web「系统设置 → Open Code Review」可热改。

---

## 3. 流程

```text
人选择范围（diff from→当前分支，或指定 path）
  → POST /api/v1/repos/:id/review   立即返回 job（pending）
  → 服务端浅 clone + ocr --format json --output
  → GET /api/v1/reviews/:id         轮询至 success/failed
  → 勾选 keys
  → POST /api/v1/reviews/:id/fix    为当前仓库创建 semi 任务并入队
```

修复任务跳过规则 / 频次 / 冷却。dsh prompt 仍用现有事件字段：标题、`path:line`、意见正文。

整仓 `scan_all=true` 仅 API 提供，Web 默认不开放。
