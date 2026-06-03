import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type MutableRefObject, type ReactNode } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Button, Toast } from '@douyinfe/semi-ui'
import { Parser as ExprParser } from 'expr-eval'
import {
  DndContext,
  DragOverlay,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useDraggable,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
} from '@dnd-kit/core'
import {
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { apiGet, apiPost, type TaskTemplate } from '../../shared/api/client'
import SchemaErrorBanner from '../../renderer/components/SchemaErrorBanner'
import SchemaRenderer from '../../renderer/SchemaRenderer'
import { parseTemplateSchema } from '../../renderer/parser'
import type { AnswerValue } from '../../renderer/types'
import { widgetRegistry } from '../../renderer/widgets'
import { showItemModes, widgetTypes, type FieldOption, type FieldSchema, type RenderPayload, type ShowItemMode, type TabSchema, type TemplateSchema, type VisibleWhen, type WidgetType } from '../../renderer/types'
import { Icon } from '../../shared/components/Icon'
import '../../styles/lh/designer.css'
import './Designer.css'

type TemplateDetailResponse = {
  template: TaskTemplate
  isLatest: boolean
  latestTemplateId: number
}

type PreviewItemResponse = {
  item: PreviewItem | null
}

type PreviewItem = {
  id: number
  externalId: string | null
  payload: unknown
}

type DraftField = FieldSchema & {
  _draftId: string
}

type DraftValidationError = {
  field: string
  message: string
  draftId?: string
}

const widgetLabels: Record<WidgetType, string> = {
  ShowItem: '展示素材',
  Group: '字段组',
  Tabs: '分页组',
  Input: '单行输入',
  TextArea: '多行文本',
  Radio: '单选',
  Tags: '标签多选',
  RichText: '富文本',
  JSONEditor: 'JSON',
  FileUpload: '文件上传',
  LLMTrigger: 'LLM 触发',
}

const widgetPrefixes: Record<WidgetType, string> = {
  ShowItem: 'show_item',
  Group: 'group',
  Tabs: 'tabs',
  Input: 'input',
  TextArea: 'text_area',
  Radio: 'radio',
  Tags: 'tags',
  RichText: 'rich_text',
  JSONEditor: 'json_editor',
  FileUpload: 'file_upload',
  LLMTrigger: 'llm_trigger',
}

const widgetIcons: Record<WidgetType, string> = {
  ShowItem: '◎',
  Group: '[ ]',
  Tabs: 'T',
  Input: 'Aa',
  TextArea: '¶',
  Radio: '◉',
  Tags: '#',
  RichText: 'R',
  JSONEditor: '{}',
  FileUpload: '↑',
  LLMTrigger: '✦',
}

const paletteGroups: Array<{ label: string, widgets: WidgetType[] }> = [
  { label: '物料', widgets: widgetTypes.filter((widget) => widget !== 'Group' && widget !== 'Tabs') },
  { label: '布局', widgets: ['Group', 'Tabs'] },
]

const nestedWidgetTypes = widgetTypes.filter((widget) => widget !== 'Group' && widget !== 'Tabs')

// Widgets that hold an answer value: visibleWhen / customRule are only meaningful on these.
const advancedConfigWidgets: WidgetType[] = ['Input', 'TextArea', 'Radio', 'Tags', 'RichText', 'JSONEditor', 'FileUpload']

// @dnd-kit drag sources. A palette drag INSERTS a new widget; a canvas drag REORDERS.
// active.data.current.source distinguishes them in onDragEnd.
type PaletteDragData = { source: 'palette', widget: WidgetType }
type CanvasDragData = { source: 'canvas', draftId: string }
type DragData = PaletteDragData | CanvasDragData

// id used by the canvas droppable so empty-canvas / below-last-field drops still land.
const CANVAS_DROPPABLE_ID = 'canvas-droppable'

// Shared parser used only to surface a non-blocking parse hint for customRule expr in the panel.
const designerExprParser = new ExprParser()

export default function TemplateDesigner() {
  const { taskId, templateId } = useParams()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  // isNew:templateId === 'new' 进入空白新建态,不拉取已有模板,保存即创建该任务的第一个版本。
  const isNew = templateId === 'new'
  // copyFromTaskId:新建态可带 ?copyFrom=<源任务 id>,从该任务最新模板克隆 schema 预填画布(仍是新建态)。
  const copyFromTaskId = isNew ? searchParams.get('copyFrom') : null
  const numericTaskId = Number(taskId)
  // 新建态用 0 占位(而非 NaN),让 isCurrentRoute 的 templateId 比较稳定;loadTemplate 在 isNew 分支提前返回,不会触发 <=0 无效判定。
  const numericTemplateId = isNew ? 0 : Number(templateId)
  const [template, setTemplate] = useState<TaskTemplate | null>(null)
  const [schema, setSchema] = useState<TemplateSchema | null>(null)
  const [title, setTitle] = useState('')
  const [fields, setFields] = useState<DraftField[]>([])
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [isLatest, setIsLatest] = useState(true)
  const [latestTemplateId, setLatestTemplateId] = useState<number | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [schemaError, setSchemaError] = useState<{ field: string, message: string } | null>(null)
  const [taskMismatch, setTaskMismatch] = useState(false)
  const [previewItem, setPreviewItem] = useState<PreviewItem | null>(null)
  const [previewLoading, setPreviewLoading] = useState(false)
  const [previewError, setPreviewError] = useState('')
  // activeDrag:当前正在拖拽的来源(palette 新增 / canvas 重排),驱动 DragOverlay 预览与卡片半透明态。
  const [activeDrag, setActiveDrag] = useState<DragData | null>(null)
  const [activeCanvasTab, setActiveCanvasTab] = useState('base')
  const [showSchemaPreview, setShowSchemaPreview] = useState(false)
  // 预览面板有两种子模式:原始 JSON / 表单预览(标注员看到的最终可填写表单)。
  const [previewMode, setPreviewMode] = useState<'json' | 'form'>('json')
  // previewAnswer:表单预览里临时填写的答案,仅本地 state,不提交。
  const [previewAnswer, setPreviewAnswer] = useState<AnswerValue>({})
  const loadSeq = useRef(0)
  const canvasRef = useRef<HTMLDivElement | null>(null)
  const routeRef = useRef({ taskId: numericTaskId, templateId: numericTemplateId })

  useEffect(() => {
    routeRef.current = { taskId: numericTaskId, templateId: numericTemplateId }
  }, [numericTaskId, numericTemplateId])

  const loadTemplate = useCallback(async () => {
    const requestSeq = loadSeq.current + 1
    loadSeq.current = requestSeq
    const routeTaskId = numericTaskId
    const routeTemplateId = numericTemplateId
    const isCurrentLoad = () => loadSeq.current === requestSeq

    if (isNew) {
      // 复制模式:从源任务最新模板克隆 schema 预填画布;克隆失败则退回空白新建态。仍是新建态,保存走 POST 创建本任务首版。
      if (copyFromTaskId) {
        setLoading(true)
        try {
          const list = await apiGet<TaskTemplate[]>(`/tasks/${copyFromTaskId}/templates`)
          if (!isCurrentLoad()) return
          const source = list[0]
          const parsed = source ? parseTemplateSchema(source.schemaJson) : null
          if (parsed && parsed.ok) {
            const draftFields = parsed.value.fields.map((field, index) => attachDraftIdsToField(field, `${field.name}-${index}`))
            setTemplate(null)
            setSchema(parsed.value)
            setTitle(parsed.value.title)
            setFields(draftFields)
            setSelectedId(draftFields[0]?._draftId ?? null)
            setIsLatest(true)
            setLatestTemplateId(null)
            setSchemaError(null)
            setTaskMismatch(false)
            setError('')
            setLoading(false)
            return
          }
        } catch {
          if (!isCurrentLoad()) return
          // 克隆失败:落到下方空白新建态。
        }
      }
      // 新建模式:跳过拉取,初始化一张空白可编辑模板,保存时走 POST /tasks/:id/templates 创建首版。
      setTemplate(null)
      setSchema({ title: '', layout: 'single_page', fields: [] })
      setTitle('')
      setFields([])
      setSelectedId(null)
      setIsLatest(true)
      setLatestTemplateId(null)
      setSchemaError(null)
      setTaskMismatch(false)
      setError('')
      setLoading(false)
      return
    }

    if (!Number.isFinite(routeTemplateId) || routeTemplateId <= 0) {
      setTemplate(null)
      setSchema(null)
      setTitle('')
      setFields([])
      setSelectedId(null)
      setLatestTemplateId(null)
      setSchemaError(null)
      setTaskMismatch(true)
      setError('template id 无效')
      setLoading(false)
      return
    }
    setLoading(true)
    setError('')
    setSchemaError(null)
    setTaskMismatch(false)
    try {
      const data = await apiGet<TemplateDetailResponse>(`/templates/${routeTemplateId}`)
      if (!isCurrentLoad()) return
      setTemplate(data.template)
      setIsLatest(data.isLatest)
      setLatestTemplateId(data.latestTemplateId)
      if (Number(data.template.taskId) !== routeTaskId) {
        setSchema(null)
        setTitle('')
        setFields([])
        setSelectedId(null)
        setTaskMismatch(true)
        setError('模板不属于当前 URL 中的 task，已禁止保存和 Fork。')
        return
      }

      const parsed = parseTemplateSchema(data.template.schemaJson)
      if (!parsed.ok) {
        setSchema(null)
        setTitle('')
        setFields([])
        setSelectedId(null)
        setSchemaError(parsed.error)
        return
      }
      const draftFields = parsed.value.fields.map((field, index) => attachDraftIdsToField(field, `${field.name}-${index}`))
      setSchema(parsed.value)
      setTitle(parsed.value.title)
      setFields(draftFields)
      setSelectedId(draftFields[0]?._draftId ?? null)
    } catch (error) {
      if (!isCurrentLoad()) return
      setTemplate(null)
      setSchema(null)
      setTitle('')
      setFields([])
      setSelectedId(null)
      setLatestTemplateId(null)
      setSchemaError(null)
      setTaskMismatch(true)
      setError(error instanceof Error ? error.message : '加载模板失败')
    } finally {
      if (isCurrentLoad()) {
        setLoading(false)
      }
    }
  }, [numericTaskId, numericTemplateId, isNew, copyFromTaskId])

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void loadTemplate()
  }, [loadTemplate])

  useEffect(() => {
    let cancelled = false
    const routeTaskId = numericTaskId
    if (!Number.isFinite(routeTaskId) || routeTaskId <= 0) {
      Promise.resolve().then(() => {
        if (cancelled) return
        setPreviewItem(null)
        setPreviewError('')
        setPreviewLoading(false)
      })
      return () => {
        cancelled = true
      }
    }
    Promise.resolve()
      .then(() => {
        if (cancelled) return null
        setPreviewLoading(true)
        setPreviewError('')
        return apiGet<PreviewItemResponse>(`/tasks/${routeTaskId}/item-preview`)
      })
      .then((data) => {
        if (cancelled || !data) return
        setPreviewItem(data.item ?? null)
      })
      .catch((error) => {
        if (cancelled) return
        setPreviewItem(null)
        setPreviewError(error instanceof Error ? error.message : '加载预览样本失败')
      })
      .finally(() => {
        if (!cancelled) {
          setPreviewLoading(false)
        }
      })
    return () => {
      cancelled = true
    }
  }, [numericTaskId])

  const selectedField = fields.find((field) => field._draftId === selectedId) ?? null
  const previewPayloadResult = useMemo(() => parsePreviewPayload(previewItem), [previewItem])
  const validationErrors = useMemo(() => validateDraftFields(fields), [fields])
  const validationErrorsByDraftId = useMemo(() => groupValidationErrorsByDraftId(validationErrors), [validationErrors])
  const canEdit = isLatest && !schemaError && !taskMismatch
  const saveDisabled = saving || !canEdit || validationErrors.length > 0 || fields.length === 0
  // 表单预览:把当前设计的 schema 走渲染引擎解析,渲成标注员真正会看到的可填写表单。
  // 配置非法(校验未通过)时 parse 仍可能成功,但若结构不可解析则给出提示。
  const previewSchemaResult = useMemo(
    () => parseTemplateSchema(JSON.stringify(buildTemplatePayload(title, fields, schema))),
    [title, fields, schema],
  )

  // focusFirstError:校验未通过时,选中并滚动到首个出错字段,形成「校验不通过→回到配置」闭环。
  // 返回是否存在错误(供保存前拦截判断)。
  const focusFirstError = useCallback(() => {
    if (validationErrors.length === 0) return false
    const targetDraftId = validationErrors.find((item) => item.draftId)?.draftId
    if (!targetDraftId) return true
    // 出错字段若在某个分页 Tab 内,先切回基础信息画布(顶层字段都在那渲染),保证目标卡片可见。
    setActiveCanvasTab('base')
    setSelectedId(targetDraftId)
    // 等画布按新选中态重渲染后再滚动定位。
    window.requestAnimationFrame(() => {
      const node = canvasRef.current?.querySelector(`[data-draft-id="${targetDraftId}"]`)
      if (node instanceof HTMLElement) {
        node.scrollIntoView({ behavior: 'smooth', block: 'center' })
      }
    })
    return true
  }, [validationErrors])

  function handleSaveClick() {
    // 出现校验错误时,先把用户带回首个出错字段,而不是静默禁用按钮。
    if (focusFirstError()) return
    void saveTemplate()
  }
  const canvasTabsField = fields.find((field) => field.widget === 'Tabs') ?? null
  // 画布按子导航 tab 切换内容:基础信息=全部顶层字段(可编辑,与原行为一致,分页组仍在此可选中编辑);
  // 内容 tab=该分页 tab 的字段(只读预览,编辑走右侧已选中的分页组属性面板)。
  const isBaseCanvasTab = activeCanvasTab === 'base'
  const activeTabIndex = isBaseCanvasTab ? -1 : Number(activeCanvasTab.replace('tab-', ''))
  const baseCanvasFields = fields
  const activeTabFields = (!isBaseCanvasTab && canvasTabsField ? (canvasTabsField.tabs?.[activeTabIndex]?.fields ?? []) : []) as DraftField[]

  function appendField(widget: WidgetType) {
    if (!canEdit) return
    const field = createDefaultField(widget, fields)
    setFields((current) => [...current, field])
    setSelectedId(field._draftId)
  }

  function deleteField(fieldId: string) {
    if (!canEdit) return
    setFields((current) => {
      const index = current.findIndex((field) => field._draftId === fieldId)
      if (index < 0) return current
      const next = current.filter((field) => field._draftId !== fieldId)
      if (selectedId === fieldId) {
        setSelectedId(next[index]?._draftId ?? next[index - 1]?._draftId ?? null)
      }
      return next
    })
  }

  function copyField(fieldId: string) {
    if (!canEdit) return
    const index = fields.findIndex((field) => field._draftId === fieldId)
    if (index < 0) return
    const copy = cloneDraftField(fields[index], fields)
    setFields([...fields.slice(0, index + 1), copy, ...fields.slice(index + 1)])
    setSelectedId(copy._draftId)
  }

  function moveField(fieldId: string, direction: -1 | 1) {
    if (!canEdit) return
    setFields((current) => reorderFieldByOffset(current, fieldId, direction))
  }

  function insertWidgetBeforeTarget(widget: WidgetType, targetFieldId: string | null) {
    if (!canEdit) return
    setFields((current) => {
      const field = createDefaultField(widget, current)
      const targetIndex = targetFieldId === null ? current.length : current.findIndex((item) => item._draftId === targetFieldId)
      const insertIndex = targetIndex < 0 ? current.length : targetIndex
      const next = [...current.slice(0, insertIndex), field, ...current.slice(insertIndex)]
      setSelectedId(field._draftId)
      return next
    })
  }

  // 拖拽传感器:Pointer 带 8px 触发距离阈值,避免点击物料按钮(追加字段)被误判为拖拽;
  // Keyboard 接 sortable 的方向键坐标算法,提供键盘可达性(本次迁移的主要收益)。
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 8 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  function handleDragStart(event: DragStartEvent) {
    if (!canEdit) return
    const data = event.active.data.current as DragData | undefined
    setActiveDrag(data ?? null)
  }

  // onDragEnd 用 active.data.current.source 区分两类拖拽:
  // - palette:在 over 目标字段前插入新 widget(over 为画布占位/空白时追加到末尾)。
  // - canvas:把被拖字段移动到 over 目标字段的位置(reorderFieldsToTarget 保留「插到目标前」语义)。
  function handleDragEnd(event: DragEndEvent) {
    setActiveDrag(null)
    if (!canEdit) return
    const data = event.active.data.current as DragData | undefined
    if (!data) return
    const overId = event.over?.id
    if (data.source === 'palette') {
      const targetFieldId = !overId || overId === CANVAS_DROPPABLE_ID ? null : String(overId)
      if (widgetTypes.includes(data.widget)) {
        insertWidgetBeforeTarget(data.widget, targetFieldId)
      }
      return
    }
    // canvas 重排:over 落在另一张顶层字段卡上才移动,落到自身/画布空白不动。
    if (!overId || overId === CANVAS_DROPPABLE_ID || overId === data.draftId) return
    setFields((current) => reorderFieldsToTarget(current, data.draftId, String(overId)))
  }

  function handleDragCancel() {
    setActiveDrag(null)
  }

  function updateSelected(patch: Partial<FieldSchema>) {
    if (!canEdit || !selectedField) return
    setFields((current) => current.map((field) => (
      field._draftId === selectedField._draftId ? normalizeDraftField({ ...field, ...patch }) : field
    )))
  }

  function discardChanges() {
    if (!schema) return
    const draftFields = schema.fields.map((field, index) => attachDraftIdsToField(field, `${field.name}-${index}`))
    setTitle(schema.title)
    setFields(draftFields)
    setSelectedId(draftFields[0]?._draftId ?? null)
    setError('')
  }

  function selectCanvasTab(index: number) {
    if (!canvasTabsField) return
    setActiveCanvasTab(`tab-${index}`)
    setSelectedId(canvasTabsField._draftId)
  }

  function addCanvasTab() {
    if (!canEdit) return
    if (!canvasTabsField) {
      const tabsField = createDefaultField('Tabs', fields)
      setFields((current) => [...current, tabsField])
      setSelectedId(tabsField._draftId)
      setActiveCanvasTab('tab-0')
      return
    }
    const nextIndex = (canvasTabsField.tabs?.length ?? 0) + 1
    const nextTab = attachDraftIdsToTab({
      label: `Tab ${nextIndex}`,
      fields: [createNestedDefaultField(nextNestedFieldName(fields, `${canvasTabsField.name}_tab${nextIndex}`, 'Input'), 'Input')],
    })
    setFields((current) => current.map((field) => (
      field._draftId === canvasTabsField._draftId
        ? normalizeDraftField({ ...field, tabs: [...(field.tabs ?? []), nextTab] })
        : field
    )))
    setSelectedId(canvasTabsField._draftId)
    setActiveCanvasTab(`tab-${nextIndex - 1}`)
  }

  function exportSchemaJSON() {
    const blob = new Blob([JSON.stringify(buildTemplatePayload(title, fields, schema), null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = `task-${numericTaskId}-template-r${template?.version ?? 1}.json`
    anchor.click()
    URL.revokeObjectURL(url)
  }

  function isCurrentRoute(routeTaskId: number, routeTemplateId: number) {
    return routeRef.current.taskId === routeTaskId && routeRef.current.templateId === routeTemplateId
  }

  async function saveTemplate() {
    if (!numericTaskId || saveDisabled || taskMismatch) return
    const routeTaskId = numericTaskId
    const routeTemplateId = numericTemplateId
    setSaving(true)
    setError('')
    try {
      const body = buildTemplatePayload(title, fields, schema)
      const created = await apiPost<TaskTemplate>(`/tasks/${routeTaskId}/templates`, body)
      if (!isCurrentRoute(routeTaskId, routeTemplateId)) return
      Toast.success(`模板 v${created.version ?? ''} 已保存`)
      navigate(`/owner/tasks/${routeTaskId}/templates/${created.id}`)
    } catch (error) {
      if (!isCurrentRoute(routeTaskId, routeTemplateId)) return
      setError(error instanceof Error ? error.message : '保存模板失败')
    } finally {
      setSaving(false)
    }
  }

  async function forkTemplate() {
    if (!numericTaskId || !schema || fields.length === 0 || taskMismatch) return
    const routeTaskId = numericTaskId
    const routeTemplateId = numericTemplateId
    setSaving(true)
    setError('')
    try {
      const created = await apiPost<TaskTemplate>(`/tasks/${routeTaskId}/templates`, buildTemplatePayload(title, fields, schema))
      if (!isCurrentRoute(routeTaskId, routeTemplateId)) return
      Toast.success(`已 Fork 为 v${created.version ?? ''}`)
      navigate(`/owner/tasks/${routeTaskId}/templates/${created.id}`)
    } catch (error) {
      if (!isCurrentRoute(routeTaskId, routeTemplateId)) return
      setError(error instanceof Error ? error.message : 'Fork 模板失败')
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return <div style={pageStyle}>加载模板...</div>
  }

  return (
    <div style={pageStyle}>
      <div className="template-designer-toolbar" style={{ ...toolbarStyle, background: 'var(--color-surface)', padding: 'var(--space-lg) var(--space-xl)', borderRadius: 'var(--radius-lg)', boxShadow: 'var(--shadow-sm)', border: '1px solid var(--color-border-light)' }}>
        <div>
          <div className="template-designer-breadcrumb">
            <span>任务负责人后台</span>
            <span>/</span>
            <Link to={`/owner/tasks/${numericTaskId}/templates`}>模板搭建</Link>
            <span>/</span>
            <span>T-{numericTaskId} · {isNew ? '新建' : `r${template?.version ?? '-'}`}</span>
          </div>
          <h1 style={{ ...headingStyle, marginTop: 'var(--space-sm)' }}>模板搭建器（Designer）</h1>
          <div className="template-designer-subtitle">拖拽物料、配置联动与校验规则，发布后由标注工作台直接消费。</div>
          <div className="template-designer-version-row">
             <span className="designer-version">{isNew ? '新建模板' : `当前版本 r${template?.version ?? '-'}`}</span>
             <span className="designer-task-link">绑定任务 T-{numericTaskId}</span>
             <span className={isLatest ? 'template-designer-status template-designer-status--latest' : 'template-designer-status template-designer-status--readonly'}>{isLatest ? 'LATEST / EDITABLE' : 'READONLY'}</span>
          </div>
        </div>
        <div style={toolbarActionsStyle}>
          {!isLatest && latestTemplateId ? (
            <Link to={`/owner/tasks/${numericTaskId}/templates/${latestTemplateId}`} style={latestTemplateLinkStyle}>查看最新版本</Link>
          ) : null}
          <Button aria-label="预览 Schema" onClick={() => setShowSchemaPreview((current) => !current)} theme="light">预览</Button>
          <Button aria-label="导出 Schema JSON" onClick={exportSchemaJSON} theme="light">导出 Schema JSON</Button>
          {taskMismatch ? null : canEdit ? (
            <>
              <Button aria-label="Discard" disabled={saving} onClick={discardChanges} theme="light">重置修改</Button>
              {/* 仅校验未通过时不禁用按钮,改为点击时聚焦首个出错字段(闭环);其它阻断条件仍禁用。 */}
              <Button aria-label="Save as new version" disabled={saving || !canEdit || fields.length === 0} loading={saving} theme="solid" onClick={handleSaveClick}>保存并发布版本 r{(template?.version ?? 0) + 1}</Button>
            </>
          ) : (
            <Button aria-label="Fork as new version" disabled={saving || !schema} loading={saving} theme="solid" onClick={() => void forkTemplate()}>Fork 为新版本</Button>
          )}
          <span className="template-designer-avatar" aria-label="当前用户">O</span>
        </div>
      </div>

      {error ? <div role="alert" style={{ ...alertStyle, margin: 'var(--space-md) 0' }}>{error}</div> : null}
      {schemaError ? <div style={{ margin: 'var(--space-md) 0' }}><SchemaErrorBanner error={schemaError} role="owner" /></div> : null}
      {validationErrors.length > 0 ? (
        <div role="alert" style={{ ...alertStyle, margin: 'var(--space-md) 0', background: '#fff1f0' }}>
          <div style={{ fontWeight: 600, marginBottom: 4 }}>存在配置错误 ({validationErrors.length}):</div>
          {validationErrors.map((item) => <div key={`${item.field}-${item.message}`} style={{ fontSize: 13 }}>• {item.field}: {item.message}</div>)}
        </div>
      ) : null}
      {showSchemaPreview ? (
        <div aria-label="schema preview panel" style={{ margin: 'var(--space-md) 0' }}>
          <div role="tablist" aria-label="preview mode" className="canvas-tabs" style={{ marginBottom: 'var(--space-sm)' }}>
            <button type="button" role="tab" aria-selected={previewMode === 'json'} className={`canvas-tab${previewMode === 'json' ? ' canvas-tab--active' : ''}`} onClick={() => setPreviewMode('json')}>原始 JSON</button>
            <button type="button" role="tab" aria-selected={previewMode === 'form'} className={`canvas-tab${previewMode === 'form' ? ' canvas-tab--active' : ''}`} onClick={() => setPreviewMode('form')}>表单预览</button>
          </div>
          {previewMode === 'json' ? (
            <pre aria-label="schema preview" style={jsonPreviewStyle}>{JSON.stringify(buildTemplatePayload(title, fields, schema), null, 2)}</pre>
          ) : previewSchemaResult.ok ? (
            <div aria-label="form preview" style={formPreviewStyle}>
              <div className="lh-muted lh-text-13" style={{ marginBottom: 'var(--space-sm)' }}>
                标注员视角的最终表单(可填写预览,不会提交)。
              </div>
              <SchemaRenderer
                schema={previewSchemaResult.value}
                payload={previewPayloadResult.payload}
                value={previewAnswer}
                onChange={setPreviewAnswer}
              />
            </div>
          ) : (
            <div role="alert" style={{ ...alertStyle, background: '#fff1f0' }}>当前配置无法渲染为表单预览:{previewSchemaResult.error.message}</div>
          )}
        </div>
      ) : null}

      <DndContext
        sensors={sensors}
        collisionDetection={closestCenter}
        onDragStart={handleDragStart}
        onDragEnd={handleDragEnd}
        onDragCancel={handleDragCancel}
      >
        <div className="template-designer-grid" style={{ marginTop: 'var(--space-lg)' }}>
          <aside className="template-designer-palette" style={panelStyle}>
            <div style={{ borderBottom: '1px solid var(--color-border-light)', paddingBottom: 'var(--space-sm)', marginBottom: 'var(--space-sm)' }}>
              <h2 style={{ ...subHeadingStyle, fontSize: 'var(--text-base)' }}>组件物料</h2>
            </div>
            <div style={paletteStyle}>
              {paletteGroups.map((group) => (
                <section key={group.label} className="template-designer-palette-group">
                  <div className="palette-group__title">{group.label}</div>
                  {group.widgets.map((widget) => (
                    <PaletteItem
                      key={widget}
                      widget={widget}
                      disabled={!canEdit}
                      onAppend={() => appendField(widget)}
                    />
                  ))}
                </section>
              ))}
            </div>
          </aside>

          <main className="template-designer-canvas" style={{ ...panelStyle, minHeight: 360 }}>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-lg)' }}>
              <label style={fieldStyle}>
                <span style={{ fontWeight: 600 }}>模板名称</span>
                <input aria-label="template_title" disabled={!canEdit} value={title} onChange={(event) => setTitle(event.target.value)} style={inputStyle} placeholder="输入模板标题..." />
              </label>

              <div style={{ borderTop: '1px solid var(--color-border-light)', paddingTop: 'var(--space-lg)' }}>
                <div role="tablist" aria-label="canvas tabs" className="canvas-tabs">
                  <button type="button" role="tab" aria-selected={activeCanvasTab === 'base'} className={`canvas-tab${activeCanvasTab === 'base' ? ' canvas-tab--active' : ''}`} onClick={() => setActiveCanvasTab('base')}>基础信息</button>
                  {(canvasTabsField?.tabs ?? []).map((tab, index) => (
                    <button key={tab._draftId ?? `${tab.label}-${index}`} type="button" role="tab" aria-selected={activeCanvasTab === `tab-${index}`} className={`canvas-tab${activeCanvasTab === `tab-${index}` ? ' canvas-tab--active' : ''}`} onClick={() => selectCanvasTab(index)}>{tab.label}</button>
                  ))}
                  <button type="button" aria-label="新增画布 Tab" className="canvas-tab canvas-tab--add" disabled={!canEdit} onClick={addCanvasTab}>+ 新 Tab</button>
                  <span className="canvas-hint">拖拽字段卡调整顺序</span>
                </div>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 'var(--space-md)' }}>
                  <div style={{ fontWeight: 600 }}>画布区域 (Canvas)</div>
                  <PreviewItemStatus
                    item={previewItem}
                    loading={previewLoading}
                    loadError={previewError}
                    parseError={previewPayloadResult.error}
                  />
                </div>

                <CanvasDropZone canvasRef={canvasRef}>
                  {isBaseCanvasTab ? (
                    baseCanvasFields.length === 0 ? (
                      <EmptyCanvasDiagram />
                    ) : (
                      <SortableContext items={baseCanvasFields.map((field) => field._draftId)} strategy={verticalListSortingStrategy}>
                        {baseCanvasFields.map((field, index) => (
                          <CanvasField
                            key={field._draftId}
                            field={field}
                            errors={validationErrorsByDraftId.get(field._draftId) ?? []}
                            isFirst={index === 0}
                            isLast={index === baseCanvasFields.length - 1}
                            previewPayload={previewPayloadResult.payload}
                            selected={field._draftId === selectedId}
                            disabled={!canEdit}
                            onSelect={() => setSelectedId(field._draftId)}
                            onCopy={() => copyField(field._draftId)}
                            onDelete={() => deleteField(field._draftId)}
                            onMoveDown={() => moveField(field._draftId, 1)}
                            onMoveUp={() => moveField(field._draftId, -1)}
                          />
                        ))}
                      </SortableContext>
                    )
                  ) : activeTabFields.length === 0 ? (
                    <div style={{ padding: 'var(--space-xl)', textAlign: 'center', color: 'var(--color-text-muted)' }}>
                      该 Tab 暂无字段。在右侧「属性 · 分页组」中为此 Tab 添加字段。
                    </div>
                  ) : (
                    <>
                      <div style={{ marginBottom: 'var(--space-sm)', fontSize: 'var(--text-sm)', color: 'var(--color-text-secondary)' }}>
                        只读预览 · 编辑此 Tab 的字段请在右侧「属性 · 分页组」中操作
                      </div>
                      {activeTabFields.map((child, index) => (
                        <CanvasField
                          key={child._draftId ?? `${child.name}-${index}`}
                          field={child}
                          errors={[]}
                          isFirst={index === 0}
                          isLast={index === activeTabFields.length - 1}
                          previewPayload={previewPayloadResult.payload}
                          selected={false}
                          disabled
                          readOnly
                          onSelect={() => undefined}
                          onCopy={() => undefined}
                          onDelete={() => undefined}
                          onMoveDown={() => undefined}
                          onMoveUp={() => undefined}
                        />
                      ))}
                    </>
                  )}
                </CanvasDropZone>
              </div>
            </div>
          </main>

          <aside className="template-designer-property" style={panelStyle}>
            <PropertyPanel
              field={selectedField}
              errors={selectedField ? validationErrorsByDraftId.get(selectedField._draftId) ?? [] : []}
              fields={fields}
              disabled={!canEdit}
              onChange={updateSelected}
            />
          </aside>
        </div>

        {/* DragOverlay:拖拽时跟随指针/键盘焦点的预览,palette 显示物料名,canvas 显示字段名。 */}
        <DragOverlay dropAnimation={null}>
          {activeDrag ? (
            <div style={dragOverlayStyle}>
              {activeDrag.source === 'palette'
                ? `+ ${widgetLabels[activeDrag.widget]}`
                : (fields.find((field) => field._draftId === activeDrag.draftId)?.name ?? '字段')}
            </div>
          ) : null}
        </DragOverlay>
      </DndContext>
    </div>
  )
}

