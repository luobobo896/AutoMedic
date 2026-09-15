# AutoMedic 代码审查报告（第 2 路：任务执行与并发）

- 审查时间：2026-09-15
- 范围：`server/internal/service/**`（rule / ingest / task / review 执行链路与状态机）、`server/internal/execx/**`、`server/internal/git/**`、`server/internal/dsh/**`、`server/internal/logging/**`、`server/cmd/server/main.go`、`configs/`（dsh.timeout_sec、permission_mode、worker 并发等）
- 基线：`3c10e9d`（`fix(web): 统一 token 与响应式，消除死类名和图表色漂移`，2026-09-09 15:54）
- 方法：源码通读 + 规则基线（30-concurrency / 25-go / 19 / 12 / 13 / 14）对照 + 在 **仓库副本**（`/tmp`）中补 5 个临时用例做实证复现 + 执行既有测试
- 约束：只读审查；未修改任何产品代码，唯一产出是本报告

## 结论（先看这个）

执行链路的主干（原子抢占、仓库级串行锁、进程组回收、指令文件清单清理、SSH 口令走环境变量）在上一轮之后确实修好了，`go test -race` 在现有用例上全绿。

但**执行结果的正确性**仍有三个硬伤，而且都已在本轮实证复现：

1. **半自动确认会把别的任务的改动提交并推送到自己的分支，自己的补丁丢失**，任务照样标 `success`（P0-1）。
2. **dsh 非零退出（含超时、启动失败）被当成成功**：推一个与基线完全相同的空分支，事件闭环标记为「已修复」，随后**同指纹的真实告警被去重丢弃**，平台从此不再处理该故障（P0-2）。
3. 触发上面第 2 条判定的「无代码变更」检测本身是失效的：平台自己写下的 `AUTOMEDIC.md` / `AGENTS.md` 让 `git status --porcelain` 永不为空（P1-1）。

第 2、3 条叠加后，**dsh 崩溃 = 静默假成功 + 告警被永久抑制**，这是本轮最该先修的一条。

---

## P0 — 必须先修

### P0-1 半自动确认提交/推送别的任务的改动，本任务的补丁丢失

- 位置：`server/internal/service/task.go:511-523`（Confirm 的变更判定）、`server/internal/service/task.go:402-406`（finalize「已有提交」分支）、`server/internal/git/workspace.go:86`（工作区目录仍是 `projectKey/repoID-repoName`，无 taskID）
- 证据（在仓库副本中复现，产品代码未改）：
  1. 同仓库先跑任务 A（semi）→ `confirming`，补丁入 `tasks.patch`；工作区被 A 的未提交改动占用；
  2. 再跑任务 B（semi）→ `Execute` 的 `Prepare` 在该共享目录里 `checkout -f` + `reset --hard` + `clean -fd`，A 的改动被清掉，B 留下自己的未提交改动；
  3. 人对 A 点「确认」→ A 的分支上出现的是 **B 的 `other.go`**，A 的 `service.go` 修复缺失，A 仍标 `success`：

     ```
     A 最终 status=success fix_commit=cadca54c...
     远端 automedic/fix-1-...:other.go => "package main"          # B 的产物
     远端 automedic/fix-1-... 上 A 的修复缺失，service.go 仍是：
       func Level(u *User) string { return u.Profile.Level }      # 原始缺陷代码
     ```
  4. 变体同样复现：B 先完成推送，A 再确认 → `ChangedFiles` 为空且 `HEAD != task.BaseCommit`，代码走「工作区已提交，跳过 commit」分支，A 的分支直接指向 **B 的提交**。
- 影响：确认动作会把不属于本任务的改动提交进目标分支（可能是半成品、可能是另一个未审的修复），本任务的补丁静默丢失，任务与事件却记录为「修复成功」；人工确认这道安全闸门在并发场景下失去意义。
- 建议（按成本从低到高）：
  1. Confirm 前强校验 `wsDir` 的当前分支 == `task.Branch`、`HEAD == task.BaseCommit`、`ChangedFiles` 为空，任一不满足就**拒绝**（而不是复用工作区）并走重建路径；
  2. 工作区目录带 `taskID`（`projectKey/<taskID>-<repoID>-<repoName>`），从根上消除「同一目录被多个任务的状态复用」；
  3. 半自动任务在 `confirming` 期间对工作区加「保留」标记（例如 `.automedic/hold`），`Prepare` 遇到保留标记直接换目录。

