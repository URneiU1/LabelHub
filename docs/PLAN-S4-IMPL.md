# S4 Implementation Plan — 异步多格式导出 + Owner 数据看板

> 配套 `PLAN.md` §Sprint 4(449-465 行)。本文件是给 **code session(Codex)** 的可执行落地版,粒度对齐 `docs/PLAN-S2-IMPL.md`:逐天 / 逐任务 / 先写测试再实现 / 给出文件路径与代码骨架 / 每步带验证命令。
>
> **三个已定决策**(写文档前已和 Chen 确认):
> 1. **范围** = 必交全详 + 加分作可选章节(本文 §Day1–§Day5 必交,§Bonus A/B/C 可选,时间紧可整章砍掉,砍了要在 acceptance 写清"砍 X,理由 Y")。
> 2. **图表库** = **VChart**(`@visactor/react-vchart`),字节自家、与 Semi 同源,契合"字节赛 + AI Agent"叙事。
> 3. **下载** = **HMAC 签名 token + API 流式下载**,可移植、不依赖 nginx 配置,符合 plan"签名 URL"措辞。

---

## 0. 现状盘点(S0–S3 已经埋好的脚手架,S4 不要重造)

执行前先认清:S4 的**数据库 schema 和大部分 model 字段在 S0 就建好了**,S4 是"接线",不是"建表"。

| 资产 | 位置 | 现状 | S4 怎么用 |
|---|---|---|---|
| `exports` 表 | `migration/001_initial_schema.up.sql` | `format ENUM('json','jsonl','csv','xlsx')`、`filter JSON`、`field_map JSON`、`status ENUM('queued','running','succeeded','failed')`、`file_path`、`file_size`、`row_count`、`error_msg`、`finished_at` 全有 | **无需新迁移**(除非做 Bonus A Markdown,要 ALTER enum 加 `'md'`) |
| `Export` model | `model/models.go:250` | 字段与表对齐(`Filter *string`、`FieldMap *string`、`Status`、`FilePath NullString`、`FileSize *uint64`、`RowCount *int`、`FinishedAt NullTime`) | 直接用 |
| `tasks.distribution` / `quota_per_user` | `001` + `Task` model | `distribution ENUM('first_come','assigned','quota')` 默认 `first_come`;`quota_per_user INT` | 必交只验 `first_come`;Bonus C 用 `assigned`/`quota` |
| `task_assignees` 表 | `001`(`task_id,user_id,item_id`) | 已存在 | Bonus C 用 |
| outbox publisher | `service/outbox/publisher.go` | **topic 无关**:`asynq.NewTask(event.Topic, payload)` + `SKIP LOCKED` 认领 + 幂等 TaskID + stale 恢复 | 只要写 `OutboxEvent{Topic:"export"}` 就自动入队 |
| asynq worker mux | `apps/ai-worker/cmd/worker/main.go:44` | 已注册 `ai:review` / `ai:dry-run` / `noop:ping` | 加一行 `mux.HandleFunc("export", handlers.handleExport)` |
| 共享 module 范式 | `pkg/llmreview`(module `labelhub.local/llmreview`) | api 与 worker 都 import 它 | **新建 `pkg/exporter` 同理**,放跨模块共享的编码器 / token / 执行核心 |
| S1 同步 JSON 导出 | `handler/export.go` + `service/export/export.go` | `GET /tasks/:taskId/export/json` 同步返回 rows;有 `loadApprovedRows`(gorm join)、`AIReviewToMap`/`HumanReviewToMap`/`DecodeJSONFallback` 可复用约定 | 保留向后兼容;S4 demo 走异步路径 |
| `item-preview` 题目预览 | S2 已做(`GET /tasks/:taskId/item-preview`) | plan §Sprint4"题目预览(Renderer 干跑)"**已完成** | **不重做**,§Day5 验收里勾掉即可 |
| audit | `service/audit/audit.go`(`audit.Write(tx, LogEntry)`) | gorm tx 内写 audit_logs | enqueue 时写 `event:"queued"` |

### 0.1 关键架构约束(必须遵守,否则编译不过)

- **ai-worker 是独立 module(`labelhub-ai-worker`),用裸 `database/sql`,不能 import `labelhub-api/internal/*`**(Go internal 包跨模块禁止)。
- 因此 **格式编码器 / 字段映射 / HMAC token / 异步执行核心 `Run()` 全部放 `pkg/exporter`(新模块,裸 `*sql.DB`)**,api 和 worker 都能 import。
- API 侧的 **enqueue / 历史列表 / download-url**(走 gorm + outbox + audit)留在 `apps/api/internal/service/export`。
- worker 的 `handleExport` 是 5 行适配器:解析 `export_id` → 调 `exporter.Run(ctx, sqlDB, exportID, dir)`。

### 0.2 目标数据流(异步导出一条龙)