// 物料按钮:useDraggable 提供拖入画布的能力,onClick 仍负责「点击追加」(两者并存)。
// PointerSensor 的 8px 距离阈值保证小幅点击不会被吞成拖拽。
function PaletteItem({ widget, disabled, onAppend }: {
  widget: WidgetType
  disabled: boolean
  onAppend: () => void
}) {
  const data: PaletteDragData = { source: 'palette', widget }
  // Semi <Button> 的 ref 指向组件实例而非 DOM,@dnd-kit 需要 DOM 节点,故把拖拽 ref/监听器
  // 挂在外层 <div> 上;内层 Button 仍负责点击追加与视觉样式。
  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({
    id: `palette-${widget}`,
    data,
    disabled,
  })
  return (
    <div
      ref={setNodeRef}
      style={{ opacity: isDragging ? 0.5 : 1, cursor: disabled ? undefined : 'grab', touchAction: 'none' }}
      {...attributes}
      {...listeners}
    >
      <Button
        aria-label={`Add ${widget}`}
        className="palette-item"
        disabled={disabled}
        onClick={onAppend}
        theme="light"
      >
        <span className={`palette-item__icon${widget === 'LLMTrigger' ? ' palette-item__icon--purple' : widget === 'ShowItem' ? ' palette-item__icon--show' : ''}`}>{widgetIcons[widget]}</span>
        <span>{widgetLabels[widget]}</span>
      </Button>
    </div>
  )
}