### P0-2 dsh 非零退出被判成功，推空分支，并让同指纹告警被永久去重

- 位置：`server/internal/service/task.go:328-332`（`if runErr != nil && rr == nil` 恒不成立）、`server/internal/dsh/runner.go:121-131`（`Run` 永远返回非 nil 的 `*RunResult`）、`server/internal/service/task.go:173-176` + `396-406`（Execute 路径 `task.BaseCommit` 仍为空串，`sha == task.BaseCommit` 永不成立，于是走「已有提交，跳过 commit」）、`server/internal/service/rule.go:142-146`（同指纹已成功则过滤后续事件）
- 证据（仓库副本，假 dsh 直接 `exit 1`，不改任何文件）：

  ```
  dsh exit!=0 且无代码变更 => status=success stage=done err="" exit=1
  changed_files=null patch=0 diff_stat="" summary="[fake-dsh] 启动失败：无法连接模型服务"
  远端分支: automedic/fix-1-20260915-150102
  远端 master..automedic/fix-1-... diff: ""        # 空 diff：推了一个等于基线的分支
  同指纹再次投递 => action=dropped reason="同指纹已由任务 #1 修复成功，已过滤" tasks=[]
  ```

  真 dsh 的超时走 `execx` 的 `DeadlineExceeded`（`execx.go:117-119`），同样落进这条路径。
- 影响：① 任务与事件状态假成功，终端里只有 dsh 的报错输出，列表显示「修复成功」；② 远端多出一个无意义的修复分支；③ 因为 `FingerprintBlock` 认定「同指纹已修复成功」，**后续同样的真实告警会被静默丢弃**，故障再也不会被处理——这是本报告最严重的一条。
- 建议：
  1. `runErr != nil`（非零退出、超时、信号）一律判 `failed`，把 `ExitCode`、错误原因写入 `error_msg`；删除 `&& rr == nil` 这个死条件；
  2. Execute 路径在调用 `finalize` 前把 `base_commit` 先落库（或直接把 `run` 里算出的 `res.BaseCommit` 传给 `finalize`），不要让「HEAD 是否等于基线」的判断依赖一个空串；
  3. `finalize` 里「无本地变更但 HEAD ≠ 基线」这条复用路径，应只允许 Confirm/重试推送场景（`CanResumeFinalize`），Execute 首跑场景直接判失败。

---

## P1 — 高风险或缺闭环

### P1-1 「无代码变更」检测失效：平台自己的指令文件让工作区永不为空

- 位置：`server/internal/service/task.go:335-339`（在 `CleanupArtifacts` 之前调 `ChangedFiles` 判定 `noChange`）、`server/internal/dsh/runner.go:425-440`（`WriteInstruction` 在工作区新建 `AUTOMEDIC.md` / `AGENTS.md`）
- 证据：在副本里打印该处 `changed` 的实测值 —— `changed="[.automedic/ AGENTS.md AUTOMEDIC.md]"`，`len(changed)==0` 永远不成立；`TestE2ENoCodeChangeIgnored` 之所以通过，是因为假 dsh 在 `result.json` 里自报 `no_code_change:true`，而不是靠文件状态判定。
- 影响：`noChange` 退化为「完全听 dsh 自报」。dsh 崩溃、被 kill、或忘了写 `result.json` 时会被判为「有变更」并进入提交/推送分支（与 P0-2 叠加），平台产物还会被算进 `changed_files`/`diff_stat` 之类的中间值。
- 建议：判定前先排除平台产物（按 `runner.managedInstructionNames()` + `.automedic/` 过滤 `git status --porcelain` 的结果），或先 `CleanupArtifacts` 再判定，并补一条「dsh 无产物但有改动」的回归用例。

### P1-2 运行时配置并发读写（`-race` 已复现）