```
Owner 点"导出" (选 format + field_map + include_reviews)
  └─ POST /tasks/:taskId/exports
        └─ 单事务: INSERT exports(status=queued) + INSERT outbox_events(topic=export, payload={export_id}) + audit(queued)
              └─ 返回 {id, status:"queued"}
outbox publisher goroutine (已存在, 1s tick)
  └─ SKIP LOCKED 认领 pending → asynq.NewTask("export", payload) 入队 (幂等 TaskID)
ai-worker (asynq mux)
  └─ handleExport → exporter.Run(sqlDB, exportID, EXPORT_DIR):
        UPDATE exports SET status=running
        SELECT 行 (approved submissions join items join revisions [+reviews])
        apply field_map → 选取/重命名字段
        encode 到 <EXPORT_DIR>/<task_id>/<uuid>.<ext> (temp→rename 原子落盘)
        UPDATE exports SET status=succeeded, file_path, file_size, row_count, finished_at
        INSERT audit_logs(exported)
        (失败: UPDATE status=failed, error_msg + audit)
前端轮询 GET /tasks/:taskId/exports → 看到 succeeded
  └─ 点下载 → GET /tasks/:taskId/exports/:id/download-url → {url: "/api/v1/exports/download?token=HMAC..."}
        └─ window.open(url) → GET /exports/download?token=... (公开路由, token 即鉴权) → 流式返回文件
```

---

## Pre-flight(Day 0,≤45 min)

### Task 0.1：基线 checkpoint

```bash
cd ~/Desktop/LabelHub
git status                       # 工作区干净
go test ./apps/api/... ./apps/ai-worker/... ./pkg/llmreview   # 全绿
pnpm -F web test && pnpm -F web lint && pnpm -F web build      # 全绿
git checkout -b s4-async-export
```

确认以下命题(读代码核对,不要假设):
- [ ] `exports` 表 status enum 含 `running`(worker 用)→ `grep running migration/001_initial_schema.up.sql`
- [ ] `OutboxEvent.Topic` 是 `size:64` 字符串、`Payload` 是 JSON 字符串 → `model/models.go:332`
- [ ] outbox publisher 在 server 启动时跑 → `main.go:45 startOutboxPublisher`
- [ ] worker mux 注册位置 → `apps/ai-worker/cmd/worker/main.go:44-46`

### Task 0.2：新依赖 + 环境变量(一次性装齐,避免中途打断)

```bash
# 1. excelize (XLSX StreamWriter) —— 放进新 pkg/exporter 模块
mkdir -p pkg/exporter
cd pkg/exporter && go mod init labelhub.local/exporter && go get github.com/xuri/excelize/v2@latest && cd ../..

# 2. go.work 注册新模块
# 编辑 go.work, use(...) 块加一行: ./pkg/exporter

# 3. api / worker 依赖 pkg/exporter (两个 go.mod 都 require + replace 走 workspace)
cd apps/api && go get labelhub.local/exporter && cd ../..
cd apps/ai-worker && go get labelhub.local/exporter && cd ../..

# 4. 前端图表库
pnpm -F web add @visactor/react-vchart @visactor/vchart
```

`.env.example` 追加(`EXPORT_DIR` 已存在,只补 secret):

```dotenv
# --- Export download (S4) ---
EXPORT_DOWNLOAD_SECRET=change-me-to-a-random-32+-char-string   # HMAC 签名密钥, 缺失则 download-url 端点 fatal/500
EXPORT_DOWNLOAD_TTL=600                                         # 签名 URL 有效期(秒), 默认 10min
```

> ⚠️ 本地 `.env` 也补上,值随机生成:`openssl rand -hex 32`。**绝不入仓**(`.env` 已 gitignore)。

验证:`go build ./...` + `pnpm -F web build` 都过(此时只是装依赖,不应有编译错误)。

---

## Day 1 — `pkg/exporter` 编码器核心(纯函数 / 高 TDD 密度,不碰 DB)

> 目标:把"一批行 → 各格式字节流"做成纯函数全测过。**这是 S4 工程质量分(Sprint 5)的主力单测来源,先把它做扎实。**

### 文件落点

```
pkg/exporter/
  row.go              # Row 类型 + 字段顺序约定
  fieldmap.go         # FieldMap 解析 + apply(选取 + 重命名)
  encode_json.go      # EncodeJSON  (整体 JSON 数组)
  encode_jsonl.go     # EncodeJSONL (一行一 JSON)
  encode_csv.go       # EncodeCSV   (encoding/csv, BOM 头给 Excel 认中文)
  encode_xlsx.go      # EncodeXLSX  (excelize StreamWriter 流式)
  encode.go           # Encode(format, w, rows, cols) 总分发 + 格式注册表
  *_test.go           # 每个编码器一份 golden 测试
```

### Task 1.1：`Row` + 列定义(先定数据契约)

```go
// row.go
package exporter

// Row 是导出的一条扁平记录:有序键值。用 []Cell 而非 map 以保证列顺序稳定
// (CSV/XLSX 必须列顺序确定;JSON/JSONL 也按这个顺序序列化以便 diff)。
type Cell struct {
	Key   string
	Value any
}
type Row []Cell

// Column 描述导出的一列:源字段名 + 导出名(重命名后)。
type Column struct {
	Source string // 源 key, 如 "answer.score" 或 "ai_review.verdict"
	Export string // 导出列名, 默认 == Source, field_map 可改
}
```

测试 `row_test.go`:`TestRow_ToMapPreservesNothingExtra`(Row→map 往返不丢键);列顺序断言。

