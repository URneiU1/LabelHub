# S2 v1 Design Spec — Template Renderer & Editable Designer

**Status**: Approved (via /brainstorming), awaiting implementation
**Date**: 2026-05-22
**Entry baseline**: `4df816b` (S1.5 数据资产隔离已落)
**Sprint 2 budget**: 6 天(原 4-4.5 天 + 1.5 天 Designer 编辑能力)
**Contest target**: 字节 AI 全栈挑战赛 #1(deadline 2026-07-15)

## 1. 背景

S1.5 commit `4df816b` 当前分数:代码质量 8.5 / 架构 8.5 / 业务理解 9.0。

冲第一名的下一档分数靠 S2 落地 **schema 驱动 + 可演示构造能力**。原先 PLAN-S2.md 把 Designer 砍为"纯静态 3 栏",但 architect-review 发现 master PLAN.md S2 完成标准要求"Owner 全程拖拽搭出 qa_quality schema",而 Sprint 6 章节只承接 Tabs/Group + polish,**不承接 dnd-kit 拖拽 / meta-circular**。砍掉构造能力 = contest demo Designer 只是只读浏览器,丢评委关键印象。

经 /brainstorming 重新定位 Designer 下限到"能 from 0 构造 schema",但避开 dnd-kit 和 meta-circular 的实现风险。

## 2. 锁定决策(来自 brainstorm)

| 决策 | 选择 | 影响 |
|---|---|---|
| Designer 下限 | 能构造(点击加物料) | master PLAN.md S2 完成标准 ≥80% 达成 |
| 构造档位 | Append + delete + 简易属性表单 | 无 dnd-kit;9 widget PropForm 用 3 个 building block 拼起来 DRY |
| Save 语义 | 每次 Save 建新版本(append-only history) | 同事务更新 `tasks.template_id`;已有 submission 锁自己的 template_version snapshot |
| Timeline | Sprint 2 一次到位 4.5 → 6 天 | 周末加班 1.5 天,Sprint 6 不动 |
| Renderer 落点 | `apps/web/src/renderer/` 纯 TS 模块 | S2 v1 只有 web 一个消费者;workspace package 等 mobile 出现再升 |
| Validation | 最小校验(name 唯一 / widget ∈ 9 enum / required 是 bool) | 完整 JSON Schema 校验推到 S3 |

## 3. 架构

### 3.1 Designer 三栏(mode 二态)

| Version | Mode | Toolbar | 编辑能力 |
|---|---|---|---|
| `isLatest=true` | **edit** | `Save as v(N+1)` / `Discard changes` | 物料库可点 / 字段可删 / 属性面板可编 |
| `isLatest=false`(历史版本) | **readonly** | `Fork as new version`(POST 当前 schema 副本)| 全部 disabled,右栏只显示 JSON |

`Discard` 永远回到"刚加载的当前 version 的 snapshot",不跨 version。

| 栏 | 宽度 | 行为 |
|---|---|---|
| 左 物料库 | 280px | 9 widget 卡片;edit 模式点击 = append 到画布末尾;readonly 模式全 disabled |
| 中 画布 | flex | `DraftCanvas` shell(不是 SchemaRenderer)+ 每字段 `CanvasFieldShell` 包 `WidgetPreview` |
| 右 属性面板 | 360px | 选中态:对应 PropForm_X;无选中态:`<pre>` 显示当前 schema_json |

### 3.2 关键设计:`DraftCanvas` ≠ `SchemaRenderer`

SchemaRenderer 服务 Plaza/Queue 的运行时表单。Designer 中栏复用 widget registry 但用自己的 shell:

```tsx
<DraftCanvas mode={mode}>
  {draft.fields.map(field => (
    <CanvasFieldShell
      key={field._draftId}
      selected={field._draftId === selectedId}
      onSelect={() => selectId(field._draftId)}
      onDelete={() => removeField(field._draftId)}
      disabled={mode === 'readonly'}
    >
      <WidgetPreview field={field} />
    </CanvasFieldShell>
  ))}
</DraftCanvas>
```

