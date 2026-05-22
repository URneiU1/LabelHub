# S2 v1 Implementation Plan — Template Renderer & Editable Designer

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship Schema-driven Renderer (Plaza/Queue) + editable Designer (append/delete/property edit, save = new version) backed by 4 template API routes, in 6 working days, to clear master PLAN.md Sprint 2 完成标准 at ≥80% with no Sprint 6 spillover.

**Architecture:** Backend extends `apps/api/internal/handler/template.go` (NEW, follows the same per-resource handler pattern as the in-progress Phase 2 refactor's `task.go`/`labeler.go`/etc). Frontend introduces `apps/web/src/renderer/` (runtime, Plaza+Queue consumers) and `apps/web/src/modules/template/` (Designer + List). `DraftCanvas` shell stays separate from `SchemaRenderer` so editor state never pollutes runtime. Field identity uses `_draftId` so rename never breaks selection. Save = new version POST + same-transaction `tasks.template_id` bump, with `task` row locked first to avoid deadlock.

**Tech Stack:** Go 1.22 + Gin + GORM + sqlmock (backend) · React 19 + TypeScript + Vite + Semi Design (frontend) · Vitest + @testing-library/react (NEW for S2) · MySQL 8 · pnpm workspace.

**Source spec:** `docs/PLAN-S2.md` (committed `13822e3`) — all section citations like `§3.3` refer to this file.

---

## Pre-flight (Day 0, ≤30 min)

### Task 0: Baseline checkpoint

**Files:**
- Verify: working tree state

- [ ] **Step 1: Confirm spec committed and Phase 2 refactor state**

Run:
```bash
cd /Users/dadadineiyou/Desktop/LabelHub
git log --oneline -5
git status --short
```

Expected: `docs/PLAN-S2.md` is committed (HEAD at `13822e3` or later). Uncommitted files may include the in-progress Phase 2 handler split. Do NOT commit those on the user's behalf.

- [ ] **Step 2: Confirm backend tests pass from `apps/api/` directory**

Run:
```bash
cd /Users/dadadineiyou/Desktop/LabelHub/apps/api
go test ./...
```

Expected: all packages green. If tests fail because of the user's in-progress refactor, ASK before proceeding — do not attempt to fix unrelated mid-refactor failures.

- [ ] **Step 3: Confirm DB + seed**

Run:
```bash
docker compose -f deploy/docker-compose.yml ps
make seed
docker exec labelhub-mysql mysql -ulabelhub -plabelhub_dev labelhub -e "SELECT id, title, status, template_id FROM tasks WHERE id=1;"
```

Expected: qa_quality task exists, has a `template_id` pointing to a template row.

---

## Day 1 — Backend Template Routes + Seed Upsert

**Goal:** 4 new routes (`GET /tasks/:id/templates`, `POST /tasks/:id/templates`, `GET /templates/:id` with `isLatest`, `POST /tasks/:id/templates/validate`); seed becomes upsert; concurrency-safe POST with row-locked task; all behind sqlmock tests.

**Files this day touches:**
- Create: `apps/api/internal/handler/template.go` (the new handler)
- Create: `apps/api/internal/handler/template_test.go` (sqlmock tests)
- Create: `apps/api/internal/handler/template_validate.go` (validation function, separate for testability)
- Create: `apps/api/internal/handler/template_validate_test.go`
- Modify: `apps/api/cmd/server/main.go` (register TemplateHandler)
- Modify: `apps/api/cmd/seed/main.go::seedQAQuality` (upsert by `schema_hash`)
- Modify: `apps/api/internal/handler/s1_helpers.go` (add `registerAllHandlers` line for TemplateHandler if user's split already exists)

### Task 1.1: Schema validation function (TDD, no DB)

**Why first:** Pure function, no Gin/GORM. Tests pin behavior before HTTP layer touches it. Per spec §6 validate route + §2 lock row "校验 name 唯一 / widget ∈ 9 enum / required bool / min/max 类型与 `min <= max`".

**Files:**
- Create: `apps/api/internal/handler/template_validate.go`
- Create: `apps/api/internal/handler/template_validate_test.go`

- [ ] **Step 1: Write failing tests**

Create `apps/api/internal/handler/template_validate_test.go`:

```go
package handler

import (
	"testing"
)

func TestValidateTemplateSchema(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		wantValid bool
		wantField string // first error's field, "" if none
		wantMsg   string // substring of first error's message
	}{
		{
			name: "happy minimal",
			raw: `{"title":"t","layout":"single_page","fields":[
				{"name":"a","widget":"Input","label":"A","required":false}
			]}`,
			wantValid: true,
		},
		{
			name:      "fields empty",
			raw:       `{"title":"t","layout":"single_page","fields":[]}`,
			wantValid: false, wantField: "fields", wantMsg: "non-empty",
		},
		{
			name: "duplicate name",
			raw: `{"title":"t","layout":"single_page","fields":[
				{"name":"a","widget":"Input"},
				{"name":"a","widget":"Radio"}
			]}`,
			wantValid: false, wantField: "fields[1].name", wantMsg: "duplicate",
		},
		{
			name: "unknown widget",
			raw: `{"title":"t","layout":"single_page","fields":[
				{"name":"a","widget":"Slider"}
			]}`,
			wantValid: false, wantField: "fields[0].widget", wantMsg: "widget not in enum",
		},
		{
			name: "required non-bool via JSON string",
			raw: `{"title":"t","layout":"single_page","fields":[
				{"name":"a","widget":"Input","required":"yes"}
			]}`,
			wantValid: false, wantField: "fields[0].required", wantMsg: "must be bool",
		},
		{
			name: "min > max",
			raw: `{"title":"t","layout":"single_page","fields":[
				{"name":"a","widget":"Input","minLength":10,"maxLength":5}
			]}`,
			wantValid: false, wantField: "fields[0].maxLength", wantMsg: "min > max",
		},
		{
			name: "malformed JSON",
			raw:  `{not json`,
			wantValid: false, wantField: "$", wantMsg: "invalid JSON",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validateTemplateSchema(tc.raw)
			if tc.wantValid {
				if len(errs) != 0 {
					t.Fatalf("expected valid, got errors: %+v", errs)
				}
				return
			}
			if len(errs) == 0 {
				t.Fatal("expected errors, got none")
			}
			if errs[0].Field != tc.wantField {
				t.Errorf("Field = %q, want %q", errs[0].Field, tc.wantField)
			}
			if !contains(errs[0].Message, tc.wantMsg) {
				t.Errorf("Message %q does not contain %q", errs[0].Message, tc.wantMsg)
			}
		})
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/api && go test ./internal/handler/ -run TestValidateTemplateSchema -v`
Expected: FAIL (validateTemplateSchema undefined)

- [ ] **Step 3: Implement validator**

Create `apps/api/internal/handler/template_validate.go`:

```go
package handler

import (
	"encoding/json"
	"fmt"
)

// allowedWidgets:S2 v1 锁定的 9 个核心物料,与 spec §3.5 widgetPrefixMap 严格一致。
var allowedWidgets = map[string]struct{}{
	"ShowItem": {}, "Input": {}, "TextArea": {}, "Radio": {}, "Tags": {},
	"RichText": {}, "JSONEditor": {}, "FileUpload": {}, "LLMTrigger": {},
}

// ValidationError 是 POST /templates/:id/validate 响应里 errors[] 的单条结构。
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// validateTemplateSchema 实现 spec §2 + §6 的"最小校验":
//   - JSON 解析(失败 → field="$")
//   - fields 必填非空数组
//   - name 在 fields[] 内唯一
//   - widget ∈ allowedWidgets
//   - required 必须是 bool(或缺省)
//   - 同字段 minLength 和 maxLength 都必须是非负整数 且 min <= max(若两者都给)
//
// requiredWhen / regex 等高级校验推到 S3(spec §12)。
func validateTemplateSchema(raw string) []ValidationError {
	var parsed struct {
		Fields []map[string]any `json:"fields"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return []ValidationError{{Field: "$", Message: "invalid JSON: " + err.Error()}}
	}
	var errs []ValidationError
	if len(parsed.Fields) == 0 {
		errs = append(errs, ValidationError{Field: "fields", Message: "fields must be non-empty"})
		return errs
	}
	seenNames := make(map[string]int, len(parsed.Fields))
	for i, f := range parsed.Fields {
		path := fmt.Sprintf("fields[%d]", i)
		name, _ := f["name"].(string)
		if name == "" {
			errs = append(errs, ValidationError{Field: path + ".name", Message: "name is required"})
		} else if _, dup := seenNames[name]; dup {
			errs = append(errs, ValidationError{Field: path + ".name", Message: "duplicate name " + name})
		} else {
			seenNames[name] = i
		}
		widget, _ := f["widget"].(string)
		if _, ok := allowedWidgets[widget]; !ok {
			errs = append(errs, ValidationError{Field: path + ".widget", Message: "widget not in enum: " + widget})
		}
		if reqRaw, has := f["required"]; has {
			if _, ok := reqRaw.(bool); !ok {
				errs = append(errs, ValidationError{Field: path + ".required", Message: "required must be bool"})
			}
		}
		minLen, hasMin := numericField(f, "minLength")
		maxLen, hasMax := numericField(f, "maxLength")
		if hasMin && minLen < 0 {
			errs = append(errs, ValidationError{Field: path + ".minLength", Message: "minLength must be >= 0"})
		}
		if hasMax && maxLen < 0 {
			errs = append(errs, ValidationError{Field: path + ".maxLength", Message: "maxLength must be >= 0"})
		}
		if hasMin && hasMax && minLen > maxLen {
			errs = append(errs, ValidationError{Field: path + ".maxLength", Message: "min > max"})
		}
	}
	return errs
}

func numericField(f map[string]any, key string) (int, bool) {
	raw, ok := f[key]
	if !ok {
		return 0, false
	}
	switch v := raw.(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	default:
		return 0, false
	}
}
```

- [ ] **Step 4: Run tests, expect PASS**

Run: `cd apps/api && go test ./internal/handler/ -run TestValidateTemplateSchema -v`
Expected: all 7 sub-tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/dadadineiyou/Desktop/LabelHub
git add apps/api/internal/handler/template_validate.go apps/api/internal/handler/template_validate_test.go
git commit -m "feat(s2-d1): template schema minimal validator with 7-case test"
```

---

### Task 1.2: TemplateHandler skeleton + GET routes (sqlmock TDD)

**Why:** GET routes are pure-read, perfect to lock with sqlmock before touching the transactional POST. Per spec §6 routes table + §6 `isLatest` contract.

**Files:**
- Create: `apps/api/internal/handler/template.go`
- Create: `apps/api/internal/handler/template_test.go`

- [ ] **Step 1: Write failing test for `TestListTemplates_OwnerOnly`**

Create `apps/api/internal/handler/template_test.go`:

```go
package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"

	"labelhub-api/internal/auth"
)

// 列出 task 下所有 template 版本;owner 看自己的 task,其他 owner 403。
func TestListTemplates_OwnerOnly(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	claims := &auth.Claims{UserID: 7, Username: "owner2", Roles: []string{"owner"}}

	// loadOwnedTask 第一步:SELECT tasks
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id"}).AddRow(1, 99)) // owned by 99, not 7

	r := newGinWithClaims(claims)
	NewTemplateHandler(db).Register(r)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tasks/1/templates", nil))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// owner 看自己的 task 时返回 versions DESC
func TestListTemplates_HappyPath(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	claims := &auth.Claims{UserID: 1, Username: "owner1", Roles: []string{"owner"}}

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id"}).AddRow(1, 1))
	// templates list
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_templates.+ORDER BY .version. DESC`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version", "schema_json"}).
			AddRow(101, 1, 2, `{}`).AddRow(100, 1, 1, `{}`))

	r := newGinWithClaims(claims)
	NewTemplateHandler(db).Register(r)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tasks/1/templates", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data []struct {
			ID      uint64 `json:"id"`
			Version int    `json:"version"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Data) != 2 || resp.Data[0].Version != 2 {
		t.Errorf("expected 2 rows DESC, got %+v", resp.Data)
	}
}

// GET /templates/:id 返回 isLatest + latestTemplateId
func TestGetTemplate_ReturnsIsLatest(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	claims := &auth.Claims{UserID: 1, Username: "owner1", Roles: []string{"owner"}}

	// 1. SELECT template by id
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_templates. WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version", "schema_json"}).
			AddRow(100, 1, 1, `{}`))
	// 2. SELECT tasks for ownership check (CanReadTask via policy)
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).AddRow(1, 1, "published"))
	// 3. latest template lookup for isLatest computation
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_templates.+ORDER BY .version. DESC`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(101)) // latest is 101, not 100

	r := newGinWithClaims(claims)
	NewTemplateHandler(db).Register(r)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/templates/100", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			Template         map[string]any `json:"template"`
			IsLatest         bool           `json:"isLatest"`
			LatestTemplateID uint64         `json:"latestTemplateId"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Data.IsLatest {
		t.Error("expected isLatest=false (template 100 vs latest 101)")
	}
	if resp.Data.LatestTemplateID != 101 {
		t.Errorf("expected latestTemplateId=101, got %d", resp.Data.LatestTemplateID)
	}
}
```

- [ ] **Step 2: Run tests, expect FAIL on `NewTemplateHandler` undefined**

Run: `cd apps/api && go test ./internal/handler/ -run TestListTemplates -v`
Expected: build error "NewTemplateHandler undefined".

- [ ] **Step 3: Create TemplateHandler skeleton**

Create `apps/api/internal/handler/template.go`:

```go
package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"labelhub-api/internal/httpx"
	"labelhub-api/internal/middleware"
	"labelhub-api/internal/model"
	"labelhub-api/internal/policy"
)

// TemplateHandler 负责 task_templates 资源的 CRUD + 校验。
// 设计原则:
//   - 路由组分两批:`/tasks/:taskId/templates` 限 owner;`/templates/:templateId` 允许 labeler/reviewer 读已发布 task 的模板
//   - POST 创建新版本时同事务锁 task 行 → 读 MAX(version) → INSERT → UPDATE tasks.template_id(spec §6)
type TemplateHandler struct {
	db *gorm.DB
}

func NewTemplateHandler(db *gorm.DB) TemplateHandler {
	return TemplateHandler{db: db}
}

func (h TemplateHandler) Register(api gin.IRouter) {
	// task 级:列出 / 创建 / 验证
	api.GET("/tasks/:taskId/templates", middleware.RequireRoles("owner", "admin"), h.ListByTask)
	api.POST("/tasks/:taskId/templates", middleware.RequireRoles("owner", "admin"), h.Create)
	api.POST("/tasks/:taskId/templates/validate", middleware.RequireRoles("owner", "admin"), h.Validate)
	// 资源级:GET 单个(labeler/reviewer 用)
	api.GET("/templates/:templateId", middleware.RequireRoles("owner", "admin", "labeler", "reviewer"), h.Get)
}

// ListByTask 列出 task 下所有 template 版本,owner 自己的或 admin。
func (h TemplateHandler) ListByTask(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	var templates []model.TaskTemplate
	if err := h.db.Where("task_id = ?", task.ID).Order("version DESC").Find(&templates).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list templates")
		return
	}
	httpx.PageOK(c, templates, httpx.Page{})
}

// Get 读单个 template,响应携带 isLatest + latestTemplateId(spec §6)。
func (h TemplateHandler) Get(c *gin.Context) {
	templateID, ok := parseIDParam(c, "templateId")
	if !ok {
		return
	}
	var template model.TaskTemplate
	if err := h.db.First(&template, templateID).Error; err != nil {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "template not found")
		return
	}
	// 复用 policy.CanReadTask 判定:owner 看自己的,labeler/reviewer 看 published
	var task model.Task
	if err := h.db.First(&task, template.TaskID).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "task not found for template")
		return
	}
	claims, _ := middleware.Claims(c)
	if !policy.CanReadTask(claims, task) {
		httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "template not visible")
		return
	}
	// 查 latest template id for the same task
	var latest model.TaskTemplate
	if err := h.db.Where("task_id = ?", template.TaskID).Order("version DESC").Select("id").First(&latest).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load latest")
		return
	}
	httpx.OK(c, gin.H{
		"template":         template,
		"isLatest":         template.ID == latest.ID,
		"latestTemplateId": latest.ID,
	})
}

// Create / Validate 见 Task 1.3 / Task 1.1。
func (h TemplateHandler) Create(c *gin.Context) {
	httpx.Error(c, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Create comes in Task 1.3")
}

func (h TemplateHandler) Validate(c *gin.Context) {
	httpx.Error(c, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Validate comes in Task 1.4")
}
```

- [ ] **Step 4: Run tests, expect 3 PASS**