### Task 1.2：`FieldMap`(TDD)

`field_map` 存进 `exports.field_map`,前端传形如:
```json
{"include_reviews": true,
 "columns": [
   {"source": "external_id", "export": "题目ID"},
   {"source": "answer", "export": "标注答案"},
   {"source": "ai_review.verdict", "export": "AI判定"}
 ]}
```

```go
// fieldmap.go
type FieldMap struct {
	IncludeReviews bool     `json:"include_reviews"`
	Columns        []Column `json:"columns"` // 空 == 导出全部默认列
}

// ParseFieldMap: nil/空字符串 → 零值(全列默认导出)。非法 JSON → error。
func ParseFieldMap(raw *string) (FieldMap, error) { ... }

// Apply: 把一条完整行按 FieldMap.Columns 选取 + 重命名;Columns 空则原样返回。
// 缺失的 source 列填 nil(不报错,导出场景容忍缺列)。
func (fm FieldMap) Apply(full Row) Row { ... }

// ColumnsOrDefault: Columns 空时, 从首行推导全列顺序作为默认表头。
func (fm FieldMap) ColumnsOrDefault(sample Row) []Column { ... }
```

测试 `fieldmap_test.go`(先写,RED):
- `TestParseFieldMap_NilReturnsEmpty`
- `TestParseFieldMap_InvalidJSONErrors`
- `TestApply_SelectsAndRenames`(只留 columns 列且改名)
- `TestApply_EmptyColumnsReturnsFull`
- `TestApply_MissingSourceFillsNil`

### Task 1.3：四个编码器(逐个 TDD,golden 断言)

每个编码器签名统一:`func EncodeX(w io.Writer, cols []Column, rows []Row) (rowCount int, err error)`。

**`encode_json.go`** —— 整体数组,`json.NewEncoder(w).SetEscapeHTML(false)`;大整数精度沿用 S3 教训(payload/answer 已是 `any`,从 `DecodeJSONFallback` 来,不再二次 stringify)。
测试:`TestEncodeJSON_ArrayShapeAndOrder`、`TestEncodeJSON_EmptyRowsYieldsEmptyArray`。

**`encode_jsonl.go`** —— 一行一对象,`\n` 分隔,无尾随空行规则明确(末行也带 `\n`)。
测试:`TestEncodeJSONL_OneObjectPerLine`、`TestEncodeJSONL_CountMatchesRows`。

**`encode_csv.go`** —— `encoding/csv`;**先写 UTF-8 BOM(`\xEF\xBB\xBF`)**让 Excel 正确识别中文;嵌套值(map/array)`json.Marshal` 成字符串塞进单元格;表头用 `Column.Export`。
测试:`TestEncodeCSV_HeaderUsesExportNames`、`TestEncodeCSV_NestedValueJSONStringified`、`TestEncodeCSV_HasUTF8BOM`。

**`encode_xlsx.go`** —— **excelize StreamWriter**(流式,扛大文件):
```go
func EncodeXLSX(w io.Writer, cols []Column, rows []Row) (int, error) {
	f := excelize.NewFile()
	defer f.Close()
	sw, err := f.NewStreamWriter("Sheet1")
	if err != nil { return 0, err }
	// 表头
	header := make([]any, len(cols))
	for i, c := range cols { header[i] = c.Export }
	if err := sw.SetRow("A1", header); err != nil { return 0, err }
	// 数据行
	for i, row := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, i+2)
		vals := make([]any, len(cols))
		for j, c := range cols { vals[j] = stringifyCell(pick(row, c.Source)) }
		if err := sw.SetRow(cell, vals); err != nil { return i, err }
	}
	if err := sw.Flush(); err != nil { return len(rows), err }
	return len(rows), f.Write(w)
}
```
测试:`TestEncodeXLSX_OpensWithExcelizeAndHasHeader`(用 excelize 读回校验表头 + 行数);`TestEncodeXLSX_EmptyRowsHeaderOnly`。

### Task 1.4：`Encode` 总分发 + 格式注册表

```go
// encode.go
var ErrUnsupportedFormat = errors.New("exporter: unsupported format")

func Encode(format string, w io.Writer, cols []Column, rows []Row) (int, error) {
	switch format {
	case "json":  return EncodeJSON(w, cols, rows)
	case "jsonl": return EncodeJSONL(w, cols, rows)
	case "csv":   return EncodeCSV(w, cols, rows)
	case "xlsx":  return EncodeXLSX(w, cols, rows)
	// case "md": Bonus A
	default: return 0, ErrUnsupportedFormat
	}
}

func FileExtension(format string) string { // "xlsx"→".xlsx", "md"→".md" ... }
func ContentType(format string)   string { // 下载时设 Content-Type }
```
测试:`TestEncode_RoutesByFormat`、`TestEncode_UnknownFormatErrors`。

**Day 1 验证**:`cd pkg/exporter && go test ./... -cover`(目标该包 ≥ 90% line);`go vet ./...`。

---

## Day 2 — 异步导出链路:API 入队 + 历史列表 + worker 消费

### Task 2.1：`pkg/exporter` 的行加载 + `Run()`(裸 `*sql.DB`,sqlmock TDD)

