# AutoMedic 代码审查报告（第 1 路：安全与多租户隔离）

- 审查时间：2026-09-15
- 基线：`3c10e9d`（`rtk git log -1` 实测：`3c10e9d3d1b02e55f313367cf3e2aa4f4148935f fix(web): 统一 token 与响应式，消除死类名和图表色漂移`）
- 范围：`server/internal/api/**`（router/middleware/rbac/handlers/token/context/resource/task/llm）、`server/internal/auth/**`、`server/internal/crypto/**`、`server/internal/ws/**`、`server/internal/store/**`（租户过滤、默认数据、迁移）、`server/internal/model` 鉴权相关字段
- 方法：源码通读 → 跨层追踪影响面（`service`/`git`/`ocr` 仅作为 P0 影响验证，不在本路范围）→ 在 `/tmp` 临时副本中用 7 个探针做端到端实证复现 → 对照规范基线与项目文档
- 约束：只读审查。未修改任何产品代码或配置，未 `git add/commit/push`；本文件是本路唯一新增文件
- 规范基线：`13-security.md`（资源级鉴权与租户隔离、外部输入校验、密钥不落日志、安全事件日志含主体/资源/结果/trace id、高权限动作最小权限与审计）、`05-api.md`（异常转换纪律：只抛业务异常、错误体不泄露内部实现、状态码与错误语义稳定）、`07-postgresql.md`（字段/约束以 Schema 与迁移为准）、`19-code-development.md`（复用优先、最小概念）、`22-agent-behavior.md`（事实/推断分离、闭环验证）
- 项目文档基线：`README.md`、`docs/接口文档.md`、`docs/部署文档.md`、`docs/代码审查报告-2026-09-08.md`、`docs/代码审查报告-2026-09-09.md`

## 结论（先看这个）

路由层权限位（`RequirePerm`）、`tdb` 租户过滤、WS 归属校验、密钥不外泄（`json:"-"`、`mask`）这些"骨架"已经立住：本轮实测跨租户读任务/日志/项目/仓库、跨租户 WS 订阅全部被拒（404），同租户 WS 正常升级（101）。但**授权模型本身留下两个可跨租户利用的高危口子**，且都不是"漏加一个守卫"那么简单：

1. **权限授予没有边界**：租户管理员默认拥有 `role:create/update` + `user:update`，而创建/修改角色时不校验"哪些权限码不允许租户自授"（`rbac.go:453-528`）。实测非超管租户管理员可自建含 `tenant:create/update/delete`、`model:update`、`settings:update` 的角色、绑给自己（同一 token 立即生效），随后**成功创建并删除租户**、改写全局模型 `base_url`（`ocr/llm.go:34-35` 会把解密后的厂家 API Key 连同该 URL 交给 OCR 子进程）、覆盖 Web 仍在暴露的 `dsh.patch_template`/`dsh.home`。这等于把 2026-09-09 的 P0-B/P0-A 从后门重新打开。
2. **跨租户引用凭证无人校验**：`repo.credential_id` 在创建/更新仓库时完全不做租户校验（`resource.go:166-190`、`206-223`）。实测攻击者租户在自有仓库上引用受害者租户的凭证后触发连通性测试，受害者凭证明文（Basic `victim-user:victim-http-secret-XYZ`）被发送到攻击者控制的 HTTP 服务器。

另外三件本周应一并处理：跨租户取消他人运行中任务（`task.go:178-198` 先发取消信号后校验归属，实测受害者任务被改成 `cancelled`）；改口令后旧 `refresh`/`access` 仍然可用（实测改密后旧 refresh 续期 200）；登录限流按 IP 且与账号无关（实测 8 次错误口令后**超管本人用正确口令也被 429**），且失败计数表无上限清理。

---

## P0 — 必须先修

### P0-1 租户管理员可自授平台级权限 → 跨租户破坏、全局大模型密钥外泄、重开 Web 可执行入口

- 位置：
  - `server/internal/api/rbac.go:453-479`（`CreateRole` 原样接受请求里的 `permissions`）
  - `server/internal/api/rbac.go:481-528`（`UpdateRole` 同样按提交覆盖权限）
  - `server/internal/store/seed_rbac.go:128-157`（`SetRolePermissions` 只按 `model.PermissionCatalog` 过滤"码是否合法"，不判断"谁能授予"）
  - `server/internal/model/rbac.go:262-277`（`tenantBusinessPermissions()` 只在**内置角色默认值**层面排除 `settings:update`/`model:update`/`tenant:*`）
  - 影响放大点：`server/internal/ocr/llm.go:34-35`（`OCR_LLM_URL` = 可被改写的 `provider.base_url`，`OCR_LLM_TOKEN` = 解密后的厂家 API Key）、`server/internal/api/handlers.go:163-164`（Web 仍可写 `dsh.patch_template`、`dsh.home`）