- 位置：`server/internal/api/handlers.go:25-26`、`133-207`（`settingsMu` 只包住 HTTP 读改写）、`server/internal/dsh/runner.go:116`（`r.cfg.TimeoutSec` 无锁读）、`server/internal/service/task.go:442`（`e.cfg.Git.ReleaseHook`）、`server/internal/service/review.go:168`（`e.cfg.OCR.TimeoutSec`）
- 证据：`-race` 输出（副本中新写的用例：HTTP 侧调 `UpdateSettings`，worker 侧按 `runner.go:116` 的方式读同一字段）：

  ```
  WARNING: DATA RACE
  Write at 0x00c000539680 by goroutine 18:
    internal/api.(*Handlers).UpdateSettings()
        .../server/internal/api/handlers.go:166
  ```

  `handlers.go:25` 的注释「避免与 worker 读配置产生数据竞争」与实际不符：worker 从不取这把锁。
- 影响：数据竞争（值撕裂、未定义行为），配置改动可能只对部分 worker 生效或完全不生效；`-race` 下任何真实配置变更都会直接报竞态。
- 建议：配置改为不可变快照（`atomic.Value` 持有 `*Config`，写时整体替换，读方一次性取快照），或让执行路径也走同一把锁；顺带把 `UpdateSettings` 的说明从「仅内存生效」改为明确的持久化边界。

### P1-3 `IgnoreTask` / `CancelTask` 无状态谓词、无影响行数检查

- 位置：`server/internal/api/task.go:200-216`（取消/忽略均无 `WHERE status IN (...)`）、`server/internal/api/task.go:178-198`（Cancel 不检查 `RowsAffected`，对 `confirming`/`success`/`failed` 任务静默无效却返回 200）
- 证据：`IgnoreTask` 直接 `Where("id = ?").Updates(status=ignored)`；`CancelTask` 只对 `pending/running` 生效但返回值不校验；`flow_test.go:391` 只断言 HTTP 200，不断言最终状态。
- 影响：`running` 任务被「忽略」后进程仍在跑，仍会 commit + push + 触发发布钩子（用户看到「已忽略」，代码却推上去了）；`confirming` 任务点取消是静默空操作。状态机与真实执行脱节。
- 建议：两个接口都加状态谓词 + `RowsAffected` 校验，`RowsAffected==0` 返回 409；`running` 的忽略/取消必须先 `exec.Cancel(id)` 并等待执行侧收敛。

### P1-4 抢占失败被当成任务失败写库（Confirm / 重试推送）

- 位置：`server/internal/api/task.go:238-244`（`ConfirmTask` 的 goroutine 对**任何** error 都写 `status=failed`）、`server/internal/api/task.go:139-145`（重试推送同款）、`server/internal/service/task.go:466-471`（`claim.RowsAffected==0` 时返回的「任务不在待确认或可重试推送状态」）
- 证据：双击确认 → 第二个请求抢占失败 → 该错误被原样写成 `failed + error_msg`；此时第一个请求仍在执行，稍后才写 `success`。两次写入无顺序保证，最终状态取决于调度。
- 影响：正常操作（双击、重复提交、重试撞车）会把进行中的任务短暂或最终标成失败并带上误导性的 `error_msg`；`RetryTask` 路径同样受影响。
- 建议：把「已被其它执行者占用」定义成独立错误类型，API 层返回 409 且**不写库**；只有业务失败才落 `failed`。

### P1-5 dsh 子进程继承服务端完整环境变量

- 位置：`server/internal/execx/execx.go:50-52`（`cmd.Env = append(osEnvironFiltered(), buildEnv(spec.Env)...)`）、`server/internal/execx/env.go:5`（`os.Environ()`）、`server/internal/dsh/runner.go:181-203`（`buildEnv` 只是「追加/覆盖」）
- 证据：`docs/dsh集成说明.md:133` 写的是「`Runner.buildEnv()` 构造的最小环境」，实现上是「宿主 `os.Environ()` + 覆盖项」。因此 `AUTOMEDIC_SECURITY_SECRET_KEY`、`AUTOMEDIC_DB_DSN`、`AUTOMEDIC_AUTH_JWT_SECRET` 等按文档推荐方式注入的密钥，全部出现在 dsh（及其派生的任意子进程）的环境里。
- 影响：dsh 由模型驱动、工作区内容可被仓库文件影响（提示注入），一旦它执行 `env`/`printenv` 或子进程读取环境，攻击面从「工作区」扩大到「平台主密钥 + 数据库口令 + JWT 签名密钥」。
- 建议：为 dsh 构造白名单环境（`PATH`、`HOME`、`DSH_*`、模型 `*_API_KEY`、`git` 无关项），不要把 `AUTOMEDIC_*` 全量透传；同时修正文档表述。