行加载放 `pkg/exporter` 用裸 SQL,api 和 worker 都能用(api 用 `gormDB.DB()` 取 `*sql.DB`)。

```go
// loadrows.go (pkg/exporter)
// LoadApprovedRows: approved submissions join task_items join submission_revisions
// (+可选 latest ai_review / human_review)。等价于 service/export/export.go 的 loadApprovedRows,
// 但用裸 database/sql, 不依赖 gorm/internal。
func LoadApprovedRows(ctx context.Context, db *sql.DB, taskID uint64, includeReviews bool) ([]Row, error) { ... }
```
SQL(对照现有 gorm 版):
```sql
SELECT s.id, ti.id, ti.external_id, ti.payload, sr.answer
FROM submissions s
JOIN task_items ti ON ti.id = s.item_id
JOIN submission_revisions sr ON sr.id = s.current_revision_id
WHERE s.task_id = ? AND s.status = 'approved'
ORDER BY s.id
```
`includeReviews` 时再批量取每个 submission 的最新 `ai_reviews` / `human_reviews`(`ORDER BY id DESC` 去重),拼成 `ai_review.*` / `human_review.*` 嵌套键,字段名沿用 `service/export/dto.go` 的 `AIReviewToMap`/`HumanReviewToMap` 约定(verdict/overall_score/dimensions/reason/prompt_version/created_at)。`payload`/`answer`/`dimensions` 用 `json.Unmarshal` 解码失败回退原字符串(等价 `DecodeJSONFallback`)。

```go
// run.go (pkg/exporter) —— 异步执行核心, worker 调它
type RunResult struct { FilePath string; FileSize uint64; RowCount int }

func Run(ctx context.Context, db *sql.DB, exportID uint64, baseDir string) error {
	// 1. 加载 export 行 (SELECT format, field_map, include_reviews, task_id);
	//    状态机:仅当 status IN ('queued','running') 才继续 (幂等, 重投递不重跑)
	//    UPDATE exports SET status='running' WHERE id=? AND status='queued' (RowsAffected 防并发)
	// 2. fm := ParseFieldMap(field_map);rows := LoadApprovedRows(ctx,db,taskID, fm.IncludeReviews || include_reviews)
	// 3. cols := fm.ColumnsOrDefault(rows[0]);applied := apply fm 到每行
	// 4. 写文件: <baseDir>/<task_id>/<uuid><ext> ; 先写 .tmp 再 os.Rename 原子落盘
	//    f,_ := os.Create(tmp); n,_ := Encode(format, f, cols, applied); f.Sync(); rename
	// 5. UPDATE exports SET status='succeeded', file_path=?, file_size=?, row_count=?, finished_at=NOW()
	// 6. INSERT audit_logs(entity_type='export', entity_id, to_state='succeeded', actor_type='system', event='exported')
	//   失败任意一步: UPDATE status='failed', error_msg=<sanitized> + audit('failed'); 返回 err 让 asynq 决定是否重试
}
```
> **错误信息脱敏**(S3 教训):写 `error_msg` 前裁剪,绝不把底层 driver / 文件系统路径细节灌进去(参照 worker `ai_review.go` 的 sanitize 习惯)。

测试 `run_test.go`(`go-sqlmock` 或起真 sql.DB + 临时目录):
- `TestRun_QueuedToSucceededWritesFile`(跑通 → 文件存在 + status succeeded + row_count 对)
- `TestRun_NonQueuedIsNoop`(status 已 succeeded → 直接返回,不重写)
- `TestRun_EncodeErrorMarksFailed`(注入坏 format → status failed + error_msg 非空)
- `TestRun_AtomicTempRename`(中途无半截文件)

### Task 2.2：API enqueue + 历史列表(gorm + sqlmock TDD,在 `service/export`)

```go
// service/export/enqueue.go
type EnqueueInput struct {
	Task           model.Task
	CreatedBy      uint64
	Format         string  // 校验 ∈ {json,jsonl,csv,xlsx} (+md if Bonus A)
	FieldMap       *string // 原始 JSON, 入库前用 exporter.ParseFieldMap 校验合法
	IncludeReviews bool
}
const exportTopic = "export"

func Enqueue(db *gorm.DB, in EnqueueInput) (model.Export, error) {
	// 校验 format / field_map
	// 单事务:
	//   Create(&Export{TaskID, CreatedBy, Format, FieldMap, IncludeReviews, Status:"queued"})
	//   payload, _ := json.Marshal(map[string]uint64{"export_id": export.ID})
	//   Create(&OutboxEvent{Topic: exportTopic, Payload: string(payload), Status:"pending"})
	//   audit.Write(tx, {EntityType:"export", EntityID:export.ID, ToState:"queued", ActorType:"user", ActorID:&CreatedBy, Event:"queued", Payload:{task_id,format}})
}

func ListByTask(db *gorm.DB, taskID uint64, limit int) ([]model.Export, error) {
	// SELECT * FROM exports WHERE task_id=? ORDER BY id DESC LIMIT ? (默认 limit 50)
}
```
测试(sqlmock,对照 `service/outbox/publisher_test.go` 的 ExpectBegin/Commit 写法):
- `TestEnqueue_CreatesExportAndOutboxInOneTx`(ExpectBegin → INSERT exports → INSERT outbox_events → audit → Commit)
- `TestEnqueue_RejectsUnknownFormat`
- `TestEnqueue_RejectsInvalidFieldMapJSON`
- `TestListByTask_OrdersByIDDesc`

