# LabelHub 交接：P1 / P2 待办（2026-06-01）

> **完成状态（2026-06-01）**：本文清单中的 P1 / P2 已完成实现。保留本文作为审计记录；验证结果与后续环境事项见 `docs/CHANGELOG.md`。
>
> 给下一个 session 的接力文档。源审查报告:`docs/FULL_CODE_REVIEW_2026-06-01.md`(完整证据链)。
> 本文只列**尚未完成**的项 + 上手方式。已完成的见 `docs/CHANGELOG.md` 顶部两条
> (`p0-codereview-fixes`、`h05-arbitration-ui`)。

---

## 0. 当前基线(已完成,别重做)

| 已修 | 说明 |
|---|---|
| H-01 ~ H-04、H-07 | 5 个 P0 后端 High,见 CHANGELOG `p0-codereview-fixes` |
| H-05 | 仲裁 UI 闭环(`ArbitrationQueue.tsx` + 「仲裁」tab) |
| H-06 | **仅记录+备配置**:`deploy/Caddyfile` 已加 HSTS、`docs/HTTPS_AND_HARDENING.md` 已写;**未上线**,等域名 |
| M-05 | 仲裁附件下载授权(`canDownloadUpload` 白名单已含 `needs_arbitration`) |
| M-10 | 假 demo 改 `?demo=1` 显式开关 + 空状态 |

**测试基线**:`go test ./apps/api/... ./apps/ai-worker/... ./pkg/exporter ./pkg/llmreview ./pkg/reviewsampling`
= 378 通过;`pnpm -F web test` = 171 通过;`go vet` / `lint` / `tsc` / `build` / `git diff --check` 全干净。

**环境限制(沿用上轮)**:
- 本机无 rootless Docker → `go test -tags=integration ./apps/api/internal/integration` 跑不了。
  上轮新增的两个集成回归(`TestExpiredLeaseDoesNotReclaimItemUnderReview`、
  `TestOverlapArbitrationDownMigrationGuardsAgainstOverlapData`)只编译验证过,**待 CI/有 Docker 复跑**。
- CodeGraph 未初始化(`.codegraph/` 不存在);需要可先 `codegraph init -i`。
- 所有 file:line 以源报告为准,**改前先核对当前行号**(本轮改过若干文件,行号有漂移)。

---

## 1. 先定 Open Questions(挡着 P1 的产品决策)

这几条不定下来,M-01/M-02/M-03 没法"对"地修:

1. **`finished_items` 语义**:是"已处理 item"还是"approved item"?当前 approve 才 +1、reject 不 +1(M-03)。
   → 建议:表示"已定稿(approved+rejected)item",reject 也 +1;若要区分,再单列 approved 计数。
2. **overlap 较早 peer 的归宿**(M-02):达成共识后,先提交那份长期停在 `submitted`。需要一个明确的
   evidence 终态(如 `consensus_evidence` / `superseded`),不要复用 `submitted`。
3. **发布后能否新增 item**:**本轮 H-02 已决定 = 不能**(题集发布即冻结,`ensureTaskDraft` 拦截
   `BatchUpdateItems`/`ImportItems`/`ImportItemsFile`)。若产品要支持热更新,需改成 dataset version 方案。
4. **`quotaPerUser` 是否属发布冻结字段**:当前冻结 distribution/overlap/sampling,但仍允许改 quota 数值。
5. ~~Reviewer demo fallback~~ → 已解决(M-10,`?demo=1` 显式开关)。

---

## 2. P1 — S8 验收前

### M-01 [Medium] overlap 共识比较完整答案,带附件天然冲突
- **位置**:`apps/api/internal/service/submission/overlap.go` → `canonicalAnswer` / `decideOverlapOutcome`;
  附件 key 来自 `apps/api/internal/handler/upload.go` → `storageKey`(随机 32 字节)。
- **问题**:共识判定对**完整 answer JSON** 精确比较;FileUpload 的 storage key 每次随机,
  即使语义标签相同,两人的附件 key 不同 → 误判 `needs_arbitration`。
- **改法**:按 schema 定义"共识投影",比较时排除附件身份字段(FileUpload 值),或给字段配 comparator。
  需要读 template schema 拿字段类型(可复用本轮 H-03 的 `collectFileUploadFieldNames` 思路递归找 FileUpload 字段,
  比较时把这些字段从 canonical 里剔除)。
- **测试**:overlap 题两人语义相同但 upload key 不同 → 应达成共识(不进仲裁)。`overlap_test.go` 已有骨架。

### M-02 [Medium] overlap 较早 peer 长期停在 submitted
- **位置**:`apps/api/internal/service/submission/submission.go` → `Save` 的 overlap 分支
  (`overlapWaiting` 只 `releaseOverlapClaim`,不改先前 peer 的 submission 状态)。
