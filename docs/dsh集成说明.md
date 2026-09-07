# AutoMedic × DeepSeek Harness（dsh）集成说明

> 本文说明 AutoMedic 如何调用官方 `dsh --profile headless`、如何注入模型与上下文、如何采集终端日志、如何解析修复结果。
> 对应源码：`server/internal/dsh/`（`runner.go` 调用封装、`prompt.go` 任务文本生成）、`server/internal/execx/`（流式执行）、`server/internal/git/`（隔离工作区）。
> 对应配置：`server/configs/config.yaml` 的 `dsh:` 段（可在 Web「系统设置」页热改，重启后回落到配置文件）。

---

## 1. 集成原则（硬性约束）

| 约束 | 实现方式 |
| --- | --- |
| 只调用官方 CLI | 全程 `exec` 外部 `dsh` 进程，不 import 任何 dsh 内部包。仓库「审查」另走官方 `ocr` CLI，不替代 dsh |
| 只用 headless profile | 命令模板固定带 `--profile headless` |
| 隔离工作区 | 每次修复在 `git.workspace_root/<project>/<id>-<repo>` 独立目录执行，`dsh` 进程 `cwd` 即该目录（dsh 的 sandbox workspaceRoot 取进程 cwd） |
| 不内嵌 Web UI | 平台只读取 dsh 的 stdout/stderr，不启动、不代理 dsh 的交互式界面 |
| 过程终端化 | stdout/stderr 逐行采集 → WebSocket 广播（`ws.Hub`）→ 批量落库 `task_logs` |
| 结果结构化 | 解析 `.automedic/result.json`，兜底解析输出中的文本块 |

---

## 2. 调用形态

### 2.1 命令模板

配置字段 `dsh.command_template`（Go template 风格，占位符为字符串替换，非 text/template）：

```
{{.Bin}} --profile headless {{.Patches}} "$(cat {{.TaskFile}})"
```

渲染后实际执行的命令形如：

```bash
/usr/local/bin/dsh --profile headless \
  --patch /tmp/am-dsh-xxxx/model.patch.yml \
  "$(cat /tmp/am-dsh-xxxx/task.txt)"
```

| 占位符 | 含义 |
| --- | --- |
| `{{.Bin}}` | `dsh.bin`，dsh 可执行文件路径 |
| `{{.Profile}}` | 固定 `headless` |
| `{{.Patches}}` | 渲染后的 `--patch <file>` 串 |
| `{{.TaskFile}}` | 任务文本文件路径 |

**为什么 headless 只传一个参数**：headless profile 的入口签名只接受一段 task 文本（不支持交互式多轮输入）。因此模型、上下文、额外参数**不走命令行**，而是通过 cordis 的 `--patch` 覆盖层注入。这也是 AutoMedic 不把 `--model` 之类的参数硬编码在模板里的原因——版本演进时改配置即可，无需改代码。

`dsh.use_shell: true` 时命令经 `/bin/sh -c` 执行（模板含 `$(cat …)` 与引号，必须走 shell）；置为 `false` 则由内置 `shlex` 分词后直接 exec。

### 2.2 Patch 覆盖层（模型与上下文注入）

配置字段 `dsh.patch_template`：

```yaml
- id: agent-default-model
  config:
    provider: {{.Provider}}
    model: {{.Model}}
    inputContextTokens: {{.InputContext}}
    outputContextTokens: {{.OutputContext}}
{{.ExtraYAML}}
```

渲染结果（示例）：

```yaml
- id: agent-default-model
  config:
    provider: deepseek-official
    model: deepseek-v4-flash
    inputContextTokens: 1048576
    outputContextTokens: 131072
```

取值来源与优先级：

| 字段 | 来源 | 缺省兜底 |
| --- | --- | --- |
| `provider` | `providers.key`（厂家标识，如 `deepseek-official`） | `deepseek-official` |
| `model` | `llm_models.slug`（传给 dsh 的模型标识） | 空 |
| `inputContextTokens` | 模型的 `input_context` | `131072` |
| `outputContextTokens` | 模型的 `output_context` | `65536` |
| `{{.ExtraYAML}}` | 模型 `extra_params`（JSON）自动平铺为 YAML 键值对 | 空 |

模型的**输入/输出上下文是逐模型独立配置的**（Web「模型配置」页），这正是需求中「每个模型独立配置输入输出上下文大小」的落点：平台把它渲染进 patch 层交给 dsh，而不是写死在命令行。

---