Run: `cd apps/api && go test ./internal/handler/ -run "TestListTemplates|TestGetTemplate" -v`
Expected: 3 sub-tests PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/api/internal/handler/template.go apps/api/internal/handler/template_test.go
git commit -m "feat(s2-d1): TemplateHandler with List + Get(isLatest) routes"
```

---

### Task 1.3: POST templates with transactional version bump

**Why:** This is the structural keystone. Per spec §6 implementation pseudocode + §5 backend ownership of concurrency.

**Files:**
- Modify: `apps/api/internal/handler/template.go::Create`
- Modify: `apps/api/internal/handler/template_test.go` (add `TestCreateTemplate_*`)

- [ ] **Step 1: Write failing tests for happy path + duplicate-key + version bump**

Append to `apps/api/internal/handler/template_test.go`:

```go
// POST templates 必须:锁 task → 读 MAX(version) → INSERT v(N+1) → UPDATE tasks.template_id,全在一个事务。
func TestCreateTemplate_BumpsVersionAndUpdatesTaskTemplateID(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	claims := &auth.Claims{UserID: 1, Username: "owner1", Roles: []string{"owner"}}

	// loadOwnedTask 先 SELECT tasks
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id"}).AddRow(1, 1))

	mock.ExpectBegin()
	// 锁 task 行
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title"}).AddRow(1, "qa"))
	// MAX(version) under lock
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_templates.+MAX`).
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(1))
	// INSERT new template v2
	mock.ExpectExec(`(?is)^INSERT INTO .task_templates.`).
		WillReturnResult(sqlmock.NewResult(202, 1))
	// UPDATE tasks.template_id = 202
	mock.ExpectExec(`(?is)^UPDATE .tasks. SET .template_id.`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	body := `{"fields":[{"name":"a","widget":"Input","required":false}]}`
	r := newGinWithClaims(claims)
	NewTemplateHandler(db).Register(r)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequestRaw(http.MethodPost, "/tasks/1/templates", body))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// uk_task_version 撞了 → 409 而不是 500
func TestCreateTemplate_DuplicateKeyReturns409(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	claims := &auth.Claims{UserID: 1, Username: "owner1", Roles: []string{"owner"}}

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id"}).AddRow(1, 1))
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_templates.+MAX`).
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(1))
	// 模拟 MySQL Error 1062 Duplicate entry
	mock.ExpectExec(`(?is)^INSERT INTO .task_templates.`).
		WillReturnError(&mysqlDupErr{Number: 1062, Msg: "Duplicate entry '1-2' for key 'uk_task_version'"})
	mock.ExpectRollback()

	body := `{"fields":[{"name":"a","widget":"Input"}]}`
	r := newGinWithClaims(claims)
	NewTemplateHandler(db).Register(r)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequestRaw(http.MethodPost, "/tasks/1/templates", body))

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// 422 — invalid body 应该在事务开始前就 reject(不开 ExpectBegin)
func TestCreateTemplate_InvalidBodyReturns422(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	claims := &auth.Claims{UserID: 1, Username: "owner1", Roles: []string{"owner"}}

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id"}).AddRow(1, 1))
	// 没 ExpectBegin — invalid body 应该直接 422

	body := `{"fields":[]}` // empty fields per validator
	r := newGinWithClaims(claims)
	NewTemplateHandler(db).Register(r)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequestRaw(http.MethodPost, "/tasks/1/templates", body))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// 测试用:接受原始 JSON body 的请求构造(s1_integration_test.go 的 jsonRequest 序列化 map,这里要传 raw string)
func jsonRequestRaw(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// mysqlDupErr 模拟 *mysql.MySQLError(避开真实 driver 依赖)
type mysqlDupErr struct {
	Number uint16
	Msg    string
}

func (e *mysqlDupErr) Error() string { return e.Msg }
```

Also add import for `bytes` at top of file:

```go
import (
	"bytes"
	"encoding/json"
	...
)
```

- [ ] **Step 2: Run, expect FAIL (Create returns 501)**

Run: `cd apps/api && go test ./internal/handler/ -run TestCreateTemplate -v`
Expected: FAIL with 501.

- [ ] **Step 3: Implement Create**

Replace `Create` in `apps/api/internal/handler/template.go`:

```go
import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"labelhub-api/internal/httpx"
	"labelhub-api/internal/middleware"
	"labelhub-api/internal/model"
	"labelhub-api/internal/policy"
)

type createTemplateRequest struct {
	Title  *string          `json:"title"`
	Layout *string          `json:"layout"`
	Fields []map[string]any `json:"fields" binding:"required"`
}

// errConflictTemplate 内部标记 uk_task_version 撞,handler 转 409。
var errConflictTemplate = errors.New("template version conflict")

func (h TemplateHandler) Create(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	var req createTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "fields is required")
		return
	}
	// 重新组装成校验器期望的 shape,跑最小校验(spec §6)
	schemaMap := gin.H{
		"title":  derefStr(req.Title, task.Title),
		"layout": derefStr(req.Layout, "single_page"),
		"fields": req.Fields,
	}
	schemaJSON, err := json.Marshal(schemaMap)
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "failed to encode schema")
		return
	}
	if errs := validateTemplateSchema(string(schemaJSON)); len(errs) > 0 {
		httpx.ErrorWithDetails(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR",
			"schema validation failed", gin.H{"errors": errs})
		return
	}

	claims, _ := middleware.Claims(c)
	hash := sha256.Sum256(schemaJSON)
	schemaHash := hex.EncodeToString(hash[:])

	var newT model.TaskTemplate
	err = h.db.Transaction(func(tx *gorm.DB) error {
		// 1. 锁 task 行 — 统一锁顺序避免与 ClaimItem / Save 死锁(spec §6)
		var lockedTask model.Task
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", task.ID).First(&lockedTask).Error; err != nil {
			return err
		}
		// 2. 在 task 锁保护下读 MAX(version)
		var maxV int
		if err := tx.Model(&model.TaskTemplate{}).
			Where("task_id = ?", task.ID).
			Select("COALESCE(MAX(version),0)").Scan(&maxV).Error; err != nil {
			return err
		}
		// 3. INSERT new template
		newT = model.TaskTemplate{
			TaskID:     task.ID,
			Version:    maxV + 1,
			SchemaJSON: string(schemaJSON),
			SchemaHash: schemaHash,
			CreatedBy:  claims.UserID,
		}
		if err := tx.Create(&newT).Error; err != nil {
			if isDuplicateKey(err) {
				return errConflictTemplate
			}
			return err
		}
		// 4. 同事务更新 tasks.template_id
		return tx.Model(&model.Task{}).Where("id = ?", task.ID).
			Update("template_id", newT.ID).Error
	})
	if errors.Is(err, errConflictTemplate) {
		httpx.Error(c, http.StatusConflict, "CONFLICT", "a newer template version already exists, please refresh")
		return
	}
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create template")
		return
	}
	httpx.OK(c, gin.H{"template": newT})
}

func derefStr(p *string, fallback string) string {
	if p == nil || *p == "" {
		return fallback
	}
	return *p
}

// isDuplicateKey:既兼容 mysql driver 的 1062,也接受任何 message 含 "Duplicate entry" 的 error(sqlmock 的 mysqlDupErr)。
func isDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	// 不直接 import mysql driver 避免循环依赖,字符串匹配兜底
	return strings.Contains(err.Error(), "Duplicate entry") ||
		strings.Contains(err.Error(), "uk_task_version")
}
```

- [ ] **Step 4: Run, expect 3 PASS**

Run: `cd apps/api && go test ./internal/handler/ -run TestCreateTemplate -v`
Expected: 3 sub-tests PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/api/internal/handler/template.go apps/api/internal/handler/template_test.go
git commit -m "feat(s2-d1): POST templates with task-row-locked transactional version bump"
```

---

### Task 1.4: Validate endpoint + register in main.go

**Why:** Cap Day 1 with the validate route and wire the new handler into the running server.

**Files:**
- Modify: `apps/api/internal/handler/template.go::Validate`
- Modify: `apps/api/cmd/server/main.go` (register TemplateHandler)
- Modify: `apps/api/internal/handler/s1_helpers.go::registerAllHandlers` (test helper for security_test)

- [ ] **Step 1: Add test for validate route**

Append to `apps/api/internal/handler/template_test.go`:

```go
func TestValidateTemplate_RejectsDuplicateNames(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	claims := &auth.Claims{UserID: 1, Username: "owner1", Roles: []string{"owner"}}

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id"}).AddRow(1, 1))
	// no DB writes — validate is pure
	body := `{"fields":[
		{"name":"a","widget":"Input"},
		{"name":"a","widget":"Radio"}
	]}`
	r := newGinWithClaims(claims)
	NewTemplateHandler(db).Register(r)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, jsonRequestRaw(http.MethodPost, "/tasks/1/templates/validate", body))

	if rec.Code != http.StatusOK { // validate ALWAYS returns 200; result lives in body.valid
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			Valid  bool              `json:"valid"`
			Errors []ValidationError `json:"errors"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Data.Valid {
		t.Error("expected valid=false")
	}
	if len(resp.Data.Errors) == 0 || resp.Data.Errors[0].Field != "fields[1].name" {
		t.Errorf("expected duplicate-name error on fields[1].name, got %+v", resp.Data.Errors)
	}
}
```

- [ ] **Step 2: Run, expect FAIL (Validate returns 501)**

Run: `cd apps/api && go test ./internal/handler/ -run TestValidateTemplate -v`
Expected: FAIL.

- [ ] **Step 3: Implement Validate**

Replace `Validate` in `template.go`:

```go
func (h TemplateHandler) Validate(c *gin.Context) {
	if _, ok := loadOwnedTask(h.db, c); !ok {
		return
	}
	var req createTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.OK(c, gin.H{
			"valid":  false,
			"errors": []ValidationError{{Field: "$", Message: "invalid JSON body"}},
		})
		return
	}
	schemaMap := gin.H{
		"title":  derefStr(req.Title, ""),
		"layout": derefStr(req.Layout, "single_page"),
		"fields": req.Fields,
	}
	raw, _ := json.Marshal(schemaMap)
	errs := validateTemplateSchema(string(raw))
	httpx.OK(c, gin.H{
		"valid":  len(errs) == 0,
		"errors": errs,
	})
}
```

- [ ] **Step 4: Register TemplateHandler in main.go**

Modify `apps/api/cmd/server/main.go`. Find the section where other handlers are registered (e.g., `handler.NewTaskHandler(...)`) and add immediately after:

```go
handler.NewTemplateHandler(database).Register(authedAPI)
```

If the user's Phase 2 refactor uses `registerAllHandlers` helper, also add `NewTemplateHandler(db).Register(r)` to that helper in `s1_helpers.go`.

- [ ] **Step 5: Run all handler tests, build server, expect all green**

Run:
```bash
cd apps/api && go test ./... && go build ./cmd/server
```

Expected: all tests PASS, server builds without error.

- [ ] **Step 6: Commit**

```bash
git add apps/api/internal/handler/template.go apps/api/internal/handler/template_test.go apps/api/cmd/server/main.go apps/api/internal/handler/s1_helpers.go
git commit -m "feat(s2-d1): validate endpoint + register TemplateHandler in main"
```

---

### Task 1.5: Seed upsert by schema_hash

**Why:** Spec acceptance #10 ("change qa_quality_review.json → re-seed → Plaza shows new label"). Current seed uses `FirstOrCreate` + `Attrs` which never updates existing v1.

**Files:**
- Modify: `apps/api/cmd/seed/main.go::seedQAQuality` (the section that creates the template)

- [ ] **Step 1: Locate current seed template block**

Run: `grep -n 'TaskTemplate\|template' apps/api/cmd/seed/main.go | head -20`

Find the block (~line 134-147 per earlier exploration) that does:
```go
schema, err := os.ReadFile(...)
template := model.TaskTemplate{...}
tx.Where("task_id = ? AND version = ?", task.ID, 1).Attrs(template).FirstOrCreate(&template)
```

- [ ] **Step 2: Replace with hash-aware upsert**

In `apps/api/cmd/seed/main.go::seedQAQuality`, replace the `tx.Where(...).Attrs(template).FirstOrCreate(...)` line and its surrounding context with:

```go
schema, err := os.ReadFile(projectFile("tools/seed/templates/qa_quality_review.json"))
if err != nil {
	return err
}
hash := sha256.Sum256(schema)
schemaHash := hex.EncodeToString(hash[:])

template := model.TaskTemplate{
	TaskID:     task.ID,
	Version:    1,
	SchemaJSON: string(schema),
	SchemaHash: schemaHash,
	CreatedBy:  owner.ID,
}
var existing model.TaskTemplate
err = tx.Where("task_id = ? AND version = ?", task.ID, 1).First(&existing).Error
if errors.Is(err, gorm.ErrRecordNotFound) {
	if err := tx.Create(&template).Error; err != nil {
		return err
	}
	// 第一次 seed 把 task.template_id 指向 v1
	if err := tx.Model(&model.Task{}).Where("id = ?", task.ID).Update("template_id", template.ID).Error; err != nil {
		return err
	}
} else if err != nil {
	return err
} else if existing.SchemaHash != schemaHash {
	// v1 已存在但 schema 变了 — UPDATE
	if err := tx.Model(&existing).Updates(map[string]any{
		"schema_json": template.SchemaJSON,
		"schema_hash": schemaHash,
	}).Error; err != nil {
		return err
	}
	log.Printf("seed: updated qa_quality template v1 (hash %s → %s)", existing.SchemaHash[:8], schemaHash[:8])
}
```

Add to imports at top of file:

```go
import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	...
	"gorm.io/gorm"
)
```

- [ ] **Step 3: Verify by running seed twice with different schema**

Run:
```bash
cd /Users/dadadineiyou/Desktop/LabelHub
make seed                                                # first run
# Note current schema_json hash
docker exec labelhub-mysql mysql -ulabelhub -plabelhub_dev labelhub -N -e "SELECT schema_hash FROM task_templates WHERE task_id=1 AND version=1;"

# Modify a label
sed -i.bak 's/"label": "原始数据"/"label": "原始数据 TEST"/' tools/seed/templates/qa_quality_review.json
make seed                                                # second run, should UPDATE
docker exec labelhub-mysql mysql -ulabelhub -plabelhub_dev labelhub -N -e "SELECT JSON_EXTRACT(schema_json, '\$.fields[0].label') FROM task_templates WHERE task_id=1 AND version=1;"

# Revert
mv tools/seed/templates/qa_quality_review.json.bak tools/seed/templates/qa_quality_review.json
make seed
```

Expected: second `SELECT` returns `"原始数据 TEST"`. After revert + third seed, the value goes back.

- [ ] **Step 4: Commit**

```bash
git add apps/api/cmd/seed/main.go
git commit -m "feat(s2-d1): seed upserts qa_quality template by schema_hash"
```

---

## Day 2 — Vitest + Renderer Module + 5 Simple Widgets

**Goal:** Vitest test stack lives in `apps/web/`; `apps/web/src/renderer/` has parser/validator/types and 5 simple widgets (Input/TextArea/Radio/Tags/ShowItem); SchemaRenderer can render qa_quality fixture's 12 fields (4 are still placeholders until Day 3).

**Files this day touches:**
- Create: `apps/web/vitest.config.ts`
- Modify: `apps/web/package.json` (add deps + `test` script)
- Create: `apps/web/src/renderer/types.ts`
- Create: `apps/web/src/renderer/parser.ts` + `.test.ts`
- Create: `apps/web/src/renderer/validator.ts` + `.test.ts`
- Create: `apps/web/src/renderer/widgets/{Input,TextArea,Radio,Tags,ShowItem}.tsx`
- Create: `apps/web/src/renderer/widgets/index.ts` (registry)
- Create: `apps/web/src/renderer/SchemaRenderer.tsx` + `.test.tsx`
- Create: `apps/web/src/renderer/__fixtures__/qa_quality.ts` (test fixture)

### Task 2.1: Vitest infrastructure

**Files:**
- Create: `apps/web/vitest.config.ts`
- Create: `apps/web/src/test-setup.ts`
- Modify: `apps/web/package.json`

- [ ] **Step 1: Install deps**

Run:
```bash
cd /Users/dadadineiyou/Desktop/LabelHub
pnpm --filter web add -D vitest @testing-library/react @testing-library/jest-dom @testing-library/user-event jsdom
```

Note: `--filter web` matches the package name in `apps/web/package.json`. If the package name is different (e.g., `@labelhub/web`), use that. Verify with `cat apps/web/package.json | head -5` if unsure.

- [ ] **Step 2: Create vitest config**

Create `apps/web/vitest.config.ts`:

```typescript
import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    globals: false,
    setupFiles: ['./src/test-setup.ts'],
    include: ['src/**/*.test.{ts,tsx}'],
    css: false,
  },
})
```

Create `apps/web/src/test-setup.ts`:

```typescript
import '@testing-library/jest-dom/vitest'
```

- [ ] **Step 3: Add scripts to package.json**

Modify `apps/web/package.json`, add to `scripts`:

```json
{
  "scripts": {
    "test": "vitest run",
    "test:watch": "vitest"
  }
}
```

- [ ] **Step 4: Write a smoke test to verify setup**

Create `apps/web/src/test-setup.smoke.test.ts`:

```typescript
import { describe, it, expect } from 'vitest'

describe('vitest setup', () => {
  it('runs', () => {
    expect(1 + 1).toBe(2)
  })
  it('has jest-dom matchers', () => {
    const div = document.createElement('div')
    div.textContent = 'hello'
    expect(div).toHaveTextContent('hello')
  })
})
```

- [ ] **Step 5: Run and verify green**

Run: `cd apps/web && pnpm test`
Expected: 2 tests PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/web/vitest.config.ts apps/web/src/test-setup.ts apps/web/src/test-setup.smoke.test.ts apps/web/package.json apps/web/pnpm-lock.yaml pnpm-lock.yaml
git commit -m "feat(s2-d2): vitest + testing-library + jsdom setup"
```

---

### Task 2.2: Renderer types + parser (TDD)