### Task 2.3：Export handler 路由(sqlmock TDD)

`handler/export.go` 扩展(`NewExportHandler(db, downloadSecret, ttl)`):

```go
func (h ExportHandler) Register(api gin.IRouter) {
	api.GET ("/tasks/:taskId/export/json", RequireRoles("owner","admin"), h.ExportJSON)        // S1 同步, 保留
	api.POST("/tasks/:taskId/exports",       RequireRoles("owner","admin"), h.CreateExport)     // S4 异步入队
	api.GET ("/tasks/:taskId/exports",       RequireRoles("owner","admin"), h.ListExports)      // S4 历史
	api.GET ("/tasks/:taskId/exports/:exportId/download-url", RequireRoles("owner","admin"), h.DownloadURL) // Day3
}
// RegisterPublic 挂在未鉴权的 api 组(token 即鉴权):
func (h ExportHandler) RegisterPublic(api gin.IRouter) {
	api.GET("/exports/download", h.Download) // Day3
}
```
`CreateExport`:`loadOwnedTask(h.db, c)` 拿 task(已做 owner 边界)→ 解析 body(`format`/`field_map`/`include_reviews`,`field_map` 用 `c.GetRawData` 存原始 JSON 避免大整数精度坍塌,沿用 S3 `apiPostRawJSON` 教训)→ `export.Enqueue(...)` → `httpx.OK(c, gin.H{"id":..,"status":"queued"})`。

`main.go` 注册改两行:
```go
exportHandler := handler.NewExportHandler(database, exportDownloadSecret(), exportDownloadTTL())
exportHandler.Register(authedAPI)
exportHandler.RegisterPublic(api)   // 公开路由
```

测试 `export_test.go`(sqlmock + gin test context,对照 `template_test.go`):
- `TestCreateExport_QueuesAndReturnsID`
- `TestCreateExport_ForbiddenForNonOwner`(`loadOwnedTask` 已覆盖,补一条)
- `TestCreateExport_RejectsBadFormat`(400)
- `TestListExports_ReturnsHistory`

### Task 2.4:worker 消费 `export`(worker 测试)

```go
// apps/ai-worker/cmd/worker/export.go
import "labelhub.local/exporter"

func (h workerHandlers) handleExport(ctx context.Context, t *asynq.Task) error {
	var p struct{ ExportID uint64 `json:"export_id"` }
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		h.logger.Warn("invalid export payload", zap.Error(err)); return err // 不可解析 → 不重试(返回 SkipRetry 更佳)
	}
	dir := envOrDefault("EXPORT_DIR", "./data/exports")
	if err := exporter.Run(ctx, h.db, p.ExportID, dir); err != nil {
		h.logger.Error("export run failed", zap.Uint64("export_id", p.ExportID), zap.Error(err))
		return err // 可重试错误交给 asynq (MaxRetry 5); Run 已把状态写 failed
	}
	return nil
}
```
`main.go:46` 后加:`mux.HandleFunc("export", handlers.handleExport)`。

测试 `export_test.go`(worker 包,起临时 sql.DB 或 sqlmock + tmp dir):
- `TestHandleExport_RunsAndWritesFile`
- `TestHandleExport_InvalidPayloadDoesNotPanic`

**Day 2 验证(端到端手测一次)**:
```bash
docker-compose -f deploy/docker-compose.yml up -d
make api & make worker &            # 或各自终端跑;别用后台僵尸进程
# 用 seed 后的 owner1 登录, POST /tasks/<id>/exports {"format":"csv"}
# 看 exports 行 queued→running→succeeded, ./data/exports/<task>/<uuid>.csv 落盘
go test ./apps/api/... ./apps/ai-worker/... ./pkg/exporter
```

---

## Day 3 — HMAC 签名下载 + 导出面板前端

### Task 3.1:`pkg/exporter` 下载 token(纯函数 TDD)

```go
// token.go (pkg/exporter)
// 签名格式: base64url("<exportID>.<expUnix>") + "." + base64url(HMAC-SHA256(secret, "<exportID>.<expUnix>"))
func SignDownloadToken(secret string, exportID uint64, exp time.Time) string { ... }

var ErrTokenInvalid = errors.New("exporter: invalid download token")
var ErrTokenExpired = errors.New("exporter: download token expired")

// VerifyDownloadToken: 常量时间比较 (hmac.Equal) 防时序攻击;过期单独返回 ErrTokenExpired。
func VerifyDownloadToken(secret, token string, now time.Time) (exportID uint64, err error) { ... }
```
测试 `token_test.go`:
- `TestSignVerify_RoundTrip`
- `TestVerify_TamperedSignatureFails`
- `TestVerify_ExpiredFails`
- `TestVerify_GarbageFails`
- `TestVerify_WrongSecretFails`

### Task 3.2:download-url 生成 + 流式下载端点(sqlmock TDD)