- 证据（实测，探针 6，非超管 tenant_admin 同一 token 全程）：

  ```text
  攻击者初始 is_super=false permissions=40 个
  租户管理员创建含平台级权限的角色: status=200 code=0 data=map[code:self-promo tenant_id:1 ...]
  自绑新角色: status=200 code=0
  绑定后 permissions=[... model:update ... settings:update ... tenant:create tenant:delete tenant:update ...]
  租户管理员创建平台租户: status=200 code=0 data=map[id:2 key:rogue-tenant name:偷偷建的租户]
  租户管理员删除租户: status=200 code=0
  覆盖 patch_template: status=200 code=0；当前 patch_template="- id: agent-default-model\n  config:\n    provider: evil\n    model: evil\n" home=/tmp/attacker-home
  ```

  代码路径核对：`auth.RequirePerm` 只查 `p.Can(code)`（`auth/auth.go:95-105`、`327-341`），`CreateRole/UpdateRole` 无"平台码白名单"校验；`PrincipalFromToken` 每个请求重载权限（`auth/auth.go:280-293`），所以自授后**无需重新登录立即生效**。
- 影响：
  1. 跨租户与平台级破坏：`CreateTenant`/`UpdateTenant`/`DeleteTenant` 全部不受租户过滤（`rbac.go:565-692`），自授 `tenant:delete` 后可删除他人租户的 `users`/`roles`（该租户无项目时，`rbac.go:677-690`），或改名/停用他人租户。
  2. 全局密钥外泄：自授 `model:update` 后可改共享 `providers.base_url`（`llm.go:80-124` 无租户维度，模型配置平台级共享），OCR/审查链路会把解密后的 API Key 发到攻击者端点（`ocr/llm.go:34-35`）。
  3. 重开 P0-A 的 Web 可执行入口：自授 `settings:update` 后可改写 `dsh.patch_template`（每次任务渲染成 `--patch` 配置文件喂给 dsh，`dsh/runner.go:156-179`、`86-98`）与 `dsh.home`（`dsh/runner.go:190-192`，写 `DSH_HOME` 环境变量）。
- 建议：为权限码建立"可授予层级"。最小实现：在 `model` 中给出 `PlatformOnlyPermissions`（`tenant:read/create/update/delete`、`model:update`、`settings:update`），`CreateRole`/`UpdateRole` 对非超管直接剔除或拒绝这些码（`SetRolePermissions` 前先过滤，返回 400 说明原因）；`UpdateRole` 还要禁止非超管把**已有**平台码保留在自己租户的角色里。回归测试：非超管创建含 `tenant:create` 的角色必须失败/被过滤，且自授后 `POST /tenants` 仍 403。

### P0-2 跨租户引用凭证 → 他人 git 凭据被发送到攻击者控制的服务器

- 位置：
  - `server/internal/api/resource.go:166-190`（`CreateRepo`：只有 `ProjectID` 走 `requireTenantOfProject`，`credential_id` 直接来自请求体）
  - `server/internal/api/resource.go:206-223`（`UpdateRepo`：字段白名单含 `credential_id`，无归属校验）
  - 使用点：`server/internal/api/resource.go:239-269`（`TestRepo`）、`271-337`（`GetRepoTree`/`GetRepoFile`）、`server/internal/service/task.go:724-741`（`resolveAuth` 解密 `Credential.SecretEnc`）、`server/internal/git/workspace.go:374-390`（`http_auth`/`http_token` 把 `用户名:密钥` 写进 clone URL 的 userinfo）
- 证据（实测，探针 1）：

  ```text
  攻击者用他人凭证建仓库: status=200 code=0 msg=ok
  落库后 repo.credential_id=<受害者凭证 id> (攻击者租户=1, 受害者租户=2)
  触发连通性测试(ls-remote): status=500 msg=连通性测试失败：exit=128 fatal: Authentication failed for 'http://127.0.0.1:<port>/victim.git/'
  结果：受害者凭证已发送到攻击者服务器 Authorization=Basic dmljdGltLXVzZXI6dmljdGltLWh0dHAtc2VjcmV0LVhZWg==
  ```

  `Basic dmljdGltLXVzZXI6dmljdGltLWh0dHAtc2VjcmV0LVhZWg==` 解码即 `victim-user:victim-http-secret-XYZ`——受害租户的明文凭据。