**Files:**
- Create: `apps/web/src/renderer/types.ts`
- Create: `apps/web/src/renderer/parser.ts`
- Create: `apps/web/src/renderer/parser.test.ts`

- [ ] **Step 1: Define types**

Create `apps/web/src/renderer/types.ts`:

```typescript
// 9 个核心 widget(spec §3.5 widgetPrefixMap 严格一致)
export type WidgetType =
  | 'ShowItem'
  | 'Input'
  | 'TextArea'
  | 'Radio'
  | 'Tags'
  | 'RichText'
  | 'JSONEditor'
  | 'FileUpload'
  | 'LLMTrigger'

export type ShowItemMode = 'auto' | 'text' | 'video' | 'image' | 'markdown' | 'json'

// 一条 field 的 schema(parsed 后,不含 _draftId — _draftId 只在 Designer 内部)
export interface FieldSchema {
  name: string
  widget: WidgetType
  label?: string
  required?: boolean
  // widget-specific
  options?: Array<string | number>      // Radio / Tags
  minLength?: number                    // Input / TextArea
  maxLength?: number                    // Input / TextArea
  maxFiles?: number                     // FileUpload
  prompt?: string                       // LLMTrigger
  targetField?: string                  // LLMTrigger
  path?: string                         // ShowItem,e.g. "$payload"
  mode?: ShowItemMode                   // ShowItem
}

export interface TemplateSchema {
  title?: string
  layout: 'single_page'
  fields: FieldSchema[]
}

// parser 返回结果的 result-type pattern,避免 throw 让调用方 try/catch
export type ParseResult<T> =
  | { ok: true; value: T }
  | { ok: false; error: string }

export type AnswerValue = Record<string, unknown>
```

- [ ] **Step 2: Write parser tests**

Create `apps/web/src/renderer/parser.test.ts`:

```typescript
import { describe, it, expect } from 'vitest'
import { parseTemplateSchema, parseAnswer } from './parser'

describe('parseTemplateSchema', () => {
  it('parses valid qa_quality-like schema', () => {
    const raw = JSON.stringify({
      title: 't',
      layout: 'single_page',
      fields: [{ name: 'a', widget: 'Input', label: 'A' }],
    })
    const result = parseTemplateSchema(raw)
    expect(result.ok).toBe(true)
    if (result.ok) {
      expect(result.value.fields).toHaveLength(1)
      expect(result.value.fields[0].widget).toBe('Input')
    }
  })

  it('rejects malformed JSON', () => {
    const result = parseTemplateSchema('{not json')
    expect(result.ok).toBe(false)
    if (!result.ok) expect(result.error).toMatch(/JSON/)
  })

  it('rejects missing fields array', () => {
    const result = parseTemplateSchema(JSON.stringify({ title: 't', layout: 'single_page' }))
    expect(result.ok).toBe(false)
  })

  it('rejects fields not an array', () => {
    const result = parseTemplateSchema(JSON.stringify({ layout: 'single_page', fields: 'oops' }))
    expect(result.ok).toBe(false)
  })

  it('rejects unknown widget', () => {
    const result = parseTemplateSchema(JSON.stringify({
      layout: 'single_page',
      fields: [{ name: 'a', widget: 'Slider' }],
    }))
    expect(result.ok).toBe(false)
    if (!result.ok) expect(result.error).toMatch(/widget/)
  })

  it('accepts empty string as empty schema error', () => {
    const result = parseTemplateSchema('')
    expect(result.ok).toBe(false)
  })
})

describe('parseAnswer', () => {
  it('parses valid answer JSON', () => {
    const result = parseAnswer(JSON.stringify({ a: 1, b: 'two' }))
    expect(result.ok).toBe(true)
    if (result.ok) expect(result.value).toEqual({ a: 1, b: 'two' })
  })
  it('returns ok:false on bad JSON', () => {
    expect(parseAnswer('{').ok).toBe(false)
  })
  it('returns ok:false when not an object', () => {
    expect(parseAnswer('"string"').ok).toBe(false)
    expect(parseAnswer('[]').ok).toBe(false)
  })
  it('treats empty string as empty object', () => {
    const result = parseAnswer('')
    expect(result.ok).toBe(true)
    if (result.ok) expect(result.value).toEqual({})
  })
})
```