```go
// handler/export.go
func (h ExportHandler) DownloadURL(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c); if !ok { return }
	export := <SELECT exports WHERE id=:exportId AND task_id=task.ID AND status='succeeded'>
	// 不存在 / 未完成 → 404 / 409
	token := exporter.SignDownloadToken(h.secret, export.ID, time.Now().Add(h.ttl))
	httpx.OK(c, gin.H{"url": "/api/v1/exports/download?token="+url.QueryEscape(token),
	                  "expiresIn": int(h.ttl.Seconds())})
}

func (h ExportHandler) Download(c *gin.Context) {
	id, err := exporter.VerifyDownloadToken(h.secret, c.Query("token"), time.Now())
	// ErrTokenExpired → 410 Gone; 其它 → 401
	export := <SELECT exports WHERE id=? AND status='succeeded'>   // 再查一次拿 file_path/format
	// 防路径穿越: filepath.Clean + 确认在 EXPORT_DIR 前缀内
	c.Header("Content-Disposition", `attachment; filename="task-<taskID>-<format>-<id><ext>"`)
	c.Header("Content-Type", exporter.ContentType(export.Format))
	c.File(absPath)   // gin 流式
}
```
测试:
- `TestDownloadURL_SignsForSucceededExport`
- `TestDownloadURL_404ForMissingOrUnfinished`
- `TestDownload_ValidTokenStreamsFile`(tmp 文件 + 真 token)
- `TestDownload_ExpiredTokenReturns410`
- `TestDownload_TamperedTokenReturns401`
- `TestDownload_PathTraversalBlocked`(file_path 注入 `../` → 拒绝)

### Task 3.3:前端导出面板(vitest TDD)

在 `Dashboard.tsx` 选中任务区,把现有"导出数据"(单按钮同步 JSON)升级为**导出面板** section。建议抽成 `apps/web/src/modules/owner/ExportPanel.tsx`(Dashboard.tsx 已 1642 行,别再往里堆)。

`ExportPanel` 能力:
- **格式选择**:Semi `RadioGroup` json / jsonl / csv / xlsx
- **字段映射**:从 `selected.template` 的 `export_fields`(S2 已存 schema)列出可选列 + 每列重命名 `Input` + 全局"含审核记录"`Switch`(对应 `include_reviews`)
- **导出按钮** → `apiPostRawJSON('/tasks/:id/exports', {format, field_map, include_reviews})`
- **历史列表**:`apiGet('/tasks/:id/exports')` Semi `Table`(format / status 徽章 / row_count / 创建时间 / 操作);succeeded 行显示"下载"
- **轮询**:有 `queued`/`running` 行时每 2s 刷新 `GET /exports`,全 succeeded/failed 停;沿用 S3 的 task-switch stale guard(切任务后晚到响应不污染)
- **下载**:点下载 → `apiGet('/tasks/:id/exports/:eid/download-url')` → `window.open(data.url)`

`shared/api` 若无 `apiPostRawJSON` 已在 S3 加过(README 提到),直接复用。

测试 `ExportPanel.test.tsx`(vitest + RTL,mock fetch):
- `renders format options and history`
- `posts export with selected format and field map`
- `polls until no queued/running rows`(用 fake timers)
- `download opens signed url`(spy `window.open`)
- `ignores stale responses after task switch`

**Day 3 验证**:`pnpm -F web test -- ExportPanel`、`pnpm -F web lint`、`pnpm -F web build`;手测:浏览器导出 4 格式各一份 → 下载 → `jq .` 校验 json/jsonl,Excel/Numbers 打开 csv/xlsx 中文不乱码。

---

## Day 4 — Owner Dashboard 数据看板(stats 端点 + VChart)

### Task 4.1:统计端点(sqlmock TDD)

```go
// handler/task.go 或新 handler/stats.go
// GET /tasks/:taskId/stats  (owner/admin, 走 loadOwnedTask)
type TaskStats struct {
	Progress        struct{ Total, Finished int } `json:"progress"`        // task_items
	StatusBreakdown map[string]int                `json:"statusBreakdown"` // submissions 各状态计数
	PassRate        float64                       `json:"passRate"`        // approved / (approved+rejected)
	AIvsHuman       struct{ Compared, Disagree int; Rate float64 } `json:"aiVsHuman"`
	DimensionAvg    []struct{ Name string; Avg float64 }           `json:"dimensionAverages"`
}
```
实现要点:
- **Progress**:`SELECT total_items, finished_items FROM tasks WHERE id=?`(已维护好,见 worker `tasks.finished_items + 1`)。
- **StatusBreakdown**:`SELECT status, COUNT(*) FROM submissions WHERE task_id=? GROUP BY status`。
- **PassRate**:approved / (approved + rejected),分母 0 时返回 0。
- **AIvsHuman**:取既有 `ai_verdict` 又有终审 human verdict 的 submissions;映射 `ai pass↔human approve` / `ai fail↔human reject`,不一致计 disagree;`rate = disagree/compared`。
- **DimensionAvg**:**Go 侧聚合**(别写脆弱的 MySQL JSON_TABLE):`SELECT dimensions FROM ai_reviews WHERE submission_id IN (该 task 的 approved) AND status='succeeded'`,逐条 `json.Unmarshal` 成 `[]{name,score}`,按 name 累加求均值。这块逻辑抽成纯函数 `aggregateDimensions([]string) []DimAvg` 单独 TDD。

