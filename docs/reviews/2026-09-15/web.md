# AutoMedic 前端与 UI 规范审查报告（第 3 路 · 只读）

- **基线**：`3c10e9d`（`fix(web): 统一 token 与响应式，消除死类名和图表色漂移`）
- **范围**：`web/src/**`、`web/index.html`、`web/vite.config.js`、`scripts/check-ui-style.sh`
- **规范基线**：`docs/UI优化方案-2026-09-09.md`（下称《UI 方案》）、`scripts/check-ui-style.sh`；规则 `00-core`/`11-frontend`/`27-frontend-design`/`13-security`/`05-api`/`19-code-development`
- **契约对照**：`server/internal/api/router.go`、`docs/接口文档.md`
- **方式**：只读静态审查 + 4 条可复现命令；未修改任何产品代码

---

## 一、结论先行

**《UI 方案》三项 P0 已真实落地**（响应式栅格、死类名清理、token 落盘），图表色单源、按钮四态 CSS、移动端 44px 触区、首帧主题防闪烁、Login `100dvh` 均可复核通过；`lint:ui` 与 `build` 双绿，token 配色对比度抽检 12 组全部达 WCAG AA。

但**「按钮规范」与「写操作防重」两条没有闭环**，并新增 1 个 P0 级不可逆风险：

| 级别 | 问题 | 一句话影响 |
|---|---|---|
| **P0** | 任务级写操作无 in-flight 守卫，服务端也无状态守卫/幂等 | 双击「重试修复」创建两个 dsh 修复任务；双击「确认修复并推送」重复 commit/push/发布钩子 |
| P1 | `AmButton.vue` 是零引用死组件 | §7.1 的 88px 最小宽度与 16em 截断规则在 139 处真实按钮上完全未生效 |
| P1 | EP 主色派生色阶未桥接 | 二级按钮 hover 底色/边框取 EP 默认蓝 `#18222b/#213d5b`，非 token、非 `--am-primary`，一屏两套蓝 |
| P1 | 写操作 loading 防重只覆盖 9 处，7 个视图创建类写操作裸奔 | 双击「保存/新建」可重复落库（租户/用户/凭证/模型/字典/仓库） |
| P1 | `docs/接口文档.md` 3 处与实现/前端不一致 | WS 鉴权、WS 帧格式、端到端示例均会误导接入方 |
| P2 | 断点第二来源、token 外硬编码色、`VITE_WS_BASE` 死配置、401 去重窗口过窄、页面内无权限门控、内联样式残留、`100vh` 与安全区、WS 重连无退避、UI chunk 体积 | 见 §三 P2；均为局部可收敛项 |

**给协调者的处置建议**：P0 与 P1-1～P1-3 属同一主题（按钮语义与写操作收敛），建议合并为一个「按钮规范收口」小 CL；P1-4 是文档修正，可独立提交。

---

## 二、验证记录（命令与结果）

| # | 命令 | 结果 | 备注 |
|---|---|---|---|
| 1 | `rtk git log -1` | `3c10e9d3d1b02e55f313367cf3e2aa4f4148935f fix(web): 统一 token 与响应式，消除死类名和图表色漂移` | 基线与任务要求一致 |
| 2 | `cd web && rtk npm ci` | `added 148 packages in 6s`，退出码 0 | 成功；仅 3 条 `install-scripts` 未审批告警（esbuild / fsevents / vue-demi），未影响后续 build |
| 3 | `cd web && rtk npm run lint:ui` | `✓ UI token 约束全部通过`（`bash ../scripts/check-ui-style.sh`），退出码 0 | 字号/圆角/hex/断点四项检查均零违规 |
| 4 | `cd web && rtk npm run build` | `vite v6.4.3`，`✓ 2271 modules transformed`，`✓ built in 5.38s`；产物 `index.css 402.84 kB`、`vendor 159.91 kB`、`index 215.01 kB`、`chart 1,036.31 kB`、`ui 1,089.01 kB` | 退出码 0；大 chunk 见 P2-9 |
| 5 | `node -e`（对比度抽检，非任务要求） | 12 组前景/背景全部 ≥4.5:1，最低 `#9a9aa3 on #29292e = 5.19:1` | token 配色满足 §量化标准表对比度行 |