// 画布放置区:useDroppable 让空画布与字段卡之间/末尾的空白也能接收 palette 拖入。
// 保留 canvasRef(focusFirstError 滚动定位与 data-draft-id 查询依赖它)。
function CanvasDropZone({ canvasRef, children }: {
  canvasRef: MutableRefObject<HTMLDivElement | null>
  children: ReactNode
}) {
  const { setNodeRef } = useDroppable({ id: CANVAS_DROPPABLE_ID })
  return (
    <div
      ref={(node) => {
        canvasRef.current = node
        setNodeRef(node)
      }}
      aria-label="canvas drop zone"
      style={canvasStyle}
    >
      {children}
    </div>
  )
}

function CanvasField({
  field,
  errors,
  isFirst,
  isLast,
  previewPayload,
  selected,
  disabled,
  readOnly = false,
  onSelect,
  onCopy,
  onDelete,
  onMoveDown,
  onMoveUp,
}: {
  field: DraftField
  errors: DraftValidationError[]
  isFirst: boolean
  isLast: boolean
  previewPayload: RenderPayload
  selected: boolean
  disabled: boolean
  readOnly?: boolean
  onSelect: () => void
  onCopy: () => void
  onDelete: () => void
  onMoveDown: () => void
  onMoveUp: () => void
}) {
  const Widget = widgetRegistry[field.widget]
  // Canvas stays compact like the org mockup (name/type/label per card); the live
  // widget preview is opt-in per card so a long template doesn't become a wall of
  // rendered controls. ShowItem is a display widget (its preview IS the source data
  // being labeled), so it stays expanded; input controls default collapsed. Full
  // WYSIWYG is still one click away via the 预览 button.
  const [showPreview, setShowPreview] = useState(field.widget === 'ShowItem')
  // useSortable:顶层可编辑字段用 _draftId 作为 sortable id 接入排序;
  // 只读(Tab 内预览)与禁用态不挂拖拽,listeners 仅绑在拖拽手柄上,不影响卡片内点击/编辑。
  const data: CanvasDragData = { source: 'canvas', draftId: field._draftId }
  const { attributes, listeners, setNodeRef, setActivatorNodeRef, transform, transition, isDragging } = useSortable({ id: field._draftId, data, disabled: readOnly || disabled })
  const sortableStyle: CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition: transition ?? undefined,
  }
  const baseStyle = isDragging ? draggingCanvasItemStyle : selected ? selectedCanvasItemStyle : canvasItemStyle
  return (
    <section
      ref={readOnly ? undefined : setNodeRef}
      aria-label={`canvas field ${field.name}`}
      data-draft-id={field._draftId}
      className={`template-designer-field canvas-field${selected ? ' canvas-field--selected' : ''}${field.widget === 'LLMTrigger' ? ' canvas-field--llm' : ''}${field.widget === 'ShowItem' ? ' canvas-field--show' : ''}`}
      style={{ ...baseStyle, ...sortableStyle }}
    >
      <span aria-hidden="true" style={leftPortStyle} />
      <span aria-hidden="true" style={rightPortStyle} />
      <div style={{ ...canvasItemHeaderStyle, background: selected ? 'var(--color-bg)' : '#fafafa' }}>
        <button type="button" aria-label={`select ${field.name}`} onClick={readOnly ? undefined : onSelect} style={selectFieldButtonStyle}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-sm)' }}>
            <strong style={{ fontSize: 'var(--text-base)', color: selected ? 'var(--color-accent)' : 'var(--color-text)' }}>{field.name}</strong>
            <span style={{ fontSize: 11, padding: '1px 6px', background: 'white', border: '1px solid var(--color-border-light)', borderRadius: 4, color: 'var(--color-text-muted)' }}>{widgetLabels[field.widget]}</span>
          </div>
          <span style={{ fontSize: 'var(--text-sm)', color: 'var(--color-text-secondary)', marginTop: 2 }}>{field.label}</span>
        </button>
        {readOnly ? null : (
          <div style={fieldActionsStyle}>
            <span ref={setActivatorNodeRef} style={{ display: 'inline-flex', cursor: disabled ? undefined : 'grab', touchAction: 'none' }} {...attributes} {...listeners}>
              <Button size="small" theme="light" disabled={disabled} aria-label={`drag ${field.name}`} icon={<span>⠿</span>} />
            </span>
            <div style={{ display: 'flex', background: 'white', border: '1px solid var(--color-border-light)', borderRadius: 'var(--radius-sm)' }}>
              <Button size="small" theme="borderless" disabled={disabled || isFirst} onClick={onMoveUp} aria-label={`move up ${field.name}`} icon={<Icon name="up" size={14} />} />
              <Button size="small" theme="borderless" disabled={disabled || isLast} onClick={onMoveDown} aria-label={`move down ${field.name}`} icon={<Icon name="down" size={14} />} />
            </div>
            <Button size="small" theme="light" disabled={disabled} onClick={onCopy} aria-label={`copy ${field.name}`} icon={<Icon name="copy" size={14} />} />
            <Button size="small" theme="light" disabled={disabled} onClick={onDelete} aria-label={`delete ${field.name}`} type="danger" icon={<Icon name="close" size={14} />} />
          </div>
        )}
      </div>
      {errors.length > 0 ? (
        <div aria-label={`validation ${field.name}`} style={fieldErrorListStyle}>
          {errors.map((item) => <div key={`${item.field}-${item.message}`} style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
            {item.field}: {item.message}
          </div>)}
        </div>
      ) : null}
      {(field.widget === 'Radio' || field.widget === 'Tags') && field.options?.length ? (
        <div className="canvas-options template-designer-option-row">
          {field.options.map((option) => <span key={option} className="canvas-option">{option}</span>)}
        </div>
      ) : null}
      {field.widget === 'Group' || field.widget === 'Tabs' ? (
        <NestedCanvasPreview field={field} />
      ) : (
        <div style={{ padding: '0 16px 12px' }}>
          <button
            type="button"
            aria-label={`toggle preview ${field.name}`}
            aria-expanded={showPreview}
            onClick={() => setShowPreview((value) => !value)}
            style={previewToggleStyle}
          >
            {showPreview ? '▾ 收起控件预览' : '▸ 预览控件'}
          </button>
          {showPreview ? (
            <div style={{ ...widgetPreviewStyle, opacity: selected ? 1 : 0.8, marginTop: 8 }}>
              <Widget
                field={field}
                value={field.widget === 'Tags' ? [] : ''}
                answer={{}}
                payload={previewPayload}
                readOnly
                onChange={() => undefined}
              />
            </div>
          ) : null}
        </div>
      )}
    </section>
  )
}