- [ ] **Step 3: Run, expect FAIL (parser doesn't exist)**

Run: `cd apps/web && pnpm test src/renderer/parser.test.ts`
Expected: FAIL.

- [ ] **Step 4: Implement parser**

Create `apps/web/src/renderer/parser.ts`:

```typescript
import type { ParseResult, TemplateSchema, AnswerValue, WidgetType } from './types'

const ALLOWED_WIDGETS: WidgetType[] = [
  'ShowItem', 'Input', 'TextArea', 'Radio', 'Tags',
  'RichText', 'JSONEditor', 'FileUpload', 'LLMTrigger',
]

export function parseTemplateSchema(raw: string): ParseResult<TemplateSchema> {
  if (!raw) {
    return { ok: false, error: 'schema is empty' }
  }
  let obj: unknown
  try {
    obj = JSON.parse(raw)
  } catch (e) {
    return { ok: false, error: 'invalid JSON: ' + (e instanceof Error ? e.message : String(e)) }
  }
  if (!isObject(obj)) {
    return { ok: false, error: 'schema must be an object' }
  }
  const fields = (obj as Record<string, unknown>).fields
  if (!Array.isArray(fields)) {
    return { ok: false, error: 'fields must be an array' }
  }
  for (let i = 0; i < fields.length; i++) {
    const f = fields[i]
    if (!isObject(f)) {
      return { ok: false, error: `fields[${i}] must be an object` }
    }
    const widget = (f as Record<string, unknown>).widget
    if (typeof widget !== 'string' || !ALLOWED_WIDGETS.includes(widget as WidgetType)) {
      return { ok: false, error: `fields[${i}].widget unknown: ${widget}` }
    }
    const name = (f as Record<string, unknown>).name
    if (typeof name !== 'string' || name === '') {
      return { ok: false, error: `fields[${i}].name is required` }
    }
  }
  const layout = (obj as Record<string, unknown>).layout
  return {
    ok: true,
    value: {
      title: (obj as Record<string, unknown>).title as string | undefined,
      layout: (layout === 'single_page' ? 'single_page' : 'single_page') as 'single_page',
      fields: fields as TemplateSchema['fields'],
    },
  }
}

export function parseAnswer(raw: string): ParseResult<AnswerValue> {
  if (!raw) {
    return { ok: true, value: {} }
  }
  let obj: unknown
  try {
    obj = JSON.parse(raw)
  } catch (e) {
    return { ok: false, error: 'invalid JSON: ' + (e instanceof Error ? e.message : String(e)) }
  }
  if (!isObject(obj)) {
    return { ok: false, error: 'answer must be an object' }
  }
  return { ok: true, value: obj as AnswerValue }
}

function isObject(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}
```

- [ ] **Step 5: Run, expect PASS**

Run: `cd apps/web && pnpm test src/renderer/parser.test.ts`
Expected: all parser tests PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/web/src/renderer/types.ts apps/web/src/renderer/parser.ts apps/web/src/renderer/parser.test.ts
git commit -m "feat(s2-d2): renderer types + parseTemplateSchema/parseAnswer with tests"
```

---

### Task 2.3: Validator (TDD)

**Files:**
- Create: `apps/web/src/renderer/validator.ts` + `.test.ts`

- [ ] **Step 1: Write tests**

Create `apps/web/src/renderer/validator.test.ts`:

```typescript
import { describe, it, expect } from 'vitest'
import { validateAnswer } from './validator'
import type { TemplateSchema } from './types'

const template: TemplateSchema = {
  layout: 'single_page',
  fields: [
    { name: 'summary', widget: 'Input', required: true, maxLength: 80 },
    { name: 'comment', widget: 'TextArea', required: true, minLength: 10 },
    { name: 'note', widget: 'Input', required: false, maxLength: 50 },
  ],
}

describe('validateAnswer', () => {
  it('passes valid answer', () => {
    const result = validateAnswer(template, {
      summary: 'good',
      comment: 'this is sufficiently long',
    })
    expect(result.valid).toBe(true)
    expect(result.errors).toEqual({})
  })

  it('flags missing required field', () => {
    const result = validateAnswer(template, { comment: 'something long' })
    expect(result.valid).toBe(false)
    expect(result.errors.summary).toMatch(/required/)
  })

  it('flags maxLength violation', () => {
    const result = validateAnswer(template, {
      summary: 'x'.repeat(81),
      comment: 'something long',
    })
    expect(result.errors.summary).toMatch(/max/)
  })

  it('flags minLength violation', () => {
    const result = validateAnswer(template, {
      summary: 'ok',
      comment: 'short',
    })
    expect(result.errors.comment).toMatch(/min/)
  })

  it('does not flag optional empty field', () => {
    const result = validateAnswer(template, {
      summary: 'ok',
      comment: 'sufficiently long content',
    })
    expect(result.errors.note).toBeUndefined()
  })
})
```

- [ ] **Step 2: Run, expect FAIL**

Run: `cd apps/web && pnpm test src/renderer/validator.test.ts`
Expected: FAIL.

- [ ] **Step 3: Implement validator**

Create `apps/web/src/renderer/validator.ts`:

```typescript
import type { TemplateSchema, FieldSchema, AnswerValue } from './types'

export interface ValidationResult {
  valid: boolean
  errors: Record<string, string>  // field.name → first error message
}

export function validateAnswer(template: TemplateSchema, answer: AnswerValue): ValidationResult {
  const errors: Record<string, string> = {}
  for (const f of template.fields) {
    const value = answer[f.name]
    const err = validateField(f, value)
    if (err) errors[f.name] = err
  }
  return { valid: Object.keys(errors).length === 0, errors }
}

function validateField(f: FieldSchema, value: unknown): string | null {
  const isEmpty =
    value === undefined ||
    value === null ||
    (typeof value === 'string' && value.trim() === '') ||
    (Array.isArray(value) && value.length === 0)

  if (f.required && isEmpty) {
    return `${f.label ?? f.name} is required`
  }
  if (isEmpty) return null
  if (typeof value === 'string') {
    if (f.minLength !== undefined && value.length < f.minLength) {
      return `min length ${f.minLength}`
    }
    if (f.maxLength !== undefined && value.length > f.maxLength) {
      return `max length ${f.maxLength}`
    }
  }
  return null
}
```

- [ ] **Step 4: Run, expect PASS**

Run: `cd apps/web && pnpm test src/renderer/validator.test.ts`
Expected: all 5 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/renderer/validator.ts apps/web/src/renderer/validator.test.ts
git commit -m "feat(s2-d2): validateAnswer with required/min/max rules"
```

---

### Task 2.4: 5 simple widgets + registry + SchemaRenderer

**Why:** All 5 widgets share a uniform props contract `{field, value, onChange, readOnly?}` (spec §3.5). SchemaRenderer dispatches via a registry map.

**Files:**
- Create: `apps/web/src/renderer/widgets/Input.tsx`
- Create: `apps/web/src/renderer/widgets/TextArea.tsx`
- Create: `apps/web/src/renderer/widgets/Radio.tsx`
- Create: `apps/web/src/renderer/widgets/Tags.tsx`
- Create: `apps/web/src/renderer/widgets/ShowItem.tsx` (port from `shared/components/ShowItem.tsx` + path/mode awareness)
- Create: `apps/web/src/renderer/widgets/index.ts` (registry)
- Create: `apps/web/src/renderer/SchemaRenderer.tsx`
- Create: `apps/web/src/renderer/__fixtures__/qa_quality.ts`
- Create: `apps/web/src/renderer/SchemaRenderer.test.tsx`

- [ ] **Step 1: Define common props type**

Append to `apps/web/src/renderer/types.ts`:

```typescript
export interface WidgetProps<T = unknown> {
  field: FieldSchema
  value: T | undefined
  onChange: (v: T) => void
  readOnly?: boolean
}
```

- [ ] **Step 2: Implement Input widget**

Create `apps/web/src/renderer/widgets/Input.tsx`:

```typescript
import { Input as SemiInput } from '@douyinfe/semi-ui'
import type { WidgetProps } from '../types'

export function InputWidget({ field, value, onChange, readOnly }: WidgetProps<string>) {
  return (
    <SemiInput
      aria-label={field.label ?? field.name}
      value={value ?? ''}
      onChange={(v) => onChange(v)}
      maxLength={field.maxLength}
      disabled={readOnly}
      placeholder={field.label}
    />
  )
}
```

- [ ] **Step 3: Implement TextArea widget**

Create `apps/web/src/renderer/widgets/TextArea.tsx`:

```typescript
import { TextArea as SemiTextArea } from '@douyinfe/semi-ui'
import type { WidgetProps } from '../types'

export function TextAreaWidget({ field, value, onChange, readOnly }: WidgetProps<string>) {
  return (
    <SemiTextArea
      aria-label={field.label ?? field.name}
      value={value ?? ''}
      onChange={(v) => onChange(v)}
      maxLength={field.maxLength}
      disabled={readOnly}
      rows={4}
    />
  )
}
```

- [ ] **Step 4: Implement Radio widget**

Create `apps/web/src/renderer/widgets/Radio.tsx`:

```typescript
import { Radio, RadioGroup } from '@douyinfe/semi-ui'
import type { WidgetProps } from '../types'

export function RadioWidget({ field, value, onChange, readOnly }: WidgetProps<string | number>) {
  const options = field.options ?? []
  return (
    <RadioGroup
      aria-label={field.label ?? field.name}
      value={value as string | number | undefined}
      onChange={(e) => onChange(e.target.value as string | number)}
      disabled={readOnly}
    >
      {options.map((opt) => (
        <Radio key={String(opt)} value={opt}>{String(opt)}</Radio>
      ))}
    </RadioGroup>
  )
}
```

- [ ] **Step 5: Implement Tags widget (multi-select)**

Create `apps/web/src/renderer/widgets/Tags.tsx`:

```typescript
import { CheckboxGroup, Checkbox } from '@douyinfe/semi-ui'
import type { WidgetProps } from '../types'

export function TagsWidget({ field, value, onChange, readOnly }: WidgetProps<string[]>) {
  const options = field.options ?? []
  return (
    <CheckboxGroup
      aria-label={field.label ?? field.name}
      value={value ?? []}
      onChange={(v) => onChange(v as string[])}
      disabled={readOnly}
    >
      {options.map((opt) => (
        <Checkbox key={String(opt)} value={String(opt)}>{String(opt)}</Checkbox>
      ))}
    </CheckboxGroup>
  )
}
```

- [ ] **Step 6: Implement ShowItem with path + mode (Finding 4 闭环)**

Create `apps/web/src/renderer/widgets/ShowItem.tsx`:

```typescript
import type { WidgetProps, ShowItemMode } from '../types'

// ShowItem 不接 value/onChange — 它是只读的 payload 展示物料(spec §3.5)。
// 接受 fullAnswer + payload(从父级传入,通过 SchemaRenderer.context 或 prop)。
// 简化版 v1:从 props 接 payload,按 field.path 取数据,按 field.mode 路由渲染。
export interface ShowItemProps {
  field: WidgetProps['field']
  payload: Record<string, unknown>      // 整道题的 payload(item.payload JSON 解析后)
}

export function ShowItemWidget({ field, payload }: ShowItemProps) {
  const data = resolvePath(payload, field.path ?? '$payload')
  const mode = field.mode ?? 'auto'
  const resolvedMode: ShowItemMode = mode === 'auto' ? detectMode(data) : mode
  return (
    <section aria-label={field.label ?? field.name}>
      {renderByMode(resolvedMode, data)}
    </section>
  )
}

function resolvePath(payload: Record<string, unknown>, path: string): unknown {
  // S2 v1 只支持 "$payload"(整 payload)和 "$payload.field" 一层访问。
  // 复杂 JSONPath 推迟。
  if (path === '$payload' || path === '$') return payload
  const m = path.match(/^\$payload\.(\w+)$/)
  if (m) return payload[m[1]]
  return undefined
}

function detectMode(data: unknown): ShowItemMode {
  if (typeof data === 'string') {
    if (/\.(mp4|webm|ogg)$/i.test(data)) return 'video'
    if (/\.(png|jpe?g|gif|webp|svg)$/i.test(data)) return 'image'
    if (/^https?:\/\//.test(data) && data.includes('.md')) return 'markdown'
    return 'text'
  }
  if (data && typeof data === 'object') return 'json'
  return 'text'
}

function renderByMode(mode: ShowItemMode, data: unknown) {
  switch (mode) {
    case 'video':
      return <video src={String(data)} controls style={{ maxWidth: '100%' }} />
    case 'image':
      return <img src={String(data)} alt="" style={{ maxWidth: '100%' }} />
    case 'markdown':
      // S2 v1 降级为 <pre>;Sprint 6 接 react-markdown(spec §12)
      return <pre style={{ whiteSpace: 'pre-wrap' }}>{String(data)}</pre>
    case 'json':
      return <pre style={{ whiteSpace: 'pre-wrap' }}>{JSON.stringify(data, null, 2)}</pre>
    case 'text':
    default:
      return <pre style={{ whiteSpace: 'pre-wrap' }}>{String(data ?? '')}</pre>
  }
}
```

- [ ] **Step 7: Create widget registry**

Create `apps/web/src/renderer/widgets/index.ts`:

```typescript
import type { WidgetType } from '../types'
import { InputWidget } from './Input'
import { TextAreaWidget } from './TextArea'
import { RadioWidget } from './Radio'
import { TagsWidget } from './Tags'
import { ShowItemWidget } from './ShowItem'

// Day 3 fills the remaining 4
export const widgetRegistry: Partial<Record<WidgetType, React.ComponentType<any>>> = {
  Input: InputWidget,
  TextArea: TextAreaWidget,
  Radio: RadioWidget,
  Tags: TagsWidget,
  ShowItem: ShowItemWidget,
}
```

- [ ] **Step 8: Implement SchemaRenderer + qa_quality fixture**

Create `apps/web/src/renderer/__fixtures__/qa_quality.ts`:

```typescript
// 复制自 tools/seed/templates/qa_quality_review.json 的 12-field 结构,作测试稳定来源。
export const qaQualityTemplate = {
  title: 'qa_quality_review',
  layout: 'single_page' as const,
  fields: [
    { name: 'source_display',     widget: 'ShowItem' as const,   label: '原始数据',   path: '$payload', mode: 'auto' as const },
    { name: 'relevance_score',    widget: 'Radio' as const,      label: '相关性',     required: true, options: [1, 2, 3, 4, 5] },
    { name: 'accuracy_score',     widget: 'Radio' as const,      label: '准确性',     required: true, options: [1, 2, 3, 4, 5] },
    { name: 'format_score',       widget: 'Radio' as const,      label: '格式合规',   required: true, options: [1, 2, 3, 4, 5] },
    { name: 'safety_score',       widget: 'Radio' as const,      label: '安全性',     required: true, options: [1, 2, 3, 4, 5] },
    { name: 'issue_tags',         widget: 'Tags' as const,       label: '问题类型',   options: ['事实错误', '遗漏要点', '格式问题', '安全风险', '表达冗余', '无明显问题'] },
    { name: 'summary',            widget: 'Input' as const,      label: '一句话总评', required: true, maxLength: 80 },
    { name: 'comment',            widget: 'TextArea' as const,   label: '详细评语',   required: true, minLength: 10 },
    { name: 'revision_suggestion', widget: 'RichText' as const,  label: '修订建议' },
    { name: 'corrected_answer',   widget: 'JSONEditor' as const, label: '修正后答案' },
    { name: 'evidence_files',     widget: 'FileUpload' as const, label: '证据素材' },
    { name: 'ai_precheck',        widget: 'LLMTrigger' as const, label: 'AI 预评分' },
  ],
}
```

Create `apps/web/src/renderer/SchemaRenderer.tsx`:

```typescript
import type { TemplateSchema, AnswerValue } from './types'
import { widgetRegistry } from './widgets'

export interface SchemaRendererProps {
  template: TemplateSchema
  answer: AnswerValue
  onChange: (next: AnswerValue) => void
  readOnly?: boolean
  // payload 用于 ShowItem(从 item.payload 解析)
  payload?: Record<string, unknown>
}

export function SchemaRenderer({ template, answer, onChange, readOnly, payload }: SchemaRendererProps) {
  return (
    <div role="form" aria-label={template.title}>
      {template.fields.map((field) => {
        const Widget = widgetRegistry[field.widget]
        const isShowItem = field.widget === 'ShowItem'
        const fieldId = `field-${field.name}`
        return (
          <div key={field.name} style={{ marginBottom: 16 }}>
            <label htmlFor={fieldId} style={{ display: 'block', marginBottom: 4 }}>
              {field.label ?? field.name}
              {field.required ? <span style={{ color: 'red' }}> *</span> : null}
            </label>
            {Widget ? (
              isShowItem ? (
                <Widget field={field} payload={payload ?? {}} />
              ) : (
                <Widget
                  field={field}
                  value={answer[field.name]}
                  onChange={(v: unknown) => onChange({ ...answer, [field.name]: v })}
                  readOnly={readOnly}
                />
              )
            ) : (
              // Day 3 还没填的 widget — placeholder
              <em style={{ color: '#999' }}>widget {field.widget} not implemented yet</em>
            )}
          </div>
        )
      })}
    </div>
  )
}
```

- [ ] **Step 9: Write SchemaRenderer integration test**

Create `apps/web/src/renderer/SchemaRenderer.test.tsx`:

```typescript
import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { SchemaRenderer } from './SchemaRenderer'
import { qaQualityTemplate } from './__fixtures__/qa_quality'
import type { AnswerValue } from './types'

function Harness() {
  const [answer, setAnswer] = useState<AnswerValue>({})
  return (
    <SchemaRenderer
      template={qaQualityTemplate}
      answer={answer}
      onChange={setAnswer}
      payload={{ prompt: 'demo prompt', model_answer: 'demo answer' }}
    />
  )
}

describe('SchemaRenderer', () => {
  it('renders all 12 qa_quality fields by label', () => {
    render(<Harness />)
    // 12 labels per qa_quality_review.json (spec §10 验收 #2)
    expect(screen.getByText('原始数据')).toBeInTheDocument()
    expect(screen.getByText('相关性')).toBeInTheDocument()
    expect(screen.getByText('准确性')).toBeInTheDocument()
    expect(screen.getByText('格式合规')).toBeInTheDocument()
    expect(screen.getByText('安全性')).toBeInTheDocument()
    expect(screen.getByText('问题类型')).toBeInTheDocument()
    expect(screen.getByText('一句话总评')).toBeInTheDocument()
    expect(screen.getByText('详细评语')).toBeInTheDocument()
    expect(screen.getByText('修订建议')).toBeInTheDocument()
    expect(screen.getByText('修正后答案')).toBeInTheDocument()
    expect(screen.getByText('证据素材')).toBeInTheDocument()
    expect(screen.getByText('AI 预评分')).toBeInTheDocument()
  })

  it('user typing in summary updates answer state', async () => {
    const user = userEvent.setup()
    render(<Harness />)
    const summaryInput = screen.getByLabelText('一句话总评') as HTMLInputElement
    await user.type(summaryInput, 'good response')
    expect(summaryInput.value).toBe('good response')
  })
})
```

- [ ] **Step 10: Run, expect PASS**

Run: `cd apps/web && pnpm test`
Expected: all vitest tests pass.

- [ ] **Step 11: Commit**

```bash
git add apps/web/src/renderer/
git commit -m "feat(s2-d2): 5 simple widgets + SchemaRenderer + 12-field qa_quality test"
```

---

## Day 3 — 4 Complex Widgets + Plaza/Queue Migration

**Goal:** Implement remaining 4 widgets (RichText/JSONEditor/FileUpload/LLMTrigger), migrate Plaza and Queue to SchemaRenderer, kill `LabelAnswer` hardcoded interface (MEDIUM 4 闭环).

**Files this day touches:**
- Create: `apps/web/src/renderer/widgets/{RichText,JSONEditor,FileUpload,LLMTrigger}.tsx`
- Modify: `apps/web/src/renderer/widgets/index.ts` (register 4 more)
- Create: `apps/web/src/renderer/components/SchemaErrorBanner.tsx`
- Modify: `apps/web/src/modules/labeler/Plaza.tsx` (rip out LabelAnswer)
- Modify: `apps/web/src/modules/reviewer/Queue.tsx` (use SchemaRenderer readOnly)
- Modify: `apps/web/src/shared/api/client.ts` (extend Task / Bundle types with schemaJson, revision.answer)

### Task 3.1: 4 complex widgets

**Files:**
- Create: `apps/web/src/renderer/widgets/RichText.tsx`
- Create: `apps/web/src/renderer/widgets/JSONEditor.tsx`
- Create: `apps/web/src/renderer/widgets/FileUpload.tsx`
- Create: `apps/web/src/renderer/widgets/LLMTrigger.tsx`

- [ ] **Step 1: RichText (downgraded to TextArea storing HTML — spec §12)**

Create `apps/web/src/renderer/widgets/RichText.tsx`:

```typescript
import { TextArea } from '@douyinfe/semi-ui'
import type { WidgetProps } from '../types'

// S2 v1 降级:TextArea 存 HTML 字符串(spec §12 / Sprint 1 砍项表 #2)。
// TipTap 真工具栏推到 Sprint 6。
export function RichTextWidget({ field, value, onChange, readOnly }: WidgetProps<string>) {
  return (
    <TextArea
      aria-label={field.label ?? field.name}
      value={value ?? ''}
      onChange={(v) => onChange(v)}
      placeholder="HTML 片段(Sprint 6 接 TipTap)"
      disabled={readOnly}
      rows={4}
    />
  )
}
```

- [ ] **Step 2: JSONEditor (textarea + parse-on-blur)**

Create `apps/web/src/renderer/widgets/JSONEditor.tsx`:

```typescript
import { TextArea, Banner } from '@douyinfe/semi-ui'
import { useState } from 'react'
import type { WidgetProps } from '../types'

// S2 v1 用 TextArea + blur 解析。monaco 接入推 Sprint 6(spec §12)。
export function JSONEditorWidget({ field, value, onChange, readOnly }: WidgetProps<unknown>) {
  const initial = value === undefined ? '' : JSON.stringify(value, null, 2)
  const [text, setText] = useState(initial)
  const [error, setError] = useState<string | null>(null)

  const handleBlur = () => {
    if (!text.trim()) {
      onChange(undefined)
      setError(null)
      return
    }
    try {
      const parsed = JSON.parse(text)
      onChange(parsed)
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'invalid JSON')
    }
  }

  return (
    <div>
      <TextArea
        aria-label={field.label ?? field.name}
        value={text}
        onChange={setText}
        onBlur={handleBlur}
        disabled={readOnly}
        rows={6}
        style={{ fontFamily: 'monospace' }}
      />
      {error ? <Banner type="warning" description={`JSON 解析失败:${error}`} /> : null}
    </div>
  )
}
```

- [ ] **Step 3: FileUpload (uses apiUpload from shared client)**

Create `apps/web/src/renderer/widgets/FileUpload.tsx`:

```typescript
import { Button, Toast } from '@douyinfe/semi-ui'
import { useRef, useState } from 'react'
import { apiUpload } from '../../shared/api/client'
import type { WidgetProps } from '../types'

interface UploadedRef {
  id: number
  originalName: string
}

// Bundle parent should pass taskId via field-specific context.
// S2 v1:简化为直接读 URL — Plaza/Queue 知道 taskId,通过 component-level prop 注入。
// 但 widgetRegistry 接口不能给 widget 加额外 prop。所以这里从 URL 解析 taskId(/owner/tasks/:id/...)。
function currentTaskId(): number {
  const match = window.location.pathname.match(/\/tasks\/(\d+)/)
  return match ? parseInt(match[1], 10) : 0
}

export function FileUploadWidget({ field, value, onChange, readOnly }: WidgetProps<UploadedRef[]>) {
  const fileInput = useRef<HTMLInputElement>(null)
  const [busy, setBusy] = useState(false)
  const files = value ?? []
  const maxFiles = field.maxFiles ?? 5

  const handleUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    if (files.length >= maxFiles) {
      Toast.warning(`最多 ${maxFiles} 个文件`)
      return
    }
    setBusy(true)
    try {
      const form = new FormData()
      form.append('file', file)
      form.append('task_id', String(currentTaskId()))
      const uploaded = await apiUpload<{ id: number; originalName: string }>('/upload', form)
      onChange([...files, { id: uploaded.id, originalName: uploaded.originalName }])
    } catch (err) {
      Toast.error(err instanceof Error ? err.message : '上传失败')
    } finally {
      setBusy(false)
      if (fileInput.current) fileInput.current.value = ''
    }
  }

  const handleRemove = (id: number) => {
    onChange(files.filter((f) => f.id !== id))
  }

  return (
    <div aria-label={field.label ?? field.name}>
      <input
        ref={fileInput}
        type="file"
        onChange={handleUpload}
        disabled={readOnly || busy}
        style={{ marginBottom: 8 }}
      />
      <ul style={{ paddingLeft: 16 }}>
        {files.map((f) => (
          <li key={f.id}>
            {f.originalName} (#{f.id})
            {!readOnly ? (
              <Button size="small" onClick={() => handleRemove(f.id)} style={{ marginLeft: 8 }}>移除</Button>
            ) : null}
          </li>
        ))}
      </ul>
    </div>
  )
}
```

- [ ] **Step 4: LLMTrigger (S2 v1 不自动写回 target,Sprint 3 接 spec §14 风险表)**

Create `apps/web/src/renderer/widgets/LLMTrigger.tsx`:

```typescript
import { Button, Banner } from '@douyinfe/semi-ui'
import { useState } from 'react'
import { apiPost } from '../../shared/api/client'
import type { WidgetProps } from '../types'

interface LLMResult {
  text: string
  provider: string
}

export function LLMTriggerWidget({ field, value, onChange, readOnly }: WidgetProps<string>) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const trigger = async () => {
    setBusy(true)
    setError(null)
    try {
      const result = await apiPost<LLMResult>('/llm/inline', {
        prompt: field.prompt ?? '',
        input: {},
      })
      // S2 v1 把返回写自己的字段(targetField 推 S3);通过 onChange 持久化
      onChange(result.text)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'LLM 调用失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div aria-label={field.label ?? field.name}>
      <Button onClick={trigger} loading={busy} disabled={readOnly}>触发 AI 预评分</Button>
      {value ? (
        <pre style={{ marginTop: 8, padding: 8, background: '#f5f5f5', whiteSpace: 'pre-wrap' }}>
          {value}
        </pre>
      ) : null}
      {error ? <Banner type="danger" description={error} /> : null}
    </div>
  )
}
```

- [ ] **Step 5: Register all 4 in widget registry**

Modify `apps/web/src/renderer/widgets/index.ts`:

```typescript
import type { WidgetType } from '../types'
import { InputWidget } from './Input'
import { TextAreaWidget } from './TextArea'
import { RadioWidget } from './Radio'
import { TagsWidget } from './Tags'
import { ShowItemWidget } from './ShowItem'
import { RichTextWidget } from './RichText'
import { JSONEditorWidget } from './JSONEditor'
import { FileUploadWidget } from './FileUpload'
import { LLMTriggerWidget } from './LLMTrigger'

export const widgetRegistry: Record<WidgetType, React.ComponentType<any>> = {
  Input: InputWidget,
  TextArea: TextAreaWidget,
  Radio: RadioWidget,
  Tags: TagsWidget,
  ShowItem: ShowItemWidget,
  RichText: RichTextWidget,
  JSONEditor: JSONEditorWidget,
  FileUpload: FileUploadWidget,
  LLMTrigger: LLMTriggerWidget,
}
```

- [ ] **Step 6: Re-run SchemaRenderer test — the placeholder fallback should now never trigger**

Run: `cd apps/web && pnpm test src/renderer/SchemaRenderer.test.tsx`
Expected: all 12 field labels render real widgets.

- [ ] **Step 7: Commit**

```bash
git add apps/web/src/renderer/widgets/ apps/web/src/renderer/SchemaRenderer.test.tsx
git commit -m "feat(s2-d3): 4 complex widgets (RichText/JSONEditor/FileUpload/LLMTrigger)"
```

---

### Task 3.2: SchemaErrorBanner

**Files:**
- Create: `apps/web/src/renderer/components/SchemaErrorBanner.tsx`

- [ ] **Step 1: Implement banner with role-aware text (Finding 8)**

Create `apps/web/src/renderer/components/SchemaErrorBanner.tsx`:

```typescript
import { Banner } from '@douyinfe/semi-ui'

export interface SchemaErrorBannerProps {
  error: string
  role: 'owner' | 'admin' | 'labeler' | 'reviewer'
}

export function SchemaErrorBanner({ error, role }: SchemaErrorBannerProps) {
  const isOwnerSide = role === 'owner' || role === 'admin'
  return (
    <Banner
      type="danger"
      description={
        isOwnerSide
          ? `模板 schema 解析失败:${error}`
          : '当前任务的模板配置有误,请联系任务管理员重新发布'
      }
    />
  )
}
```

- [ ] **Step 2: Commit (small file, no test needed yet — banner is dumb)**

```bash
git add apps/web/src/renderer/components/SchemaErrorBanner.tsx
git commit -m "feat(s2-d3): SchemaErrorBanner with role-aware copy"
```

---

### Task 3.3: Plaza migration — kill LabelAnswer

**Files:**
- Modify: `apps/web/src/modules/labeler/Plaza.tsx`
- Modify: `apps/web/src/shared/api/client.ts` (extend types)

- [ ] **Step 1: Extend TaskBundle type to include parsed-ready schemaJson + revision.answer**

Inspect `apps/web/src/shared/api/client.ts` for current `TaskBundle` shape and add fields:

```typescript
export interface Task {
  id: number
  title: string
  description: string | null
  baselineDescription: string | null
  status: string
  totalItems: number
  finishedItems: number
  templateId?: number | null
}

export interface TaskTemplate {
  id: number
  taskId: number
  version: number
  schemaJson: string  // RAW string from backend; frontend parses
  schemaHash?: string
}

export interface SubmissionRevision {
  id: number
  submissionId: number
  revisionNo: number
  answer: string  // RAW JSON string
  draft: boolean
}

export interface TaskBundle {
  task: Task
  item: { id: number; payload: string /* JSON string */; status: string; claimedBy: number | null }
  template?: TaskTemplate | null
  submission?: { id: number; status: string } | null
  revision?: SubmissionRevision | null
  warnings?: string[]
}
```

If types already exist, only add `schemaJson` and `answer` as RAW strings.

- [ ] **Step 2: Read current Plaza shape to know what to delete**

Run: `head -50 apps/web/src/modules/labeler/Plaza.tsx`

Identify `LabelAnswer` interface and `emptyAnswer` constant — they will be deleted.

- [ ] **Step 3: Rewrite Plaza.tsx body (preserving login redirect, task loading)**

Open `apps/web/src/modules/labeler/Plaza.tsx` and replace the form rendering section. Key shape:

```typescript
import { useCallback, useEffect, useState } from 'react'
import { Button, Toast } from '@douyinfe/semi-ui'
import { apiGet, apiPost, type Task, type TaskBundle } from '../../shared/api/client'
import { SchemaRenderer } from '../../renderer/SchemaRenderer'
import { parseTemplateSchema, parseAnswer } from '../../renderer/parser'
import { validateAnswer } from '../../renderer/validator'
import { SchemaErrorBanner } from '../../renderer/components/SchemaErrorBanner'
import type { AnswerValue, TemplateSchema } from '../../renderer/types'

export default function LabelerPlaza() {
  const [tasks, setTasks] = useState<Task[]>([])
  const [bundle, setBundle] = useState<TaskBundle | null>(null)
  const [template, setTemplate] = useState<TemplateSchema | null>(null)
  const [schemaError, setSchemaError] = useState<string | null>(null)
  const [answer, setAnswer] = useState<AnswerValue>({})
  const [submitting, setSubmitting] = useState(false)

  const loadTasks = useCallback(async () => {
    try {
      const data = await apiGet<Task[]>('/labeler/tasks')
      setTasks(data)
    } catch (err) {
      Toast.error(err instanceof Error ? err.message : '加载任务失败')
    }
  }, [])

  useEffect(() => { void loadTasks() }, [loadTasks])

  // 当 bundle 变化时,parse template + 历史 answer
  useEffect(() => {
    if (!bundle?.template?.schemaJson) {
      setTemplate(null)
      setSchemaError(bundle ? '当前任务未配置模板' : null)
      setAnswer({})
      return
    }
    const parsed = parseTemplateSchema(bundle.template.schemaJson)
    if (!parsed.ok) {
      setTemplate(null)
      setSchemaError(parsed.error)
      return
    }
    setTemplate(parsed.value)
    setSchemaError(null)
    // 历史 answer:如果有 revision,载入它
    if (bundle.revision?.answer) {
      const parsedAnswer = parseAnswer(bundle.revision.answer)
      setAnswer(parsedAnswer.ok ? parsedAnswer.value : {})
    } else {
      setAnswer({})
    }
  }, [bundle])

  const claim = useCallback(async (taskId: number) => {
    try {
      const data = await apiGet<TaskBundle>(`/tasks/${taskId}/claim`)
      setBundle(data)
    } catch (err) {
      Toast.error(err instanceof Error ? err.message : '领题失败')
    }
  }, [])

  const submit = useCallback(async () => {
    if (!bundle || !template) return
    const validation = validateAnswer(template, answer)
    if (!validation.valid) {
      Toast.error('请检查必填和长度:' + Object.entries(validation.errors).map(([k, v]) => `${k}: ${v}`).join('; '))
      return
    }
    setSubmitting(true)
    try {
      await apiPost(`/tasks/${bundle.task.id}/items/${bundle.item.id}/submit`, { answer })
      Toast.success('已提交')
      setBundle(null)
      setAnswer({})
    } catch (err) {
      Toast.error(err instanceof Error ? err.message : '提交失败')
    } finally {
      setSubmitting(false)
    }
  }, [bundle, template, answer])

  // payload for ShowItem
  const itemPayload: Record<string, unknown> = (() => {
    if (!bundle?.item?.payload) return {}
    try { return JSON.parse(bundle.item.payload) } catch { return {} }
  })()

  return (
    <div style={{ padding: 24 }}>
      <h1>Labeler 广场</h1>
      <section style={{ marginBottom: 24 }}>
        {tasks.map(t => (
          <Button key={t.id} onClick={() => claim(t.id)} style={{ marginRight: 8 }}>
            {t.title} ({t.finishedItems}/{t.totalItems})
          </Button>
        ))}
      </section>

      {schemaError ? <SchemaErrorBanner error={schemaError} role="labeler" /> : null}

      {bundle && template ? (
        <section>
          <SchemaRenderer
            template={template}
            answer={answer}
            onChange={setAnswer}
            payload={itemPayload}
          />
          <Button theme="solid" type="primary" onClick={submit} loading={submitting} style={{ marginTop: 16 }}>
            提交
          </Button>
        </section>
      ) : null}
    </div>
  )
}
```

- [ ] **Step 4: Type check + lint**

Run:
```bash
cd apps/web && pnpm tsc --noEmit && pnpm lint
```

Expected: 0 errors. If TaskBundle shape mismatch causes errors, adjust types in `client.ts` to match what the backend returns (`bundle.item.payload` is JSON string; `bundle.template.schemaJson` is string).

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/modules/labeler/Plaza.tsx apps/web/src/shared/api/client.ts
git commit -m "feat(s2-d3): Plaza migrates to SchemaRenderer, LabelAnswer interface deleted"
```

---

### Task 3.4: Queue (reviewer) migration

**Files:**
- Modify: `apps/web/src/modules/reviewer/Queue.tsx`

- [ ] **Step 1: Inspect current Queue shape**

Run: `head -60 apps/web/src/modules/reviewer/Queue.tsx`

Identify how it currently displays the submission/answer.

- [ ] **Step 2: Migrate to readOnly SchemaRenderer**

Replace Queue's submission-detail rendering with:

```typescript
import { useEffect, useState, useCallback } from 'react'
import { Button, Toast } from '@douyinfe/semi-ui'
import { apiGet, apiPost, type TaskBundle } from '../../shared/api/client'
import { SchemaRenderer } from '../../renderer/SchemaRenderer'
import { parseTemplateSchema, parseAnswer } from '../../renderer/parser'
import { SchemaErrorBanner } from '../../renderer/components/SchemaErrorBanner'
import type { AnswerValue, TemplateSchema } from '../../renderer/types'

interface QueueItem {
  id: number
  taskId: number
  itemId: number
  status: string
}

export default function ReviewerQueue() {
  const [queue, setQueue] = useState<QueueItem[]>([])
  const [detail, setDetail] = useState<TaskBundle | null>(null)
  const [template, setTemplate] = useState<TemplateSchema | null>(null)
  const [schemaError, setSchemaError] = useState<string | null>(null)
  const [answer, setAnswer] = useState<AnswerValue>({})
  const [reason, setReason] = useState('')

  const loadQueue = useCallback(async () => {
    try {
      const data = await apiGet<QueueItem[]>('/reviewer/submissions')
      setQueue(data)
    } catch (err) {
      Toast.error(err instanceof Error ? err.message : '加载队列失败')
    }
  }, [])

  useEffect(() => { void loadQueue() }, [loadQueue])

  // detail 变化时 parse
  useEffect(() => {
    if (!detail?.template?.schemaJson) {
      setTemplate(null)
      setSchemaError(detail ? '提交关联的模板缺失' : null)
      return
    }
    const parsed = parseTemplateSchema(detail.template.schemaJson)
    if (!parsed.ok) {
      setTemplate(null)
      setSchemaError(parsed.error)
      return
    }
    setTemplate(parsed.value)
    setSchemaError(null)
    if (detail.revision?.answer) {
      const parsedAnswer = parseAnswer(detail.revision.answer)
      setAnswer(parsedAnswer.ok ? parsedAnswer.value : {})
    } else {
      setAnswer({})
    }
  }, [detail])

  const loadDetail = useCallback(async (taskId: number, itemId: number) => {
    try {
      const data = await apiGet<TaskBundle>(`/tasks/${taskId}/items/${itemId}`)
      setDetail(data)
    } catch (err) {
      Toast.error(err instanceof Error ? err.message : '加载详情失败')
    }
  }, [])

  const review = useCallback(async (verdict: 'approve' | 'reject' | 'revise') => {
    if (!detail?.submission) return
    if ((verdict === 'reject' || verdict === 'revise') && reason.trim().length < 5) {
      Toast.error('reject/revise 必填详细理由(≥5 字符)')
      return
    }
    try {
      await apiPost(`/submissions/${detail.submission.id}/review`, { verdict, reason })
      Toast.success(`已 ${verdict}`)
      setDetail(null)
      setReason('')
      void loadQueue()
    } catch (err) {
      Toast.error(err instanceof Error ? err.message : '审核失败')
    }
  }, [detail, reason, loadQueue])

  const itemPayload: Record<string, unknown> = (() => {
    if (!detail?.item?.payload) return {}
    try { return JSON.parse(detail.item.payload) } catch { return {} }
  })()

  return (
    <div style={{ padding: 24 }}>
      <h1>Reviewer 队列</h1>
      <section style={{ marginBottom: 24 }}>
        {queue.map(q => (
          <Button key={q.id} onClick={() => loadDetail(q.taskId, q.itemId)} style={{ marginRight: 8 }}>
            #{q.id} task {q.taskId} item {q.itemId}
          </Button>
        ))}
      </section>

      {schemaError ? <SchemaErrorBanner error={schemaError} role="reviewer" /> : null}

      {detail && template ? (
        <section>
          <SchemaRenderer
            template={template}
            answer={answer}
            onChange={() => { /* readonly */ }}
            readOnly
            payload={itemPayload}
          />
          <div style={{ marginTop: 16 }}>
            <textarea
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder="reject/revise 时的理由(≥5 字符)"
              style={{ width: '100%', minHeight: 60 }}
            />
            <div style={{ marginTop: 8 }}>
              <Button type="primary" onClick={() => review('approve')} style={{ marginRight: 8 }}>Approve</Button>
              <Button type="danger" onClick={() => review('reject')} style={{ marginRight: 8 }}>Reject</Button>
              <Button onClick={() => review('revise')}>Revise</Button>
            </div>
          </div>
        </section>
      ) : null}
    </div>
  )
}
```

- [ ] **Step 3: Verify build + lint**

Run:
```bash
cd apps/web && pnpm tsc --noEmit && pnpm lint && pnpm build
```

Expected: 0 errors. Build passes with the documented lottie + chunk warnings.

- [ ] **Step 4: Manual smoke (API + dev server)**

Tell the user: "请你在终端跑 `cd ~/Desktop/LabelHub && export API_PORT=18181 && cd apps/api && go run cmd/server/main.go` + 另一个终端 `pnpm --dir apps/web dev`,跑起来告诉我。" (Per user's CLAUDE.md rule: don't spawn dev servers myself.)

Then via curl/manual: labeler1 → claim → submit → reviewer1 sees + approves. Spot-check 12 fields all render.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/modules/reviewer/Queue.tsx
git commit -m "feat(s2-d3): Queue migrates to readOnly SchemaRenderer with parsed answer"
```

---

## Day 4 — Designer edit-mode core

**Goal:** Designer page can render `qa_quality` template, append fields via material palette, delete fields, select fields, with selection surviving rename — all via local draft state.

**Files this day touches:**
- Create: `apps/web/src/modules/template/Designer.tsx`
- Create: `apps/web/src/modules/template/components/{MaterialPalette,DraftCanvas,CanvasFieldShell,WidgetPreview,VersionBadge}.tsx`
- Create: `apps/web/src/modules/template/hooks/useDesignerState.ts` + `.test.ts`
- Create: `apps/web/src/modules/template/utils/nextFieldName.ts` + `.test.ts`
- Modify: `apps/web/src/App.tsx` (add route)

### Task 4.1: nextFieldName utility (TDD)

**Files:**
- Create: `apps/web/src/modules/template/utils/nextFieldName.ts` + `.test.ts`

- [ ] **Step 1: Write tests**

Create `apps/web/src/modules/template/utils/nextFieldName.test.ts`:

```typescript
import { describe, it, expect } from 'vitest'
import { nextFieldName } from './nextFieldName'
import type { DraftField } from '../types'

const mk = (name: string, widget: string): DraftField => ({
  _draftId: 'x', name, widget: widget as DraftField['widget'],
})

describe('nextFieldName', () => {
  it('returns input_1 when none exist', () => {
    expect(nextFieldName('Input', [])).toBe('input_1')
  })
  it('fills gap after delete (spec §3.4 critical case)', () => {
    const existing = [mk('input_1', 'Input'), mk('input_3', 'Input')]
    expect(nextFieldName('Input', existing)).toBe('input_2')
  })
  it('walks past existing dense block', () => {
    const existing = [mk('input_1', 'Input'), mk('input_2', 'Input'), mk('input_3', 'Input')]
    expect(nextFieldName('Input', existing)).toBe('input_4')
  })
  it('handles JSONEditor as json_editor (spec §3.4)', () => {
    expect(nextFieldName('JSONEditor', [])).toBe('json_editor_1')
  })
  it('handles ShowItem as show_item', () => {
    expect(nextFieldName('ShowItem', [])).toBe('show_item_1')
  })
  it('handles LLMTrigger as llm_trigger', () => {
    expect(nextFieldName('LLMTrigger', [])).toBe('llm_trigger_1')
  })
  it('ignores fields of different widget when counting', () => {
    const existing = [mk('input_1', 'Input'), mk('radio_1', 'Radio')]
    expect(nextFieldName('Input', existing)).toBe('input_2')
    expect(nextFieldName('Radio', existing)).toBe('radio_2')
  })
})
```

- [ ] **Step 2: Run, expect FAIL**

Run: `cd apps/web && pnpm test src/modules/template/utils/nextFieldName.test.ts`

- [ ] **Step 3: Implement (spec §3.4 widgetPrefixMap pinned)**

Create `apps/web/src/modules/template/utils/nextFieldName.ts`:

```typescript
import type { WidgetType } from '../../../renderer/types'
import type { DraftField } from '../types'

// spec §3.4:9 widget 前缀写死,避免 camelToSnake 处理连续大写不一致
const widgetPrefixMap: Record<WidgetType, string> = {
  ShowItem:    'show_item',
  Input:       'input',
  TextArea:    'text_area',
  Radio:       'radio',
  Tags:        'tags',
  RichText:    'rich_text',
  JSONEditor:  'json_editor',
  FileUpload:  'file_upload',
  LLMTrigger:  'llm_trigger',
}

export function nextFieldName(widget: WidgetType, existing: DraftField[]): string {
  const prefix = widgetPrefixMap[widget]
  let i = 1
  while (existing.some((f) => f.name === `${prefix}_${i}`)) i++
  return `${prefix}_${i}`
}

export { widgetPrefixMap }
```

Create `apps/web/src/modules/template/types.ts` for `DraftField`:

```typescript
import type { FieldSchema } from '../../renderer/types'

export interface DraftField extends FieldSchema {
  _draftId: string
}
```

- [ ] **Step 4: Run, expect 7 PASS**

Run: `cd apps/web && pnpm test src/modules/template/utils/nextFieldName.test.ts`
Expected: all 7 sub-tests PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/modules/template/utils/ apps/web/src/modules/template/types.ts
git commit -m "feat(s2-d4): nextFieldName + widgetPrefixMap + DraftField type"
```

---

### Task 4.2: useDesignerState hook (TDD)

**Files:**
- Create: `apps/web/src/modules/template/hooks/useDesignerState.ts` + `.test.ts`

- [ ] **Step 1: Write tests for append/remove/update/discard + rename-doesnt-break-selection**

Create `apps/web/src/modules/template/hooks/useDesignerState.test.ts`:

```typescript
import { describe, it, expect } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useDesignerState } from './useDesignerState'
import type { DraftField } from '../types'