**Runtime renderer 不被 editor 状态污染**。

### 3.3 Field identity: `_draftId`

```ts
type DraftField = FieldSchema & { _draftId: string }
```

- 选中态 / removeField / updateField 全用 `_draftId`,name 改了不影响选中
- 内部 state 仅 array(无 Map),9-15 字段下 `findIndex(_draftId)` O(n) 足够
- Save 前 strip `_draftId`:`fields.map(({_draftId, ...f}) => f)`
- ID 来源:`crypto.randomUUID()`(浏览器原生,无依赖);测试注入固定 factory:`useDesignerState(initial, { idFactory: () => 'test-id-1' })`

### 3.4 默认 name 生成(deterministic)

```ts
// widgetPrefixMap:9 widget 的 snake_case 前缀写死,避免 camelToSnake 处理 JSONEditor / LLMTrigger / ShowItem
// 这种连续大写时各写各的(j_s_o_n_editor vs json_editor 不一致会让测试和验收都乱)
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

function nextFieldName(widget: WidgetType, existing: DraftField[]): string {
  const prefix = widgetPrefixMap[widget]
  let i = 1
  while (existing.some(f => f.name === `${prefix}_${i}`)) i++
  return `${prefix}_${i}`
}
```

While-find-first-unused,避免删后再 append 撞名(`input_1`/`input_3` 删 `input_2` 后下一个 append 拿 `input_2` 填空)。

### 3.5 属性表单 DRY 拆分

3 个 building block + 9 个轻量 wrapper:

```
forms/blocks/
  CommonPropsForm.tsx     // name (uniqueness check) + label + required
  OptionsPropsForm.tsx    // options[] add / remove / reorder
  LengthPropsForm.tsx     // minLength + maxLength
forms/
  PropForm_Input.tsx       // Common + Length
  PropForm_TextArea.tsx    // Common + Length
  PropForm_Radio.tsx       // Common + Options
  PropForm_Tags.tsx        // Common + Options
  PropForm_RichText.tsx    // Common
  PropForm_JSONEditor.tsx  // Common
  PropForm_FileUpload.tsx  // Common + maxFiles
  PropForm_LLMTrigger.tsx  // Common + prompt + target_field
  PropForm_ShowItem.tsx    // Common + path + mode(5 enum)
```

总 ~250 LoC 而非 9×50。改 "required 不能为空" 只动 CommonPropsForm 一处。

## 4. 数据流

```
mount: GET /api/v1/templates/:templateId
  → 响应含 { template, isLatest, latestTemplateId }
  → parseTemplateSchema(template.schemaJson) → 给每个 field 加 _draftId
  → mode = template.isLatest ? 'edit' : 'readonly'
  → useDesignerState(draftFields, mode)

点物料(edit 模式):
  → appendField(widget) — 内部:_draftId = uuid(), name = nextFieldName(widget, current)
点字段: selectId(field._draftId)
改属性: updateField(_draftId, patch) — Canvas + 表单同步
点删: removeField(_draftId)
点 Save:
  → POST /api/v1/tasks/:taskId/templates  body = stripDraftIds(draft)
  → 成功 → nav /owner/tasks/:taskId/templates/:newId(自动进 edit 模式,因为 newId === latest)
点 Discard: setDraft(originalSnapshotForThisVersion)
点 Fork as new (readonly 模式): 同 Save,POST 当前已加载的旧版本副本
```

## 5. 错误处理