**未覆盖（诚实声明）**：本路为只读静态审查，未启动浏览器，因此《UI 方案》交付前检查要求的「375/768/1024/1440px 四档 + 亮/暗模式实测、Console/Network 检查、Tab 焦点顺序」**未做**，§三中的响应式子项均为静态推断，需第 1/2 路或后续实测确认。

---

## 三、问题清单

### P0-1 任务级写操作无防重：重复 push / 重复 dsh 修复任务

**位置**

- 前端按钮：`web/src/views/TaskDetail.vue:12`（确认修复并推送）、`:13`（驳回）、`:14-22`（重试）、`:23`（取消）
- 前端处理函数：`web/src/views/TaskDetail.vue:331` `confirmFix`、`:341` `rejectFix`、`:351` `retryPushDo`、`:357` `retryTaskDo`、`:372` `cancelTaskDo`
- 列表页同类：`web/src/views/TaskList.vue:127` `retry`、`:138` `cancel`
- 后端：`server/internal/api/task.go:218-246` `ConfirmTask`

**证据**

1. `TaskDetail.vue:331/341/351/357/372` 与 `TaskList.vue:127/138` 六个处理器**均无 `acting`/`saving`/`submitting` 之类 in-flight 标志**，对应按钮也无 `:loading` / 运行时 `:disabled`（全库 `:loading` 绑定仅 14 处，`TaskDetail.vue`、`TaskList.vue` 均为 0）。
2. 唯一的抑制是**状态判断**而非**在途标志**：`TaskDetail.vue:195` `canRetry` 只依赖 `task.value.status`，而刷新要等 `:338` `setTimeout(..., 1500)` / `:354` `setTimeout(..., 800)`；`TaskList.vue:112/117` 的 `canRetry`/`canCancel` 依赖行数据，`load()` 也是异步。因此在刷新窗口内（0.8–1.5s）按钮**仍然可用**，双击可靠触发两次请求。
3. 后端无兜底：`server/internal/api/task.go:218-246` 只做「任务存在 + 归属」校验，**没有 `status == confirming` 状态守卫、没有幂等键**，随后 `go h.exec.Confirm(...)` 无条件执行；`RetryTask` 语义为「复制为新任务」（`docs/接口文档.md:552`），即两次请求 = 两个 dsh 修复任务。
4. `docs/接口文档.md:555` 明确 confirm 的动作链为「重建工作区 → apply 补丁 → commit → push → 钩子」，全部是**不可逆外部副作用**。

**影响**：重复推送代码、重复执行发布钩子、重复消耗 dsh/模型额度；对同一任务产生两条互相竞争的修复流水线。这是本轮唯一「用户一次误操作即造成不可逆后果」的问题。

**规范依据**：《UI 方案》§7.1「异步操作必须 `loading` + 禁重复提交」与验收清单「异步按钮均有 loading + 防重复提交」；`11-frontend.md` MUST「请求层统一处理…重复提交」；`05-api.md` MUST「创建、支付、回调和重试型写操作提供幂等策略」。

**建议**（3 行内可落地）：`TaskDetail.vue` 增加 `const acting = ref('')`，五个处理器首行 `if (acting.value) return` + `acting.value = 'confirm'`，`finally` 复位，并把 `:loading="acting === 'confirm'"` 绑到四个按钮；后端 `ConfirmTask` 用 `WHERE id = ? AND status = 'confirming'` 的条件更新做状态守卫（前端防误触 + 后端防重复，两层都要）。

### P1-1 「按钮规范」只做了四态，规格表与封装全部悬空：AmButton 零引用

**位置**：`web/src/components/AmButton.vue:1-63`（整体）、`:8`（类名输出）、`:52-53`（死样式）；`web/src/styles/index.css:376-392`

**证据**

