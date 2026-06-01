# HANDOFF — Designer 布局对齐（属性栏常驻右侧）

> 给一个**专门改模板搭建器（Designer）布局**的 session。目标:让 Designer 完全对齐组织方设计稿,核心是**属性配置面板常驻右侧第三栏**(点字段→右栏出现/更新),而不是掉到画布下面。
> 只改 Designer（`apps/web/src/modules/template/`）这一块,别动其它模块。改完跑测试 + 部署 + 浏览器在多个视口宽度验证。

部署:rsync 工作树 → `ubuntu@43.155.210.70:/home/ubuntu/labelhub/`（`ssh -i ~/Downloads/labelhub.pem`）→ `docker compose --env-file deploy/.env -f deploy/docker-compose.prod.yml up -d --build web`。线上 http://43.155.210.70。Designer 路由:`/owner/tasks/<taskId>/templates/<templateId>`(如 `/owner/tasks/1/templates/1`、`/owner/tasks/2/templates/2`)。

---

## 1. 设计稿（目标）

三栏布局,**始终三栏并排**:
```
┌──────────┬───────────────────────────┬──────────────────┐
│ 物料/布局 │ 画布(基础信息|标注|+新Tab) │ 属性配置          │
│ (左栏)    │ 字段卡 …                   │ 基础|校验|联动     │
│          │                            │ 字段名/标签/必填… │
└──────────┴───────────────────────────┴──────────────────┘
```
- 右栏「属性配置 · <字段名>」常驻,顶部有 `基础 / 校验 / 联动` 三个 tab,下面是该字段的属性表单(字段名、标签、必填、占位符、最大长度;校验规则:正则、自定义函数;字段联动规则)。
- **点中画布里的字段 → 右栏内容切换成该字段的属性**。右栏永远在右边,不下沉。

参考截图见用户消息（image #5,商品清洗 v3 模板）。

---

## 2. 当前问题 + 根因（已定位）

**症状**:在 < 1600px 的视口(几乎所有笔记本:1280/1440),属性配置面板掉到**画布下方**(整宽),不在右侧。

**根因**:`apps/web/src/modules/template/Designer.css`
```css
.template-designer-grid {                /* L6 */
  grid-template-columns: 200px minmax(420px, 1fr) 320px;   /* 三栏:物料|画布|属性 */
}
@media (max-width: 1599px) {             /* L136 ← 元凶,断点太高 */
  .template-designer-grid { grid-template-columns: 180px minmax(0, 1fr); }  /* 砍成两栏 */
  .template-designer-property { grid-column: 1 / -1; }     /* 属性栏被甩到整宽(下方) */
}
@media (max-width: 768px) { … 单列堆叠 … }   /* L146,这个保留 */
```
即 1600px 以下就强制两栏 + 属性下沉。绝大多数演示设备是 1280-1440 → 永远下沉。

外层还有全局 App 侧栏(`.lh-shell__side` 220px)占宽,所以 Designer 实际可用宽 ≈ 视口 - 220 - padding。

---

## 3. 要做的改动

目标:**三栏在常见桌面宽度(≥~1100px Designer 可用宽,即视口 ≥~1280)保持并排**,只有真正窄屏(平板/手机)才堆叠。