测试:
- `TestTaskStats_AggregatesCounts`(sqlmock 喂 group by 结果)
- `TestAggregateDimensions_AveragesByName`(纯函数,喂多条 dimensions JSON)
- `TestAggregateDimensions_SkipsMalformed`(坏 JSON 跳过不崩)
- `TestTaskStats_ZeroDivisionSafe`(无数据 passRate=0)

### Task 4.2:VChart 看板(vitest)

新建 `apps/web/src/modules/owner/StatsBoard.tsx`。四块:
1. **进度** — 环形/条形 progress(finished/total)
2. **状态分布 + 通过率** — 柱状(各 submission 状态计数)+ 通过率大数字
3. **AI vs 人工差异率** — 一致/不一致占比(饼或堆叠条)
4. **各维度均分** — 横向柱状(VChart `bar`,维度名做 y 轴)

VChart 用法骨架:
```tsx
import { VChart } from '@visactor/react-vchart'
const spec = {
  type: 'bar', data: [{ id: 'dim', values: dims }],
  xField: 'name', yField: 'avg',
  // Editorial 配色: 用 tokens.css 的 --color-* 喂 color scheme
}
<VChart spec={spec} />
```
**测试注意**:jsdom 无 canvas,vitest 里 `vi.mock('@visactor/react-vchart', () => ({ VChart: (props)=> <div data-testid="vchart" data-spec={JSON.stringify(props.spec)} /> }))`,断言 spec 里的数据正确而非渲染像素。
- `StatsBoard.test.tsx`:`renders four metric blocks`、`passes dimension averages into chart spec`、`shows pass rate number`。

接进 Dashboard 选中任务区(`item-preview` 题目预览 S2 已有,确认入口还在即可,无需重做)。

**Day 4 验证**:`pnpm -F web test -- StatsBoard`、lint、build;手测:Dashboard 数字与 `docker exec ... mysql` 直查 `SELECT status,COUNT(*)...` 一致。

---

## Day 5 — first_come 独立验收 + 全绿 + 验收文档 + demo 剧本

### Task 5.1:`first_come` 独立验收(必交完成标准之一)

`first_come` 在 S1 `claim.go` 已实现。本步只**补一条明确的验收测试**证明它独立可用:
- `submission` 包测试 `TestClaim_FirstComeServesNextAvailable`(已有类似的话补断言:两个 labeler 并发领,各拿不同 item,无重复;`SKIP LOCKED` 生效)。
- 写进 acceptance:"分发=first_come 全程可独立验收,无需 assigned/quota"。

### Task 5.2:全套测试全绿(Sprint 5 会再加严,这里先保 S4 不破)

```bash
go test ./apps/api/... ./apps/ai-worker/... ./pkg/exporter ./pkg/llmreview -count=1
pnpm -F web test && pnpm -F web lint && pnpm -F web build
git diff --check
```

### Task 5.3:验收文档 `docs/S4_ACCEPTANCE.md`(对照 PLAN.md 完成标准)

必交完成标准逐条勾:
- [ ] JSON / JSONL / CSV / XLSX 四格式导出文件 `jq` + Excel 打开字段正确
- [ ] 字段映射:选列 + 重命名 + 含审核记录开关生效
- [ ] 导出历史列表 + 签名下载 URL(过期/篡改被拒)
- [ ] Dashboard 数字(进度/通过率/AI vs 人工/各维度均分)与 DB 直查一致
- [ ] `first_come` 分发独立验收
- [ ] 仅用 JSON 题目导入也能完整跑通新建任务流程(import 在 S1 已做,确认未破)
- [ ] 题目预览(Renderer 干跑,S2 已有)

加分项做了的才勾,砍了的写"砍 X,理由 Y"。

### Task 5.4:Demo 剧本片段(并入 Sprint 7 总剧本)

> Owner 进任务 → 看板(进度/通过率/AI vs 人工差异/维度均分,VChart 动效)→ 配字段映射(重命名两列 + 开"含审核记录")→ 选 XLSX 导出 → 历史列表 queued→succeeded(asynqmon 同步可见)→ 下载 → Excel 打开中文正常。**Markdown 导出做了的话 +30s 单独展示对接模型训练。**

---

## §Bonus A —（可选)Markdown 导出器 ★★★

> 张满宣讲会强调"对接模型训练",做了是亮点。时间不够整章砍,acceptance 写明。