1. **零引用**：`grep -rn "AmButton" web/src` 只命中 `components/AmButton.vue` 自身；没有任何视图 import。全库 21 个视图/布局文件共 **139 行含 `<el-button`**（`ProjectDetail.vue` 34、`ModelConfig.vue` 11、`TokenList.vue` 10、`ProjectList.vue` 10 为前四），全部是裸 `el-button`。
2. **规格表未生效**：`styles/index.css:376-385` 的 `.el-button` 只设置 `min-height`/`padding-inline`/`border-radius`/`cursor`/`transition`，**无 `min-width`**；`:386` `.el-button--small` 同样无。即《UI 方案》§7.1 规格表的「medium 最小宽度 88px / small 64px」与「超长 `text-overflow: ellipsis` + `max-width: 16em` + `title`」在全库真实按钮上**均未实现**——这正是上轮审计第 9 条「按钮无最小宽度与截断规则」（P1）的状态。
3. **该类名双死**：`AmButton.vue:8` 输出 `am-btn--${type}`（primary/secondary/ghost/danger 四个值），而样式表只定义了 `am-btn--small`(:52)/`am-btn--large`(:53)；`am-btn--${type}` 永远拿不到定义，`am-btn--small/large` 因为 `:class` 不含 size 而**永远不会被输出**。这与上轮 P0「死类名（`am-btn-primary`/`am-btn-soft`）」是同一类缺陷，只是搬进了新组件。
4. 附带：`AmButton.vue:10` 用 `label.length > 12` 判 `title`，与 `:55` 的 `max-width: 16em` 不一致（13–16 字会拿到无意义的 `title`），也不符合 §7.2 参考实现的 16。

**影响**：封装层建了但没接线，规范约束只存在于文档里；144 处（行数计 139）真实按钮仍无最小宽度与截断规则，长按钮文案（如「确认修复并推送」「重试推送」）在窄容器里会撑宽布局而不是省略。同时新增了一份死代码与死类名。

**规范依据**：《UI 方案》§7.1 规格表、§7.2/§7.3 替换清单、§1.2 规则 C2「同语义同实现」；`19-code-development.md`「删除纪律」（无引用组件不得直接删，须先举证报告）+「禁止的过度设计」。

**建议**（二选一，勿两头做）：① 接入 `AmButton`，优先级 `ProjectDetail.vue`(34) → `ModelConfig.vue`(11) → `TokenList/ProjectList`(各 10)；② 若短期不接入，则删 `AmButton.vue`，把 `min-width: var(--am-control-min-w)` + `.am-btn__label` 那两段 ellipsis 规则搬进 `index.css` 的 `.el-button`（约 4 行），立刻覆盖全部真实按钮。

### P1-2 写操作 loading 防重覆盖不足：7 个视图的创建/删除/切换裸奔

**位置**（无任何守卫的写处理器）

| 文件 | 行 | 写操作 |
|---|---|---|
| `web/src/views/TenantList.vue` | `:85` `submit` | 新建/更新租户 |
| `web/src/views/UserList.vue` | `:130` `submit`、`:144` `remove` | 新建/更新/删除用户 |
| `web/src/views/CredentialList.vue` | `:131` `submit`、`:144` `toggle`、`:155` `remove` | 凭证增改删 |
| `web/src/views/ModelConfig.vue` | `:410` `submitProvider`、`:423` `removeProvider`、`:484` `submitModel`、`:499` `toggleModel`、`:500` `setDefault`、`:502` `removeModel` | 厂商/模型增改删 |
| `web/src/views/DictList.vue` | `:167` `submit`、`:188` `toggle`、`:193` `remove` | 字典增改删 |
| `web/src/views/RepoList.vue` | `:158` `submit`、`:176` `testConn`、`:181` `remove` | 仓库增改删/连通性测试 |
| `web/src/views/EventList.vue` | `:172` `replay` | 事件重放（会新开修复任务） |

**证据**：`grep -rn ":loading=" web/src/views web/src/layout` 仅 14 处，集中在 9 个已守卫视图——`Settings.vue:105`、`ProjectList.vue:139`、`Login.vue:24`、`ProjectDetail.vue:186/281/337/414`、`TokenList.vue:81`、`RepoReviewDrawer.vue:31/134`、`RoleList.vue:53/110`、`AppLayout.vue:76`。上表 7 个文件中 `TenantList.vue`、`ModelConfig.vue`、`DictList.vue`、`TaskDetail.vue` 的 `:loading`/`loading` 命中数为 **0**。这些 `submit()` 形如 `if (form.value.id) await updateX(...) else await createX(...)`（如 `TenantList.vue:91-92`、`ModelConfig.vue:416-417`），中间没有任何在途判断，服务端创建接口也无幂等键（`router.go:99/106/120/128/139/174/190` 均为直连 create handler）。