### P1-6 审查（OCR）链路无并发上限、无去重、重启后不收敛

- 位置：`server/internal/service/review.go:77-82`（`go e.executeReview(job.ID)`，每次请求起一个无界 goroutine）、`server/internal/service/review.go:149-160`（直接置 `running`）、`server/cmd/server/main.go:80-86`（只启动了任务执行器的 scanner）
- 证据：`StartRepoReview` 只做入参校验，没有「同仓库已有 running 审查」的判定；进程重启后 `review_jobs.status=running` 无任何恢复逻辑，Web 会一直轮询。
- 影响：有 `repo:review` 权限的账号连点即可并发起 N 份完整 clone + OCR（LLM 计费）任务，无背压；重启后残留 `running` 的审查任务永久占用 UI 状态。
- 建议：复用执行器 worker 池或加信号量（如 1–2 并发/仓库），启动时把 `running` 的审查任务标记为失败并提示重跑；顺带评估「同仓库同时只允许一个审查」。

---

## P2 — 改进项

| 位置 | 问题 | 证据 | 影响 / 建议 |
|---|---|---|---|
| `service/task.go:44` | worker 并发硬编码 4、队列 1024，配置里无对应项 | `limit: 4`；`configs/*.yaml` 无 worker 段 | 容量只能改代码；建议加 `execution.workers` / `execution.queue_size` 并做上限校验 |
| `git/workspace.go:332-335`、`execx/execx.go:146-181` | 所有 git 网络操作（clone/fetch/push）无自身超时 | `RunSimpleEnv` 无 `Timeout`，`runOut` 也不传；只有 dsh（`runner.go:116`）与 TestRepo/ListRepoTree（60s）有超时 | 远端黑洞时占住 worker 直到 scanner 兜底（默认 35 分钟）；建议给 clone/fetch/push 显式超时 |
| `service/task.go:92-103` | scanner 超时判定基于 `updated_at`（仅阶段切换时更新），无心跳；批量 UPDATE 不检查行数 | 判定与 `Updates(...)` 之间无状态复核 | 长阶段（push、release）可能被误判；与执行侧收尾写库存在竞争，建议执行中定期 touch + 条件更新 |
| `api/task.go:157-169` | `RetryTask` 整份复制源任务结果字段（`patch`/`workspace`/`base_commit`/`summary`），只重置了部分 | `cp := src` 后仅清 `status/mode/retry/...` | 新任务在真正开跑前就可能被 `CanResumeFinalize` 认定「可续跑推送」，带上一个任务的产物；建议显式清空结果字段 |
| `api/handlers.go:165-167`、`125` | `dsh.timeout_sec` 无范围校验；`GetSettings` 回显 `release_hook` 原文 | 直接赋值；hook 可能含 CI token | `0`/负值会让 dsh 无超时（scanner 5 分钟兜底）；hook 回显对 `settings:read` 是信息泄露 |
| `logging/logging.go:46-60` | 日志文件在启动时按当天命名，长跑进程跨天不换文件；过期清理只在启动执行 | `name := ... time.Now().Format("2006-01-02")` | 长期运行会把多天日志写进同一天的文件，`retain` 无法按天生效；建议按天重开或接 `lumberjack` 式轮转 |
| `config/config.go:264`、`dsh/runner.go:35-37` | `permission_mode` 只有 Web 侧白名单，配置文件/环境变量可设任意值 | `AUTOMEDIC_DSH_PERMISSION_MODE` 直接覆盖 | 误配成 `danger-full-access` 会放大 dsh 的影响面；建议启动时校验，非 `workspace-write` 拒绝启动（README 明确要求） |
| `service/ingest.go:191-249`、`service/rule.go:131-148`、`model/model.go:269` | 指纹去重是 check-then-act，`events(project_id,fingerprint)` 无唯一索引 | 先 `First` 再 `Create`；`Fingerprint` 只有普通索引 | 并发投递同一告警可产生重复事件/重复任务（`CompactDuplicateEvents` 只能事后收敛）；建议加唯一索引 + `ON CONFLICT` |
| `cmd/server/main.go:141-170` | janitor 扫描 `workspace_root` 下所有目录，可能删掉 `.ssh/known_hosts` | 遍历 `root` 全部 entry，仅按 mtime 判定 | 删掉后下一次 SSH 连接重新 TOFU；建议跳过 `.ssh` 之类的非工作区目录 |