1. **迁移 008(已备好占位)**:`git mv docs/bonus-a-migration/008_export_markdown_format.*.sql apps/api/internal/migration/` 即可激活(内容已写好:up 把 `format` enum 加 `'md'`,down 缩回前先删 md 行)。**别提前移**——放在 migration/ 下 `make api` 就会自动 apply。
2. `pkg/exporter/encode_markdown.go`:`EncodeMarkdown(w, cols, rows)` —— 输出 GFM 表格(`| col | col |` + `| --- |` 分隔 + 转义单元格内 `|` 和换行);嵌套值 ```` ```json ```` 代码块或单行 stringify(二选一,文档定一种)。
3. `Encode`/`FileExtension`/`ContentType` 加 `md` 分支;format 校验白名单加 `md`;前端格式 RadioGroup 加一项。
4. 测试:`TestEncodeMarkdown_GFMTableOpensInTypora`(行列数对、表头一致、特殊字符转义)。
5. 验收:Markdown 导出能被 typora / VS Code 预览打开,表头与 CSV 一致。

## §Bonus B —（可选)JSONL / Excel 题目导入 ★★

> JSON 导入 S1 已做(`POST /tasks/:taskId/items/import`,`handler/task.go:156`)。本项把同一入口扩成多格式。

- 入口加 `format` 或按 `Content-Type` / 文件后缀分流:JSONL 逐行解析、XLSX 用 excelize 读首 sheet(首行表头 → 每行一 item payload)。
- 复用 S1 已有的 item 落库逻辑(`importItemsRequest` → 批量 insert task_items),只换"字节流 → []item" 的解析层(放 `pkg/exporter` 反过来叫导入有点怪,可新建 `pkg/importer` 或就放 handler 内)。
- 测试:`TestImportItems_JSONL`、`TestImportItems_XLSX`,字段与 JSON 导入对齐。
- 验收:JSONL / Excel 导入与 JSON 导入字段一致。

## §Bonus C —（可选)`assigned` + `quota` 分发 ★★

> 课题"分发策略任选其一",`first_come` 已满足必交。做这项是加分。`task_assignees` 表已存在,无需迁移。

`claim.go` 的认领改造(按 `task.Distribution` 分支):
- **`assigned`**:候选 item 限定 `JOIN task_assignees ta ON ta.item_id = task_items.id AND ta.user_id = ?`(只领指派给自己的)。
- **`quota`**:领之前先 `SELECT COUNT(*) FROM submissions WHERE task_id=? AND labeler_id=?` ≥ `quota_per_user` 则拒(`ErrQuotaExceeded`,422)。
- Owner 指派 UI/API:`POST /tasks/:taskId/assignees {user_id, item_ids?}`(owner 边界),Dashboard 加最简指派面板。
- 测试:`TestClaim_AssignedOnlyServesAssignedItems`、`TestClaim_QuotaBlocksOverLimit`、`TestAssignees_OwnerOnly`。
- 验收:`assigned` + `quota` 各跑一遍可独立验收。

---

## 验收 Checklist(总,对照 PLAN.md §Sprint4 完成标准)

**必交**
- [ ] 4 格式导出文件字段正确(jq + Excel)
- [ ] 字段映射(选列/重命名/含审核开关)
- [ ] 异步队列(queued→running→succeeded,asynqmon 可见)+ 导出历史 + 签名下载(过期/篡改/路径穿越都拒)
- [ ] Dashboard 看板数字与 DB 直查一致(VChart 渲染)
- [ ] `first_come` 独立验收
- [ ] JSON 题目导入仍可完整跑通建任务流程
- [ ] 题目预览(S2 已有,确认入口在)

**加分(做了才勾)**
- [ ] Markdown 导出(typora 打开 + 表头一致)
- [ ] JSONL / Excel 导入(字段对齐 JSON)
- [ ] `assigned` + `quota` 各独立验收

---

## 风险 & Fallback

| 风险 | 缓解 |
|---|---|
| excelize StreamWriter 对超大 task 内存/耗时 | 必交 task 体量小(seed 12–24 条);真要大,流式已是 StreamWriter,worker 单独进程不阻塞 API |
| worker 是独立 module 不能 import internal | 已规避:共享逻辑全进 `pkg/exporter`(裸 `*sql.DB`),照 `pkg/llmreview` 范式 |
| VChart 撑大 bundle / 既有 >500k chunk warning | Vite 已对大库分包(README 提到);VChart 按需 import,只在 Owner 路由用,可 `React.lazy` |
| 签名下载 secret 泄漏 / 缺失 | `EXPORT_DOWNLOAD_SECRET` 缺失则 download-url 端点 fatal/500(不静默降级);只存本地 `.env`,不入仓 |
| 异步导出失败用户无感知 | `exports.status=failed` + `error_msg`,前端历史列表显示 failed 徽章 + 错误;可重新点导出 |
| 大整数精度(payload/answer 里的 ID) | 沿用 S3 教训:`field_map` 用 raw JSON body;行加载 `json.Unmarshal` 到 `any` 时对数字用 `json.Decoder.UseNumber()` |

---

## Execution Handoff

- 每个 code-writing turn 后按 `CODEX.md` 更新 `README.md`(完成/待改进/下一步/验证命令)。
- 提交粒度:`pkg/exporter` 编码器一组 commit、异步链路一组、下载一组、看板一组、加分各自独立 commit,信息用仓库现有 `feat(...)` / `fix(...)` 风格(对照 `git log`)。
- 顺序严格 Day1→Day5;加分项**必交全绿后**再动。
- 别用后台僵尸 dev/worker 进程(全局规则):要长跑的 `make api` / `make worker` 让 Chen 自己开终端跑,你只 `curl` / 测试验证。