- 影响：任意拥有 `repo:create`/`repo:update` 的角色（内置 `developer`、`tenant_admin` 默认都有：`model/rbac.go:236-252`）都能用可猜的自增 ID（`model.go:163` 全局自增）引用其他租户的凭证，把 `http_auth`/`http_token` 密钥骗到自己的服务器；`ssh_key` 型凭据虽不会外传私钥内容，但会被用于对攻击者指定主机/路径发起认证与推送（可借用他人身份写入第三方仓库）。此外 `GetRepo` 的 `Preload("Credential")`（`resource.go:199`）会把他人凭证的名称/用户名/类型回给攻击者，便于先探测再窃取。
- 建议：在 `CreateRepo`/`UpdateRepo` 增加 `tenantOfCredential(tid, credentialID)` 校验（与 `requireTenantOfProject` 同款），凭证与仓库必须同租户；顺带做一次历史数据体检（`SELECT id,tenant_id,credential_id` 与 `credentials` 关联，找出租户不一致的记录）。回归测试：跨租户 `credential_id` 必须 400/404，且库中不被写入。

---

## P1 — 高风险或缺闭环

### P1-1 跨租户取消他人正在执行的任务（先发信号，后校验归属）

- 位置：`server/internal/api/task.go:178-198`（`h.exec.Cancel(id)` 在归属校验之前，DB 更新才走 `tdb`）、`server/internal/service/task.go:117-127`（`cancel` 表仅按 `taskID` 索引，无租户维度）
- 证据（实测，探针 2）：攻击者（另一租户的 tenant_admin）对受害者 `running` 任务调用取消：

  ```text
  受害者任务=1 已进入 running
  跨租户取消返回: status=200 code=0 msg=ok data=map[cancelled:1]
  受害者任务最终状态=cancelled err=任务已取消
  ```

  对比：同文件的 `TaskLogs`（`task.go:74-79`）、`ConfirmTask`（`task.go:224-229`）、`RejectTask`（`task.go:254-258`）、`RetryTask`（`task.go:121-125`）都先 `tdb` 校验归属，仅 `CancelTask` 顺序倒置。
- 影响：跨租户可用性破坏——任意租户的 `task:cancel` 持有者可通过遍历任务 ID 打断其他租户在跑的修复（`Execute` 收到 `ctx.Err()` 后把任务置 `cancelled`，`service/task.go:323-326`、`368-371`），造成他人自动化修复静默失败、半成品工作区滞留。
- 建议：与其它任务接口一致，先 `h.tdb(c).Select("id").Where("status IN ?", ...)` 校验并收窄状态，再做 `exec.Cancel`；把 DB 更新改为条件更新并检查 `RowsAffected`（同时解决"取消已完成任务也返回成功"）。

### P1-2 修改/重置口令后旧会话不失效

- 位置：`server/internal/api/rbac.go:143-175`（`ChangePassword` 只更新 `password_hash`）、`server/internal/api/rbac.go:315-326`（管理员重置口令同样只写 hash）、`server/internal/auth/auth.go:216-259`（`Refresh` 只校验 `revoked`/`expires_at`/用户状态，与口令变更无关）、`auth.go:280-293`（access token 只校验签名与用户状态）
- 证据（实测，探针 5）：

  ```text
  修改密码: status=200 code=0 msg=ok
  改密后用【旧 refresh_token】续期: status=200 code=0 msg=ok
  改密后用【旧 access token】访问 profile: status=200 code=0
  用【旧口令】重新登录: status=401 msg=账号或密码错误
  ```
- 影响：口令泄露/被撞库后的标准处置（改口令）无法踢掉攻击者会话；`refresh_token` 又长期驻留浏览器 `localStorage`（2026-09-09 报告 P2 已记录 XSS 可换新会话），等于处置动作不闭环。默认 `access_token_ttl=480min`、`refresh_token_ttl=1440min`（`auth/auth.go:160-174`），窗口很长。
- 建议：改口令与管理员重置口令时，`db.Model(&model.AuthToken{}).Where("user_id = ?", uid).Update("revoked", true)`；如需立即失效 access token，可在 `User` 上加 `password_changed_at`，`PrincipalFromToken` 比较 JWT `iat`。

