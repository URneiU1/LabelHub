# LabelHub Full Code Review - 2026-06-01

## Scope And Verdict

本轮按 `goal.md` 执行交付前只读审查。覆盖任务创建、题目导入、领题、草稿、提交、多人共识、AI 审核、人工审核、仲裁、导出、鉴权、附件、Worker、Outbox、迁移、部署、前端可靠性、测试和 Demo Day 路线。

本轮未修改业务代码、未提交、未推送、未部署、未启动容器或后台服务。唯一新增文件是本报告。

结论：**当前版本不建议直接作为 S8 完成交付部署。** 未发现可直接远程接管主机或越权读取全部租户数据的 Critical 问题，但存在 7 个 High 阻塞项。其中最危险的是租约回收与审核状态共用 `task_items.status='claimed'`，会把审核中的题目重新放回领取池，并允许旧审核结果终结后来被其他人重新领取的题目。

## Findings

### H-01 [High] 租约回收会重开审核中的题目，旧审核结果可终结新认领

- 位置：`apps/api/internal/service/submission/claim.go:57`，`apps/api/internal/service/submission/helpers.go:93`，`apps/api/internal/service/submission/submission.go:124`，`apps/api/internal/service/review/review.go:243`，`apps/ai-worker/cmd/worker/ai_review.go:303`
- 触发条件：题目提交后进入 `ai_reviewing` 或 `human_reviewing`，审核耗时超过任务的 `lease_timeout_minutes`，之后任意标注员再次领题。
- 影响：`releaseExpiredClaims` 会扫描全部超时的 `task_items.status='claimed'` 并重置为 `available`。审核中的题目也使用同一个 item 状态，因此可被第二位标注员重新领取。旧的人审或 AI 完成逻辑只校验 item 仍为 `claimed`，不校验当前 `claimed_by`，可把后来重新领取的题目直接写成 `finished`。
- 证据链：`ClaimNext -> releaseExpiredClaims -> UPDATE task_items status=available`；提交后 item 保持 `claimed`；`review.Apply` 和 `worker.complete` 最终均以 `id + status='claimed'` 写完 item。
- 最小修复：只回收仍处于编辑态的认领，例如关联 submission 仅为 `draft/revising`；审核态不得回收。最终写 item 时增加预期认领人或显式 item review 状态校验。
- 建议测试：集成测试覆盖超时的 `human_reviewing` 不会被重领；旧 AI 完成回调不能终结已被其他人重新领取的 item。

### H-02 [High] 发布后仍可覆盖题目 payload，AI 审核读取的是可变当前值

- 位置：`apps/api/internal/handler/task_manage.go:28`，`apps/api/internal/handler/task_manage.go:58`，`apps/api/internal/handler/task.go:558`，`apps/ai-worker/cmd/worker/ai_review.go:215`
- 触发条件：Owner 在任务发布后或提交等待 AI 审核期间调用批量题目编辑；发布后继续导入也会改变任务题集。
- 影响：提交记录只保存 answer revision，没有保存题目 payload 快照。Worker 审核时 join 当前 `task_items.payload`。因此同一 submission 的 AI 输入可以在提交后被改写，人工审查和审计也无法还原提交时事实。发布后的题集规模也可能变化。
- 证据链：`BatchUpdateItems` 只校验 task ownership 和 item task_id，不校验任务或 item 状态；`ImportItems` 同样没有发布态限制；`loadReviewInput` 从 `task_items ti` 读取当前 `ti.payload`。
- 最小修复：发布后禁止覆盖既有 item payload；如果业务必须支持热更新，新增 item version 和 submission payload snapshot，并在 AI、人审和导出统一读取快照。
- 建议测试：发布任务并提交后尝试批量改 item，接口应拒绝；或验证 AI 输入固定为提交时 snapshot。

### H-03 [High] 嵌套 FileUpload 不会绑定到 revision，24 小时后可被清理