const seed: DraftField[] = [
  { _draftId: 'd1', name: 'a', widget: 'Input', label: 'A' },
]

// fixed id factory for test stability
function fixedIds() {
  let n = 100
  return () => `id-${n++}`
}

describe('useDesignerState', () => {
  it('append adds with deterministic name', () => {
    const { result } = renderHook(() => useDesignerState(seed, { idFactory: fixedIds() }))
    act(() => result.current.append('Input'))
    expect(result.current.draft).toHaveLength(2)
    expect(result.current.draft[1].name).toBe('input_1')  // 'a' was Input but not 'input_X', so input_1 is free
    expect(result.current.draft[1]._draftId).toBe('id-100')
  })

  it('append→remove→append fills gap (spec §3.4)', () => {
    const { result } = renderHook(() => useDesignerState([], { idFactory: fixedIds() }))
    act(() => result.current.append('Input'))   // input_1
    act(() => result.current.append('Input'))   // input_2
    act(() => result.current.append('Input'))   // input_3
    act(() => result.current.remove('id-101'))  // remove input_2
    act(() => result.current.append('Input'))   // should be input_2 (filled gap)
    const names = result.current.draft.map(f => f.name)
    expect(names).toEqual(['input_1', 'input_3', 'input_2'])
  })

  it('rename does not change _draftId (selection survives)', () => {
    const { result } = renderHook(() => useDesignerState(seed, { idFactory: fixedIds() }))
    act(() => result.current.select('d1'))
    expect(result.current.selectedId).toBe('d1')
    act(() => result.current.update('d1', { name: 'renamed' }))
    expect(result.current.selectedId).toBe('d1')
    expect(result.current.draft[0].name).toBe('renamed')
    expect(result.current.draft[0]._draftId).toBe('d1')
  })

  it('discard reverts to initial', () => {
    const { result } = renderHook(() => useDesignerState(seed, { idFactory: fixedIds() }))
    act(() => result.current.append('Radio'))
    expect(result.current.draft).toHaveLength(2)
    act(() => result.current.discard())
    expect(result.current.draft).toHaveLength(1)
    expect(result.current.draft[0]._draftId).toBe('d1')
  })

  it('stripDraftIds removes _draftId for Save body', () => {
    const { result } = renderHook(() => useDesignerState(seed, { idFactory: fixedIds() }))
    act(() => result.current.append('Input'))
    const stripped = result.current.stripDraftIds()
    expect(stripped).toHaveLength(2)
    stripped.forEach(f => expect(f).not.toHaveProperty('_draftId'))
  })
})
```

- [ ] **Step 2: Run, expect FAIL**

Run: `cd apps/web && pnpm test src/modules/template/hooks/useDesignerState.test.ts`

- [ ] **Step 3: Implement hook**

Create `apps/web/src/modules/template/hooks/useDesignerState.ts`:

```typescript
import { useCallback, useMemo, useRef, useState } from 'react'
import type { WidgetType, FieldSchema } from '../../../renderer/types'
import type { DraftField } from '../types'
import { nextFieldName } from '../utils/nextFieldName'