---

## 验证输出摘要

| 检查项 | 命令 | 结果 |
|---|---|---|
| 基线 | `rtk git log -1` | `3c10e9d3d1b02e55f313367cf3e2aa4f4148935f fix(web): 统一 token 与响应式…` |
| 范围测试 | `cd server && rtk go test ./internal/service/... ./internal/git/... ./internal/dsh/... ./cmd/server/...` | **42 passed / 4 packages**（含 `service` 的 6 个假 dsh E2E） |
| E2E（假 dsh） | `rtk proxy go test -run TestE2E -v -count=1 ./internal/service/` | 6/6 PASS：`AutoFixCommitPushRelease`、`SemiConfirmThenPush`、`Reject`、`NoCodeChangeIgnored`、`WorkspaceCleanupAfterReuse`、`RetryPushAfterConfirmFailure` |
| 竞态 | `rtk proxy go test -race -count=1 ./internal/service/ ./internal/git/ ./internal/dsh/` | 现有用例全绿（并发/失败路径无覆盖，见下节） |
| 实证复现 | 把 `server/` 复制到 `/tmp` 副本，新增 5 个临时用例（3 个文件）后 `go test -run TestRepro…` | P0-1（两个变体）、P0-2、P1-1、P1-2 全部复现；**产品代码零改动**，副本仅用于取证 |

复现骨架（放在仓库副本的 `internal/service` 下，配合既有 `newE2E` 测试装置）：

```go
e := newE2E(t, model.FixModeSemi)
ra := e.ingest(t)                                   // 任务 A（指纹 1）
e.ex.Execute(ctx, ra.TaskIDs[0])                    // A → confirming，补丁入 tasks.patch
// 换成只写 other.go 的假 dsh，投递指纹 2 的任务 B
rb, _ := Ingest(e.db, e.project.ID, nil, &IngestInput{ /* Fingerprint: "fp-B" */ })
e.ex.Execute(ctx, rb.TaskIDs[0])                    // B 在共享工作区留下未提交改动
e.ex.Confirm(ctx, ra.TaskIDs[0], "sen", "LGTM")     // 确认 A
// 断言：A 的分支上出现 B 的 other.go，A 的 service.go 修复缺失，A 状态 = success
```

## 并发 / 状态机关键分支的测试覆盖缺口

1. **无任何并发用例**：`Execute` 双跑、`Confirm` 双跑、`Execute × Confirm` 竞争、`lockRepo` 串行性都没有测试；现有用例全部是单 goroutine 顺序调用。
2. **dsh 失败路径零覆盖**：非零退出 / 超时 / 无 `result.json` 三个分支都没测，正是 P0-2 藏身之处。
3. **`scanner` 与 `Cancel` 无覆盖**：`Executor.Start`（worker 池 + 30s scanner）从未被测试调用，`Cancel` 只被 `flow_test.go:391` 命中 HTTP 200，不断言状态。
4. **同仓库多任务交叉用例缺失**：唯一覆盖复用工作区的是 `TestE2EWorkspaceCleanupAfterReuse`（同仓库第二次自动任务），它验证的是「基线正确」，覆盖不到「半自动确认期间被别的任务改写」。
5. **无 `-race` 下的配置变更用例**：`TestUpdateSettingsIgnoresDangerousFields` 只验证字段被忽略，验证不到并发读写。
6. **审查链路**：`StartRepoReview` 的入参校验、模型选择、进度写入都有测试，但「重启后 running 的 job」「同仓库并发审查」无覆盖。

## 上轮已修项复核（本路旧项）