- 位置：`apps/web/src/renderer/SchemaRenderer.tsx:48`，`apps/web/src/renderer/SchemaRenderer.tsx:59`，`apps/web/src/renderer/widgets/FileUpload.tsx:31`，`apps/api/internal/service/submission/uploads.go:100`，`apps/api/internal/service/submission/uploads.go:107`，`apps/api/cmd/server/main.go:261`
- 触发条件：Designer 将 `FileUpload` 放进 `Group` 或 `Tabs`，标注员上传并提交。
- 影响：前端支持递归渲染嵌套字段，但后端附件扫描只遍历根级 `schema.Fields`。提交返回成功，上传记录却没有绑定 revision，临时文件清理器随后可将真实作答证据视为孤儿删除。
- 证据链：Renderer 递归进入 `Group/Tabs`；FileUpload 把 `storageKey` 写入答案；`uploadedFileKeys` 只循环根字段并过滤 `Widget == "FileUpload"`；清理器删除超时临时文件。
- 最小修复：后端递归遍历 schema leaf fields；提交时对答案中看起来像 upload key 的字段做完整归属校验，不能静默跳过。
- 建议测试：Group 和 Tabs 各放一个 FileUpload，提交后确认 uploaded_file 绑定 revision，清理器不删除文件；浏览器 smoke 下载附件。

### H-04 [High] Redis 短时故障会让 Outbox 永久进入 failed，缺少恢复入口

- 位置：`apps/api/internal/service/outbox/publisher.go:82`，`apps/api/internal/service/outbox/publisher.go:88`，`apps/api/internal/service/outbox/publisher.go:121`，`apps/api/internal/service/outbox/publisher.go:128`，`apps/api/cmd/server/main.go:197`
- 触发条件：Redis 在 Outbox publisher 连续约 5 次轮询期间不可用。默认轮询间隔为 1 秒。
- 影响：事件约 5 秒后变为 `failed`。Publisher 只领取 `pending`，没有自动重试或运维 requeue。AI review 尚有 sweeper 兜底，但 export 事件可永久卡死。
- 证据链：`claimPending` 只查询 `StatusPending`；enqueue 失败递增 `retry_count`，达到 5 后写 `StatusFailed`；stale recovery 只恢复 `processing`。
- 最小修复：为瞬态失败增加指数退避和 `next_attempt_at`；提供 operator requeue；不要将 Redis 短时中断直接视为不可恢复终态。
- 建议测试：fake enqueue 连续失败超过 5 次后恢复，事件最终仍可 publish；覆盖 export 事件。

### H-05 [High] 仲裁后端已存在，但 Reviewer UI 没有仲裁队列入口

- 位置：`apps/api/internal/handler/reviewer.go:129`，`apps/api/internal/handler/reviewer.go:172`，`apps/web/src/modules/reviewer/Queue.tsx:372`
- 触发条件：多人重复标注答案冲突，submission 进入 `needs_arbitration`。
- 影响：后端支持按 `status=needs_arbitration` 查询，但 Reviewer 页面固定请求 `/reviewer/submissions`，没有状态参数、切换入口或冲突对比视图。正常用户无法处理 S8 的核心闭环。
- 证据链：Reviewer API 允许 `human_reviewing` 和 `needs_arbitration`；前端队列请求不带 query，运行时代码没有仲裁筛选入口。
- 最小修复：Reviewer 增加普通审核/仲裁 tab 或 filter，仲裁详情展示同 item 的冲突答案和证据。
- 建议测试：制造两份冲突答案，Reviewer UI 能看到、选择并仲裁，最终 item 进入 terminal 状态。

### H-06 [High] 公网演示仍为明文 HTTP，登录凭据和 JWT 可被窃听重放

- 位置：`docs/HANDOFF.md:6`，`docs/HANDOFF.md:18`，`docs/HANDOFF.md:20`，`submission/DEMO_ENV.md:12`，`apps/web/src/shared/api/client.ts:33`
- 触发条件：评估人员通过公网、共享网络或不可信链路访问当前 IP 直连演示站。
- 影响：演示账号密码、存于 localStorage 的 access token、API 流量和 asynqmon basic auth 都经过明文 HTTP，可被旁路观察并重放。该风险不适合作为公开交付默认形态。
- 证据链：交付文档明确记录 `http://43.155.210.70`、`CADDY_SITE_ADDRESS=:80` 和 HTTP asynqmon；Web 将 access token 持久化到 localStorage。
- 最小修复：绑定域名并启用 HTTPS，HTTP 强制跳转 HTTPS，调整 CORS，切换后轮换账号、JWT、Redis 和 asynqmon 凭据。
- 建议测试：`curl` 验证 HTTP 仅返回 HTTPS redirect；浏览器登录和 `/asynqmon/` 全程 TLS；检查 HSTS。