function NestedCanvasPreview({ field }: { field: DraftField }) {
  if (field.widget === 'Group') {
    return (
      <div style={nestedCanvasPreviewStyle} aria-label={`nested preview ${field.name}`}>
        {(field.fields ?? []).map((child, index) => <NestedCanvasRow key={`${child.name}-${index}`} child={child} />)}
      </div>
    )
  }
  return (
    <div style={nestedCanvasPreviewStyle} aria-label={`nested preview ${field.name}`}>
      {(field.tabs ?? []).map((tab, index) => (
        <div key={`${tab.label}-${index}`} style={nestedCanvasTabStyle}>
          <strong>{tab.label}</strong>
          <div style={nestedCanvasRowsStyle}>
            {tab.fields.map((child, childIndex) => <NestedCanvasRow key={`${child.name}-${childIndex}`} child={child} />)}
          </div>
        </div>
      ))}
    </div>
  )
}

const NESTED_INPUT_HINT: Record<string, string> = {
  Input: '单行输入…',
  Textarea: '多行文本…',
  RichText: '富文本编辑…',
  JSON: '{ } JSON 编辑器',
  FileUpload: '↑ 文件 / 图片',
}

// 嵌套子字段的控件预览:把真实控件/选项铺出来,对齐 demo 的「饱满卡」观感
function NestedFieldPreview({ child }: { child: FieldSchema }) {
  if ((child.widget === 'Radio' || child.widget === 'Tags') && child.options?.length) {
    return (
      <div className="canvas-options template-designer-option-row">
        {child.options.map((option) => <span key={option} className="canvas-option">{option}</span>)}
      </div>
    )
  }
  if (child.widget === 'ShowItem') {
    return (
      <div style={nestedShowItemBoxStyle}>
        {child.label || child.name} · 只读展示{child.path ? ` · ${child.path}` : ''}
      </div>
    )
  }
  if (child.widget === 'LLMTrigger') {
    return (
      <span style={nestedLlmBoxStyle}>
        <Icon name="sparkle" size={12} />
        LLM 触发组件
      </span>
    )
  }
  return <div style={nestedFieldBoxStyle}>{NESTED_INPUT_HINT[child.widget] ?? child.widget}</div>
}

function NestedCanvasRow({ child }: { child: FieldSchema }) {
  return (
    <div style={nestedCanvasRowStyle} aria-label={`nested field ${child.name}`}>
      <div style={nestedCanvasRowHeadStyle}>
        <span style={nestedCanvasWidgetStyle}>{child.widget}</span>
        <strong>{child.name}</strong>
        <span>{child.label}</span>
      </div>
      <NestedFieldPreview child={child} />
    </div>
  )
}

function EmptyCanvasDiagram() {
  return (
    <div style={emptyCanvasStyle}>
      <div style={emptyDiagramStyle} aria-hidden="true">
        <div style={{ ...emptyNodeStyle, gridColumn: '1 / 2' }} />
        <div style={emptyConnectorStyle} />
        <div style={{ ...emptyNodeStyle, gridColumn: '3 / 4' }} />
        <div style={{ ...emptyNodeStyle, gridColumn: '2 / 3', gridRow: '2 / 3', border: '1px solid var(--color-accent)' }} />
      </div>
      <div style={{ fontWeight: 600, color: 'var(--color-text)' }}>从左侧物料面板添加字段开始搭建</div>
      <div style={{ marginTop: 'var(--space-xs)', color: 'var(--color-text-muted)', fontSize: 'var(--text-sm)' }}>
        字段会以节点形式进入画布，并同步到右侧属性面板。
      </div>
    </div>
  )
}

function PreviewItemStatus({ item, loading, loadError, parseError }: {
  item: PreviewItem | null
  loading: boolean
  loadError: string
  parseError: string
}) {
  let text = '暂无导入样本, 使用示例 payload'
  if (loading) {
    text = '加载预览样本...'
  } else if (loadError) {
    text = `预览样本加载失败: ${loadError}`
  } else if (parseError) {
    text = parseError
  } else if (item) {
    text = `Preview item #${item.id}${item.externalId ? ` · ${item.externalId}` : ''}`
  }
  return <div aria-label="preview_item_status" style={previewStatusStyle}>{text}</div>
}

type PropertyTab = 'basic' | 'validation' | 'logic'