| 场景 | 处理 |
|---|---|
| 挂载时 schema_json 坏 | `<SchemaErrorBanner>`,提供 "新建空 schema" 入口 |
| **POST 409**(`uk_task_version` 撞) | 后端事务 + duplicate key fallback 已防主路径;真撞了 → 前端 toast "已有人创建了新版本,请刷新",**不前端静默 retry**(撞了说明真有别人在改,刷新看最新更安全)|
| POST 422(name 重复 / widget 非 enum / required 非 bool) | error 列表逐个高亮到 PropForm_X 的字段 |
| name 实时重复(本地改一样) | CommonPropsForm 实时校验,撞了红框 + Save disabled |
| 本地无变化 | Save 按钮 disabled |
| 离开页有未保存(刷新/关页) | `beforeunload` 原生 confirm |
| SPA 内部路由跳转 | **不拦** — react-router `useBlocker` 推到 Sprint 6 polish |
| readonly 模式点 Save | 不存在(按钮文案是 Fork as new version)|

**后端兜底声明**:POST /templates 后端必须通过 `transaction + SELECT FOR UPDATE + duplicate-key fallback` 保证一致性,**不依赖前端 retry**。

## 6. 后端 API contract

| 路由 | 方法 | 权限 | 行为 |
|---|---|---|---|
| `/api/v1/tasks/:taskId/templates` | GET | owner/admin(`loadOwnedTask`)| 列出该 task 下所有版本,按 version DESC |
| `/api/v1/tasks/:taskId/templates` | POST | owner/admin | 创建新版本(并发安全见下);**同事务更新 `tasks.template_id=新ID`**;body `{title?: string, layout?: 'single_page', fields: FieldSchema[]}` — title 缺省取 `task.title`,layout 缺省 `'single_page'`(S2 v1 唯一支持),fields 必填非空 |
| `/api/v1/templates/:templateId` | GET | owner/admin (own task) / labeler+reviewer (published task) | 响应 `{ template, isLatest, latestTemplateId }` |
| `/api/v1/tasks/:taskId/templates/validate` | POST | owner/admin | 校验草稿 schema(还没存盘),body 同 POST templates;返回 `{valid, errors: [{field, message}]}` |

**POST templates 实现伪码**:

```go
tx.Transaction(func(tx *gorm.DB) error {
    // 1. SELECT MAX(version) FOR UPDATE — 行锁防赛跑
    var maxV int
    tx.Set("gorm:query_option", "FOR UPDATE").
       Model(&TaskTemplate{}).Where("task_id=?", taskID).
       Select("COALESCE(MAX(version),0)").Scan(&maxV)

    // 2. INSERT new template
    newT := &TaskTemplate{TaskID: taskID, Version: maxV+1, SchemaJSON: body, SchemaHash: sha256(body)}
    if err := tx.Create(newT).Error; err != nil {
        if isDuplicateKey(err) { return errConflict409 }   // 防御性兜底
        return err
    }

    // 3. 同事务把 task 指向新模板 — 否则新 claim 还在 v1
    if err := tx.Model(&Task{}).Where("id=?", taskID).
        Update("template_id", newT.ID).Error; err != nil {
        return err
    }
    return nil
})
```

**业务规则**:
- ✓ 新 claim / 新 submission 走新模板(`Task.TemplateID = latest`)
- ✓ 已有 submissions 继续绑自己的 `template_version` snapshot(已存,无需改)
- ✓ Owner 想"回滚"就 Fork 旧版本 → 自动成为新 latest
- ✓ 失败回滚 task.template_id