### H-07 [High] `010_overlap_arbitration.down.sql` 在真实 overlap 数据下无法回滚

- 位置：`apps/api/internal/migration/010_overlap_arbitration.up.sql:1`，`apps/api/internal/migration/010_overlap_arbitration.down.sql:1`
- 触发条件：升级后任意 item 已有两位不同 labeler 的 submission，再执行 down migration。
- 影响：down 文件先恢复 `UNIQUE KEY uk_item(item_id)`，此时重复 item_id 已存在，MySQL 会直接失败。紧急回滚路径不可用，应用回退可能卡在启动迁移阶段。
- 证据链：up 将唯一约束从 `item_id` 改为 `(item_id,labeler_id)`；down 在合并、归档或拒绝重复行之前先添加单列唯一索引。
- 最小修复：明确并实现回滚数据策略：归档重复 submission 后再加唯一索引，或让 down 在 preflight 阶段以清晰错误拒绝并要求 restore。
- 建议测试：迁移 smoke：up、插入同 item 两条 submission、down，确认策略可执行。

### M-01 [Medium] overlap 共识比较完整答案，带附件时天然容易冲突

- 位置：`apps/api/internal/service/submission/overlap.go:38`，`apps/api/internal/service/submission/overlap.go:47`，`apps/api/internal/handler/upload.go:125`，`apps/api/internal/handler/upload.go:313`
- 触发条件：overlap 题含 FileUpload，两个标注员上传各自证据后提交语义相同的标签。
- 影响：共识逻辑 canonicalize 后比较完整 JSON；附件 key 每次随机生成并带扩展名。即使语义字段相同，附件 key 也不同，答案会进入不必要的仲裁。
- 证据链：`canonicalAnswer` 对完整 answer 做精确比较；`storageKey` 使用随机 32 字节。
- 最小修复：按 schema 定义共识投影，排除附件身份字段或为字段配置 comparator。
- 建议测试：相同语义标签配不同 upload key 时按产品规则达成共识。

### M-02 [Medium] overlap 达成共识后，较早提交的 peer 会长期停在 submitted

- 位置：`apps/api/internal/service/submission/submission.go:124`，`apps/api/internal/service/submission/submission.go:173`，`apps/api/internal/service/submission/submission.go:179`
- 触发条件：第一位 labeler 提交后等待，第二位 labeler 提交相同答案并达成共识。
- 影响：调度 AI、人审或自动通过只作用于当前 submission。较早 peer 没有明确 terminal 或 evidence 状态，可能长期停在 `submitted`，污染状态统计、Labeler 历史和审计解释。
- 证据链：waiting 只释放 claim；consensus 调度当前 `sub`；只有 conflict 分支批量更新 peers 为 `needs_arbitration`。
- 最小修复：定义 peer 证据终态，例如 `consensus_evidence/superseded`，或将 item outcome 与 evidence submission 明确拆开统计。
- 建议测试：两人相同答案后校验两条 submission 的最终状态与统计。

### M-03 [Medium] reject 会完成 item，但不会增加 task.finished_items

- 位置：`apps/api/internal/service/review/review.go:243`，`apps/api/internal/service/review/review.go:253`，`apps/api/internal/handler/stats.go:82`，`apps/web/src/modules/owner/TaskManagePanel.tsx:126`
- 触发条件：人工审核对 submission 执行 reject。
- 影响：item 进入 `finished`，但任务完成计数只在 approve 时增加。Dashboard 和 Owner 列表展示的进度可能永远达不到完成。
- 证据链：approve/reject 都写 item finished；只有 `to == approved` 才更新 task `finished_items + 1`；统计读取该字段。
- 最小修复：明确 `finished_items` 语义。若表示已处理 item，则所有 terminal item 均累加；若表示 approved，则改名并单独提供完成计数。
- 建议测试：reject 后 task 进度按定义变化，approved/rejected 分项仍准确。