**影响**：双击「保存/新建」产生重复记录（租户、用户、凭证、字典项、厂商/模型、仓库）；`EventList.vue:172` 重复点击会重复重放事件并额外开任务。属于「误操作率最低」北极星指标的直接违背。

**规范依据**：同 P0-1（《UI 方案》§7.1 + 验收清单；`11-frontend.md` MUST 请求层统一处理重复提交）。

**建议**：在这 7 个视图各加一个 `const saving = ref(false)`，`submit()` 改 `if (saving.value) return; saving.value = true; try { ... } finally { saving.value = false }`，按钮 `:loading="saving"`；每个视图 3–5 行，不引入新抽象。

### P1-3 EP 主色派生色阶未桥接：二级按钮 hover 走 EP 默认蓝（一屏两套蓝）

**位置**：`web/src/styles/index.css:77`（只重定向 `--el-color-primary`）

**证据**

1. `styles/index.css:77-98` 的 EP 桥接只覆盖 `--el-color-primary` 与字号/圆角/表面/文本，**没有**覆盖 EP 的派生色阶。
2. `web/node_modules/element-plus/theme-chalk/dark/css-vars.css`（单行）中派生色阶是**硬编码的 EP 默认蓝**：`--el-color-primary:#409eff`、`--el-color-primary-light-3:#3375b9`、`-light-5:#2a598a`、`-light-7:#213d5b`、`-light-8:#1d3043`、`-light-9:#18222b`、`-dark-2:#66b1ff`。
3. `web/node_modules/element-plus/theme-chalk/el-button.css:1` 的基础 `.el-button`（= secondary）把这些变量接进了交互态：`--el-button-hover-text-color:var(--el-color-primary)`、**`--el-button-hover-bg-color:var(--el-color-primary-light-9)`**、**`--el-button-hover-border-color:var(--el-color-primary-light-7)`**、`--el-button-outline-color:var(--el-color-primary-light-5)`、`:active` 的 `--el-button-active-border-color:var(--el-color-primary)`。
4. 因此占绝大多数的裸 `el-button` 悬停时：底色 `#18222b`、边框 `#213d5b`（EP 蓝，非 token），文字转 `--am-primary`；与 `styles/index.css:393-424` 为 primary/danger/link 精心定义的 token 态形成**两套蓝**。

**影响**：违反《UI 方案》§3.1 状态对照表 secondary 行（规定 default `bg --am-bg-inset` / 边 `--am-border`，hover **只换边框** `--am-border-strong`，press `bg --am-bg`）；实际实现是 hover 同时改文字色+底色+边框，并且引入 EP 默认蓝色族。这是全站最高频的交互态（几乎所有次要按钮）。

**规范依据**：《UI 方案》§1.2 规则 C1「唯一事实源」、§3.1 状态对照表、验收清单「同一页面无两套主色蓝」；`27-frontend-design.md` MUST「色彩纪律：单一签名 accent 色主导」「组件库默认样式必须经项目主题 token 定制，禁止裸用默认主色交付」。

**建议**：在 `styles/index.css:77` 附近补 6 行桥接，例如 `--el-color-primary-light-3/-5/-7/-8/-9` 与 `--el-color-primary-dark-2` 全部分别指向 `color-mix(in srgb, var(--am-primary) X%, var(--am-bg))`（或直接给 `--am-*` 常量）；再按 §3.1 给 `.el-button`（默认型）补 `hover{border-color: var(--am-border-strong)}`、`active{background: var(--am-bg)}` 两条，让 secondary 与 primary 的视觉权重真正分层。

### P1-4 `docs/接口文档.md` 与实现/前端 3 处不一致

**位置与证据**