## 3. 任务文本（Prompt）构造

`server/internal/dsh/prompt.go` 的 `BuildTaskText()` 生成**自包含**的任务文本，dsh 在完全无上下文的 headless 进程中也能独立工作：

```
【生产事件】
  来源 / 级别 / 标题 / 时间 / 指纹
  正文
  堆栈（截断）

【项目背景】
  project.context（业务语义、架构约定）

【目标仓库】
  名称 / URL / 分支 / 语言 / 关注路径（code_paths）

【仓库结构】
  目录树（深度 2，跳过 .git）

【修复要求】
  1. 只在当前工作区内修改代码，禁止访问工作区之外
  2. 最小改动，不重构、不改无关文件
  3. 必须能编译/通过已有测试
  4. 无法定位或非代码问题 → 输出 no_code_change=true，不要改任何文件

【输出格式】
  写入 .automedic/result.json，并在最后输出：
  ===AUTOMEDIC_RESULT===
  SUMMARY: <一句话>
  DIAGNOSIS: <根因>
  FILES: a.go, b.go
  CONFIDENCE: high|medium|low
  VERIFICATION: <验证方式>
  NO_CODE_CHANGE: true|false
  ===AUTOMEDIC_RESULT_END===
```

规则可挂 `prompt_template` 覆盖追加内容（如"必须补充单元测试"、"禁止修改 migrations 目录"）。

---

## 4. 环境变量

`Runner.buildEnv()` 构造的最小环境：

| 变量 | 值 | 说明 |
| --- | --- | --- |
| `DSH_PERMISSION_MODE` | `workspace-write`（可配） | **关键安全措施**：dsh 只能写隔离工作区 |
| `DSH_TELEMETRY_MODE` | `DISABLED` | 关闭遥测 |
| `DSH_HOME` | 可选 | dsh 配置/凭证目录 |
| `NO_COLOR` / `FORCE_COLOR` | `1` / `0` | 输出不带 ANSI，终端展示更干净 |
| `TERM` / `CI` | `dumb` / `1` | 非交互模式 |
| `<PROVIDER>_API_KEY` | 厂家 API Key | 见下表 |
| `dsh.env` 中的自定义项 | KEY=VALUE | 追加/覆盖 |

### 厂家 API Key 环境变量映射

在「模型配置 → 厂家」填写的 API Key 以 **AES-256-GCM 加密**落库，运行时解密后仅通过环境变量传给 dsh 子进程，**不写日志、不落库、不返回前端**：

| 厂家 kind | 环境变量 |
| --- | --- |
| deepseek | `DEEPSEEK_API_KEY` |
| openai | `OPENAI_API_KEY` |
| anthropic | `ANTHROPIC_API_KEY` |
| gemini | `GEMINI_API_KEY` |
| qwen | `DASHSCOPE_API_KEY` |
| zhipu | `ZHIPU_API_KEY` |
| moonshot | `MOONSHOT_API_KEY` |
| doubao | `ARK_API_KEY` |
| 其他 | `<KEY 大写化>_API_KEY`（`-`/空格转 `_`） |

> 厂家不填 API Key 时，平台不注入该变量，dsh 会回落到自身凭证文件（`DSH_HOME`）。

---

## 5. 日志采集链路

```
dsh 子进程 stdout/stderr
      │  execx.Run 逐行读取（4MB 缓冲，独立进程组）
      ▼
Sink(stream, line)                 // stream: stdout | stderr | sys
      ├─► ws.Hub.Publish(taskID)   // 实时广播到浏览器终端
      └─► service.LogWriter        // 累积 → 满 30 条或 1s → 批量 INSERT task_logs
```

- 前端 `TaskDetail` 优先走 WebSocket `/ws/tasks/:id`；断线自动降级为轮询 `GET /api/v1/tasks/:id/logs?after_seq=N`。
- 所有写入命令前会打一行 `sys` 日志：`[dsh] $ <command>`，且经 `redactSecrets()` 脱敏（`KEY`/`TOKEN` 类变量值替换为 `前3***后3`）。
- 单次执行超时：`dsh.timeout_sec`（默认 1800s），超时杀整个进程组，避免残留子进程。

---

## 6. 结果解析

`Runner.ParseResult()` 按以下优先级：

1. **`.automedic/result.json`**（首选）