### P1-3 登录限流维度错误：按 IP 计数、与账号无关，且失败表无上限

- 位置：`server/internal/api/rbac.go:18-60`（`loginGuard` 仅以 IP 为 key，8 次失败锁 1 分钟）、`81-84`（进入登录逻辑前只查 IP）、`87`（任何失败都记到 IP 上）
- 证据（实测，探针 4，同一来源 IP）：

  ```text
  第 1 次错误口令: status=401 msg=账号或密码错误
  第 8 次错误口令: status=401 msg=账号或密码错误
  受害者本人用正确口令登录: status=429 code=429 msg=登录失败次数过多，请稍后再试
  另一个账号(超管)用正确口令登录: status=429 code=429 msg=登录失败次数过多，请稍后再试
  loginGuard.byIP 当前条目=1（成功登录才删除，失败条目无过期清理）
  30 个不同来源 IP 后条目=31（仅成功登录会删除，进程内无上限）
  ```

  部署侧放大器：`docs/部署文档.md:346` 要求反代场景设置 `AUTOMEDIC_SERVER_TRUSTED_PROXIES`，但当前默认值为空（`server/configs/config.yaml:15`、`config.docker.yaml:16`）。未设置时 `c.ClientIP()` 恒为代理对端 IP，全部用户共用同一失败计数 → 任意未认证者 8 次错误口令即可锁死所有租户登录 1 分钟。
- 影响：① 可利用的登录 DoS（尤其反代未按文档配置时）；② 没有账号维度锁定与退避，攻击者换 IP（或 8 次/分钟/IP 分布式）即可无限爆破单一账号，`13-security.md` SHOULD「速率限制、账户锁定」未闭环；③ 失败表只增不删（仅成功登录清理），进程生命周期内无界增长。
- 建议：改为 (账号, IP) 双维度：账号维度做递增退避 + 锁定（阈值/窗口可配置），IP 维度只做宽松上限；对锁定响应加 `Retry-After` 并记审计；定时/惰性清理过期条目（如按 lastSeen 淘汰），并在部署文档中把 `trusted_proxies` 从"建议"提为反代部署的必填项。

### P1-4 高权限操作没有审计日志与 trace id

- 位置：`server/internal/api/router.go:30`（只挂 `gin.Recovery()`，无请求日志/请求 ID 中间件）；`server/internal/api/rbac.go:69-95`（`slog` 覆盖登录成功/失败）；`server/internal/api/rbac.go:615`、`server/internal/api/handlers.go:208`、`server/internal/api/event.go:277`（其余全部日志点）
- 证据：`server/internal/api` 目录下 `slog` 调用仅上述几处；`CreateUser`/`UpdateUser`/`DeleteUser`/`CreateRole`/`UpdateRole`/`DeleteRole`/`CreateTenant`/`DeleteTenant`/`UpdateSettings`/`Create|Update|DeleteCredential`/`DeleteProject` 等均无审计记录（`handlers.go:133-209`、`rbac.go:212-692`、`resource.go:66-596` 全文核对）。
- 影响：P0-1 这类"自授权限→创建/删除租户→改全局模型"的攻击在当前日志里几乎不可追溯（只有一条 `settings updated`，且无操作者、无租户、无 trace id）；违反 `13-security.md` MUST「安全事件日志包含时间、主体、资源、结果和 trace id」「高权限动作需最小权限和审计」，也无法满足事后取证。
- 建议：加一个统一审计辅助函数（时间、user_id、username、tenant_id、请求 ID、资源类型/ID、动作、结果、关键入参摘要），在用户/角色/租户/设置/凭证/项目删除等写操作上调用；请求 ID 可由中间件从 `X-Request-ID` 透传或生成并回写响应头。

---

## P2 — 改进项