function PropertyPanel({ field, errors, fields, disabled, onChange }: {
  field: DraftField | null
  errors: DraftValidationError[]
  fields: DraftField[]
  disabled: boolean
  onChange: (patch: Partial<FieldSchema>) => void
}) {
  const [activeTab, setActiveTab] = useState<PropertyTab>('basic')
  // Advanced config (visibleWhen / customRule) only applies to answer-holding widgets.
  const supportsAdvanced = field ? advancedConfigWidgets.includes(field.widget) : false

  // Reset to 基础 whenever a different field is selected, so the panel never opens on an
  // advanced tab that the newly selected widget cannot show. React-recommended
  // "adjust state during render" pattern, avoids an effect-driven cascading render.
  const lastDraftId = useRef<string | null>(field?._draftId ?? null)
  if (lastDraftId.current !== (field?._draftId ?? null)) {
    lastDraftId.current = field?._draftId ?? null
    if (activeTab !== 'basic') {
      setActiveTab('basic')
    }
  }

  if (!field) {
    return (
      <>
        <h2 style={subHeadingStyle}>属性</h2>
        <pre style={jsonPreviewStyle}>{JSON.stringify({ fields: fields.map(stripDraftField) }, null, 2)}</pre>
      </>
    )
  }
  const duplicateName = fields.some((candidate) => candidate._draftId !== field._draftId && candidate.name === field.name)
  const currentTab = activeTab !== 'basic' && !supportsAdvanced ? 'basic' : activeTab
  return (
    <>
      <h2 style={subHeadingStyle}>属性</h2>
      <div style={propertyStackStyle}>
        {errors.length > 0 ? (
          <div aria-label={`selected validation ${field.name}`} style={fieldErrorListStyle}>
            {errors.map((item) => <div key={`${item.field}-${item.message}`}>{item.field}: {item.message}</div>)}
          </div>
        ) : null}
        {supportsAdvanced ? (
          <div role="tablist" aria-label="property tabs" style={propertyTabBarStyle}>
            <PropertyTabButton tab="basic" current={currentTab} label="基础" onSelect={setActiveTab} />
            <PropertyTabButton tab="validation" current={currentTab} label="校验" onSelect={setActiveTab} />
            <PropertyTabButton tab="logic" current={currentTab} label="联动" onSelect={setActiveTab} />
          </div>
        ) : null}

        {currentTab === 'basic' ? (
          <div role="tabpanel" aria-label="property tab 基础" style={propertyStackStyle}>
            <label style={fieldStyle}>
              name
              <input aria-label="field_name" disabled={disabled} value={field.name} onChange={(event) => onChange({ name: event.target.value })} style={inputStyle} />
            </label>
            {duplicateName ? <span style={errorTextStyle}>name 已存在</span> : null}
            <label style={fieldStyle}>
              label
              <input aria-label="field_label" disabled={disabled} value={field.label} onChange={(event) => onChange({ label: event.target.value })} style={inputStyle} />
            </label>
            <label style={checkboxRowStyle}>
              <input aria-label="field_required" type="checkbox" disabled={disabled} checked={field.required === true} onChange={(event) => onChange({ required: event.target.checked })} />
              required
            </label>
            {(field.widget === 'Input' || field.widget === 'TextArea') ? (
              <LengthControls field={field} disabled={disabled} onChange={onChange} />
            ) : null}
            {(field.widget === 'Radio' || field.widget === 'Tags') ? (
              <label style={fieldStyle}>
                options
                <textarea
                  aria-label="field_options"
                  disabled={disabled}
                  value={(field.options ?? []).join('\n')}
                  onChange={(event) => onChange({ options: parseOptionsInput(event.target.value) })}
                  style={textareaStyle}
                />
              </label>
            ) : null}
            {field.widget === 'ShowItem' ? <ShowItemControls field={field} disabled={disabled} onChange={onChange} /> : null}
            {field.widget === 'Group' ? <GroupControls field={field} fields={fields} disabled={disabled} onChange={onChange} /> : null}
            {field.widget === 'Tabs' ? <TabsControls field={field} fields={fields} disabled={disabled} onChange={onChange} /> : null}
            {field.widget === 'FileUpload' ? (
              <label style={fieldStyle}>
                maxFiles
                <input
                  aria-label="field_max_files"
                  disabled={disabled}
                  type="number"
                  min={1}
                  value={field.maxFiles ?? 5}
                  onChange={(event) => onChange({ maxFiles: positiveNumberInput(event.target.value, 1) })}
                  style={inputStyle}
                />
              </label>
            ) : null}
            {field.widget === 'LLMTrigger' ? <LLMTriggerControls field={field} disabled={disabled} fields={fields} onChange={onChange} /> : null}
          </div>
        ) : null}

        {currentTab === 'validation' ? (
          <div role="tabpanel" aria-label="property tab 校验" style={propertyStackStyle}>
            <CustomRuleControls field={field} disabled={disabled} onChange={onChange} />
          </div>
        ) : null}

        {currentTab === 'logic' ? (
          <div role="tabpanel" aria-label="property tab 联动" style={propertyStackStyle}>
            <VisibleWhenControls field={field} fields={fields} disabled={disabled} onChange={onChange} />
          </div>
        ) : null}
      </div>
    </>
  )
}

function PropertyTabButton({ tab, current, label, onSelect }: {
  tab: PropertyTab
  current: PropertyTab
  label: string
  onSelect: (tab: PropertyTab) => void
}) {
  const active = current === tab
  return (
    <button
      type="button"
      role="tab"
      aria-selected={active}
      aria-label={`property_tab_${tab}`}
      onClick={() => onSelect(tab)}
      style={active ? propertyTabButtonActiveStyle : propertyTabButtonStyle}
    >
      {label}
    </button>
  )
}

function VisibleWhenControls({ field, fields, disabled, onChange }: {
  field: DraftField
  fields: DraftField[]
  disabled: boolean
  onChange: (patch: Partial<FieldSchema>) => void
}) {
  // Controlling field can be any OTHER answer-holding field (excludes self, ShowItem, LLMTrigger, Group, Tabs).
  const candidates = fields.filter((candidate) => (
    candidate._draftId !== field._draftId && advancedConfigWidgets.includes(candidate.widget)
  ))
  const visibleWhen = field.visibleWhen
  const controllingField = visibleWhen?.field ?? ''
  const usesNotEmpty = visibleWhen?.notEmpty === true
  const equalsText = usesNotEmpty ? '' : equalsToText(visibleWhen)

  function selectControllingField(nextField: string) {
    if (!nextField) {
      onChange({ visibleWhen: undefined })
      return
    }
    // Default to equals='' so the schema is parser-valid the moment a field is chosen.
    onChange({ visibleWhen: { field: nextField, equals: visibleWhen && !usesNotEmpty ? visibleWhen.equals : '' } })
  }

  function setMode(notEmpty: boolean) {
    if (!controllingField) return
    onChange({ visibleWhen: notEmpty ? { field: controllingField, notEmpty: true } : { field: controllingField, equals: '' } })
  }

  function setEquals(value: string) {
    if (!controllingField) return
    onChange({ visibleWhen: { field: controllingField, equals: value } })
  }

  return (
    <div style={nestedEditorStyle}>
      <span style={nestedEditorLabelStyle}>条件显示 (visibleWhen)</span>
      <span style={hintTextStyle}>选择一个控制字段后, 仅当其值满足条件时才显示当前字段。清空控制字段即移除联动。</span>
      <label style={fieldStyle}>
        控制字段
        <select
          aria-label="visible_when_field"
          disabled={disabled}
          value={controllingField}
          onChange={(event) => selectControllingField(event.target.value)}
          style={inputStyle}
        >
          <option value="">（无, 始终显示）</option>
          {candidates.map((candidate) => (
            <option key={candidate._draftId} value={candidate.name}>{candidate.name}</option>
          ))}
        </select>
      </label>
      {controllingField ? (
        <>
          <label style={checkboxRowStyle}>
            <input
              aria-label="visible_when_not_empty"
              type="checkbox"
              disabled={disabled}
              checked={usesNotEmpty}
              onChange={(event) => setMode(event.target.checked)}
            />
            仅要求控制字段非空 (notEmpty)
          </label>
          {usesNotEmpty ? null : (
            <label style={fieldStyle}>
              等于该值 (equals)
              <input
                aria-label="visible_when_equals"
                disabled={disabled}
                value={equalsText}
                onChange={(event) => setEquals(event.target.value)}
                style={inputStyle}
                placeholder="例如 reject"
              />
            </label>
          )}
        </>
      ) : null}
    </div>
  )
}

function CustomRuleControls({ field, disabled, onChange }: {
  field: DraftField
  disabled: boolean
  onChange: (patch: Partial<FieldSchema>) => void
}) {
  const customRule = field.customRule
  const expr = customRule?.expr ?? ''
  const message = customRule?.message ?? ''
  const parseHint = useMemo(() => {
    if (!expr.trim()) return ''
    try {
      designerExprParser.parse(expr)
      return ''
    } catch (error) {
      return error instanceof Error ? error.message : '表达式无法解析'
    }
  }, [expr])

  function update(nextExpr: string, nextMessage: string) {
    if (!nextExpr.trim() && !nextMessage.trim()) {
      onChange({ customRule: undefined })
      return
    }
    onChange({ customRule: { expr: nextExpr, message: nextMessage } })
  }

  return (
    <div style={nestedEditorStyle}>
      <span style={nestedEditorLabelStyle}>自定义校验 (customRule)</span>
      <span style={hintTextStyle}>表达式为真即视为通过。可用变量: value, len(value), answer.&lt;字段名&gt;。运算符用 and / or / not, 例如 len(value) &lt;= 35。</span>
      <label style={fieldStyle}>
        表达式 (expr)
        <textarea
          aria-label="custom_rule_expr"
          disabled={disabled}
          value={expr}
          onChange={(event) => update(event.target.value, message)}
          style={textareaStyle}
          placeholder='len(value) <= 35'
        />
      </label>
      {parseHint ? <span aria-label="custom_rule_expr_hint" style={errorTextStyle}>表达式解析提示: {parseHint}</span> : null}
      <label style={fieldStyle}>
        错误提示 (message)
        <input
          aria-label="custom_rule_message"
          disabled={disabled}
          value={message}
          onChange={(event) => update(expr, event.target.value)}
          style={inputStyle}
          placeholder="不能超过 35 个字符"
        />
      </label>
    </div>
  )
}

function equalsToText(visibleWhen: VisibleWhen | undefined): string {
  if (!visibleWhen || !('equals' in visibleWhen) || visibleWhen.equals === undefined) return ''
  return typeof visibleWhen.equals === 'string' ? visibleWhen.equals : String(visibleWhen.equals)
}

function LengthControls({ field, disabled, onChange }: {
  field: DraftField
  disabled: boolean
  onChange: (patch: Partial<FieldSchema>) => void
}) {
  return (
    <div style={twoColumnStyle}>
      <label style={fieldStyle}>
        minLength
        <input
          aria-label="field_min_length"
          disabled={disabled}
          type="number"
          min={0}
          value={field.minLength ?? ''}
          onChange={(event) => onChange({ minLength: optionalNumberInput(event.target.value) })}
          style={inputStyle}
        />
      </label>
      <label style={fieldStyle}>
        maxLength
        <input
          aria-label="field_max_length"
          disabled={disabled}
          type="number"
          min={0}
          value={field.maxLength ?? ''}
          onChange={(event) => onChange({ maxLength: optionalNumberInput(event.target.value) })}
          style={inputStyle}
        />
      </label>
    </div>
  )
}