1. **WS 鉴权描述错误**：`docs/接口文档.md:54` 与 `:654` 均写 `/ws/tasks/:id`「无鉴权（内网使用）」。实际 `server/internal/api/router.go:204-205` 对该路由挂了 `d.Auth.Middleware()` + `auth.RequirePerm(model.PermTaskRead)`，令牌由 `Sec-WebSocket-Protocol: automedic.<jwt>` 携带（`server/internal/auth/auth.go:348-362` 的 `bearerToken` 兜底读该头），前端 `web/src/api/index.js:270` 正是这样传的。
2. **WS 帧格式错误**：`docs/接口文档.md:539-543` 给出 `{ "type": "log", "data": { "seq": ..., "stream": ..., "content": ... } }`（多一层 `data`）。实现是**扁平**结构 —— `server/internal/ws/hub.go:14-23` 的 `Message` 直接 `json:"type"/"seq"/"stream"/"content"/"status"/"stage"`；前端也按扁平解析（`web/src/views/TaskDetail.vue:266-271` 读 `msg.seq`/`msg.stream`/`msg.content`/`msg.status`/`msg.stage`）。即**实现与前端一致，只有文档错了**（按文档写客户端的第三方会拿不到日志）。
3. **端到端示例已失效**：`docs/接口文档.md:663-693` 的 6 步示例每一步都带 `-H "X-Admin-Token: $ADMIN"`，而同一文档 `:52` 明确「`server.admin_token`（旧 `X-Admin-Token` 令牌）**已废弃，无任何中间件读取**，请勿再依赖」。照抄示例第 1 步即 401。

**影响**：接入方按文档实现 WS 会握手 401 或收到解析不了的帧；示例脚本不可运行，文档作为契约的可信度受损。第 1 点还有安全沟通风险：文档声称「无鉴权」可能诱导部署方不做鉴权评估。

**规范依据**：`05-api.md` MUST「字段名、类型、可空性和枚举值必须有明确契约」、提交前检查「文档、客户端类型和实现同步更新」；`00-core.md`「不凭规则文件猜测项目字段、API…先从源码确认」。

**建议**：三处按实现改写（WS 鉴权段补「子协议传 JWT，需 `task:read`，部署时 Nginx 必须放行 `Upgrade`（模板见 `deploy/nginx.conf:40-49`）」，帧格式去掉 `data` 层，示例改用 `Authorization: Bearer`）。此项仅改文档，可独立提交，**不阻塞**前端改动。

### P2 清单