| # | 位置 | 问题 | 证据 | 影响 | 建议 |
|---|---|---|---|---|---|
| P2-1 | `api/handlers.go:163-164` | Web 仍可写 `dsh.patch_template`（每次任务渲染成 `--patch` 配置层，`dsh/runner.go:86-98,156-179`）与 `dsh.home`（`DSH_HOME` 环境变量，`runner.go:190-192`） | 探针 6 实测 `PUT /settings` 覆盖成功并回读 | 经 P0-1 自授 `settings:update` 后可影响所有租户的 dsh 进程配置，是 P0-A"收回 Web 可执行入口"的残留面 | 二者一并移出 Web 可写集合，或加"仅平台超管 + 值校验（路径白名单/禁止 YAML 注入）" |
| P2-2 | `api/response.go:39-59` | `BadRequest`/`ServerError` 直接把内部错误文本回给客户端 | 实测：`duplicated key not allowed`、`json: cannot unmarshal string into Go struct field userIn.tenant_id of type uint` | 泄露内部结构/字段名与存储约束，违反 `05-api.md`「禁止把原始异常直接抛给调用方」「内部类名不得出现在响应里」 | 边界层统一映射为业务码 + 固定文案，原始错误只进日志（含 trace id） |
| P2-3 | `api/resource.go:225-236`（`DeleteRepo`）、`api/resource.go:130-146`（`DeleteProject`）、`api/task.go:200-216`（`IgnoreTask`）、`api/task.go:187-192`（`CancelTask`） | 对不属于当前租户的资源返回 200 成功，实际 0 行受影响 | 实测：`删除他人仓库: status=200 code=0`，随后 `仓库仍存在=true`；`删除他人项目: status=200`，项目仍在 | 接口语义假成功，客户端误判；与 `05-api.md`「HTTP 状态码、业务错误码稳定一致」不符 | 更新/删除后检查 `RowsAffected`，为 0 时返回 404 |
| P2-4 | `api/context.go:26-37`、`api/resource.go:80-84`、`api/resource.go:502-503`、`api/rbac.go:463-470` | 超管未带 `X-Tenant-ID` 时 `tenant(c)=0`，`tdb` 退化为不过滤，新建对象会写成 `tenant_id=0` 孤儿数据 | 代码路径：`p.TenantID = h.tenant(c)` / `cd.TenantID = h.tenant(c)` / `tid := h.tenant(c)` 后直接 `Create` | 数据不可见于任何租户视图，后续统计/隔离校验出现空洞（2026-09-09 报告已记为 P2，仍在） | 写操作要求显式租户上下文（超管必须带 `X-Tenant-ID`，否则 400） |
| P2-5 | `api/event.go:82-84`、`api/rbac.go:102-104` | 采集令牌与 refresh token 支持 `?token=`/`?refresh_token=` 查询串（接口文档 `docs/接口文档.md:53` 已固化该用法） | 代码路径确认，`13-security.md` MUST「Token 不进入日志」 | 查询串几乎必然落进反代/网关访问日志与浏览器历史 | 逐步弃用查询串传入，过渡期在文档标注"仅应急使用"，并确保访问日志脱敏 |
| P2-6 | `auth/auth.go:271-277` | JWT 校验未固定算法（未用 `jwt.WithValidMethods`），也未校验 `iss/aud` | 代码路径 | 当前依赖库对 key 类型的校验兜底，属纵深防御缺口 | 显式 `WithValidMethods([]string{"HS256"})`，签发/校验带 `iss`（可选 `aud`） |
| P2-7 | `auth/auth.go:143-150` | `loadPrincipal` 仅凭角色 **code** 判超管，未同时要求 `role.tenant_id=0` | 代码路径；当前所有绑定入口都已校验租户（`rbac.go:363-405`），故不可直接利用 | 任何新增/旁路的 `user_roles` 写入路径都会直接变成超管（P0-B 的"信任点"仍在绑定侧） | 判定改为 `r.Code == RoleSuperAdmin && r.TenantID == 0`；并在 DB 层对平台角色加约束 |
| P2-8 | `cmd/server/main.go:119-138` | WS 历史/状态数据源只按 `task_id` 查询，无租户维度；隔离完全依赖 `handlers.go:81-93` 前置校验 | 代码路径 + 实测跨租户 404（当前有效） | 任何绕过 handler 的调用路径都会泄露他人任务日志 | Source 增加租户参数（`History(taskID, tenantID, ...)`），或把归属校验下沉到 hub |

---

## 上轮已修项复核

