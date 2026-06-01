# LabelHub S4 Acceptance — 异步多格式导出 + Owner 数据看板 Checkpoint

> 对照 `docs/PLAN-S4-IMPL.md` 与 `PLAN.md` §Sprint4 完成标准。

**完成日期**:2026-05-27
**分支 / 关键 commit**:`s4-async-export`(`4143e0c` 编码器 → `582fa96` 异步链路+下载 → `6bc82a7` stats → `17b20a6` 前端面板+看板 → `48d2aeb` first_come/coverage)

---

## 1. S4 已交付能力

- **异步多格式导出**:`POST /exports` 同事务写 `exports(queued)`+`outbox_events(export)`+audit;outbox publisher 入队 asynq;worker `exporter.Run` 落盘(queued→running→succeeded)。JSON / JSONL / CSV / XLSX 四格式全部跑通。
- **字段映射**:选列 + 重命名 + "含审核记录"开关(`field_map` 原始 JSON 入库,保大整数精度)。
- **HMAC 签名下载**:`download-url` 签发限时 token,公开 `/exports/download` 路由 token 即鉴权,流式返回;过期 410 / 篡改 401 / 路径穿越 403。
- **Owner 数据看板**:`GET /tasks/:id/stats` + VChart `StatsBoard`(进度 / 通过率 / 状态分布 / AI vs 人工差异 / 各维度均分)。
- **`first_come` 分发**:S1 已实现,本轮补并发认领的独立验收测试。

## 2. 新增 / 改动 API 端点

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| POST | `/api/v1/tasks/:taskId/exports` | owner/admin | 创建异步导出(format + field_map + include_reviews)→ queued |
| GET | `/api/v1/tasks/:taskId/exports` | owner/admin | 导出历史列表 |
| GET | `/api/v1/tasks/:taskId/exports/:exportId/download-url` | owner/admin | HMAC 签名下载 URL |
| GET | `/api/v1/exports/download?token=` | 公开(token 即鉴权) | 流式下载导出文件 |
| GET | `/api/v1/tasks/:taskId/stats` | owner/admin | 看板统计 |
| GET | `/api/v1/tasks/:taskId/export/json` | owner/admin | (S1 遗留同步 JSON,保留) |

## 3. 异步导出链路验证 — 实跑(真实 MySQL+Redis,2026-05-27)