```json
{
  "summary": "空指针因未判空导致，已补充 nil 检查并加测试",
  "diagnosis": "order.GetUser() 在订单无用户时返回 nil，后续直接访问 .ID 触发 panic",
  "changed_files": ["internal/service/order.go", "internal/service/order_test.go"],
  "confidence": "high",
  "verification": "go test ./internal/service/... 通过",
  "no_code_change": false
}
```

2. **输出文本块**（兜底）：解析 `SUMMARY:` / `DIAGNOSIS:` / `FILES:` 等行。
3. 都没有 → 取输出末 40 行作为 summary。

解析结果写入 `tasks` 表：`summary`、`diagnosis`、`changed_files`、`patch`、`diff_stat`、`dsh_exit_code`、`dsh_cmd`、`input_context`/`output_context`、`duration_ms`。

---

## 7. 工作区与"无代码变更"判定

平台会在工作区写入指令文件（`AUTOMEDIC.md` / `AGENTS.md`）帮助 dsh 理解约定。为避免这些文件被误判为"修复改动"：

- `WriteInstruction()` **只新建原本不存在的文件**，并把新建清单写入 `.automedic/managed-files.json`；
- 提交前 `CleanupArtifacts()` 按清单精准删除这些文件，并移除 `.automedic/` 目录；
- `no_code_change` 判定优先级：**`result.json.no_code_change` → `git status` 是否有改动**。

`no_code_change = true` 的处理：任务标记 `ignored`（非代码问题 / 无法定位），**不产生补丁、不提交、不推送**，在 Web 上可查看 dsh 给出的诊断说明。

---

## 8. 全自动 / 半自动流程

```
Enqueue(taskID)
   │
   ├─ prepare     解析模型 / 解密凭证 / 准备隔离工作区（clone 或 fetch，切 automedic/fix-<id> 分支）
   ├─ dsh         Runner.Run（流式日志 + 结果解析）
   ├─ verify      git diff 判定改动 → 生成 patch / diff_stat
   │
   ├─ 半自动(semi) → 状态 confirming，等待人工在 Web 上「确认」或「驳回」
   │       ├─ 确认 → 重建工作区 + git apply 补丁 → commit → push → 发布钩子
   │       └─ 驳回 → 状态 rejected，工作区回退
   │
   └─ 全自动(auto) → 直接 commit → push（受 git.auto_push 控制）→ 发布钩子
```

「发布」= 可配置命令 `git.release_hook`（项目 `release_hook` 非空时覆盖），在 **push 成功之后**、隔离工作区目录内由 `/bin/sh -c` 执行。不是独立发布流水线。

崩溃恢复：服务启动时 scanner 会把残留的 `running` 任务重新置为 `pending` 并重新入队。

---

## 9. 配置速查

```yaml
dsh:
  bin: "/usr/local/bin/dsh"                 # 建议绝对路径
  home: ""                                  # 留空用 dsh 默认
  command_template: '{{.Bin}} --profile headless {{.Patches}} "$(cat {{.TaskFile}})"'
  use_shell: true
  timeout_sec: 1800
  permission_mode: "workspace-write"        # 强烈建议不要改成 danger-full-access
  instruction_file: "AUTOMEDIC.md"
  output_format: "text"
  patch_template: |-
    - id: agent-default-model
      config:
        provider: {{.Provider}}
        model: {{.Model}}
        inputContextTokens: {{.InputContext}}
        outputContextTokens: {{.OutputContext}}
    {{.ExtraYAML}}
  env:
    - "DSH_TELEMETRY_MODE=DISABLED"
    - "PATH=/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
```

环境变量覆盖：`AUTOMEDIC_DSH_BIN`、`AUTOMEDIC_DSH_HOME`、`AUTOMEDIC_DSH_PERMISSION_MODE`。

---

## 10. 手动验证命令

```bash
# 1. 确认 dsh 可用
dsh --version

# 2. 直接跑一次 headless（不含 patch 注入）
dsh --profile headless "分析当前仓库并输出一句话摘要"

# 3. 通过平台投递一个测试事件（<TOKEN> 在 Web「项目详情 → 令牌」生成）
curl -X POST http://127.0.0.1:8080/api/v1/ingest/events \
  -H "X-AM-Token: <TOKEN>" -H "Content-Type: application/json" \
  -d '{"source":"custom","level":"error","title":"测试：空指针","message":"panic: runtime error: invalid memory address","stack":"main.main()\n\t/app/main.go:12"}'

# 4. 观察终端输出
open http://127.0.0.1:8080/#/tasks/1
```

完整端到端脚本：`server/scripts/smoke.sh`。