function ShowItemControls({ field, disabled, onChange }: {
  field: DraftField
  disabled: boolean
  onChange: (patch: Partial<FieldSchema>) => void
}) {
  return (
    <>
      <label style={fieldStyle}>
        path
        <input aria-label="field_path" disabled={disabled} value={field.path ?? '$payload'} onChange={(event) => onChange({ path: event.target.value })} style={inputStyle} />
      </label>
      <label style={fieldStyle}>
        mode
        <select aria-label="field_mode" disabled={disabled} value={field.mode ?? 'auto'} onChange={(event) => onChange({ mode: event.target.value as ShowItemMode })} style={inputStyle}>
          {showItemModes.map((mode) => <option key={mode} value={mode}>{mode}</option>)}
        </select>
      </label>
    </>
  )
}

function GroupControls({ field, fields, disabled, onChange }: {
  field: DraftField
  fields: DraftField[]
  disabled: boolean
  onChange: (patch: Partial<FieldSchema>) => void
}) {
  const childFields = field.fields ?? []
  return (
    <NestedFieldsEditor
      label="group_fields"
      disabled={disabled}
      fields={childFields}
      allFields={fields}
      parentName={field.name}
      onChange={(fields) => onChange({ fields })}
    />
  )
}

function TabsControls({ field, fields, disabled, onChange }: {
  field: DraftField
  fields: DraftField[]
  disabled: boolean
  onChange: (patch: Partial<FieldSchema>) => void
}) {
  const tabs = field.tabs ?? []
  function updateTab(index: number, patch: Partial<{ label: string, fields: FieldSchema[] }>) {
    onChange({
      tabs: tabs.map((tab, currentIndex) => (
        currentIndex === index ? { ...tab, ...patch } : tab
      )),
    })
  }
  function addTab() {
    const nextIndex = tabs.length + 1
    onChange({
      tabs: [...tabs, {
        label: `Tab ${nextIndex}`,
        _draftId: createDraftId(),
        fields: [createNestedDefaultField(nextNestedFieldName(fields, `${field.name}_tab${nextIndex}`, 'Input'), 'Input')],
      }],
    })
  }
  function deleteTab(index: number) {
    onChange({ tabs: tabs.filter((_, currentIndex) => currentIndex !== index) })
  }
  return (
    <div style={nestedEditorStyle}>
      <span style={nestedEditorLabelStyle}>tabs</span>
      {tabs.map((tab, index) => (
        <div key={tab._draftId} style={nestedPanelStyle}>
          <label style={fieldStyle}>
            tab_label
            <input
              aria-label={`tab_label_${index + 1}`}
              disabled={disabled}
              value={tab.label}
              onChange={(event) => updateTab(index, { label: event.target.value })}
              style={inputStyle}
            />
          </label>
          <NestedFieldsEditor
            label={`tab_${index + 1}_fields`}
            disabled={disabled}
            fields={tab.fields}
            allFields={fields}
            parentName={`${field.name}_tab${index + 1}`}
            onChange={(nextFields) => updateTab(index, { fields: nextFields })}
          />
          <Button disabled={disabled || tabs.length <= 1} onClick={() => deleteTab(index)} aria-label={`delete tab ${index + 1}`}>删除分页</Button>
        </div>
      ))}
      <Button disabled={disabled} onClick={addTab} aria-label="add tab">新增分页</Button>
    </div>
  )
}

function NestedFieldsEditor({ label, fields, allFields, parentName, disabled, onChange }: {
  label: string
  fields: FieldSchema[]
  allFields: FieldSchema[]
  parentName: string
  disabled: boolean
  onChange: (value: FieldSchema[]) => void
}) {
  function updateChild(index: number, patch: Partial<FieldSchema>) {
    onChange(fields.map((child, currentIndex) => (
      currentIndex === index ? normalizeFieldSchema({ ...child, ...patch }) : child
    )))
  }
  function changeChildWidget(index: number, widget: WidgetType) {
    onChange(fields.map((child, currentIndex) => (
      currentIndex === index ? createNestedFieldForWidget(child.name, widget, child.label, child._draftId) : child
    )))
  }
  function addChild(widget: WidgetType) {
    onChange([...fields, createNestedDefaultField(nextNestedFieldName(allFields, parentName, widget), widget)])
  }
  function deleteChild(index: number) {
    onChange(fields.filter((_, currentIndex) => currentIndex !== index))
  }
  function moveChild(index: number, direction: -1 | 1) {
    const nextIndex = index + direction
    if (nextIndex < 0 || nextIndex >= fields.length) return
    const next = [...fields]
    const [field] = next.splice(index, 1)
    next.splice(nextIndex, 0, field)
    onChange(next)
  }

  return (
    <div style={nestedEditorStyle}>
      <span style={nestedEditorLabelStyle}>{label}</span>
      {fields.map((child, index) => (
        <div key={child._draftId} style={nestedFieldRowStyle}>
          <div style={twoColumnStyle}>
            <label style={fieldStyle}>
              child_name
              <input
                aria-label={`${label}_child_name_${index + 1}`}
                disabled={disabled}
                value={child.name}
                onChange={(event) => updateChild(index, { name: event.target.value })}
                style={inputStyle}
              />
            </label>
            <label style={fieldStyle}>
              child_widget
              <select
                aria-label={`${label}_child_widget_${index + 1}`}
                disabled={disabled}
                value={child.widget}
                onChange={(event) => changeChildWidget(index, event.target.value as WidgetType)}
                style={inputStyle}
              >
                {nestedWidgetTypes.map((widget) => <option key={widget} value={widget}>{widget}</option>)}
              </select>
            </label>
          </div>
          <label style={fieldStyle}>
            child_label
            <input
              aria-label={`${label}_child_label_${index + 1}`}
              disabled={disabled}
              value={child.label}
              onChange={(event) => updateChild(index, { label: event.target.value })}
              style={inputStyle}
            />
          </label>
          <label style={checkboxRowStyle}>
            <input
              aria-label={`${label}_child_required_${index + 1}`}
              type="checkbox"
              disabled={disabled}
              checked={child.required === true}
              onChange={(event) => updateChild(index, { required: event.target.checked })}
            />
            required
          </label>
          {(child.widget === 'Radio' || child.widget === 'Tags') ? (
            <label style={fieldStyle}>
              child_options
              <textarea
                aria-label={`${label}_child_options_${index + 1}`}
                disabled={disabled}
                value={(child.options ?? []).join('\n')}
                onChange={(event) => updateChild(index, { options: parseOptionsInput(event.target.value) })}
                style={textareaStyle}
              />
            </label>
          ) : null}
          <div style={fieldActionsStyle}>
            <Button disabled={disabled || index === 0} onClick={() => moveChild(index, -1)} aria-label={`move up ${label} child ${index + 1}`}>上移</Button>
            <Button disabled={disabled || index === fields.length - 1} onClick={() => moveChild(index, 1)} aria-label={`move down ${label} child ${index + 1}`}>下移</Button>
            <Button disabled={disabled || fields.length <= 1} onClick={() => deleteChild(index)} aria-label={`delete ${label} child ${index + 1}`}>删除</Button>
          </div>
        </div>
      ))}
      <div style={fieldActionsStyle}>
        {nestedWidgetTypes.map((widget) => (
          <Button key={widget} disabled={disabled} onClick={() => addChild(widget)} aria-label={`add ${label} ${widget}`}>添加 {widget}</Button>
        ))}
      </div>
    </div>
  )
}

function LLMTriggerControls({ field, fields, disabled, onChange }: {
  field: DraftField
  fields: DraftField[]
  disabled: boolean
  onChange: (patch: Partial<FieldSchema>) => void
}) {
  return (
    <>
      <label style={fieldStyle}>
        target_field
        <select aria-label="field_target_field" disabled={disabled} value={field.target_field ?? field.name} onChange={(event) => onChange({ target_field: event.target.value })} style={inputStyle}>
          {fields.map((candidate) => <option key={candidate._draftId} value={candidate.name}>{candidate.name}</option>)}
        </select>
      </label>
      <label style={fieldStyle}>
        prompt
        <textarea aria-label="field_prompt" disabled={disabled} value={field.prompt ?? ''} onChange={(event) => onChange({ prompt: event.target.value })} style={textareaStyle} />
      </label>
    </>
  )
}

function createDefaultField(widget: WidgetType, current: DraftField[]): DraftField {
  const name = nextFieldName(widget, current)
  return attachDraftIdsToField({
    name,
    widget,
    label: widgetLabels[widget],
    required: false,
    ...(widget === 'Group' ? { fields: [createNestedDefaultField(`${name}_input`, 'Input')] } : {}),
    ...(widget === 'Tabs' ? { tabs: [
      { label: 'Tab 1', fields: [createNestedDefaultField(`${name}_tab1_input`, 'Input')] },
      { label: 'Tab 2', fields: [createNestedDefaultField(`${name}_tab2_text`, 'TextArea')] },
    ] } : {}),
    ...(widget === 'Radio' || widget === 'Tags' ? { options: ['pass', 'reject', 'uncertain'] } : {}),
    ...(widget === 'ShowItem' ? { path: '$payload', mode: 'auto' as ShowItemMode } : {}),
    ...(widget === 'FileUpload' ? { maxFiles: 3 } : {}),
    ...(widget === 'LLMTrigger' ? { target_field: name, prompt: '请根据 payload 和当前答案给出辅助建议。' } : {}),
  }, createDraftId())
}

function createNestedDefaultField(name: string, widget: WidgetType): FieldSchema {
  return createNestedFieldForWidget(name, widget, widgetLabels[widget])
}

function createNestedFieldForWidget(name: string, widget: WidgetType, label: string, draftId?: string): FieldSchema {
  return attachDraftIdsToField({
    name,
    widget,
    label,
    required: false,
    ...(widget === 'Radio' || widget === 'Tags' ? { options: ['pass', 'reject', 'uncertain'] } : {}),
    ...(widget === 'ShowItem' ? { path: '$payload', mode: 'auto' as ShowItemMode } : {}),
    ...(widget === 'FileUpload' ? { maxFiles: 3 } : {}),
    ...(widget === 'LLMTrigger' ? { target_field: name, prompt: '请根据 payload 和当前答案给出辅助建议。' } : {}),
  }, draftId)
}

function nextFieldName(widget: WidgetType, current: DraftField[]) {
  const prefix = widgetPrefixes[widget]
  const names = new Set(current.map((field) => field.name))
  let index = 1
  while (names.has(`${prefix}_${index}`)) {
    index += 1
  }
  return `${prefix}_${index}`
}

function cloneDraftField(field: DraftField, current: DraftField[]): DraftField {
  return attachDraftIdsToField(stripDraftField({
    ...field,
    name: nextCopyFieldName(field.name, current),
  }), createDraftId())
}

function reorderFieldsToTarget(current: DraftField[], fieldId: string, targetFieldId: string) {
  const index = current.findIndex((field) => field._draftId === fieldId)
  const targetIndex = current.findIndex((field) => field._draftId === targetFieldId)
  if (index < 0 || targetIndex < 0 || index === targetIndex) return current
  const next = [...current]
  const [field] = next.splice(index, 1)
  next.splice(targetIndex, 0, field)
  return next
}