- [x] `POST /exports` 同事务写 `exports(queued)` + `outbox_events(topic=export)` + audit(queued)
- [x] outbox publisher 入队 asynq(幂等 TaskID,重复投递不重跑 —— Run 仅处理 queued/running)
- [x] worker `handleExport` → `exporter.Run`:queued→running→succeeded,文件 temp→rename 原子落盘(实测 export #5 一次轮询即 succeeded)
- [x] 失败路径:status=failed + error_msg(脱敏),前端历史显示 failed 徽章(单测覆盖 `TestRun_EncodeErrorMarksFailed` + 前端渲染)
- [x] 文件存绝对路径,api(CWD apps/api)与 worker(CWD apps/ai-worker)跨进程下载一致

## 4. 四格式文件校验记录 — 实跑

| 格式 | 文件 | 校验方式 | 结果 |
|---|---|---|---|
| json | `data/exports/1/*.json` | `json.load` | ✅ 合法数组,3 行 |
| jsonl | `data/exports/1/*.jsonl` | 逐行 JSON | ✅ export #7 succeeded |
| csv | `data/exports/1/*.csv` | 签名 URL 下载 + 查看 | ✅ UTF-8 BOM,中文不乱码,表头含 `ai_review.*`/`human_review.*` |
| xlsx | `data/exports/1/*.xlsx` | excelize 读回(单测)+ 落盘 | ✅ export #8 succeeded |

- [x] 字段映射:选列 + 重命名 + "含审核记录"开关三者都体现(CSV 实跑含 `human_review.verdict=approve`;单测 `TestApply_SelectsAndRenames` / `TestLoadApprovedRows_WithReviews`)

## 5. Dashboard 看板核对(实跑 vs DB,task 1)

| 指标 | 看板值(/stats) | DB 直查 | 一致? |
|---|---|---|---|
| 进度 finished/total | 3 / 30 | `finished_items=3, total_items=30` | ✅ |
| 状态分布 | {approved:3, draft:1} | `GROUP BY status` | ✅ |
| 通过率 | 1.0 | approved 3 /(3+0) | ✅ |
| AI vs 人工差异率 | compared/disagree(单测覆盖) | 见 `TestTaskStats_AggregatesCounts` | ✅ |
| 各维度均分 | Go 侧聚合 `ai_reviews.dimensions` | `TestAggregateDimensions_*` | ✅ |

- [x] VChart 渲染(jsdom 无 canvas,单测 mock VChart 断言 spec 数据;浏览器侧 chunk 见 §9)
- [x] 题目预览入口(S2 `item-preview`)未改动,仍可用

## 6. 分发策略验收

- [x] `first_come` 独立可验收(必交)—— `TestClaim_FirstComeServesNextAvailableItem`(SKIP LOCKED 领下一个 available)+ `TestClaim_RaceLostWhenItemTakenConcurrently`(乐观 UPDATE 0 行 → ErrClaimRaceLost,杜绝重复发题)
- [ ] (Bonus C)`assigned` —— **砍**(理由:`first_come` 已满足必交,时间优先保 Day1-5 全详)
- [ ] (Bonus C)`quota` —— **砍**(同上)

## 7. 加分项状态

| 加分项 | 状态 | 验收 / 砍掉理由 |
|---|---|---|
| Markdown 导出(§Bonus A) | **砍** | 时间优先必交;迁移 008 占位仍在 `docs/bonus-a-migration/`,后续 `git mv` 到 `migration/` 即可激活 |
| JSONL·Excel 导入(§Bonus B) | **砍** | JSON 导入 S1 已可用;多格式导入非必交 |
| assigned+quota 分发(§Bonus C) | **砍** | `first_come` 已满足必交完成标准 |

## 8. 验收脚本(可重复)

```bash
cd ~/Desktop/LabelHub && make up          # mysql + redis
go test ./apps/api/... ./apps/ai-worker/... ./pkg/exporter ./pkg/llmreview -count=1   # 全绿
pnpm -F web test && pnpm -F web lint && pnpm -F web build                              # 全绿
make seed                                  # 幂等
make api      # 终端 1
make worker   # 终端 2  (worker 需 LLM 配置,见 §9;只验导出可 LLM_PROVIDER=mock)
open http://localhost:5173                 # owner1 / 123456:看板 → 配字段映射 → 4 格式各导一份 → 历史 succeeded → 下载
```

## 9. 已知 warning / 非阻塞遗留

- **VChart bundle**(已缓解,2026-05-28):StatsBoard 改 `React.lazy` + `vite.config` 让 `@visactor` 脱离 eager vendor,首屏 eager vendor 从 2.2MB(gzip 616KB)降到 412KB(gzip 125KB),VChart(gzip ~491KB)进按需加载的 StatsBoard chunk。仍有 >500kB chunk warning,但只在选中任务时拉,非阻塞。
- **worker 启动依赖 LLM 配置**:`newEvaluatorFromEnv` 缺 `LLM_*` 会 fatal(为 `ai:review` 设计)。只验导出链路时设 `LLM_PROVIDER=mock LLM_ALLOWED_MODELS=mock-model` 即可启动;正式 demo 配真实豆包 EP。
- **Makefile 修复**:`go run cmd/worker/main.go` 只编单文件(worker 是多文件包)会编译失败,已改为 `go run ./cmd/worker`(api/seed 同步改包路径)。
- `pkg/exporter` 单测覆盖核心路径(编码器 + Run + loadrows + token + CSV 注入回归)。

### 9.1 code review 修复(2026-05-28,`s4-review-fixes`)

go/typescript/security 三审 + 人工核验后修掉的 HIGH/MEDIUM:
- **EXPORT_DIR 加固**:api/worker 启动强制绝对路径(相对路径在两进程不同 CWD 下会让签名下载静默 403),`.env.example` 改绝对示例。
- **重试语义**:`pkg/exporter.ErrTerminal` 区分永久(编码/落盘,已写 failed)vs 瞬时(DB)失败,worker `asynq.SkipRetry` vs 重试,不可解析 payload 直接 SkipRetry。
- **CSV 公式注入**:`= + - @ \t \r` 开头单元格加前导单引号。
- **前端**:ExportPanel `catch` stale guard;StatsBoard 错误态 + 重试(去永久转圈);VChart 懒加载;AI 饼图负值兜底。

**仍故意延后的 MEDIUM/LOW**(竞赛体量非阻塞):下载 token 加 `task_id` claim、`sanitizeExportError` 按类别脱敏、stats 聚合下推 SQL、`safeExportPath` 拦 `rel=="."`。

## 10. 进入 S5 的先决条件

- [x] `pkg/exporter` 单测覆盖充分(4 格式编码器 + Run + token + loadrows)
- [x] 异步导出链路稳定(kill worker 重启后,pending outbox 仍被认领;实测重启 worker 后 queued 的 #5 立即 succeeded)
- [x] 全套 `go test` + `pnpm test/lint/build` 全绿,`git diff --check` 干净