| 旧项 | 结论 | 依据（含本轮实测） |
|---|---|---|
| **P0-A** Web 可改命令模板/发布钩子 → 主机 RCE | **部分** | 已修部分：`UpdateSettings` 请求体只剩 `dsh.home/timeout_sec/permission_mode/patch_template`、`ocr.*`、`git.*`（无 bin/command_template/use_shell/env/release_hook/workspace_root，`api/handlers.go:133-157`）；`permission_mode` 仅允许 `workspace-write`（`handlers.go:168-174`）；创建/更新项目丢弃 `release_hook`（`api/resource.go:79`、`123-124`）；`tenantBusinessPermissions()` 不再给内置 `tenant_admin` 发 `settings:update`（`model/rbac.go:262-277`）；已有回归测试 `TestUpdateSettingsIgnoresDangerousFields`（`api/flow_test.go:492-539`）、`TestCreateProjectDropsReleaseHook`（`api/flow_test.go:578-594`）。**仍开部分**：① `dsh.patch_template`/`dsh.home` 仍在 Web 可写集合内（`handlers.go:163-164`），前者每次任务被渲染成 `--patch` 配置层（`dsh/runner.go:86-98,156-179`）；② 平台码 `settings:update` 可由租户管理员自授（P0-1，实测覆盖 `patch_template` 成功）。因此对"租户管理员无法从 Web 触达可执行入口"这一目标而言，修复可被绕过 |
| **P0-B** 绑定任意 `role_ids` 可成平台超管 | **已修（原始手法）／等价后果另开** | `replaceUserRoles` 现校验：`r.Code == super_admin || r.TenantID == 0` 且调用者非超管 → 直接报"不能绑定平台级角色"；`r.TenantID != userTenantID` → 报"不能绑定其他租户的角色"（`api/rbac.go:363-405`）；回归测试 `TestReplaceUserRolesRejectsPlatformRoleForTenantAdmin`（`api/flow_test.go:541-576`）。本轮实测该手法不可用，但**同一后果**可由自建平台权限角色达成（P0-1 实测：非超管自授 `tenant:*`/`model:update`/`settings:update` 并成功创建/删除租户），故该风险类并未关闭 |
| **P0-C** WS 订阅无租户、无 `task:read` | **已修** | 路由挂 `auth.RequirePerm(model.PermTaskRead)`（`api/router.go:204-207`）；handler 先 `tdb` 校验任务归属再升级（`api/handlers.go:81-93`）；Origin 收敛为同源 + `allow_origins`（`ws/hub.go:129-154`、`router.go:44`），`?token=` 入口已移除。实测：跨租户 WS 订阅 `http=404 websocket: bad handshake`，同租户 `http=101`（正对照）；跨租户 HTTP 读任务/日志/项目/仓库全部 404，跨租户 Confirm 亦 404（探针 3）。遗留纵深项见 P2-8（hub 数据源无租户维度） |
| **P1-6** 登录限流与默认口令 | **部分** | 默认口令：release 模式下 `SeedRBAC` 拒绝 `admin123`/`change-me` 并给出配置指引（`store/seed_rbac.go:53-56`；测试 `TestSeedRBACRejectsDefaultPasswordInRelease`），`docs/部署文档.md:344` 也要求首登改口令 —— 此项已闭环；**限流仍未闭环**：仅按 IP 计数、与账号无关、无账号锁定/退避（`api/rbac.go:18-60`），实测 8 次错误口令后超管正确口令也被 429（跨账号连带锁定），失败表无上限；反代默认未设 `trusted_proxies` 时全部用户共享一个计数桶（放大为登录 DoS）。另配套缺口：改口令后旧会话不失效（P1-2，实测） |

## 本轮实测通过（已确认加固，供回归基线使用）

- 跨租户读取：任务、任务日志、项目、仓库、Confirm 全部 404（探针 3）。
- WS：跨租户 404 / 同租户 101（探针 3，含正对照）。
- 角色绑定：非超管绑定平台 `super_admin` 角色被拒（`api/flow_test.go:541-576`，本轮测试通过）。
- Web 设置：`command_template`/`bin`/`use_shell`/`release_hook`/`workspace_root` 写入被忽略，`permission_mode=danger-full-access` 被拒（`api/flow_test.go:492-539`）。
- 密钥处理：`Credential.SecretEnc/PassphraseEnc`、`Provider.APIKeyEnc`、`IngestToken.TokenHash`、`User.PasswordHash` 均为 `json:"-"`（`model/model.go:188-189,217,247`、`model/rbac.go:21`）；列表接口只回 `mask`/`maskKey` 摘要（`resource.go:645-653`、`llm.go:560-568`）。
- 加密与主密钥：AES-256-GCM + 随机 nonce（`crypto/crypto.go:34-63`）；未配置主密钥时启动直接失败（`cmd/server/main.go:40-45`），JWT 密钥无硬编码默认值（`config.go:271-329`）。
- CORS：`allow_origins` 为空时不挂 CORS 中间件（不返回任何 ACAO、也不 403），配置后不回落 `*`（`api/router.go:43-55`）。
- 平台卡口：`dicts`/`settings` 写接口要求 `settings:update`（`api/router.go:171-176`）、`providers`/`models` 要求 `model:update`（`api/router.go:127-135`）、`tenants` 要求 `tenant:*`（`api/router.go:189-192`）——边界本身正确，问题在于这些码可被自授（P0-1）。