### M-04 [Medium] 仲裁自动拒绝 sibling submission 时缺少对应审计日志

- 位置：`apps/api/internal/service/review/review.go:237`，`apps/api/internal/service/review/review.go:263`
- 触发条件：Reviewer 在仲裁队列选择一条冲突 submission 并 approve 或 reject。
- 影响：同 item 的 sibling 被批量改为 rejected，但审计日志只写当前 submission。后续无法解释 sibling 为什么、由谁、在何时被拒绝。
- 证据链：sibling bulk update 和当前 submission audit write 位于同一事务，但没有 sibling audit。
- 最小修复：写 item 级 arbitration audit，记录全部 sibling id；或为每个 sibling 写状态迁移日志。
- 建议测试：仲裁后每个受影响 submission 都可从 audit 还原决策。

### M-05 [Medium] 仲裁 Reviewer 无法下载冲突 submission 的附件

- 位置：`apps/api/internal/policy/policy.go:28`，`apps/api/internal/handler/upload.go:301`
- 触发条件：Reviewer 打开 `needs_arbitration` submission，并尝试读取其 FileUpload 证据。
- 影响：Reviewer policy 允许读取仲裁 submission，但附件下载白名单只包含 `human_reviewing/approved/rejected`，漏掉 `needs_arbitration`。仲裁员无法查看关键证据。
- 证据链：队列读取和文件下载使用了不同状态白名单。
- 最小修复：附件下载状态加入 `needs_arbitration`，保持 task-level reviewer 授权校验。
- 建议测试：仲裁 reviewer 可下载；未分配 reviewer 和其他租户仍拒绝。

### M-06 [Medium] AI sweeper 用 created_at 判断 running 超时，会扫掉刚开始运行的任务

- 位置：`apps/api/internal/service/aireview/sweeper.go:119`，`apps/api/internal/service/aireview/sweeper.go:127`
- 触发条件：AI review 在 Redis 队列等待超过 stall timeout，之后刚被 Worker 领取并进入 running。
- 影响：Sweeper 仍按旧 `created_at` 将其视为 stalled，可能在合法 LLM 请求进行中切到人工 failover，浪费成本并丢弃有效结果。
- 证据链：查询同时包含 `pending/running`，统一使用 `ar.created_at < cutoff`；模型没有 running started timestamp。
- 最小修复：增加 `started_at/updated_at`，pending 按排队时间、running 按开始或心跳时间分别判断。
- 建议测试：排队很久但刚进入 running 的 review 不被 sweep；真正卡死 running 会被恢复。

### M-07 [Medium] 发布冻结存在并发竞态，检查后仍可能写入冻结字段

- 位置：`apps/api/internal/handler/task.go:239`，`apps/api/internal/handler/task.go:320`，`apps/api/internal/handler/task.go:331`
- 触发条件：一个请求编辑冻结字段，另一个请求并发把任务从 draft 发布。
- 影响：编辑请求先读取 draft，通过预检查；发布完成后，编辑请求仍以 `WHERE id=?` 更新，可能在 published 状态修改 distribution、overlap 或 sampling。
- 证据链：冻结判断和 UPDATE 之间没有事务锁；UPDATE 没有 `status='draft'` 条件。
- 最小修复：冻结字段 UPDATE 使用 `WHERE id=? AND status='draft'` 并校验 RowsAffected，或事务内锁 task。
- 建议测试：并发 barrier 测试 publish/edit，发布后冻结字段不得变化。

### M-08 [Medium] 多个 JSON 入口仍没有请求体大小上限