interface UseDesignerStateOptions {
  idFactory?: () => string
}

export function useDesignerState(initial: DraftField[], options: UseDesignerStateOptions = {}) {
  const idFactory = useRef(options.idFactory ?? (() => crypto.randomUUID()))
  const initialSnapshot = useRef(initial)
  const [draft, setDraft] = useState<DraftField[]>(initial)
  const [selectedId, setSelectedId] = useState<string | null>(null)

  const append = useCallback((widget: WidgetType) => {
    setDraft((d) => {
      const name = nextFieldName(widget, d)
      const newField: DraftField = {
        _draftId: idFactory.current(),
        name,
        widget,
        label: widgetDefaultLabel(widget, name),
        required: false,
      }
      return [...d, newField]
    })
  }, [])

  const remove = useCallback((id: string) => {
    setDraft((d) => d.filter((f) => f._draftId !== id))
    setSelectedId((s) => (s === id ? null : s))
  }, [])

  const update = useCallback((id: string, patch: Partial<FieldSchema>) => {
    setDraft((d) => d.map((f) => (f._draftId === id ? { ...f, ...patch } : f)))
  }, [])

  const discard = useCallback(() => {
    setDraft(initialSnapshot.current)
    setSelectedId(null)
  }, [])

  const stripDraftIds = useCallback((): FieldSchema[] => {
    return draft.map(({ _draftId, ...rest }) => rest)
  }, [draft])

  const selectedField = useMemo(
    () => draft.find((f) => f._draftId === selectedId) ?? null,
    [draft, selectedId],
  )

  return {
    draft, selectedId, selectedField,
    select: setSelectedId,
    append, remove, update, discard, stripDraftIds,
  }
}

function widgetDefaultLabel(widget: WidgetType, name: string): string {
  return `${widget} (${name})`
}
```

- [ ] **Step 4: Run, expect PASS**

Run: `cd apps/web && pnpm test src/modules/template/hooks/useDesignerState.test.ts`

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/modules/template/hooks/
git commit -m "feat(s2-d4): useDesignerState hook with _draftId stability"
```

---

### Task 4.3: Designer + Canvas + Material Palette (no PropertyPanel yet)

**Files:**
- Create: `apps/web/src/modules/template/components/MaterialPalette.tsx`
- Create: `apps/web/src/modules/template/components/CanvasFieldShell.tsx`
- Create: `apps/web/src/modules/template/components/DraftCanvas.tsx`
- Create: `apps/web/src/modules/template/components/WidgetPreview.tsx`
- Create: `apps/web/src/modules/template/components/VersionBadge.tsx`
- Create: `apps/web/src/modules/template/Designer.tsx`
- Modify: `apps/web/src/shared/api/client.ts` (Template GET response type)
- Modify: `apps/web/src/App.tsx` (route)

- [ ] **Step 1: WidgetPreview (read-only widget render, no value/onChange)**

Create `apps/web/src/modules/template/components/WidgetPreview.tsx`:

```typescript
import { widgetRegistry } from '../../../renderer/widgets'
import type { FieldSchema } from '../../../renderer/types'

// Designer 中栏不能把 SchemaRenderer 复用进来,因为 SchemaRenderer 要求 onChange + answer state(spec §3.2)
// 这里只调 widget registry,readOnly 视图,无 value 流转
export function WidgetPreview({ field }: { field: FieldSchema }) {
  const Widget = widgetRegistry[field.widget]
  if (!Widget) return <em style={{ color: '#999' }}>unknown widget {field.widget}</em>
  // ShowItem 需要 payload — Designer 用空 payload(预览不展示真实数据)
  if (field.widget === 'ShowItem') {
    return <Widget field={field} payload={{}} />
  }
  return <Widget field={field} value={undefined} onChange={() => {}} readOnly />
}
```

- [ ] **Step 2: CanvasFieldShell**

Create `apps/web/src/modules/template/components/CanvasFieldShell.tsx`:

```typescript
import { Button } from '@douyinfe/semi-ui'
import type { ReactNode } from 'react'

interface Props {
  selected: boolean
  disabled?: boolean
  onSelect: () => void
  onDelete: () => void
  children: ReactNode
  fieldLabel: string
}

export function CanvasFieldShell({ selected, disabled, onSelect, onDelete, children, fieldLabel }: Props) {
  return (
    <div
      role="group"
      aria-label={fieldLabel}
      onClick={onSelect}
      style={{
        position: 'relative',
        padding: 12,
        marginBottom: 12,
        border: selected ? '2px solid #1677ff' : '1px solid #e0e0e0',
        borderRadius: 4,
        background: '#fff',
        cursor: disabled ? 'default' : 'pointer',
      }}
    >
      {!disabled ? (
        <Button
          size="small"
          type="danger"
          onClick={(e) => { e.stopPropagation(); onDelete() }}
          style={{ position: 'absolute', top: 8, right: 8 }}
          aria-label={`delete ${fieldLabel}`}
        >
          删除
        </Button>
      ) : null}
      {children}
    </div>
  )
}
```

- [ ] **Step 3: DraftCanvas (the middle column)**

Create `apps/web/src/modules/template/components/DraftCanvas.tsx`:

```typescript
import type { DraftField } from '../types'
import { CanvasFieldShell } from './CanvasFieldShell'
import { WidgetPreview } from './WidgetPreview'

interface Props {
  fields: DraftField[]
  selectedId: string | null
  mode: 'edit' | 'readonly'
  onSelect: (id: string) => void
  onDelete: (id: string) => void
}

export function DraftCanvas({ fields, selectedId, mode, onSelect, onDelete }: Props) {
  if (fields.length === 0) {
    return (
      <div style={{ padding: 48, textAlign: 'center', color: '#999' }}>
        画布为空 — 从左侧物料库添加字段
      </div>
    )
  }
  return (
    <div>
      {fields.map((f) => (
        <CanvasFieldShell
          key={f._draftId}
          selected={f._draftId === selectedId}
          disabled={mode === 'readonly'}
          onSelect={() => onSelect(f._draftId)}
          onDelete={() => onDelete(f._draftId)}
          fieldLabel={f.label ?? f.name}
        >
          <div style={{ fontSize: 12, color: '#999' }}>{f.widget} · {f.name}</div>
          <div style={{ fontWeight: 'bold', marginBottom: 4 }}>{f.label ?? f.name}{f.required ? ' *' : ''}</div>
          <WidgetPreview field={f} />
        </CanvasFieldShell>
      ))}
    </div>
  )
}
```

- [ ] **Step 4: MaterialPalette (left column)**

Create `apps/web/src/modules/template/components/MaterialPalette.tsx`:

```typescript
import type { WidgetType } from '../../../renderer/types'

const ALL_WIDGETS: Array<{ type: WidgetType; description: string }> = [
  { type: 'ShowItem',   description: '展示原始数据(prompt / 答案 / 图片 / 视频 / Markdown / JSON)' },
  { type: 'Input',      description: '单行输入' },
  { type: 'TextArea',   description: '多行输入' },
  { type: 'Radio',      description: '单选' },
  { type: 'Tags',       description: '多选标签' },
  { type: 'RichText',   description: '富文本(S2 v1 降级 TextArea)' },
  { type: 'JSONEditor', description: 'JSON 编辑器' },
  { type: 'FileUpload', description: '文件上传' },
  { type: 'LLMTrigger', description: 'AI 预评分按钮' },
]

interface Props {
  disabled: boolean
  onAdd: (widget: WidgetType) => void
}

export function MaterialPalette({ disabled, onAdd }: Props) {
  return (
    <div style={{ padding: 12 }}>
      <h3 style={{ marginTop: 0 }}>物料库</h3>
      {ALL_WIDGETS.map((w) => (
        <button
          key={w.type}
          onClick={() => onAdd(w.type)}
          disabled={disabled}
          aria-label={`add ${w.type}`}
          style={{
            display: 'block', width: '100%', textAlign: 'left',
            padding: 8, marginBottom: 8,
            border: '1px solid #e0e0e0', borderRadius: 4,
            background: disabled ? '#f5f5f5' : '#fff',
            cursor: disabled ? 'not-allowed' : 'pointer',
          }}
        >
          <div style={{ fontWeight: 'bold' }}>{w.type}</div>
          <div style={{ fontSize: 12, color: '#666' }}>{w.description}</div>
        </button>
      ))}
    </div>
  )
}
```

- [ ] **Step 5: VersionBadge**

Create `apps/web/src/modules/template/components/VersionBadge.tsx`:

```typescript
import { Tag } from '@douyinfe/semi-ui'

export function VersionBadge({ version, isLatest }: { version: number; isLatest: boolean }) {
  return (
    <span>
      <Tag color={isLatest ? 'green' : 'grey'}>v{version}</Tag>
      {isLatest ? <Tag color="blue" style={{ marginLeft: 4 }}>latest</Tag> : <Tag color="grey" style={{ marginLeft: 4 }}>readonly</Tag>}
    </span>
  )
}
```

- [ ] **Step 6: Extend api client with template types**

Modify `apps/web/src/shared/api/client.ts` to add (or update if already there):

```typescript
export interface TemplateDetailResponse {
  template: TaskTemplate
  isLatest: boolean
  latestTemplateId: number
}
```