function reorderFieldByOffset(current: DraftField[], fieldId: string, direction: -1 | 1) {
  const index = current.findIndex((field) => field._draftId === fieldId)
  const nextIndex = index + direction
  if (index < 0 || nextIndex < 0 || nextIndex >= current.length) return current
  const next = [...current]
  const [field] = next.splice(index, 1)
  next.splice(nextIndex, 0, field)
  return next
}

function nextCopyFieldName(name: string, current: DraftField[]) {
  const base = name.trim() || 'field'
  const names = new Set(current.map((field) => field.name.trim()))
  let candidate = `${base}_copy`
  let index = 2
  while (names.has(candidate)) {
    candidate = `${base}_copy_${index}`
    index += 1
  }
  return candidate
}

function createDraftId(): string {
  return globalThis.crypto?.randomUUID?.() ?? `draft-${Date.now()}-${Math.random().toString(36).slice(2)}`
}

function normalizeDraftField(field: DraftField): DraftField {
  return normalizeFieldSchema(field) as DraftField
}

function attachDraftIdsToField(field: FieldSchema, draftId = createDraftId()): DraftField {
  const normalized = normalizeFieldSchema(field)
  return {
    ...normalized,
    _draftId: draftId,
    ...(normalized.fields ? { fields: normalized.fields.map((child) => attachDraftIdsToField(child)) } : {}),
    ...(normalized.tabs ? { tabs: normalized.tabs.map((tab) => attachDraftIdsToTab(tab)) } : {}),
  }
}

function attachDraftIdsToTab(tab: TabSchema): TabSchema {
  return {
    ...tab,
    _draftId: tab._draftId ?? createDraftId(),
    fields: tab.fields.map((child) => attachDraftIdsToField(child)),
  }
}

function normalizeFieldSchema(field: FieldSchema): FieldSchema {
  const next = { ...field }
  next.name = next.name.trim()
  if (next.widget !== 'Radio' && next.widget !== 'Tags') {
    delete next.options
  }
  if (next.widget !== 'Input' && next.widget !== 'TextArea') {
    delete next.minLength
    delete next.maxLength
  }
  if (next.widget !== 'ShowItem') {
    delete next.path
    delete next.mode
  }
  if (next.widget !== 'Group') {
    delete next.fields
  }
  if (next.widget !== 'Tabs') {
    delete next.tabs
  }
  if (next.widget !== 'FileUpload') {
    delete next.maxFiles
  }
  if (next.widget !== 'LLMTrigger') {
    delete next.prompt
    delete next.target_field
  }
  if (!advancedConfigWidgets.includes(next.widget)) {
    // visibleWhen / customRule only apply to answer-holding widgets; drop them when the
    // widget is switched to a display/container type so stale rules don't survive.
    delete next.visibleWhen
    delete next.customRule
  }
  return next
}

function buildTemplatePayload(title: string, fields: DraftField[], schema: TemplateSchema | null) {
  const payload: Record<string, unknown> = {
    title: title.trim() || schema?.title || 'untitled_template',
    layout: 'single_page',
    fields: fields.map(stripDraftField),
    export_fields: exportFieldNames(fields),
  }
  if (schema) {
    for (const [key, value] of Object.entries(schema)) {
      if (key.startsWith('x-')) {
        payload[key] = value
      }
    }
  }
  return payload
}

function stripDraftField(field: DraftField): FieldSchema {
  const name = field.name.trim()
  const result: FieldSchema = {
    name,
    widget: field.widget,
    label: field.label.trim() || name,
    required: field.required === true,
  }
  if (field.options) result.options = field.options
  if (field.minLength !== undefined) result.minLength = field.minLength
  if (field.maxLength !== undefined) result.maxLength = field.maxLength
  if (field.path !== undefined) result.path = field.path
  if (field.mode !== undefined) result.mode = field.mode
  if (field.maxFiles !== undefined) result.maxFiles = field.maxFiles
  if (field.prompt !== undefined) result.prompt = field.prompt
  if (field.target_field !== undefined) result.target_field = field.target_field
  if (field.visibleWhen !== undefined) result.visibleWhen = field.visibleWhen
  if (field.customRule !== undefined) result.customRule = field.customRule
  if (field.requiredWhen !== undefined) result.requiredWhen = field.requiredWhen
  if (field.fields !== undefined) result.fields = field.fields.map(stripFieldSchema)
  if (field.tabs !== undefined) {
    result.tabs = field.tabs.map((tab) => ({
      label: tab.label,
      fields: tab.fields.map(stripFieldSchema),
    }))
  }
  for (const [key, value] of Object.entries(field)) {
    if (key.startsWith('x-')) {
      result[key as `x-${string}`] = value
    }
  }
  return result
}

function stripFieldSchema(field: FieldSchema): FieldSchema {
  return stripDraftField(field as DraftField)
}

function exportFieldNames(fields: FieldSchema[]): string[] {
  const names: string[] = []
  for (const field of fields) {
    if (field.widget === 'Group') {
      names.push(...exportFieldNames(field.fields ?? []))
      continue
    }
    if (field.widget === 'Tabs') {
      for (const tab of field.tabs ?? []) {
        names.push(...exportFieldNames(tab.fields))
      }
      continue
    }
    names.push(field.name)
  }
  return names
}

function nextNestedFieldName(fields: FieldSchema[], parentName: string, widget: WidgetType) {
  const names = new Set(collectFieldNames(fields))
  const prefix = `${parentName}_${widgetPrefixes[widget]}`
  let index = 1
  while (names.has(`${prefix}_${index}`)) {
    index += 1
  }
  return `${prefix}_${index}`
}

function collectFieldNames(fields: FieldSchema[]): string[] {
  const names: string[] = []
  for (const field of fields) {
    names.push(field.name)
    if (field.fields) {
      names.push(...collectFieldNames(field.fields))
    }
    if (field.tabs) {
      for (const tab of field.tabs) {
        names.push(...collectFieldNames(tab.fields))
      }
    }
  }
  return names
}

function validateDraftFields(fields: DraftField[]): DraftValidationError[] {
  const errors: DraftValidationError[] = []
  const names = new Set<string>()
  visitFieldsForValidation(fields, 'fields', names, errors)
  validateLLMTriggerTargets(fields, 'fields', names, errors)
  return errors
}

function visitFieldsForValidation(fields: FieldSchema[], pathPrefix: string, names: Set<string>, errors: DraftValidationError[], parentDraftId?: string) {
  for (const [index, field] of fields.entries()) {
    const path = `${pathPrefix}[${index}]`
    const draftId = '_draftId' in field && typeof field._draftId === 'string' ? field._draftId : parentDraftId
    const name = field.name.trim()
    if (!name) {
      errors.push({ draftId, field: `${path}.name`, message: 'name is required' })
    } else if (names.has(name)) {
      errors.push({ draftId, field: `${path}.name`, message: `duplicate name ${name}` })
    } else {
      names.add(name)
    }
    if ((field.widget === 'Radio' || field.widget === 'Tags') && (!field.options || field.options.length === 0)) {
      errors.push({ draftId, field: `${path}.options`, message: 'options must be non-empty' })
    }
    if (field.minLength !== undefined && field.maxLength !== undefined && field.minLength > field.maxLength) {
      errors.push({ draftId, field: `${path}.maxLength`, message: 'minLength cannot exceed maxLength' })
    }
    if (field.widget === 'Group') {
      if (!field.fields || field.fields.length === 0) {
        errors.push({ draftId, field: `${path}.fields`, message: 'fields must be non-empty' })
      } else {
        visitFieldsForValidation(field.fields, `${path}.fields`, names, errors, draftId)
      }
    }
    if (field.widget === 'Tabs') {
      if (!field.tabs || field.tabs.length === 0) {
        errors.push({ draftId, field: `${path}.tabs`, message: 'tabs must be non-empty' })
      }
      for (const [tabIndex, tab] of (field.tabs ?? []).entries()) {
        if (!tab.label.trim()) {
          errors.push({ draftId, field: `${path}.tabs[${tabIndex}].label`, message: 'label is required' })
        }
        if (tab.fields.length === 0) {
          errors.push({ draftId, field: `${path}.tabs[${tabIndex}].fields`, message: 'fields must be non-empty' })
        } else {
          visitFieldsForValidation(tab.fields, `${path}.tabs[${tabIndex}].fields`, names, errors, draftId)
        }
      }
    }
  }
}

function validateLLMTriggerTargets(fields: FieldSchema[], pathPrefix: string, names: Set<string>, errors: DraftValidationError[], parentDraftId?: string) {
  for (const [index, field] of fields.entries()) {
    const path = `${pathPrefix}[${index}]`
    const draftId = '_draftId' in field && typeof field._draftId === 'string' ? field._draftId : parentDraftId
    if (field.widget === 'LLMTrigger') {
      const allowExternal = field['x-allow-external-target'] === true
      if (!allowExternal && (!field.target_field || !names.has(field.target_field.trim()))) {
        errors.push({ draftId, field: `${path}.target_field`, message: 'target_field must reference an existing field' })
      }
    }
    if (field.fields) {
      validateLLMTriggerTargets(field.fields, `${path}.fields`, names, errors, draftId)
    }
    if (field.tabs) {
      for (const [tabIndex, tab] of field.tabs.entries()) {
        validateLLMTriggerTargets(tab.fields, `${path}.tabs[${tabIndex}].fields`, names, errors, draftId)
      }
    }
  }
}

function groupValidationErrorsByDraftId(errors: DraftValidationError[]) {
  const grouped = new Map<string, DraftValidationError[]>()
  for (const error of errors) {
    if (!error.draftId) continue
    const items = grouped.get(error.draftId) ?? []
    grouped.set(error.draftId, [...items, error])
  }
  return grouped
}

function optionalNumberInput(value: string) {
  if (value.trim() === '') return undefined
  const parsed = Number(value)
  return Number.isInteger(parsed) && parsed >= 0 ? parsed : undefined
}

function positiveNumberInput(value: string, fallback: number) {
  const parsed = Number(value)
  return Number.isInteger(parsed) && parsed > 0 ? parsed : fallback
}

function parseOptionsInput(value: string): FieldOption[] {
  return value.split(/[\n,]/).map((item) => item.trim()).filter(Boolean)
}

const fallbackPreviewPayload: RenderPayload = {
  prompt: 'Preview payload',
  model_answer: 'Preview answer',
}

function parsePreviewPayload(item: PreviewItem | null): { payload: RenderPayload, error: string } {
  if (!item) {
    return { payload: fallbackPreviewPayload, error: '' }
  }
  if (isRecordValue(item.payload)) {
    return { payload: item.payload, error: '' }
  }
  return { payload: { value: item.payload }, error: '' }
}

function isRecordValue(value: unknown): value is RenderPayload {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

const pageStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-lg)',
}

const toolbarStyle: CSSProperties = {
  display: 'flex',
  justifyContent: 'space-between',
  gap: 'var(--space-lg)',
  alignItems: 'flex-start',
}

const toolbarActionsStyle: CSSProperties = {
  display: 'flex',
  gap: 'var(--space-sm)',
  alignItems: 'center',
  flexWrap: 'wrap',
}