- 位置：`apps/api/internal/handler/body_limits.go:29`，`apps/api/internal/handler/auth.go:51`，`apps/api/internal/handler/auth.go:70`，`apps/api/internal/handler/llm.go:34`，`apps/api/internal/handler/reviewer.go:386`，`apps/api/internal/handler/reviewer.go:432`，`apps/api/internal/handler/export.go:81`，`apps/api/internal/handler/task.go:472`
- 触发条件：外部或已登录客户端发送超大 JSON body。
- 影响：部分接口已经使用 `bindLimitedJSON`，但 login、refresh、inline LLM、review、batch review、异步 export 创建和 baseline 更新仍直接 `ShouldBindJSON`。可造成不必要的内存与 CPU 压力。
- 证据链：已有统一 helper，但未覆盖全部不可信 JSON 入口。
- 最小修复：按接口用途设置小上限并统一走 `bindLimitedJSON`；反向代理再设置全局兜底上限。
- 建议测试：每个入口对超限 body 返回 413。

### M-09 [Medium] 旧同步 JSON 导出仍开放，加载全量数据到 API 内存

- 位置：`apps/api/internal/handler/export.go:38`，`apps/api/internal/handler/export.go:57`，`apps/api/internal/service/export/export.go:30`
- 触发条件：Owner 对大任务调用同步 JSON export。
- 影响：异步导出已经存在，但旧同步接口仍可被调用。它在 API 请求内加载并拼接完整结果，容易造成内存峰值和长请求。
- 证据链：handler 直接调用 `export.RunJSON` 并序列化返回。
- 最小修复：移除或限制同步入口；大任务统一走异步导出。
- 建议测试：超过阈值的同步导出拒绝并提示使用异步流程。

### M-10 [Medium] Reviewer 空队列自动展示伪造 demo 数据，并模拟审核成功

- 位置：`apps/web/src/modules/reviewer/Queue.tsx:392`，`apps/web/src/modules/reviewer/Queue.tsx:404`，`apps/web/src/modules/reviewer/Queue.tsx:465`
- 触发条件：真实 reviewer 队列为空，或仲裁任务未被普通队列请求返回。
- 影响：页面静默切换到 hardcoded demoItems。操作按钮只 Toast “已记录本次审核动作”，不会写后端。演示人员和运营人员可能误以为正在处理真实数据，也会掩盖仲裁队列不可见问题。
- 证据链：`submissions.length === 0` 时返回 demoItems；无 selected 时 review 直接 Toast success。
- 最小修复：默认展示明确 empty state；demo 数据必须由显式环境变量或 URL 开关启用，并显示醒目 demo 标识。
- 建议测试：空 API 队列默认渲染空状态；仅显式 demo mode 才出现样例和模拟动作。

### M-11 [Medium] OpenAPI 和生成类型仍停留在 S5，与运行时路由漂移

- 位置：`docs/openapi.yaml:4`，`docs/openapi.yaml:119`，`submission/api/openapi.yaml:119`，`apps/web/src/shared/api/schema.d.ts:199`，`apps/api/internal/handler/labeler.go:31`
- 触发条件：评估人员、SDK 或新前端按交付 API 文档调用草稿保存或 S8 能力。
- 影响：OpenAPI 标记 `version: 0.5.0` 和 S5 contract，仍暴露 `/save`，运行时使用 `/draft`；S8 overlap、sampling、lease、daily limit 和 arbitration 合约未完整进入文档与类型。
- 证据链：临时运行 `openapi-typescript ../../docs/openapi.yaml` 后与 checked-in `schema.d.ts` 一致，说明生成类型忠实于旧文档，但旧文档本身与 router 漂移。
- 最小修复：以实际 router 和 S8 DTO 更新 OpenAPI，同步 submission copy 和生成类型。
- 建议测试：CI 生成 schema 到临时文件后 diff；加入关键 router-contract smoke。

### M-12 [Medium] CI 漏跑 reviewsampling，并缺少 vet 与 API drift gate