**Seed upsert**(关键,验收 #10 前提):
`cmd/seed/main.go::seedQAQuality` 当前 `FirstOrCreate` + `Attrs` 不更新 v1。改造:
- 算 `schema_hash = sha256(schemaJSON)`
- 查 existing v1:不存在 → Create;存在且 hash 不同 → UPDATE schema_json + schema_hash;hash 相同 → 跳过

## 7. 组件清单(新增)

```
apps/web/src/renderer/                       # Plaza/Queue 运行时用
  types.ts                                   # TemplateSchema, FieldSchema, WidgetType, AnswerValue, SchemaParseError
  parser.ts                                  # parseTemplateSchema(raw) → {ok,value}|{ok,error}; parseAnswer 同
  validator.ts                               # 必填 / 长度 / name 唯一
  SchemaRenderer.tsx                         # 接收已 parsed TemplateSchema(不吃 string)
  widgets/index.ts                           # widget name → component map(给 SchemaRenderer 和 WidgetPreview 共用)
  widgets/{ShowItem,Input,TextArea,Radio,Tags,RichText,JSONEditor,FileUpload,LLMTrigger}.tsx
  components/SchemaErrorBanner.tsx           # parse 失败统一组件,按角色分文案

apps/web/src/modules/template/               # Designer 编辑用
  List.tsx                                   # GET /tasks/:id/templates 列表 + "Fork" 列按钮
  Designer.tsx                               # mode 二态主页
  components/
    MaterialPalette.tsx                      # 左栏 9 widget 卡片
    DraftCanvas.tsx                          # 中栏 canvas 容器
    CanvasFieldShell.tsx                     # 单字段壳:选中 / 删除 / disabled
    WidgetPreview.tsx                        # widget registry 的 readOnly 视图(不接 value/onChange)
    PropertyPanel.tsx                        # 右栏 dispatcher → PropForm_X
    forms/blocks/{Common,Options,Length}PropsForm.tsx
    forms/PropForm_{9 widgets}.tsx
    VersionBadge.tsx                         # latest / v(N) / readonly 标记
  hooks/
    useDesignerState.ts                      # draft + selectedId + ops + nextFieldName
```

## 8. 测试矩阵

### 8.1 Vitest 落地

```bash
pnpm -F @labelhub/web add -D vitest @testing-library/react @testing-library/jest-dom jsdom @vitejs/plugin-react
```
- `apps/web/vitest.config.ts`:environment=jsdom + jest-dom matchers
- `apps/web/package.json`:`"test": "vitest run"`,`"test:watch": "vitest"`
- 不写 snapshot,只写显式 assertion(getByLabelText / getByRole / userEvent)

### 8.2 Vitest unit

- `parser.test.ts`:happy / 坏 JSON / 缺 fields / widget 拼错 / fields 非数组
- `validator.test.ts`:required / minLength / maxLength / name 唯一
- `useDesignerState.test.ts`:append / remove / update / discard;**关键 case**:append→remove→append 测 `nextFieldName` 不撞;rename→remove 测 `_draftId` 不依赖 name
- `nextFieldName.test.ts`:删 input_2 后再 append 拿到 input_2(填空)

### 8.3 Vitest integration

- `Designer.integration.test.tsx`:挂 fixture template → 点 Radio 卡片 2 次 → 看到 radio_1 + radio_2 → 选中 radio_1 → 改 name 为 fluency_score → 删 radio_2 → Save → 断言 POST body 不含 `_draftId`
- `Designer.readonly.test.tsx`:fixture `{isLatest: false}` → 物料库 / 删除 / 属性面板全 disabled / toolbar 显示 Fork

### 8.4 Backend(sqlmock + go test)

- `s1_template_test.go`:
  - `TestListTemplates_OwnerOnly` — non-owner 403
  - `TestGetTemplate_ReturnsIsLatest` — isLatest 字段正确
  - `TestCreateTemplate_BumpsVersionAndUpdatesTaskTemplateID` — **同事务双更新断言**
  - `TestCreateTemplate_DuplicateKeyReturns409` — 模拟撞 uk_task_version
  - `TestValidateTemplate_RejectsDuplicateNames` / `_UnknownWidget` / `_NonBoolRequired`

### 8.5 E2E smoke(Day 6 收尾)

1. S1.5 6 路径全绿(回归)
2. **构造 demo**:Owner 进 `/owner/tasks/1/templates` → 看到 v1 → 进 Designer(edit)→ 点 Radio 卡片 → 改 name → Save → URL 跳 v2 → Plaza 切 labeler1 看到新字段
3. **历史只读 demo**:Owner 进 v1 → readonly,toolbar=Fork → 点 Fork → 创建 v3(v1 副本)→ URL 跳 v3 edit 模式
4. **schema 错误 demo**:`UPDATE task_templates SET schema_json='broken' WHERE id=X` → 刷新 Plaza → SchemaErrorBanner 出现
5. **跨字段 rename 不丢选中 demo**:append Radio → 选中 → 改 name 5 次 → 选中态保持(`_draftId` 工作)
6. **Seed mutation demo**:改 `qa_quality_review.json` 一个 label → `make seed` → Plaza 立即看到新 label

## 9. Sprint 2 day-by-day(6 天)

| Day | 主体 | 关键产物 | 验证门 |
|---|---|---|---|
| **1** | Backend 4 路由 + seed upsert + 单测 | `s1_template.go` + `tasks.template_id` 同事务更新 + `isLatest` + seed sha256 upsert | go test:每路由 1 happy + 1 边界(POST 撞 409)+ seed mutation 验证 |
| **2** | Vitest 落地 + Renderer 模块 + 5 简单 widgets(Input/TextArea/Radio/Tags/ShowItem-with-path-mode)+ parser/validator | `apps/web/src/renderer/` 框架 + `parseTemplateSchema` + qa_quality fixture 渲染 12 字段 | vitest 全绿;Plaza 暂未迁移 |
| **3** | 4 复杂 widgets(JSONEditor/FileUpload/RichText/LLMTrigger)+ Plaza/Queue 迁移(MEDIUM 4 闭环) | `LabelAnswer` interface 彻底删除;Plaza/Queue 通过 SchemaErrorBanner 处理坏 schema | E2E smoke 老 happy path + 坏 schema 边界 |
| **4** | Designer edit-mode 主体:DraftCanvas / CanvasFieldShell / WidgetPreview / MaterialPalette / useDesignerState | 物料库 → 画布 append + delete + 选中态 + Discard | vitest:append/delete/discard 行为 + 选中态保持 |
| **5** | PropertyPanel:CommonPropsForm + OptionsPropsForm + LengthPropsForm + 9 PropForm_X 组合 | 选中字段 → 改 name/label/required + Radio/Tags 改 options + Input 改 maxLength | vitest:每个 PropForm 渲染 + name 实时撞验证 |
| **6** | List 页 + Designer readonly/Fork mode + Save flow + E2E smoke + Demo 剧本 + commit | `/owner/tasks/:taskId/templates` 全链路 + `docs/S2_ACCEPTANCE.md` + `docs/S2_DEMO_SCRIPT.md` | 全套 E2E + go test + lint + build 全绿 |

**砍点**:
- Day 5 PropForm 组合超时 → 砍 LengthPropsForm,maxLength/minLength 推 S3 polish
- Day 6 List 页可降级为"在 task detail 页加 templates 子表",不开独立路由

## 10. 验收标准

1. ✓ Backend 4 路由 + GET 携带 `isLatest` + POST 同事务更新 `tasks.template_id` + 后端 duplicate-key 兜底 → sqlmock 全绿
2. ✓ Labeler/Plaza:schema 驱动,**qa_quality 12 字段全渲染**(field 数从 11 修正到 12)
3. ✓ Reviewer/Queue:readOnly + answer 来自 `revision.answer`
4. ✓ Owner/Designer **edit 模式**:**能 from 0 构造** — 点 Radio → 改 name `fluency_score` → 改 label → Save as v2 → URL 跳 v2 → Plaza 看到新字段
5. ✓ Owner/Designer **readonly 模式**(开旧版本):物料/删除/属性面板全 disabled + toolbar 替换为 Fork
6. ✓ Fork:点 Fork → POST 当前 schema 副本 → URL 跳 v(N+1) edit 模式
7. ✓ `_draftId` 工作:append → 选中 → 改 name 多次,选中态保持(Vitest 验)
8. ✓ ShowItem path+mode:接受 `field.path`(`$payload`)+ `field.mode`(`auto`/`text`/`video`/`image`/`markdown`/`json`)5 分支
9. ✓ SchemaErrorBanner:故意改坏 schema_json → banner 出现,角色分文案(labeler 看"联系任务管理员",owner 看具体 parse error)
10. ✓ Seed mutation:改 `qa_quality_review.json` 一个 label → `make seed` → Plaza 立即看到新 label
11. ✓ go test ./... -race / pnpm lint / pnpm build / vitest 全绿;handler cover ≥ 35%
12. ✓ Sprint 2 demo 剧本(`docs/S2_DEMO_SCRIPT.md`):6-8 步,每步标时长,合计 ≤ 4 分钟

## 11. Review findings 闭环

| # | Finding | 处理 |
|---|---|---|
| 1 | 字段数 11→12 | ✓ 验收 #2 + Day 6 E2E 都用 12 |
| 2 | Sprint 6 不承接 dnd | ✓ S2 v1 自带 click-to-append 编辑,Sprint 6 在原计划范围内补 Tabs/Group + 可选叠加 dnd-kit;master PLAN.md S6 不动 |
| 3 | 完成标准 root mismatch | ✓ S2 v1 ≥ master 80%(无 dnd / 无 meta-circular,有构造 + 版本化);spec §1 明示 |
| 4 | ShowItem path 解析 | ✓ Day 2 + 验收 #8 显式覆盖 |
| 5 | List 复制按钮 | ✓ Designer readonly 模式 toolbar 自带 Fork;List 页加 "Fork" 列按钮 |
| 6 | x-reactions / requiredWhen | 🟡 推迟到 Sprint 6 / S3 头,spec §12 明示 "S2 v1 不支持联动" |
| 7 | Demo 剧本 | ✓ Day 6 收尾产 `docs/S2_DEMO_SCRIPT.md` |
| 8 | SchemaErrorBanner 文案分角色 | ✓ 验收 #9 |

## 12. Out of scope(S2 v1 不做)

- dnd-kit 拖拽排序 → Sprint 6
- meta-circular 属性面板 → Sprint 6(若需要)
- Formily x-reactions / requiredWhen 联动 → Sprint 6 / S3
- TipTap 真 RichText 工具栏(S2 v1 用 TextArea 标记为 HTML)→ Sprint 6
- `packages/schema-spec` workspace package → 不做(S2 v1 只有 web 一个消费者)
- React Router 内部跳转拦截 → Sprint 6 polish
- `POST /templates/:id/preview-with-item` → Designer 自带 edit 预览,这个端点 S2 不需要
- monaco-editor 接入 JSONEditor → Sprint 6 polish
- FileUpload 文件预览 → Sprint 6
- LLMTrigger 自动写回 target 字段 → S3(配合真实豆包)

## 13. 分数预期

| 维度 | S1.5 后 | S2 v1 完成后 |
|---|---|---|
| 代码质量 | 8.5 | **8.8**(模块化 + Designer DRY 拆分 + Vitest 引入 + handler cover ↑) |
| 架构逻辑 | 8.5 | **9.2**(policy + outbox + statemachine + 模板版本化 + 后端事务兜底 5 大骨架齐) |
| 业务理解 | 9.0 | **9.2**(构造能力 + 版本化语义 + immutable submission 关联) |

冲第一名的下一档分数靠 S3(AI Agent 落产品 + golden dry-run + labeler calibration)。

## 14. 风险 & 退路

| 风险 | 退路 |
|---|---|
| Day 4-5 Designer 实现超时 | 砍 OptionsPropsForm(Radio/Tags 用纯文本 CSV 输入 options),Day 5 收 1 天 |
| Day 3 LLMTrigger 自动写回 target 字段复杂 | 降级:LLMTrigger 自身展示返回文本,不自动写回(Sprint 3 再补) |
| FileUpload 跨字段复杂 | S2 v1 字段值只存文件 ID 数组,不预览 |
| POST templates 同事务更新 `tasks.template_id` 死锁 | 锁顺序:先 task,再 templates(MySQL 行锁顺序一致避免死锁) |
| Vitest setup 比预期久 | Day 2 砍简单 widget 1 个,推 Day 3 头 |