- **依赖**:Open Question #2。
- **改法**:定义 peer 证据终态,consensus 时把同 item 的先到 peer 一并迁到该态;或把 item outcome 与
  evidence submission 的统计明确拆开。注意审计日志要补对应迁移。
- **测试**:两人相同答案达成共识后,两条 submission 的最终状态与 dashboard 统计都符合定义。

### M-03 [Medium] reject 完成 item 但不增 finished_items
- **位置**:`apps/api/internal/service/review/review.go` → `Apply` 终态分支
  (`if to == approved` 才 `finished_items + 1`);统计读 `apps/api/internal/handler/stats.go`。
- **依赖**:Open Question #1。
- **改法**:若 `finished_items` = 已处理,则 approve/reject 都 +1(注意 `finishAutoApprovedItem` 与 worker
  auto-approve 路径也要一致);若 = approved,则改名并单列完成计数,前端 `TaskManagePanel`/Dashboard 进度同步。
- **测试**:reject 后任务进度按定义变化;approved/rejected 分项仍准确。

### M-04 [Medium] 仲裁自动拒绝 sibling 缺审计日志
- **位置**:`apps/api/internal/service/review/review.go` → `Apply` 里 `isArbitration` 的 sibling 批量 UPDATE
  (把同 item 其它 `needs_arbitration` 改 `rejected`),但 `audit.Write` 只写当前 submission。
- **改法**:在同一事务里给每个被打回的 sibling 写一条状态迁移审计(`needs_arbitration → rejected`,
  actor=作出仲裁决定的 reviewer,event 如 `arbitration_sibling_rejected`,payload 记 winner submission id);
  或写一条 item 级 arbitration 审计列出全部 sibling id。
- **测试**:仲裁后每个受影响 submission 都能从 audit 还原"为什么/被谁/何时"打回。
- **备注**:本轮 H-05 前端已能触发仲裁,这条让审计闭环,优先级偏高。

### M-06 [Medium] AI sweeper 用 created_at 判 running 超时,会误杀刚跑的任务
- **位置**:`apps/api/internal/service/aireview/sweeper.go`(查询对 pending/running 统一用
  `ar.created_at < cutoff`)。
- **问题**:review 在队列里排队很久、刚被 worker 领取进入 `running`,sweeper 仍按旧 `created_at` 判 stalled,
  会在合法 LLM 请求进行中切人工 failover,浪费成本+丢有效结果。
- **改法**:给 `ai_reviews` 加 `started_at`(或复用/新增 `updated_at`)——**需要一支迁移**
  (参考 `009`/`010` 的写法,注意同时给 `*.down.sql` 写好回滚,别再犯 H-07 的坑)。
  pending 按入队时间、running 按开始/心跳时间分别判超时。worker `markRunning`(`ai_review.go`)落 `started_at`。
- **测试**:排队久但刚进 running 的 review 不被 sweep;真卡死 running 仍被恢复。

### M-07 [Medium] 发布冻结存在并发竞态
- **位置**:`apps/api/internal/handler/task.go`(`TaskPoliciesFrozen` 预检查 ~239 行,后面 `UPDATE ... WHERE id=?`
  无 `status='draft'` 条件 ~320/331 行)。
- **问题**:编辑请求先读 draft 通过预检查,另一请求并发 publish 后,编辑仍以 `WHERE id=?` 落库,
  可能在 published 态改 distribution/overlap/sampling。**注意这是任务"策略字段"的竞态,与本轮 H-02 拦的"题集"
  是两回事**,H-02 没覆盖这条。
- **改法**:冻结字段 UPDATE 加 `WHERE id=? AND status='draft'` 并校验 `RowsAffected`(0 行→返回 409 让前端刷新),
  或在事务内 `SELECT ... FOR UPDATE` 锁 task 再判后写。
- **测试**:并发 barrier 测 publish/edit,发布后冻结字段不得变化。

### M-11 [Medium] OpenAPI 与生成类型停在 S5,与运行时路由漂移
- **位置**:`docs/openapi.yaml`(`version: 0.5.0`,仍暴露 `/save`,运行时是 `/draft`);
  `submission/api/openapi.yaml`(交付副本);`apps/web/src/shared/api/schema.d.ts`(生成类型,忠实于旧文档)。
- **改法**:按当前 router + S8 DTO(overlap/sampling/lease/daily limit/arbitration)更新 OpenAPI,
  同步 submission 副本,重生成 `schema.d.ts`(`openapi-typescript ../../docs/openapi.yaml`)。
- **测试/CI**:见 M-12,把"临时生成后 diff"做成 CI gate。