- 位置：`.github/workflows/ci.yml:39`
- 触发条件：纯函数 sampling 包回归，或 OpenAPI 与生成类型再次漂移。
- 影响：本地测试存在但 CI 默认 Go 测试未包含 `./pkg/reviewsampling`；CI 也未运行 `go vet` 和 OpenAPI 临时生成 diff。
- 证据链：CI Go test 列表仅包含 api、ai-worker、exporter、llmreview。
- 最小修复：补齐 reviewsampling、模块级 `go vet`、OpenAPI 临时生成 diff。
- 建议测试：故意制造 sampling 测试失败和 schema drift，确认 CI 阻断。

### M-13 [Medium] Demo 文档仍有互相矛盾或过强的承诺

- 位置：`submission/DEMO_ENV.md:98`，`submission/DEMO_ENV.md:121`，`submission/DEMO_SCRIPT.md:82`
- 触发条件：评估人员按 submission 包独立部署或登录，或现场追问消息投递语义。
- 影响：文档一处仍写 `owner1 / pass`，与实际 `123456` 冲突；部署步骤暗示 API 启动自动 seed，但生产实际需单独 seed；脚本使用 exactly-once 表述，实际更接近 durable outbox + at-least-once delivery + consumer idempotency。
- 证据链：交付文档与 handoff 不一致；Outbox 使用 TaskID 去重和重试而非端到端 exactly-once。
- 最小修复：统一账号、seed 步骤和消息语义；补一份按当前线上数据执行的 5 分钟 dry-run 清单。
- 建议测试：从空环境严格按文档部署；新评估者不看内部 handoff 也能登录和走通三角色流程。

### L-01 [Low] 生产 CSP 会阻断新增 Google Fonts

- 位置：`apps/web/index.html:7`，`apps/web/index.html:10`，`deploy/Caddyfile:5`
- 触发条件：通过生产 Caddy 打开页面。
- 影响：HTML 请求 `fonts.googleapis.com` 和 `fonts.gstatic.com`，但 CSP 的 `style-src` 与 `font-src` 只允许 self/data。浏览器回退字体，产生 console CSP 报警，视觉还原不稳定。
- 证据链：外部字体 origin 未进入 CSP。
- 最小修复：优先自托管字体；或精确加入所需 origin。
- 建议测试：生产 CSP 下浏览器检查 console 和字体 network 请求。

### L-02 [Low] 部分自定义交互控件缺少键盘操作

- 位置：`apps/web/src/modules/reviewer/Queue.tsx:601`，`apps/web/src/modules/owner/TaskManagePanel.tsx:109`
- 触发条件：键盘或辅助技术用户操作 Reviewer tab、Owner 任务列表。
- 影响：`span role="tab"` 只有 onClick，没有 tabIndex 和键盘处理；可点击 `<tr>` 也没有按钮语义。基础键盘导航不完整。
- 证据链：交互行为绑定 pointer click。
- 最小修复：使用原生 button，或完整实现 roving tabIndex 与 Enter/Space/Arrow 键。
- 建议测试：Testing Library `user.keyboard` 覆盖 tab 切换和任务选择。

## Eight-Dimension Coverage

| Dimension | Result | Main Evidence |
| --- | --- | --- |
| 业务闭环与状态机 | 未通过 | H-01、H-05、M-02、M-03、M-04 |
| MySQL / Redis / Outbox / Worker 一致性 | 未通过 | H-01、H-02、H-04、M-06 |
| API 权限与安全 | 有基础，但交付前需修 | H-06、M-05、M-08 |
| 前端可靠性、交互与可访问性 | 需修 | H-05、M-10、L-01、L-02 |
| AI 边界与失败回退 | 有幂等基础，但需修 | H-02、H-04、M-06 |
| 测试质量 | 本地门槛通过，覆盖有缺口 | M-12，缺少关键并发与迁移 smoke |
| 部署、迁移与恢复 | 未通过 | H-06、H-07、H-04 |
| Demo Day 路线 | 可演示旧主线，S8 路线不完整 | H-05、M-10、M-11、M-13 |

## Demo Day Scenarios