建议(任选其一,优先 a）：
- **(a) 降低断点 + 收窄列宽**(最简单):
  - 基础三栏列宽收窄一点,让 3 栏在更窄处也放得下,例如:
    `grid-template-columns: 180px minmax(360px, 1fr) 300px;`
  - 把 `@media (max-width: 1599px)` 整段**改成 `@media (max-width: 1024px)`**(或 1100),即只有 ≤1024 才两栏 + 属性下沉;1280/1440/1920 都保持三栏。
  - `@media (max-width: 768px)` 单列堆叠保留不动。
- (b) 若想属性栏在窄屏也"在右边",可改成抽屉/浮层(右侧 drawer),但 demo 用 (a) 足够,别过度设计。

注意:右栏 `.template-designer-property` 内已有 `PropertyPanel` 组件,**它本身已实现 `基础/校验/联动` 三个 tab**(`PropertyTab='basic'|'validation'|'logic'`,见 `Designer.tsx` 内 `PropertyPanel`/`PropertyTabButton`)——**不用重写,只需让它待在右栏**。点字段 → `setSelectedId` → `PropertyPanel` 已经会显示该字段属性。所以这件事**本质就是 CSS 断点 + 列宽**,不是大改。

---

## 4. 视觉对齐 checklist（次要,布局修好后按需对齐设计稿）

- [ ] 字段卡右上控件:设计稿是**紧凑图标按钮**(↑ ↓ 复制 ✕),当前是 `↑/↓` + 文字「复制」「删除」。可改成图标小按钮一排,更接近稿。
- [ ] 字段卡:`字段名 + 标签徽章` 一行、`字段名: xxx · 类型` 次行、选项 chips、必填 `*` 红星——大致已有,微调间距/字号即可。
- [ ] 顶栏:面包屑 + `预览 / 导出 Schema JSON / 保存并发布版本 rN` + 版本/绑定任务徽章 + 头像——**已对齐,别动**。
- [ ] 右栏标题「属性配置 · <字段名>」:当前是「属性」,可补上 ` · 字段名`。

---

## 5. 已完成（别重做,本会话已上线）

- 左栏物料:`物料 / 布局` 分组 + **带底色圆角图标盒**(蓝/紫/灰),已修好之前图标文字糊在一起的 bug(`.palette-item__icon` 在 `Designer.css`)。
- 画布顶部 `基础信息 | 判定/标注… | + 新 Tab`:**干净下划线 tab**(已修好之前的方框 bug,`.canvas-tab` 在 `styles/lh/designer.css`),且**点击会切换画布内容**:
  - `基础信息` = 全部顶层字段(可编辑)。
  - 内容 tab = 该分页 tab 的字段(**只读预览**,`CanvasField` 的 `readOnly` 模式;编辑该 tab 字段目前走右侧选中的「分页组」属性面板)。
  - 相关逻辑:`Designer.tsx` 里 `activeCanvasTab` / `baseCanvasFields` / `activeTabFields` / `CanvasField` 的 `readOnly` 入参。
- 顶栏、版本/只读态、保存/Fork 流程都正常。

(可选后续,本次不强制)内容 tab 内的**完整内联编辑**(拖拽/增删嵌套字段)——目前是只读预览。要做需把 `appendField/deleteField/copyField/moveField/handleCanvasDrop/updateSelected` 改成"对当前激活 tab 的字段数组"操作(现都只操作顶层 `fields`)。属于较大改动,布局修完若有余力再做,务必加测试。

---

## 6. 验收标准

1. 视口 **1280 / 1440 / 1920** 下,Designer 都是**三栏并排**,属性配置在**最右**,不在画布下方。
2. 点画布里任一字段 → 右栏「属性配置」立即显示/更新成该字段的 `基础/校验/联动` 表单。
3. ≤768px 时才单列堆叠(允许)。
4. 与设计稿(image #5)视觉基本一致。
5. 控制台无 error。

验证方式:部署后用浏览器(chrome-devtools MCP)在三个视口宽度截图;注意先 `localStorage` 放 `labelhub_access_token`+`labelhub_current_user`(POST `/api/v1/auth/login` owner1/123456),再开 Designer 路由;**用完关页面**。

---

## 7. 工程约定（必读）

- 改完必须跑:`pnpm -F web build && pnpm -F web lint && pnpm -F web test`(当前 173 测试全绿,别引入回归)。注意 `Designer.integration.test.tsx` 会从画布选中 `tabs_1` 等字段——别再把分页组排除出基础信息画布(否则该测试挂)。
- 只改 `apps/web/src/modules/template/Designer.tsx` 与 `Designer.css`(布局核心),必要时 `styles/lh/designer.css`(tab 样式)。别动其它模块、别动后端。
- 提交:约定式前缀(`fix(designer): …`)、无 AI 署名;在 `docs/CHANGELOG.md`「最近完成」加一条。
- 部署见本文顶部;部署 `web` 即可(纯前端)。
- 设计 token 用 `--lh-*` / `--color-*`,别硬编码颜色。