| # | 位置 | 证据与影响 | 规范依据 | 建议 |
|---|---|---|---|---|
| P2-1 | `web/src/layout/AppLayout.vue:100` | `ref(window.innerWidth < 1024)` 仍是**断点第二来源**：`constants/breakpoints.js` 已存在、`useBreakpoint` 已用于 `:94`，但初始值仍读裸 `innerWidth` 与魔法数 `1024`。`check-ui-style.sh:31` 只查 CSS `@media`，JS 侧漂移查不出来 | 《UI 方案》§6.1 规则 R1「断点单源」 | `ref(window.matchMedia(MQ.ltDesktop).matches)`，1 行 |
| P2-2 | `web/src/styles/index.css:234/241/257-259/262/279-281`；`web/src/App.vue:18` | token 体系外仍有硬编码色：终端/diff 调色 `#0d0d0f`、`#c8d3e6`、`#7aa2f7`、`#f7768e`、`#7fd962`（6 处），以及 `var(--am-bg, #161618)` 兜底值。`check-ui-style.sh:22-25` 的 hex 检查只扫 `views/ layout/ composables/ constants/`，注释说白名单是「index.css 与 index.html 的首帧兜底」，实际把整个 `styles/` 目录排除，**这些色永远不会被 lint 拦下** | 《UI 方案》§1.2 规则 C1「唯一事实源」 | 提为 `--am-term-bg/--am-term-text/--am-term-sys/--am-term-err/--am-term-ok` 等 token（6 行）；lint 白名单从「整目录」收窄为「文件+行」精度 |
| P2-3 | `deploy/hk/remote.sh:277` | 构建期注入 `VITE_WS_BASE`，但 `web/src` 全库**从未读取**（WS 地址由 `location.host`+`BASE_URL` 推导，`web/src/api/index.js:255-260`）。死配置会让后续维护者误以为 WS 基址可配 | `19-code-development.md`「配置项必须有当前消费者…禁止预留未使用开关」 | 二选一：删掉该注入，或让 `taskWSURL()` 优先取 `import.meta.env.VITE_WS_BASE` |
| P2-4 | `web/src/api/index.js:50-62`、`:57-59` | ① 401 去重标志用 `setTimeout(..., 0)` 复位，**只在同一事件循环内合并**；前后脚到达的并发 401 会重复弹「登录已失效」（且 `clearSession()` 已清掉 refresh_token，后到的请求走 `:107` 再触发一次）。② `router.replace('/login')` 不带 `?redirect=`，而路由守卫 `web/src/router/index.js:67` 带了 —— 同一件事两套实现，会话过期后用户回不到原页面 | `11-frontend.md` MUST 错误态处理；`19-code-development.md` DRY「同一业务知识只有一个权威表述」 | 去重改用 3s 时间窗（登录成功时复位）；过期跳转复用 `redirect` 参数，与守卫保持一处权威 |
| P2-5 | `web/src/views/*`（全库） | 页面内写操作按钮**不做权限门控**：`grep "can('"` 在 `web/src` 命中 0 处，唯一权限逻辑是路由级 `meta.perm`（`router/index.js:27-43`）；只有 `isSuper` 影响表单字段（`UserList.vue:61/78`、`RoleList.vue:132`）。仅有 `project:read` 的用户进入 `/projects` 会看到必然 403 的「新建/编辑/删除」 | `11-frontend.md` MUST「处理加载、空态、错误、**权限**、重试…不能只实现成功路径」（服务端仍须最终鉴权，此处只解决 UX） | 按 `meta.perm` 的一一映射用 `can('project:create')` 控制按钮显隐；不需要批量重构，先覆盖 5 个高频列表页 |
| P2-6 | `web/src/views/*.vue` | 内联 `style="` 仍约 **124 行**（`ProjectDetail.vue` 25、`TaskDetail.vue` 14、`RuleList.vue` 13、`EventList.vue` 12…），《UI 方案》§1.1 规则 D5 的「内联样式清零」（原 155 处）只完成约两成 | 《UI 方案》§1.1 规则 D5、P2 路线图第 8 条 | 按方案既定的「改到哪个文件顺手清理该文件」节奏推进，不单独立项（其中 `font-size` 已走 token，属可接受残留） |
| P2-7 | `web/src/layout/AppLayout.vue:160`、`:202`；`web/src/views/RepoTreeDrawer.vue:114`；`web/src/views/RepoReviewDrawer.vue:463/578/634`；`web/index.html:5` | Login 已按 §5.4 改 `100dvh`（`Login.vue:80`），但**应用外壳与抽屉仍是 `100vh`**（`.layout{height:100vh;overflow:hidden}`），iOS Safari 地址栏展开时底部内容被遮挡且无法滚出。另外 `viewport-fit=cover` 已声明却全库无 `env(safe-area-inset-*)` 适配 | 《UI 方案》§5.4（同一问题域）、§量化标准「固定导航栏必须给内容留出高度」 | `100vh → 100dvh`（有 `@supports` 回退），并给 `.main`/抽屉底部补 `padding-bottom: env(safe-area-inset-bottom)` |
| P2-8 | `web/src/views/TaskDetail.vue:251-260` | WS `onclose` 固定 2s 重连，**无退避无上限**：服务不可用或令牌失效时每 2 秒打一次握手（服务端每次 401 并写日志），页面开着就一直打 | `11-frontend.md` SHOULD「对外部依赖设置明确的超时、重试边界和降级策略」 | 指数退避（2s→4s→8s，上限 30s）+ 连续失败 N 次后停止并由轮询兜底 |
| P2-9 | `web/vite.config.js:24`；构建产物 | `chunkSizeWarningLimit: 1500` 把默认 500 阈值抬高以消除告警，实际 `ui` chunk 1,089 kB（gzip 342 kB）、`chart` 1,036 kB（gzip 343 kB）**无差别首屏加载**（登录页也用不到 echarts） | `11-frontend.md` SHOULD「图片、代码分包和缓存按真实性能数据优化」；`19-code-development.md`「不用配置开关隐藏坏味道」 | 图表/统计页改 `defineAsyncComponent` 或动态 `import()` 路由级懒加载，让 echarts 只在 `/statistics`、`/` 加载；阈值回调默认值 |