| Scenario | Current Result | Risk |
| --- | --- | --- |
| Owner 创建任务、导入、发布 | 基础路径可用 | 发布后 payload 仍可变，冻结有并发竞态 |
| Labeler 领取、草稿、提交 | 基础路径可用 | 审核超时后可被重领 |
| AI review 成功、失败转人工 | 基础路径存在 | Outbox failed 不恢复；running sweeper 误判 |
| Reviewer 普通人工审核 | 基础路径可用 | 空队列会伪造 demo 数据 |
| overlap 相同答案 | 后端路径存在 | peer submission 长期停在 submitted |
| overlap 冲突仲裁 | 后端部分存在 | UI 无入口，附件下载漏授权 |
| 带附件提交 | 根级字段可用 | Group/Tabs 嵌套附件可被清理 |
| Owner 导出 | 同步、异步均存在 | 同步导出大任务内存风险；Outbox 故障卡异步导出 |
| 公网评估 | 已部署 IP HTTP | 凭据与 JWT 明文传输 |

## Open Questions

1. `finished_items` 的产品语义是“已处理 item”还是“approved item”？当前命名、UI 进度和实现不一致。
2. overlap 中较早 peer submission 应作为可审计 evidence 保留在哪个明确状态？不建议长期复用 `submitted`。
3. 发布后是否允许新增 item？如果允许，应禁止覆盖已存在 item，还是采用 dataset version？
4. `quotaPerUser` 是否也属于发布冻结字段？当前冻结 distribution，但仍允许改 quota 数值。
5. Reviewer hardcoded demo fallback 是否只为静态展示临时保留？若要保留，应切到显式 demo mode。

## Verification

| Command | Result |
| --- | --- |
| `go test ./apps/api/... ./apps/ai-worker/... ./pkg/exporter ./pkg/llmreview ./pkg/reviewsampling -count=1` | PASS |
| `go vet ./apps/api/... ./apps/ai-worker/... ./pkg/exporter ./pkg/llmreview ./pkg/reviewsampling` | PASS |
| `pnpm -F web test` | PASS，20 files / 168 tests |
| `pnpm -F web lint` | PASS |
| `pnpm -F web build` | PASS；保留既有 `StatsBoard` > 500 kB chunk warning |
| `docker compose -f deploy/docker-compose.yml --env-file deploy/.env.example config` | PASS，仅做静态解析，未启动服务 |
| `git diff --check` | PASS |
| 临时生成 OpenAPI TypeScript 类型并与 checked-in `schema.d.ts` 比较 | PASS；同时证明类型与旧 OpenAPI 一致，但 contract 本身已漂移 |

## Unavailable Or Limited Verification

- `go test ./...` 和 `go vet ./...` 在 repo 根目录失败：`directory prefix . does not contain modules listed in go.work`。本报告改用明确 workspace module 路径执行。
- `go test -tags=integration ./apps/api/internal/integration -count=1 -v` 无法执行完成：本机没有可用 rootless Docker provider，报错 `rootless Docker not found, failed to create Docker provider`。
- CodeGraph 未初始化，本轮没有生成 `.codegraph/` 索引文件；结构审查使用源码读取与 `rg` 证据完成。
- 本轮按只读要求未访问或修改线上环境，因此公网 HTTP 结论来自当前 handoff 和交付文档记录。

## Repair Route

### P0 - 部署前阻断

1. 修复 H-01 租约回收与旧审核终结竞态。
2. 修复 H-02 payload 快照或发布后不可变约束。
3. 修复 H-03 嵌套附件绑定。
4. 修复 H-04 Outbox failed 恢复策略。
5. 补齐 H-05 仲裁 UI 闭环。
6. 将公网演示切到 HTTPS，处理 H-06。
7. 修复或明确阻断 H-07 overlap migration rollback。

### P1 - S8 验收前

1. 明确 overlap peer、附件 comparator、finished counter 语义。
2. 补齐仲裁 sibling 审计和附件下载。
3. 修复 AI sweeper running 时间锚点和发布冻结竞态。
4. 去掉 Reviewer 隐式 demo fallback。
5. 更新 OpenAPI、生成类型、CI gate 和 Demo 文档。

### P2 - 稳定性与体验

1. 收敛 JSON body limit。
2. 下线或限制同步 export。
3. 处理生产 CSP 字体策略。
4. 补齐键盘可访问性。