- [ ] **Step 7: Designer page (NO PropertyPanel yet — that's Day 5)**

Create `apps/web/src/modules/template/Designer.tsx`:

```typescript
import { useEffect, useState, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Button, Toast } from '@douyinfe/semi-ui'
import { apiGet, apiPost, type TemplateDetailResponse } from '../../shared/api/client'
import { parseTemplateSchema } from '../../renderer/parser'
import { SchemaErrorBanner } from '../../renderer/components/SchemaErrorBanner'
import { useDesignerState } from './hooks/useDesignerState'
import { MaterialPalette } from './components/MaterialPalette'
import { DraftCanvas } from './components/DraftCanvas'
import { VersionBadge } from './components/VersionBadge'
import type { DraftField } from './types'

export default function TemplateDesigner() {
  const { taskId = '', templateId = '' } = useParams<{ taskId: string; templateId: string }>()
  const navigate = useNavigate()
  const [detail, setDetail] = useState<TemplateDetailResponse | null>(null)
  const [initialDraft, setInitialDraft] = useState<DraftField[] | null>(null)
  const [schemaError, setSchemaError] = useState<string | null>(null)
  const state = useDesignerState(initialDraft ?? [])

  useEffect(() => {
    void (async () => {
      try {
        const data = await apiGet<TemplateDetailResponse>(`/templates/${templateId}`)
        setDetail(data)
        const parsed = parseTemplateSchema(data.template.schemaJson)
        if (!parsed.ok) {
          setSchemaError(parsed.error)
          return
        }
        // 给 fields 加 _draftId
        const drafts: DraftField[] = parsed.value.fields.map((f) => ({ ...f, _draftId: crypto.randomUUID() }))
        setInitialDraft(drafts)
        setSchemaError(null)
      } catch (err) {
        Toast.error(err instanceof Error ? err.message : '加载模板失败')
      }
    })()
  }, [templateId])

  const mode = detail?.isLatest ? 'edit' : 'readonly'
  const isReadonly = mode === 'readonly'

  const handleSave = useCallback(async () => {
    if (state.draft.length === 0) {
      Toast.error('画布不能为空')
      return
    }
    try {
      const res = await apiPost<{ template: { id: number } }>(`/tasks/${taskId}/templates`, {
        fields: state.stripDraftIds(),
      })
      Toast.success('已保存为新版本')
      navigate(`/owner/tasks/${taskId}/templates/${res.template.id}`)
    } catch (err) {
      Toast.error(err instanceof Error ? err.message : '保存失败')
    }
  }, [state, taskId, navigate])

  if (!detail || !initialDraft) {
    return <div style={{ padding: 24 }}>加载中...</div>
  }
  if (schemaError) {
    return <div style={{ padding: 24 }}><SchemaErrorBanner error={schemaError} role="owner" /></div>
  }

  return (
    <div style={{ display: 'flex', height: 'calc(100vh - 64px)' }}>
      <aside style={{ width: 280, borderRight: '1px solid #e0e0e0', overflowY: 'auto' }}>
        <MaterialPalette disabled={isReadonly} onAdd={state.append} />
      </aside>
      <main style={{ flex: 1, padding: 24, overflowY: 'auto', background: '#f5f5f5' }}>
        <header style={{ marginBottom: 24, display: 'flex', alignItems: 'center', gap: 16 }}>
          <h2 style={{ margin: 0 }}>模板设计器</h2>
          <VersionBadge version={detail.template.version} isLatest={detail.isLatest} />
          <div style={{ marginLeft: 'auto' }}>
            <Button onClick={state.discard} style={{ marginRight: 8 }} disabled={isReadonly}>Discard</Button>
            <Button type="primary" onClick={handleSave}>
              {isReadonly ? 'Fork as new version' : 'Save as new version'}
            </Button>
          </div>
        </header>
        <DraftCanvas
          fields={state.draft}
          selectedId={state.selectedId}
          mode={mode}
          onSelect={state.select}
          onDelete={state.remove}
        />
      </main>
      <aside style={{ width: 360, borderLeft: '1px solid #e0e0e0', padding: 12, overflowY: 'auto', background: '#fafafa' }}>
        <h3 style={{ marginTop: 0 }}>{state.selectedField ? '属性面板' : 'Schema JSON'}</h3>
        {state.selectedField ? (
          <div style={{ color: '#999' }}>PropertyPanel (Day 5)</div>
        ) : (
          <pre style={{ whiteSpace: 'pre-wrap', fontSize: 12 }}>
            {JSON.stringify({ fields: state.draft.map(({ _draftId, ...f }) => f) }, null, 2)}
          </pre>
        )}
      </aside>
    </div>
  )
}
```

- [ ] **Step 8: Add route**

Modify `apps/web/src/App.tsx`. Inside the `<Route element={<RequireAuth />}>` wrapper, add:

```tsx
<Route path="/owner/tasks/:taskId/templates/:templateId" element={<TemplateDesigner />} />
```

Import at top: `import TemplateDesigner from './modules/template/Designer'`

- [ ] **Step 9: Smoke test**

Run: `cd apps/web && pnpm tsc --noEmit && pnpm lint`
Expected: 0 errors.

Manual: ask user to start api + web dev server (per CLAUDE.md), navigate to `/owner/tasks/1/templates/<v1 id>`, click Material Palette → field appears → click delete → field gone → click Save → URL navigates to v(N+1).

- [ ] **Step 10: Commit**

```bash
git add apps/web/src/modules/template/ apps/web/src/App.tsx apps/web/src/shared/api/client.ts
git commit -m "feat(s2-d4): Designer page with MaterialPalette + DraftCanvas (no PropertyPanel yet)"
```

---

## Day 5 — PropertyPanel

**Goal:** Right column property forms work; selecting a field shows correct PropForm, edits flow back to the draft, name uniqueness validates live.

**Files this day touches:**
- Create: `apps/web/src/modules/template/components/forms/blocks/{CommonPropsForm,OptionsPropsForm,LengthPropsForm}.tsx`
- Create: 9 `apps/web/src/modules/template/components/forms/PropForm_*.tsx`
- Create: `apps/web/src/modules/template/components/PropertyPanel.tsx`
- Modify: `apps/web/src/modules/template/Designer.tsx` (wire PropertyPanel)

### Task 5.1: 3 building-block forms (TDD)

**Files:**
- Create: `apps/web/src/modules/template/components/forms/blocks/CommonPropsForm.tsx` + `.test.tsx`
- Create: `apps/web/src/modules/template/components/forms/blocks/OptionsPropsForm.tsx`
- Create: `apps/web/src/modules/template/components/forms/blocks/LengthPropsForm.tsx`

- [ ] **Step 1: CommonPropsForm test (name uniqueness is the trickiest)**

Create `apps/web/src/modules/template/components/forms/blocks/CommonPropsForm.test.tsx`:

```typescript
import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CommonPropsForm } from './CommonPropsForm'

const field = { _draftId: 'd1', name: 'a', widget: 'Input' as const, label: 'A', required: false }

describe('CommonPropsForm', () => {
  it('shows current name/label/required', () => {
    render(<CommonPropsForm field={field} otherNames={[]} onChange={() => {}} />)
    expect(screen.getByLabelText(/name/i)).toHaveValue('a')
    expect(screen.getByLabelText(/label/i)).toHaveValue('A')
  })

  it('typing in name calls onChange', async () => {
    const onChange = vi.fn()
    const user = userEvent.setup()
    render(<CommonPropsForm field={field} otherNames={[]} onChange={onChange} />)
    const nameInput = screen.getByLabelText(/name/i)
    await user.clear(nameInput)
    await user.type(nameInput, 'x')
    // userEvent fires onChange per keystroke; verify final state
    expect(onChange).toHaveBeenCalled()
    expect(onChange.mock.calls[onChange.mock.calls.length - 1][0]).toEqual({ name: 'x' })
  })

  it('flags duplicate name with otherNames', () => {
    render(<CommonPropsForm field={field} otherNames={['a', 'b']} onChange={() => {}} />)
    expect(screen.getByText(/duplicate|already used|占用/i)).toBeInTheDocument()
  })

  it('does NOT flag own name (field.name in otherNames is excluded by parent)', () => {
    // otherNames should exclude the field's own name; parent's responsibility
    render(<CommonPropsForm field={field} otherNames={['b', 'c']} onChange={() => {}} />)
    expect(screen.queryByText(/duplicate|already used|占用/i)).not.toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run, expect FAIL**

- [ ] **Step 3: Implement CommonPropsForm**

Create `apps/web/src/modules/template/components/forms/blocks/CommonPropsForm.tsx`:

```typescript
import { Input, Switch } from '@douyinfe/semi-ui'
import type { DraftField } from '../../../types'
import type { FieldSchema } from '../../../../../renderer/types'

interface Props {
  field: DraftField
  otherNames: string[]    // already excludes own name
  onChange: (patch: Partial<FieldSchema>) => void
}

export function CommonPropsForm({ field, otherNames, onChange }: Props) {
  const duplicateName = otherNames.includes(field.name)
  return (
    <div>
      <label style={{ display: 'block', marginBottom: 8 }}>
        <div style={{ marginBottom: 4 }}>name</div>
        <Input
          aria-label="name"
          value={field.name}
          onChange={(v) => onChange({ name: v })}
          validateStatus={duplicateName ? 'error' : 'default'}
        />
        {duplicateName ? <div style={{ color: 'red', fontSize: 12 }}>name 已被占用(duplicate)</div> : null}
      </label>
      <label style={{ display: 'block', marginBottom: 8 }}>
        <div style={{ marginBottom: 4 }}>label</div>
        <Input
          aria-label="label"
          value={field.label ?? ''}
          onChange={(v) => onChange({ label: v })}
        />
      </label>
      <label style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
        <Switch
          checked={field.required ?? false}
          onChange={(v) => onChange({ required: v })}
          aria-label="required"
        />
        <span>required</span>
      </label>
    </div>
  )
}
```

- [ ] **Step 4: Run, expect PASS**

- [ ] **Step 5: OptionsPropsForm (CSV-style for v1)**

Create `apps/web/src/modules/template/components/forms/blocks/OptionsPropsForm.tsx`:

```typescript
import { Input } from '@douyinfe/semi-ui'
import type { DraftField } from '../../../types'
import type { FieldSchema } from '../../../../../renderer/types'

interface Props {
  field: DraftField
  onChange: (patch: Partial<FieldSchema>) => void
}

// S2 v1 用 CSV 输入(逗号分隔),完整 array editor 推 Sprint 6(spec §14 风险表)
export function OptionsPropsForm({ field, onChange }: Props) {
  const csv = (field.options ?? []).join(', ')
  const handleChange = (v: string) => {
    const parts = v.split(',').map((s) => s.trim()).filter(Boolean)
    // 数字保持数字
    const options = parts.map((p) => (/^\d+$/.test(p) ? Number(p) : p))
    onChange({ options })
  }
  return (
    <label style={{ display: 'block', marginBottom: 8 }}>
      <div style={{ marginBottom: 4 }}>options (逗号分隔)</div>
      <Input
        aria-label="options"
        value={csv}
        onChange={handleChange}
        placeholder="e.g. 1, 2, 3, 4, 5"
      />
    </label>
  )
}
```

- [ ] **Step 6: LengthPropsForm**

Create `apps/web/src/modules/template/components/forms/blocks/LengthPropsForm.tsx`:

```typescript
import { InputNumber } from '@douyinfe/semi-ui'
import type { DraftField } from '../../../types'
import type { FieldSchema } from '../../../../../renderer/types'

interface Props {
  field: DraftField
  onChange: (patch: Partial<FieldSchema>) => void
}

export function LengthPropsForm({ field, onChange }: Props) {
  const minErr = field.minLength !== undefined && field.maxLength !== undefined && field.minLength > field.maxLength
  return (
    <div>
      <label style={{ display: 'block', marginBottom: 8 }}>
        <div style={{ marginBottom: 4 }}>minLength</div>
        <InputNumber
          aria-label="minLength"
          value={field.minLength ?? undefined}
          onChange={(v) => onChange({ minLength: typeof v === 'number' ? v : undefined })}
          min={0}
        />
      </label>
      <label style={{ display: 'block', marginBottom: 8 }}>
        <div style={{ marginBottom: 4 }}>maxLength</div>
        <InputNumber
          aria-label="maxLength"
          value={field.maxLength ?? undefined}
          onChange={(v) => onChange({ maxLength: typeof v === 'number' ? v : undefined })}
          min={0}
        />
      </label>
      {minErr ? <div style={{ color: 'red', fontSize: 12 }}>minLength &gt; maxLength</div> : null}
    </div>
  )
}
```

- [ ] **Step 7: Commit**

```bash
git add apps/web/src/modules/template/components/forms/blocks/
git commit -m "feat(s2-d5): 3 building-block PropForms (Common/Options/Length)"
```

---

### Task 5.2: 9 PropForm_X composers

**Files:**
- Create: `apps/web/src/modules/template/components/forms/PropForm_{9 widgets}.tsx`

- [ ] **Step 1: Create the 9 composers**

For each widget, create a thin file composing the building blocks. Pattern (use this for all 9):

Create `apps/web/src/modules/template/components/forms/PropForm_Input.tsx`:

```typescript
import { CommonPropsForm } from './blocks/CommonPropsForm'
import { LengthPropsForm } from './blocks/LengthPropsForm'
import type { DraftField } from '../../types'
import type { FieldSchema } from '../../../../renderer/types'

interface Props {
  field: DraftField
  otherNames: string[]
  onChange: (patch: Partial<FieldSchema>) => void
}

export function PropForm_Input({ field, otherNames, onChange }: Props) {
  return (
    <>
      <CommonPropsForm field={field} otherNames={otherNames} onChange={onChange} />
      <LengthPropsForm field={field} onChange={onChange} />
    </>
  )
}
```

Create `PropForm_TextArea.tsx` — same pattern as PropForm_Input (Common + Length).

Create `PropForm_Radio.tsx`:

```typescript
import { CommonPropsForm } from './blocks/CommonPropsForm'
import { OptionsPropsForm } from './blocks/OptionsPropsForm'
import type { DraftField } from '../../types'
import type { FieldSchema } from '../../../../renderer/types'

interface Props {
  field: DraftField
  otherNames: string[]
  onChange: (patch: Partial<FieldSchema>) => void
}

export function PropForm_Radio({ field, otherNames, onChange }: Props) {
  return (
    <>
      <CommonPropsForm field={field} otherNames={otherNames} onChange={onChange} />
      <OptionsPropsForm field={field} onChange={onChange} />
    </>
  )
}
```

Create `PropForm_Tags.tsx` — same as PropForm_Radio (Common + Options).

Create `PropForm_RichText.tsx`:

```typescript
import { CommonPropsForm } from './blocks/CommonPropsForm'
import type { DraftField } from '../../types'
import type { FieldSchema } from '../../../../renderer/types'

interface Props {
  field: DraftField
  otherNames: string[]
  onChange: (patch: Partial<FieldSchema>) => void
}

export function PropForm_RichText({ field, otherNames, onChange }: Props) {
  return <CommonPropsForm field={field} otherNames={otherNames} onChange={onChange} />
}
```

Create `PropForm_JSONEditor.tsx` — same as PropForm_RichText (Common only).

Create `PropForm_FileUpload.tsx`:

```typescript
import { InputNumber } from '@douyinfe/semi-ui'
import { CommonPropsForm } from './blocks/CommonPropsForm'
import type { DraftField } from '../../types'
import type { FieldSchema } from '../../../../renderer/types'

interface Props {
  field: DraftField
  otherNames: string[]
  onChange: (patch: Partial<FieldSchema>) => void
}

export function PropForm_FileUpload({ field, otherNames, onChange }: Props) {
  return (
    <>
      <CommonPropsForm field={field} otherNames={otherNames} onChange={onChange} />
      <label style={{ display: 'block', marginBottom: 8 }}>
        <div style={{ marginBottom: 4 }}>maxFiles</div>
        <InputNumber
          aria-label="maxFiles"
          value={field.maxFiles ?? undefined}
          onChange={(v) => onChange({ maxFiles: typeof v === 'number' ? v : undefined })}
          min={1}
        />
      </label>
    </>
  )
}
```

Create `PropForm_LLMTrigger.tsx`:

```typescript
import { Input } from '@douyinfe/semi-ui'
import { CommonPropsForm } from './blocks/CommonPropsForm'
import type { DraftField } from '../../types'
import type { FieldSchema } from '../../../../renderer/types'

interface Props {
  field: DraftField
  otherNames: string[]
  onChange: (patch: Partial<FieldSchema>) => void
}

export function PropForm_LLMTrigger({ field, otherNames, onChange }: Props) {
  return (
    <>
      <CommonPropsForm field={field} otherNames={otherNames} onChange={onChange} />
      <label style={{ display: 'block', marginBottom: 8 }}>
        <div style={{ marginBottom: 4 }}>prompt 模板</div>
        <Input
          aria-label="prompt"
          value={field.prompt ?? ''}
          onChange={(v) => onChange({ prompt: v })}
          placeholder="如:对以下答案打分..."
        />
      </label>
      <label style={{ display: 'block', marginBottom: 8 }}>
        <div style={{ marginBottom: 4 }}>target field (S3 启用)</div>
        <Input
          aria-label="targetField"
          value={field.targetField ?? ''}
          onChange={(v) => onChange({ targetField: v })}
          disabled
          placeholder="Sprint 3 接入"
        />
      </label>
    </>
  )
}
```

Create `PropForm_ShowItem.tsx`:

```typescript
import { Input, Select } from '@douyinfe/semi-ui'
import { CommonPropsForm } from './blocks/CommonPropsForm'
import type { DraftField } from '../../types'
import type { FieldSchema } from '../../../../renderer/types'

interface Props {
  field: DraftField
  otherNames: string[]
  onChange: (patch: Partial<FieldSchema>) => void
}

export function PropForm_ShowItem({ field, otherNames, onChange }: Props) {
  return (
    <>
      <CommonPropsForm field={field} otherNames={otherNames} onChange={onChange} />
      <label style={{ display: 'block', marginBottom: 8 }}>
        <div style={{ marginBottom: 4 }}>path</div>
        <Input
          aria-label="path"
          value={field.path ?? '$payload'}
          onChange={(v) => onChange({ path: v })}
          placeholder="$payload 或 $payload.field_name"
        />
      </label>
      <label style={{ display: 'block', marginBottom: 8 }}>
        <div style={{ marginBottom: 4 }}>mode</div>
        <Select
          aria-label="mode"
          value={field.mode ?? 'auto'}
          onChange={(v) => onChange({ mode: v as FieldSchema['mode'] })}
          optionList={[
            { value: 'auto', label: 'auto' },
            { value: 'text', label: 'text' },
            { value: 'video', label: 'video' },
            { value: 'image', label: 'image' },
            { value: 'markdown', label: 'markdown' },
            { value: 'json', label: 'json' },
          ]}
        />
      </label>
    </>
  )
}
```

- [ ] **Step 2: Type-check**

Run: `cd apps/web && pnpm tsc --noEmit`
Expected: 0 errors.

- [ ] **Step 3: Commit**

```bash
git add apps/web/src/modules/template/components/forms/PropForm_*.tsx
git commit -m "feat(s2-d5): 9 PropForm_X composers via building-block forms"
```

---

### Task 5.3: PropertyPanel dispatcher + wire into Designer

**Files:**
- Create: `apps/web/src/modules/template/components/PropertyPanel.tsx`
- Modify: `apps/web/src/modules/template/Designer.tsx`

- [ ] **Step 1: Implement dispatcher**

Create `apps/web/src/modules/template/components/PropertyPanel.tsx`:

```typescript
import type { DraftField } from '../types'
import type { FieldSchema, WidgetType } from '../../../renderer/types'
import { PropForm_Input } from './forms/PropForm_Input'
import { PropForm_TextArea } from './forms/PropForm_TextArea'
import { PropForm_Radio } from './forms/PropForm_Radio'
import { PropForm_Tags } from './forms/PropForm_Tags'
import { PropForm_RichText } from './forms/PropForm_RichText'
import { PropForm_JSONEditor } from './forms/PropForm_JSONEditor'
import { PropForm_FileUpload } from './forms/PropForm_FileUpload'
import { PropForm_LLMTrigger } from './forms/PropForm_LLMTrigger'
import { PropForm_ShowItem } from './forms/PropForm_ShowItem'

const PROP_FORMS: Record<WidgetType, React.ComponentType<{
  field: DraftField
  otherNames: string[]
  onChange: (patch: Partial<FieldSchema>) => void
}>> = {
  Input:      PropForm_Input,
  TextArea:   PropForm_TextArea,
  Radio:      PropForm_Radio,
  Tags:       PropForm_Tags,
  RichText:   PropForm_RichText,
  JSONEditor: PropForm_JSONEditor,
  FileUpload: PropForm_FileUpload,
  LLMTrigger: PropForm_LLMTrigger,
  ShowItem:   PropForm_ShowItem,
}

interface Props {
  field: DraftField
  otherNames: string[]
  onChange: (patch: Partial<FieldSchema>) => void
}

export function PropertyPanel({ field, otherNames, onChange }: Props) {
  const Form = PROP_FORMS[field.widget]
  return <Form field={field} otherNames={otherNames} onChange={onChange} />
}
```

- [ ] **Step 2: Wire into Designer**

Modify `apps/web/src/modules/template/Designer.tsx` — replace the placeholder `PropertyPanel (Day 5)` in the right aside with:

```tsx
{state.selectedField ? (
  <PropertyPanel
    field={state.selectedField}
    otherNames={state.draft.filter(f => f._draftId !== state.selectedField!._draftId).map(f => f.name)}
    onChange={(patch) => state.update(state.selectedField!._draftId, patch)}
  />
) : (
  <pre style={{ whiteSpace: 'pre-wrap', fontSize: 12 }}>
    {JSON.stringify({ fields: state.draft.map(({ _draftId, ...f }) => f) }, null, 2)}
  </pre>
)}
```

Add the import: `import { PropertyPanel } from './components/PropertyPanel'`

- [ ] **Step 3: Add Designer integration test (spec §8.3)**

Create `apps/web/src/modules/template/Designer.integration.test.tsx`:

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import TemplateDesigner from './Designer'

// Mock API client — return the template synchronously
vi.mock('../../shared/api/client', () => ({
  apiGet: vi.fn(async (path: string) => {
    if (path.startsWith('/templates/')) {
      return {
        template: {
          id: 100, taskId: 1, version: 1,
          schemaJson: JSON.stringify({
            title: 't', layout: 'single_page',
            fields: [{ name: 'a', widget: 'Input', label: 'A' }],
          }),
        },
        isLatest: true,
        latestTemplateId: 100,
      }
    }
    return []
  }),
  apiPost: vi.fn(async (path: string, body: any) => {
    return { template: { id: 101, taskId: 1, version: 2, schemaJson: '' } }
  }),
}))

describe('Designer integration', () => {
  beforeEach(() => { vi.clearAllMocks() })

  it('append → select → rename → delete → save sends stripped fields[]', async () => {
    const user = userEvent.setup()
    const { apiPost } = await import('../../shared/api/client')

    render(
      <MemoryRouter initialEntries={['/owner/tasks/1/templates/100']}>
        <Routes>
          <Route path="/owner/tasks/:taskId/templates/:templateId" element={<TemplateDesigner />} />
        </Routes>
      </MemoryRouter>
    )

    // wait initial template load
    await waitFor(() => expect(screen.getByText('模板设计器')).toBeInTheDocument())

    // append 2 Radio fields
    const addRadio = screen.getByLabelText('add Radio')
    await user.click(addRadio)
    await user.click(addRadio)

    // confirm canvas has 3 fields: initial 'a' + radio_1 + radio_2
    expect(screen.getByText(/Input · a/)).toBeInTheDocument()
    expect(screen.getByText(/Radio · radio_1/)).toBeInTheDocument()
    expect(screen.getByText(/Radio · radio_2/)).toBeInTheDocument()

    // select radio_1 by clicking its shell
    const radio1Shell = screen.getByRole('group', { name: /Radio.*radio_1|radio_1/i })
    await user.click(radio1Shell)

    // rename in property panel
    const nameInput = screen.getByLabelText('name')
    await user.clear(nameInput)
    await user.type(nameInput, 'fluency_score')

    // delete radio_2
    const deleteBtns = screen.getAllByLabelText(/^delete /)
    // radio_2 is the last delete button
    await user.click(deleteBtns[deleteBtns.length - 1])

    // save
    await user.click(screen.getByText('Save as new version'))

    // assert POST body
    await waitFor(() => expect(apiPost).toHaveBeenCalled())
    const [path, body] = (apiPost as any).mock.calls[0]
    expect(path).toBe('/tasks/1/templates')
    // 2 fields: 'a' (Input) + renamed 'fluency_score' (Radio); radio_2 deleted
    expect(body.fields).toHaveLength(2)
    expect(body.fields[1].name).toBe('fluency_score')
    expect(body.fields[1].widget).toBe('Radio')
    // _draftId stripped
    body.fields.forEach((f: any) => expect(f).not.toHaveProperty('_draftId'))
  })

  it('readonly mode disables material palette + shows Fork button', async () => {
    const { apiGet } = await import('../../shared/api/client')
    ;(apiGet as any).mockImplementationOnce(async () => ({
      template: { id: 99, taskId: 1, version: 1, schemaJson: '{"fields":[{"name":"a","widget":"Input"}]}' },
      isLatest: false,  // KEY: old version
      latestTemplateId: 200,
    }))

    render(
      <MemoryRouter initialEntries={['/owner/tasks/1/templates/99']}>
        <Routes>
          <Route path="/owner/tasks/:taskId/templates/:templateId" element={<TemplateDesigner />} />
        </Routes>
      </MemoryRouter>
    )
    await waitFor(() => expect(screen.getByText('Fork as new version')).toBeInTheDocument())
    expect(screen.getByLabelText('add Input')).toBeDisabled()
  })
})
```

- [ ] **Step 4: Run, expect PASS**

Run: `cd apps/web && pnpm test src/modules/template/Designer.integration.test.tsx`
Expected: 2 tests PASS.

- [ ] **Step 5: Smoke build**

Run: `cd apps/web && pnpm tsc --noEmit && pnpm lint && pnpm build`
Expected: 0 errors.

- [ ] **Step 6: Commit**

```bash
git add apps/web/src/modules/template/components/PropertyPanel.tsx apps/web/src/modules/template/Designer.tsx apps/web/src/modules/template/Designer.integration.test.tsx
git commit -m "feat(s2-d5): PropertyPanel dispatcher + Designer integration test (append→rename→delete→save)"
```

---

## Day 6 — Template List + readonly/Fork + E2E + Demo Script

**Goal:** Owner can navigate to a list of versions, click into Designer (edit or readonly w/ Fork), full E2E demo from scratch, docs committed, ready for review.

**Files this day touches:**
- Create: `apps/web/src/modules/template/List.tsx`
- Modify: `apps/web/src/App.tsx` (List route)
- Modify: `apps/web/src/modules/owner/Dashboard.tsx` (button into List)
- Create: `docs/S2_ACCEPTANCE.md`
- Create: `docs/S2_DEMO_SCRIPT.md`

### Task 6.1: Template List page + route + dashboard entry

**Files:**
- Create: `apps/web/src/modules/template/List.tsx`
- Modify: `apps/web/src/App.tsx`
- Modify: `apps/web/src/modules/owner/Dashboard.tsx`

- [ ] **Step 1: Implement List**

Create `apps/web/src/modules/template/List.tsx`:

```typescript
import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { Button, Table, Toast } from '@douyinfe/semi-ui'
import { apiGet, type TaskTemplate } from '../../shared/api/client'

export default function TemplateList() {
  const { taskId = '' } = useParams<{ taskId: string }>()
  const [items, setItems] = useState<TaskTemplate[]>([])

  useEffect(() => {
    void (async () => {
      try {
        const data = await apiGet<TaskTemplate[]>(`/tasks/${taskId}/templates`)
        setItems(data)
      } catch (err) {
        Toast.error(err instanceof Error ? err.message : '加载模板列表失败')
      }
    })()
  }, [taskId])

  const latestId = items[0]?.id

  return (
    <div style={{ padding: 24 }}>
      <h1>任务 #{taskId} 模板版本</h1>
      <Table
        dataSource={items}
        rowKey="id"
        columns={[
          { title: 'Version', dataIndex: 'version', render: (v: number, row: TaskTemplate) => (
            <span>
              v{v} {row.id === latestId ? <em style={{ color: '#1677ff' }}>(latest)</em> : null}
            </span>
          )},
          { title: 'Created', dataIndex: 'createdAt' },
          { title: '', render: (_: unknown, row: TaskTemplate) => (
            <Link to={`/owner/tasks/${taskId}/templates/${row.id}`}>
              <Button size="small">{row.id === latestId ? 'Edit' : 'View / Fork'}</Button>
            </Link>
          )},
        ]}
        pagination={false}
      />
    </div>
  )
}
```

- [ ] **Step 2: Add route**

Modify `apps/web/src/App.tsx`, inside `<RequireAuth>` `<AppLayout>` add:

```tsx
<Route path="/owner/tasks/:taskId/templates" element={<TemplateList />} />
```

Import: `import TemplateList from './modules/template/List'`

- [ ] **Step 3: Dashboard entry**

Modify `apps/web/src/modules/owner/Dashboard.tsx`. Find where task rows are rendered. Next to each task, add:

```tsx
<Link to={`/owner/tasks/${task.id}/templates`}>
  <Button size="small" style={{ marginLeft: 8 }}>模板</Button>
</Link>
```

- [ ] **Step 4: Type-check**

Run: `cd apps/web && pnpm tsc --noEmit && pnpm lint && pnpm build`

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/modules/template/List.tsx apps/web/src/App.tsx apps/web/src/modules/owner/Dashboard.tsx
git commit -m "feat(s2-d6): Template List page + route + Dashboard entry"
```

---

### Task 6.2: E2E smoke (manual, recorded)

**Files:** none, but produces `docs/S2_DEMO_SCRIPT.md`

- [ ] **Step 1: Start backend + frontend (ask user to run, per CLAUDE.md)**

Tell the user:
> 请你开两个终端:
> 1. `cd ~/Desktop/LabelHub && set -a && . ./.env && set +a && export API_PORT=18181 && cd apps/api && go run cmd/server/main.go`
> 2. `cd ~/Desktop/LabelHub && pnpm --dir apps/web dev`
> 跑起来告诉我。

- [ ] **Step 2: Run S1.5 happy path regression**

Use the smoke script from S1_ACCEPTANCE.md §7 with API at 18181:
1. owner1 login + `/tasks`
2. labeler1 claim + submit (random 12 fields)
3. reviewer1 queue + approve
4. owner1 export → ≥1 approved row
5. Unauth GET `/tasks` → 401

Expected: all green.

- [ ] **Step 3: Run S2 new path 1 — schema-driven Plaza**

In browser, log in as labeler1, navigate to Plaza, claim a fresh item. Verify all 12 fields render with correct labels:
- 原始数据, 相关性, 准确性, 格式合规, 安全性, 问题类型, 一句话总评, 详细评语, 修订建议, 修正后答案, 证据素材, AI 预评分

- [ ] **Step 4: Run S2 new path 2 — Designer construct**

Log in as owner1. Navigate `/owner/tasks/1/templates`. Click into latest version. Confirm:
- 3-column layout (palette left, canvas middle, JSON viewer right)
- Click "Radio" in palette → field `radio_5` appears at bottom (because qa_quality already has radio_1..4)
- Click into the new field → PropertyPanel switches to Radio props
- Edit name → `fluency_score`, edit label → `流畅度`, edit options → `1, 2, 3, 4, 5`
- Click "Save as new version"
- URL navigates to v2
- Switch to labeler1 → claim a fresh item → see 流畅度 field

- [ ] **Step 5: Run S2 new path 3 — readonly Fork**

Owner1 navigate to `/owner/tasks/1/templates`. Click into v1 (not latest now). Confirm:
- Material palette buttons disabled
- Canvas field shells have no delete buttons
- Toolbar shows "Fork as new version" instead of "Save"
- Click Fork → POST → URL navigates to v3 (copy of v1)
- v3 is now editable

- [ ] **Step 6: Run S2 new path 4 — schema error banner**

```bash
docker exec labelhub-mysql mysql -ulabelhub -plabelhub_dev labelhub -e "UPDATE task_templates SET schema_json='{not json' WHERE id=(SELECT MAX(id) FROM (SELECT id FROM task_templates) x);"
```

Refresh Plaza. Expect `SchemaErrorBanner` with labeler-side copy.

Restore: `make seed` (will UPDATE because hash mismatch).

- [ ] **Step 7: Run S2 new path 5 — _draftId survives rename**

In Designer (edit mode), click Radio in palette to add a field. Select it. Edit name 5 times (`a` → `b` → `c` → `radio_5` → `fluency_score` → `flow_score`). PropertyPanel stays focused on this field throughout (no flicker, no reset).

- [ ] **Step 8: Run S2 new path 6 — seed mutation**

```bash
cd /Users/dadadineiyou/Desktop/LabelHub
sed -i.bak 's/"label": "相关性"/"label": "相关性 (修改测试)"/' tools/seed/templates/qa_quality_review.json
make seed
```

Refresh Plaza → labeler1 sees the new label.

Restore:
```bash
mv tools/seed/templates/qa_quality_review.json.bak tools/seed/templates/qa_quality_review.json
make seed
```

- [ ] **Step 9: Write demo script**

Create `docs/S2_DEMO_SCRIPT.md`:

```markdown
# Sprint 2 Demo 剧本(4 分钟以内)

**面向**:bytedance 全栈挑战赛评委 / 内部 5 分钟答辩

| 步 | 时长 | 动作 | 评委看到 |
|---|---|---|---|
| 1 | 0:00-0:30 | owner1 登录 → `/owner/tasks/1/templates` | 模板列表 v1 |
| 2 | 0:30-1:30 | 点 v1 进 Designer → 左栏点 Radio → 改 name `fluency_score` / label `流畅度` / options `1-5` → Save | 三栏 Designer + 字段实时出现 + 保存为 v2 |
| 3 | 1:30-2:00 | 切 labeler1 → 进 Plaza → claim 一题 | qa_quality 12 字段 + 新增的 `流畅度` |
| 4 | 2:00-2:30 | labeler1 填全所有字段 → 提交 | submission → human_reviewing |
| 5 | 2:30-3:00 | 切 reviewer1 → 在 queue 看到 → approve | submission → approved |
| 6 | 3:00-3:30 | 回 owner1 → 进 v1(now readonly)→ Fork as new version | 副本 v3 进入编辑模式 |
| 7 | 3:30-4:00 | owner1 → export → 拿到 approved 数据 | JSON 含 12 + 1 字段 |

**关键卖点**:
- Schema 驱动 = 一行 JSON 配置 ↔ 整个 labeler 表单
- 模板版本化 = 修改不破坏已提交数据
- 数据资产隔离 = labeler 互相看不到对方 claim 的 item(S1.5 验证过)
- 后端事务一致性 = `tasks.template_id` 与 `task_templates` 同事务更新,不会半截
```

- [ ] **Step 10: Commit demo script**

```bash
git add docs/S2_DEMO_SCRIPT.md
git commit -m "docs(s2-d6): demo script for contest pitch"
```

---

### Task 6.3: S2 acceptance doc + final commit

**Files:**
- Create: `docs/S2_ACCEPTANCE.md`

- [ ] **Step 1: Write acceptance doc**

Create `docs/S2_ACCEPTANCE.md`:

```markdown
# LabelHub S2 v1 Acceptance — 模板系统稳定 Checkpoint

**版本基线**:Sprint 2 final commit hash
**冻结时间**:2026-MM-DD
**Entry baseline**:`4df816b` (S1.5)
**Source spec**:`docs/PLAN-S2.md`
**Implementation plan**:`docs/PLAN-S2-IMPL.md`

## 1. S2 已交付能力

| 能力 | 实现位置 | 状态 |
|---|---|---|
| 4 个 template API | `apps/api/internal/handler/template.go` | ✓ |
| Backend 同事务 task.template_id 更新 | `template.go::Create` | ✓ |
| Seed upsert by schema_hash | `cmd/seed/main.go::seedQAQuality` | ✓ |
| Renderer 模块(parser/validator/9 widgets) | `apps/web/src/renderer/` | ✓ |
| SchemaErrorBanner 角色分文案 | `apps/web/src/renderer/components/SchemaErrorBanner.tsx` | ✓ |
| Plaza schema 驱动(LabelAnswer 删除) | `apps/web/src/modules/labeler/Plaza.tsx` | ✓ |
| Queue readOnly schema 渲染 | `apps/web/src/modules/reviewer/Queue.tsx` | ✓ |
| Designer 三栏 + edit/readonly 二态 | `apps/web/src/modules/template/Designer.tsx` | ✓ |
| Template List + Fork 入口 | `apps/web/src/modules/template/List.tsx` | ✓ |
| 9 PropForm × 3 building block | `forms/PropForm_*.tsx` + `forms/blocks/` | ✓ |
| Vitest test stack | `apps/web/vitest.config.ts` | ✓ |

## 2. 后端 4 新路由

| Route | Method | Permission | Notes |
|---|---|---|---|
| `/api/v1/tasks/:taskId/templates` | GET | owner/admin | versions DESC |
| `/api/v1/tasks/:taskId/templates` | POST | owner/admin | tx + 锁 task → MAX(version)+1 → INSERT → UPDATE tasks.template_id |
| `/api/v1/templates/:templateId` | GET | owner/admin/labeler/reviewer | 携带 isLatest + latestTemplateId |
| `/api/v1/tasks/:taskId/templates/validate` | POST | owner/admin | 最小校验(name 唯一 / widget enum / required bool / min<=max) |

## 3. 12 条验收(对照 spec §10)

| # | 标准 | 结果 |
|---|---|---|
| 1 | Backend 4 路由全绿 | ✓ |
| 2 | Plaza 12 字段渲染 | ✓ |
| 3 | Queue readOnly + 历史 template_version | ✓ |
| 4 | Designer 从 0 构造 + Save as v(N+1) | ✓ |
| 5 | Old version readonly + Fork | ✓ |
| 6 | Fork 创建新版本进 edit 模式 | ✓ |
| 7 | _draftId rename 不丢选中 | ✓ |
| 8 | ShowItem path + 6 mode | ✓ |
| 9 | SchemaErrorBanner 角色分文案 | ✓ |
| 10 | Seed mutation Plaza 立即看到新 label | ✓ |
| 11 | go test -race / lint / build / vitest 全绿,handler cover ≥35% | ✓ (实际数:填入) |
| 12 | Demo 剧本 ≤4 分钟 | ✓ (`docs/S2_DEMO_SCRIPT.md`) |

## 4. 已知 warning(非阻塞)

| 项 | 严重度 | 处理时间 |
|---|---|---|
| lottie-web eval(Semi 间接依赖)+ chunk 712kB | LOW | Sprint 6 polish |
| FileUpload 仅展示 ID,无 thumbnail | LOW | Sprint 6 |
| LLMTrigger 写自己字段,不自动 target | INFO | Sprint 3(配合真实豆包)|
| RichText 降级为 TextArea+HTML | INFO | Sprint 6 接 TipTap |
| React Router 内部跳转拦截没接 | LOW | Sprint 6 polish |

## 5. Review findings 闭环

(复制 spec §11 表)

## 6. 下一步

Sprint 3 — AI Agent 落产品 + golden dry-run + Owner AI 评测配置页(spec §13 推到 9.3+ 分数)。
```

Fill the version baseline + frozen time after Step 3 commits.

- [ ] **Step 2: Final verification gate**

Run:
```bash
cd /Users/dadadineiyou/Desktop/LabelHub
cd apps/api && go test ./... -race && go test ./... -cover
cd .. && pnpm --dir apps/web test
pnpm --dir apps/web lint
pnpm --dir apps/web build
```

Expected:
- go test -race: green
- handler coverage ≥ 35%
- vitest: all green
- lint: 0 errors
- build: clean (with documented warnings)

- [ ] **Step 3: Final commit**

```bash
git add docs/S2_ACCEPTANCE.md
git commit -m "docs(s2-d6): S2 v1 acceptance — 12 criteria verified, ready for Sprint 3"
git log --oneline -15
```

Expected commit history shows Day 1-6 progression cleanly.

---

## Risks & Fallbacks (mirrors spec §14)

| Risk | Fallback |
|---|---|
| Day 4-5 超时 | 砍 OptionsPropsForm,Radio/Tags 不让 owner 编 options(用现有 qa_quality 的写死值);S3 头补 |
| Day 3 LLMTrigger 联动复杂 | 已降级:LLMTrigger 把结果写自己字段而不是 target field |
| FileUpload 文件预览复杂 | 仅展示 ID + filename,无 thumbnail / 下载预览 |
| POST templates 死锁 | 已通过"先锁 task 再 templates"统一锁顺序避免(spec §6 + 任务 1.3) |
| Vitest setup 比预期久 | Day 2 砍 ShowItem 实现到 Day 3 头(SchemaRenderer 中用 placeholder fallback 占位) |
| Day 6 List 页超时 | List 降级为 Dashboard 嵌入子表(不开独立路由),URL 直接 `/owner/tasks/1/templates/<id>` 走 |

---

## Execution Handoff

Plan complete and saved to `docs/PLAN-S2-IMPL.md`. Two execution options:

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per Task (e.g., one for Task 1.1, then Task 1.2, etc.), review the diff between tasks, fast iteration. Best when tasks have clean boundaries (this plan does).

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch with checkpoints. Slower per-task but everything stays in one context window.

**Which approach?**