| 旧项 | 结论 | 依据 |
|---|---|---|
| P0-D Confirm 不持 `lockRepo` | **部分** | `service/task.go:479` 已加 `unlock := e.lockRepo(task.RepoID)`，执行期互斥成立；但工作区目录仍无 taskID（`git/workspace.go:86`），锁只覆盖「同时刻」而不保护「确认等待期」的工作区状态 → 见本轮 P0-1 |
| P0-E Confirm 无原子抢占 | **已修** | `service/task.go:461-471` 条件更新（`confirming` 或 `failed`+有产物）+ `RowsAffected` 判定，双击不再双跑；残留问题见 P1-4（抢占失败被写成 failed） |
| P1-1 Cancel 未落库 | **已修** | `api/task.go:187-189` 无论进程是否在跑都落 `cancelled`（限 `pending/running`），`service/task.go:323-327`、`368-372` 在 `ctx.Err()` 时跳过提交推送；残留问题见 P1-3 |
| P1-2 scanner 不 cancel 进程 | **已修** | `service/task.go:96-103` 先逐条 `e.Cancel(id)` 再批量标 failed，超时阈值为 `max(5m, dsh.timeout_sec+300s)`；残留：判定基于 `updated_at` 无心跳、批量更新不校验行数（P2） |
| P1-3 配置并发读写 | **仍开** | `handlers.go:25/133-207` 的 `settingsMu` 只覆盖 HTTP 侧，worker 侧 `dsh/runner.go:116`、`task.go:442`、`review.go:168` 仍无锁读同一 `*config.Config`；`-race` 已复现（handlers.go:166 写 vs 无锁读）。唯一已修部分是 `git.Manager` 改为每次 `Prepare` 重建（`task.go:51-54`、`284`） |
| P1-4 指令文件清理 | **已修**（有副作用） | `dsh/runner.go:425-455` 只新建不覆盖并写清单，清单写失败返回错误→任务失败（`task.go:299-303`）；`runner.go:479-499` 无清单时按固定文件名兜底删除；E2E 断言 `AUTOMEDIC.md`/`AGENTS.md` 未进提交。副作用：这些文件让 `ChangedFiles` 永不为空，见 P1-1 |
| P1-5 SSH `known_hosts` 与 askpass | **已修** | `git/workspace.go:354-372`：`StrictHostKeyChecking=accept-new` + 工作区自有 `known_hosts` + `IdentitiesOnly`，口令改从 `AUTOMEDIC_SSH_PASSPHRASE` 环境变量读取（不再 `echo` 拼接），并有 `TestAuthEnvSSHAskpassUsesEnvNotEcho` 回归；残留 TOFU 首连信任风险（已知权衡） |
| P1-11 `http.Server` 超时 | **已修** | `cmd/server/main.go:90-97`：`ReadTimeout`（默认 60s）、`ReadHeaderTimeout=10s`、`WriteTimeout`（默认 120s）、`IdleTimeout=60s` |

## 建议修复顺序

1. **P0-2 + P1-1**（同一条链）：dsh 失败即 `failed`；修 `base_commit` 空串判定；无变更判定排除平台产物。这是「假成功 + 告警被抑制」的完整闭环。
2. **P0-1**：Confirm 前强校验分支/HEAD/工作区状态并拒绝不匹配场景，随后把工作区目录带上 `taskID`。
3. **P1-2**（配置快照）、**P1-3**（状态谓词 + 行数校验）、**P1-4**（抢占失败不写 failed）—— 都是小改动、低风险。
4. 第二批：P1-5（dsh 环境白名单）、P1-6（审查并发与重启收敛）、P2 中的 worker 配置项与 git 超时。

## 做得好的地方

- 任务抢占（`task.go:159-166`）与仓库级串行锁（`task.go:143-153`）的写法正确，注释解释了「为什么按 repoID 而不是 taskID」。
- 进程组整组回收在 `execx.Run` 与 `RunSimpleEnv` 两条路径都覆盖（`execx.go:69-78`、`159-168`），超时会真正杀掉派生的 git/node 进程。
- 半自动确认的补丁回放设计（`task.go:516-519` + `applyPatchFile`）思路是对的，问题出在工作区没有被独占而不是回放本身。
- 指令文件用清单 + 固定名兜底双策略清理，并明确「不覆盖仓库原有文件」（`runner.go:429-440`）——考虑到了污染用户仓库的实际风险。
- 输出脱敏（`git.runOut` 统一 `ScrubURL`、`redactSecrets`）与 `task_logs` 的 seq 批量落库都比较克制、够用。