### M-12 [Medium] CI 漏跑 reviewsampling,缺 vet 与 API drift gate
- **位置**:`.github/workflows/ci.yml`(Go test 列表只含 api/ai-worker/exporter/llmreview)。
- **改法**:Go test 补 `./pkg/reviewsampling`;加模块级 `go vet`;加"OpenAPI 临时生成 → 与 checked-in
  `schema.d.ts` diff"的 drift gate。可顺便把 `-tags=integration` 的 testcontainers 跑进 CI(CI 有 Docker)。
- **测试**:故意制造 sampling 测试失败 + schema drift,确认 CI 阻断。

---

## 3. P2 — 稳定性与体验

### M-08 [Medium] 多个 JSON 入口仍无请求体大小上限
- **位置**:已有 helper `bindLimitedJSON`(`apps/api/internal/handler/body_limits.go`),但下列仍直接
  `ShouldBindJSON`:`auth.go`(login/refresh)、`llm.go`(inline LLM)、`reviewer.go`(`ReviewSubmission`/`BatchReview`)、
  `export.go`(async create)、`task.go`(baseline 更新)。
- **改法**:按用途设小上限,统一走 `bindLimitedJSON`;反向代理(Caddy)再设全局兜底。
- **测试**:每个入口超限 body 返回 413。

### M-09 [Medium] 旧同步 JSON 导出仍开放,全量进 API 内存
- **位置**:`apps/api/internal/handler/export.go`(同步入口直接调 `export.RunJSON` 序列化返回);
  异步导出已存在。
- **改法**:移除或对大任务限制同步入口,统一走异步。
- **测试**:超阈值同步导出被拒并提示用异步。

### L-01 [Low] 生产 CSP 阻断 Google Fonts
- **位置**:`apps/web/index.html`(请求 `fonts.googleapis.com`/`fonts.gstatic.com`);
  `deploy/Caddyfile` 的 CSP `style-src`/`font-src` 只允许 self/data(本轮加 HSTS 时**未动** CSP 字体策略)。
- **改法**:优先**自托管字体**(更稳,也免第三方依赖);否则精确把所需 origin 加进 CSP。
- **测试**:生产 CSP 下浏览器 console 无 CSP 报警、字体 network 请求正常。

### L-02 [Low] 部分自定义交互控件缺键盘操作
- **位置**:`apps/web/src/modules/reviewer/Queue.tsx`(view tab 用 `span role="tab"` 仅 `onClick`,
  无 `tabIndex`/键盘处理 —— **本轮新增的「仲裁」tab 沿用了同一模式,也在此列**);
  `apps/web/src/modules/owner/TaskManagePanel.tsx`(可点击 `<tr>` 无按钮语义)。
- **改法**:换原生 `<button>`,或完整实现 roving tabIndex + Enter/Space/Arrow。
  (参考:本轮 `ArbitrationQueue` 的队列项已用原生 `<button>`,可照搬到 view tabs。)
- **测试**:Testing Library `user.keyboard` 覆盖 tab 切换与任务选择。

### M-13 [Medium] Demo 文档自相矛盾/过强承诺(归到文档轮)
- **位置**:`submission/DEMO_ENV.md`(一处仍写 `owner1/pass`,与实际 `123456` 冲突;暗示 API 启动自动 seed,
  实际 prod 需单独 seed);`submission/DEMO_SCRIPT.md`(用 exactly-once 表述,实际是 durable outbox +
  at-least-once + consumer 幂等)。
- **改法**:统一账号/seed 步骤/消息语义;补一份按当前线上数据的 5 分钟 dry-run 清单。

---

## 4. 建议执行顺序

1. **先定 §1 的 Open Questions**(尤其 #1 finished_items、#2 peer 终态),否则 M-02/M-03 会返工。
2. **M-04 仲裁审计**(让本轮 H-05 闭环的审计也闭环,代码已熟,改动小)。
3. **M-07 发布冻结竞态**(纯后端,WHERE 加条件 + RowsAffected,低风险)。
4. **M-03 / M-02**(定了语义后一起做,涉及 review.go + submission.go + 统计/前端)。
5. **M-06 AI sweeper**(要加迁移 + down,按 H-07 教训写好回滚)。
6. **M-01 overlap comparator**(读 schema 做投影,稍重)。
7. **M-11 + M-12 一起**(OpenAPI 更新 + CI drift gate,互相印证)。
8. **P2**:M-08 body limit(机械)、M-09 同步导出、L-01 字体、L-02 键盘(可顺手把 view tabs 一起改)。
9. **M-13 文档**(可并到任意一轮收尾)。

> 每项改完按基线跑 `go test`(模块级路径)+ `pnpm -F web test/lint/build` + `git diff --check`;
> 加迁移的(M-06)务必同时写 `*.down.sql` 并自测回滚。提交记录追加到 `docs/CHANGELOG.md` 顶部。