---

## 四、上轮已修项复核

| 复核项 | 本轮实测结论 | 证据 | 判定 |
|---|---|---|---|
| **token 存储与 401 递归** | 访问令牌仅内存（页面刷新靠 refresh 重建）；续期接口用 `_retry: true` + `isAuthEndpoint` 双保险，**不会递归续期**；refresh 失败只清一次会话并跳登录 | `web/src/api/index.js:10,18-27,83,121-123,127-139`；`:79-81` 注释与实现一致 | ✅ **已修**（残留 2 处健壮性问题见 P2-4） |
| **权限死循环** | `landingPath()` 只回「无 perm 或 can(perm)」的首个静态路由，无权限时回 `/403`；守卫对 `target === to.path` 再兜一层。逐一推演「无权限用户 / 只有 `task:read` / 超管 / 已登录访 `/login` / 直访 `/403`」五种路径均无环 | `web/src/router/index.js:52-77` | ✅ **已修** |
| **WS 参数** | 令牌**已不在 URL**，改由子协议 `['automedic.<jwt>', 'automedic']` 传递；服务端 `bearerToken` 兜底解析该头，且在 Upgrade 前把子协议改写为 `automedic` 避免回写令牌；浏览器校验的「服务端选中值必须在客户端列表内」已由裸 `automedic` 满足 | 前端 `web/src/api/index.js:255-271`；服务端 `server/internal/api/router.go:204-207`、`server/internal/auth/auth.go:348-362`、`server/internal/ws/hub.go:144-153,170-187` | ✅ **已修**（文档未同步见 P1-4；重连退避见 P2-8） |
| **TaskDetail 世代号** | `loadGen` 在 `loadAll` 入口自增并捕获 `gen`，在**每个 await 之后**（`getTask`/`taskLogs`/`taskPatch`）都比对 `gen !== loadGen` 提前返回；`finally` 也带 `gen === loadGen` 守卫；`watch(id)` 切换任务时先 `loadGen++` 再重置日志/补丁/WS/轮询 | `web/src/views/TaskDetail.vue:169,213-243,379-390` | ✅ **已修** |
| **写操作 loading 防重** | **部分落地**：仅 9 处有 `:loading`（`Settings:105`、`ProjectList:139`、`Login:24`、`ProjectDetail:186/281/337/414`、`TokenList:81`、`RepoReviewDrawer:31/134`、`RoleList:53/110`、`AppLayout:76`）；任务级 5 个写操作与 7 个视图的创建/删除/切换**完全无守卫** | 见 P0-1、P1-2 | ❌ **未闭环**（P0-1 + P1-2） |
| **死类名与图表色漂移** | ① 上轮死类名 `am-btn-primary`/`am-btn-soft` 全库 0 引用，**已删干净**；② 图表色单源达成：`Dashboard.vue`/`Statistics.vue` 的轴/网格/系列/图例色**全部**取自 `AM_COLORS`，视图内无 echarts 裸 hex；③ 语义色类 `.am-value--warning/success/danger`（`index.css:221-223`）已取代内联色 | `grep -rn "am-btn-primary\|am-btn-soft" web/src` → 0；`grep -rn "AM_COLORS" web/src` → `Dashboard.vue:89,118-145`、`Statistics.vue:113,157-237` 全覆盖 | ⚠️ **主体已修**，但**新增了同类的死类名**（`AmButton.vue:8` 发了没定义的 `am-btn--{type}`、`:52-53` 定义了不发的 `am-btn--{size}`）→ P1-1 |
| **响应式与断点** | ① `el-col` 响应式**已补**：`Dashboard.vue:28,38` `:xs="24" :md="14/10"`、`Statistics.vue:51,57,66,74` `:xs="24" :md="16/8/12/12"`（P0 图表被压扁问题消除）；② `useBreakpoint` 用 `matchMedia` 替换了原来 `AppLayout` 的 `resize` 轮询，监听在 `onUnmounted` 清理；③ CSS 断点已统一为 `767.98/1023.98`（`index.css:603,610` 等 7 处视图/布局媒体查询全部合规） | `web/src/composables/useBreakpoint.js:7-29`、`web/src/layout/AppLayout.vue:94-104`、`web/src/constants/breakpoints.js:6-11` | ⚠️ **主体已修**，残留 `AppLayout.vue:100` 的 `innerWidth < 1024`（P2-1）与 `100vh`（P2-7） |
| **UI token 落地（字号/圆角/颜色/断点）** | ① 字号 7 档 / 圆角 5 档 / 控件尺寸 / 间距 / 层级 / 动效 token 齐备（`index.css:5-98`），字号与圆角在 `views/layout/styles` **零裸值**；② EP 桥接把基础字号从 13.5 → `var(--am-font-md)`=14px、圆角 10 → 8px；③ 断点统一；④ 颜色抽检 **12 组对比度全部 ≥4.5:1**（最低 5.19:1）；⑤ `lint:ui` 四项检查**全绿** | `styles/index.css:29-53,74-98`；`npm run lint:ui` → `✓ UI token 约束全部通过`；`node` 对比度脚本 12/12 PASS | ⚠️ **已落地**，但颜色 token 有两处漏洞：EP 派生色阶未桥接（P1-3）与 `styles/` 目录内 6 处硬编码色逃过 lint（P2-2） |
| **按钮规范与组件四态** | ① **四态 CSS 已补齐且质量达标**：primary（`:393-405`）、danger（`:407-419`）、link/ghost（`:422-424`）、统一 `:focus-visible` 2px outline（`:388-391`）、`.is-disabled{opacity:.45;cursor:not-allowed}`（`:392`）、过渡统一 `--am-duration/--am-easing`（`:381-384`）、移动端全按钮 44px+相邻 8px 间距（`:633-640`）、`prefers-reduced-motion` 兜底（`:644-652`）——**这部分做得干净**；② 但规格表未落地：无 `min-width`、无截断+title、`AmButton` 零引用（P1-1）；③ **secondary（默认型）三态在 index.css 无任何规则**，实际由 EP 默认接管且取 EP 蓝色阶（P1-3） | `web/src/styles/index.css:376-424,633-652`；`web/src/components/AmButton.vue`（零引用）；`element-plus/theme-chalk/el-button.css:1` | ⚠️ **四态已修，按钮规格未修** |