---

## 验证命令与结果摘要

| 命令 | 结果 |
|---|---|
| `rtk git log -1` | `3c10e9d3d1b02e55f313367cf3e2aa4f4148935f fix(web): 统一 token 与响应式，消除死类名和图表色漂移`（基线一致） |
| `cd server && rtk go test ./internal/api/... ./internal/auth/... ./internal/ws/... ./internal/store/...` | `Go test: 18 passed in 4 packages`；`rtk proxy go test -count=1 ...` 明细：`ok internal/api 5.328s`、`internal/auth [no test files]`、`ok internal/ws 0.243s`、`ok internal/store 2.153s` |
| `cd server && rtk go vet ./internal/api/... ./internal/auth/... ./internal/ws/...` | 无输出（通过），退出码 0 |
| 临时副本（`/tmp/amsec.wVSHlk/server`）探针 `TestProbeCrossTenantCredentialUse` | 复现跨租户凭据外泄：受害者 Basic 凭据出现在攻击者 HTTP 服务端的 `Authorization` 头 |
| 同上 `TestProbeCrossTenantCancel` | 复现跨租户取消：返回 `200 {"cancelled":1}`，受害者任务最终 `cancelled` |
| 同上 `TestProbeCrossTenantCRUD` | 跨租户读/确认 404、跨租户 WS 404、同租户 WS 101；`DeleteRepo`/`DeleteProject` 返回 200 但数据未删（P2-3） |
| 同上 `TestProbeTenantAdminSelfGrantsPlatformPerms` | 复现权限自授：非超管创建平台权限角色→自绑→创建/删除租户、覆盖 `patch_template`/`home` 全部 200 |
| 同上 `TestProbeLoginLimiter` | 8 次错误口令后跨账号 429；失败表条目随来源 IP 线性增长（1→31） |
| 同上 `TestProbePasswordChangeKeepsSession` | 改密后旧 refresh 续期 200、旧 access 仍可访问 profile 200、旧口令登录 401 |
| 同上 `TestProbeRawErrorLeakage` | 错误响应回传内部文本：`duplicated key not allowed`、`json: cannot unmarshal string into Go struct field userIn.tenant_id of type uint` |

### 复现说明（只读约束下的做法）

探针运行在仓库之外的临时副本 `/tmp/amsec.wVSHlk/server`（`mktemp -d` + `rtk cp -R server`），复用了项目自带测试基建（`isolatedDB` 走 `AUTOMEDIC_TEST_PG_DSN` 默认 `127.0.0.1:55432` 的独立 schema，用完 `DROP SCHEMA`），**未向被测仓库写入任何文件**，未提交、未推送。仓库内本路唯一新增文件即本报告。如需长期复跑，建议把 P0-1/P0-2/P1-1 三个探针改写成 `server/internal/api/` 下的正式回归测试（当前仓库已有同类隔离测试写法可复用，见 `api/flow_test.go`）。

## 建议顺序（本周）

1. **P0-1**：给权限码加"可授予层级"，`CreateRole`/`UpdateRole` 对非超管拒绝平台码；顺带复核所有租户角色当前实际持有的权限（`role_permissions` 全表比对 `PermissionCatalog` 中的平台码）。
2. **P0-2**：`repo.credential_id` 校验同租户 + 历史数据体检。
3. **P1-1**：`CancelTask` 先校验归属再发信号，并对取消结果做条件更新。
4. **P1-2/P1-4**：改密/重置即吊销全部 refresh token；补齐用户/角色/租户/设置/凭证的审计日志与请求 ID。

第二批：登录限流改为账号维度 + 退避（P1-3）、`patch_template`/`home` 移出 Web 可写集合（P2-1）、错误响应统一脱敏（P2-2）、删除类接口按 `RowsAffected` 返回 404（P2-3）。