const headingStyle: CSSProperties = {
  margin: 'var(--space-xs) 0 0',
  fontFamily: 'var(--font-heading)',
  fontSize: 'var(--text-h1)',
}

const subHeadingStyle: CSSProperties = {
  margin: 0,
  fontFamily: 'var(--font-heading)',
  fontSize: 'var(--text-h2)',
}

const panelStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-md)',
  padding: 'var(--space-lg)',
  border: '1px solid var(--color-border-light)',
  background: 'var(--color-surface)',
  borderRadius: 'var(--radius-lg)',
  boxShadow: 'var(--shadow-md)',
  height: 'fit-content',
}

const paletteStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-sm)',
}

const canvasStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-md)',
  minHeight: 520,
  padding: 'var(--space-lg)',
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-lg)',
  backgroundColor: 'var(--color-canvas)',
  backgroundImage: 'linear-gradient(var(--color-grid-line) 1px, transparent 1px), linear-gradient(90deg, var(--color-grid-line) 1px, transparent 1px)',
  backgroundSize: '24px 24px',
  alignContent: 'start',
}

const emptyCanvasStyle: CSSProperties = {
  minHeight: 360,
  display: 'grid',
  placeItems: 'center',
  alignContent: 'center',
  textAlign: 'center',
  color: 'var(--color-text-secondary)',
  border: '1px dashed var(--color-node-border)',
  borderRadius: 'var(--radius-lg)',
  background: 'rgba(255,255,255,0.72)',
}

const emptyDiagramStyle: CSSProperties = {
  display: 'grid',
  gridTemplateColumns: '64px 48px 64px',
  gridTemplateRows: '40px 40px',
  alignItems: 'center',
  justifyItems: 'center',
  marginBottom: 'var(--space-md)',
}

const emptyNodeStyle: CSSProperties = {
  width: 58,
  height: 30,
  border: '1px solid var(--color-node-border)',
  borderRadius: 'var(--radius-md)',
  background: 'var(--color-node-bg)',
  boxShadow: 'var(--shadow-sm)',
}

const emptyConnectorStyle: CSSProperties = {
  width: 48,
  height: 1,
  background: 'var(--color-node-border)',
}

const previewStatusStyle: CSSProperties = {
  padding: 'var(--space-md)',
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-md)',
  background: 'var(--color-bg)',
  color: 'var(--color-text-secondary)',
  fontSize: 'var(--text-sm)',
}

const canvasItemStyle: CSSProperties = {
  position: 'relative',
  border: '1px solid var(--color-border-light)',
  background: 'var(--color-node-bg)',
  borderRadius: 'var(--radius-md)',
  boxShadow: 'var(--shadow-sm)',
  overflow: 'hidden',
  transition: 'all var(--duration-fast)',
}

const selectedCanvasItemStyle: CSSProperties = {
  ...canvasItemStyle,
  border: '1px solid var(--color-accent)',
  boxShadow: '0 0 0 2px var(--color-accent-soft)',
}

const draggingCanvasItemStyle: CSSProperties = {
  ...selectedCanvasItemStyle,
  opacity: 0.6,
}

const dragOverlayStyle: CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  padding: 'var(--space-sm) var(--space-md)',
  borderRadius: 'var(--radius-md)',
  border: '1px solid var(--color-accent)',
  background: 'var(--color-surface)',
  boxShadow: 'var(--shadow-md)',
  fontSize: 'var(--text-sm)',
  fontWeight: 600,
  color: 'var(--color-accent)',
  cursor: 'grabbing',
  pointerEvents: 'none',
}

const canvasItemHeaderStyle: CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'space-between',
  gap: 'var(--space-sm)',
  padding: 'var(--space-sm) var(--space-md)',
  background: '#fafafa',
  borderBottom: '1px solid var(--color-border-light)',
}

const leftPortStyle: CSSProperties = {
  position: 'absolute',
  left: -5,
  top: 24,
  width: 8,
  height: 8,
  borderRadius: 8,
  background: 'var(--color-port)',
  border: '1px solid var(--color-surface)',
}

const rightPortStyle: CSSProperties = {
  ...leftPortStyle,
  left: 'auto',
  right: -5,
}

const fieldActionsStyle: CSSProperties = {
  display: 'flex',
  flexWrap: 'wrap',
  gap: 8,
  alignItems: 'center',
}

const selectFieldButtonStyle: CSSProperties = {
  display: 'flex',
  flexDirection: 'column',
  gap: 2,
  flex: 1,
  minWidth: 0,
  border: 0,
  padding: 0,
  background: 'transparent',
  color: 'var(--color-text)',
  textAlign: 'left',
  cursor: 'pointer',
  fontFamily: 'var(--font-body)',
}

const widgetPreviewStyle: CSSProperties = {
  padding: 'var(--space-lg)',
}

const previewToggleStyle: CSSProperties = {
  padding: '3px 10px',
  fontSize: 'var(--text-sm)',
  color: 'var(--color-text-secondary)',
  background: 'var(--color-surface)',
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-sm)',
  cursor: 'pointer',
}

const nestedCanvasPreviewStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-sm)',
  padding: 'var(--space-md)',
  background: 'var(--color-surface)',
}

const nestedCanvasTabStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-sm)',
  padding: 'var(--space-sm)',
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-md)',
  background: 'var(--color-surface-subtle)',
}

const nestedCanvasRowsStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-xs)',
}

const nestedCanvasRowStyle: CSSProperties = {
  display: 'flex',
  flexDirection: 'column',
  gap: 'var(--space-xs)',
  padding: 'var(--space-sm)',
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-sm)',
  background: 'var(--color-bg)',
  color: 'var(--color-text-secondary)',
  fontSize: 'var(--text-sm)',
}

const nestedCanvasRowHeadStyle: CSSProperties = {
  display: 'grid',
  gridTemplateColumns: '72px minmax(90px, 1fr) minmax(90px, 1fr)',
  gap: 'var(--space-sm)',
  alignItems: 'center',
}

const nestedFieldBoxStyle: CSSProperties = {
  padding: '8px 12px',
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-sm)',
  background: 'var(--color-surface)',
  color: 'var(--color-text-muted)',
  fontSize: 'var(--text-sm)',
}

const nestedShowItemBoxStyle: CSSProperties = {
  ...nestedFieldBoxStyle,
  background: 'var(--color-surface-subtle)',
  color: 'var(--color-text-secondary)',
}

const nestedLlmBoxStyle: CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  gap: 6,
  padding: '8px 12px',
  border: '1px dashed #722ed1',
  borderRadius: 'var(--radius-sm)',
  background: '#f5e8ff',
  color: '#722ed1',
  fontSize: 'var(--text-sm)',
  width: 'fit-content',
}

const nestedCanvasWidgetStyle: CSSProperties = {
  fontFamily: 'var(--font-mono)',
  fontSize: 10,
  color: 'var(--color-text-muted)',
  textTransform: 'uppercase',
}

const fieldErrorListStyle: CSSProperties = {
  display: 'grid',
  gap: 2,
  padding: 'var(--space-sm) var(--space-lg)',
  borderBottom: '1px solid var(--color-border-light)',
  background: '#fff1f0',
  color: 'var(--color-danger)',
  fontSize: 'var(--text-sm)',
}

const fieldStyle: CSSProperties = {
  display: 'grid',
  gap: 6,
  color: 'var(--color-text-secondary)',
  fontSize: 'var(--text-sm)',
}

const inputStyle: CSSProperties = {
  width: '100%',
  minHeight: 40,
  border: '1px solid var(--color-border)',
  borderRadius: 'var(--radius-sm)',
  padding: '0 var(--space-md)',
  boxSizing: 'border-box',
  color: 'var(--color-text)',
  background: 'var(--color-surface)',
  fontSize: 'var(--text-base)',
}

const textareaStyle: CSSProperties = {
  ...inputStyle,
  minHeight: 120,
  padding: 'var(--space-sm) var(--space-md)',
  resize: 'vertical',
  fontFamily: 'var(--font-body)',
  lineHeight: 1.5,
}

const propertyStackStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-lg)',
}

const propertyTabBarStyle: CSSProperties = {
  display: 'flex',
  gap: 'var(--space-xs)',
  borderBottom: '1px solid var(--color-border-light)',
}

const propertyTabButtonStyle: CSSProperties = {
  flex: 1,
  border: 0,
  borderBottom: '2px solid transparent',
  background: 'transparent',
  padding: 'var(--space-sm) 0',
  color: 'var(--color-text-secondary)',
  fontFamily: 'var(--font-body)',
  fontSize: 'var(--text-sm)',
  fontWeight: 500,
  cursor: 'pointer',
}

const propertyTabButtonActiveStyle: CSSProperties = {
  ...propertyTabButtonStyle,
  color: 'var(--color-accent)',
  borderBottom: '2px solid var(--color-accent)',
  fontWeight: 600,
}

const hintTextStyle: CSSProperties = {
  color: 'var(--color-text-muted)',
  fontSize: 'var(--text-sm)',
  lineHeight: 1.5,
}

const nestedEditorStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-md)',
  padding: 'var(--space-md)',
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-md)',
  background: 'var(--color-bg)',
}

const nestedEditorLabelStyle: CSSProperties = {
  color: 'var(--color-text)',
  fontSize: 'var(--text-sm)',
  fontWeight: 600,
}

const nestedPanelStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-sm)',
  padding: 'var(--space-md)',
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-sm)',
  background: 'var(--color-surface)',
}

const nestedFieldRowStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-sm)',
  padding: 'var(--space-md)',
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-sm)',
  background: 'var(--color-surface)',
}

const twoColumnStyle: CSSProperties = {
  display: 'grid',
  gridTemplateColumns: '1fr 1fr',
  gap: 'var(--space-md)',
}

const checkboxRowStyle: CSSProperties = {
  display: 'flex',
  gap: 'var(--space-sm)',
  alignItems: 'center',
}

const jsonPreviewStyle: CSSProperties = {
  margin: 0,
  padding: 'var(--space-md)',
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-md)',
  background: 'var(--color-bg)',
  maxHeight: 600,
  overflow: 'auto',
  whiteSpace: 'pre-wrap',
  fontSize: 12,
  fontFamily: 'var(--font-mono)',
}

const formPreviewStyle: CSSProperties = {
  padding: 'var(--space-md)',
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-md)',
  background: 'var(--color-surface)',
  maxHeight: 600,
  overflow: 'auto',
}

const backLinkStyle: CSSProperties = {
  color: 'var(--color-accent)',
  textDecoration: 'none',
  fontWeight: 500,
  display: 'inline-flex',
  alignItems: 'center',
  gap: 4,
}

const latestTemplateLinkStyle: CSSProperties = {
  ...backLinkStyle,
  minHeight: 32,
  padding: '0 var(--space-md)',
  border: '1px solid var(--color-warning)',
  borderRadius: 'var(--radius-sm)',
  color: 'var(--color-warning)',
}

const alertStyle: CSSProperties = {
  padding: 'var(--space-md)',
  borderRadius: 'var(--radius-md)',
  border: '1px solid var(--color-danger)',
  background: '#fff1f0',
  color: 'var(--color-danger)',
}

const errorTextStyle: CSSProperties = {
  color: 'var(--color-danger)',
  fontSize: 'var(--text-sm)',
}