---

## 五、规范依据索引

- `docs/UI优化方案-2026-09-09.md`：§1.1 规则 D5；§1.2 规则 C1/C2；§3.1 状态对照表；§5.4 登录页视口；§6.1 规则 R1 断点单源；§7.1/§7.2/§7.3 按钮规范；§「量化标准」表；§「交付前检查」；§「验收清单」
- `scripts/check-ui-style.sh`：字号（`:11`）、圆角（`:17`）、hex 白名单（`:22-25`）、断点（`:31`）
- `~/.config/ai-rules/rules/`：`00-core.md`（最小范围/证据/OneDrive 禁用）、`11-frontend.md`（MUST 请求层统一处理重复提交与明文错误态、SHOULD 浏览器不存长期 Token、生产排查与浏览器证据）、`27-frontend-design.md`（MUST 色彩纪律与组件库 token 定制、量化标准表、交付前检查）、`13-security.md`（鉴权/最小权限/文档与实现一致性）、`05-api.md`（MUST 契约明确与幂等、提交前检查文档同步）、`19-code-development.md`（KISS、删除纪律、配置项须有消费者、DRY 单一权威）
- 契约对照：`server/internal/api/router.go:99-192,204-207`、`server/internal/auth/auth.go:348-362`、`server/internal/api/task.go:218-246`、`server/internal/ws/hub.go:14-23,144-187`、`docs/接口文档.md:52-54,537-543,650-657,663-693`

---

*本报告为只读审查产物，未修改任何产品代码；所有 `file:line` 均可在基线 `3c10e9d` 直接复核。*
