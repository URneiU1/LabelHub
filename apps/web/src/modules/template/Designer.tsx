import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type DragEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { Button, Toast } from '@douyinfe/semi-ui'
import { Parser as ExprParser } from 'expr-eval'
import { apiGet, apiPost, type TaskTemplate } from '../../shared/api/client'
import SchemaErrorBanner from '../../renderer/components/SchemaErrorBanner'
import { parseTemplateSchema } from '../../renderer/parser'
import { widgetRegistry } from '../../renderer/widgets'
import { showItemModes, widgetTypes, type FieldOption, type FieldSchema, type RenderPayload, type ShowItemMode, type TabSchema, type TemplateSchema, type VisibleWhen, type WidgetType } from '../../renderer/types'
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

const nestedWidgetTypes = widgetTypes.filter((widget) => widget !== 'Group' && widget !== 'Tabs')

// Widgets that hold an answer value: visibleWhen / customRule are only meaningful on these.
const advancedConfigWidgets: WidgetType[] = ['Input', 'TextArea', 'Radio', 'Tags', 'RichText', 'JSONEditor', 'FileUpload']

// dataTransfer markers. A palette drop INSERTS a new widget; an existing-field drop REORDERS.
const PALETTE_DRAG_PREFIX = 'labelhub/new-widget:'

// Shared parser used only to surface a non-blocking parse hint for customRule expr in the panel.
const designerExprParser = new ExprParser()

export default function TemplateDesigner() {
  const { taskId, templateId } = useParams()
  const navigate = useNavigate()
  const numericTaskId = Number(taskId)
  const numericTemplateId = Number(templateId)
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
  const [draggingFieldId, setDraggingFieldId] = useState<string | null>(null)
  const loadSeq = useRef(0)
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
  }, [numericTaskId, numericTemplateId])

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

  function moveFieldToDragTarget(fieldId: string, targetFieldId: string) {
    if (!canEdit || fieldId === targetFieldId) return
    setFields((current) => reorderFieldsToTarget(current, fieldId, targetFieldId))
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

  function handleCanvasDrop(event: DragEvent<HTMLElement>, targetFieldId: string | null) {
    if (!canEdit) return
    event.preventDefault()
    const transfer = event.dataTransfer.getData('text/plain')
    if (transfer.startsWith(PALETTE_DRAG_PREFIX)) {
      const widget = transfer.slice(PALETTE_DRAG_PREFIX.length) as WidgetType
      if (widgetTypes.includes(widget)) {
        insertWidgetBeforeTarget(widget, targetFieldId)
      }
      setDraggingFieldId(null)
      return
    }
    const draggedId = draggingFieldId ?? transfer
    if (draggedId && targetFieldId) {
      moveFieldToDragTarget(draggedId, targetFieldId)
    }
    setDraggingFieldId(null)
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
      <div style={{ ...toolbarStyle, background: 'var(--color-surface)', padding: 'var(--space-lg) var(--space-xl)', borderRadius: 'var(--radius-lg)', boxShadow: 'var(--shadow-sm)', border: '1px solid var(--color-border-light)' }}>
        <div>
          <Link to={`/owner/tasks/${numericTaskId}/templates`} style={backLinkStyle}>← 返回版本列表</Link>
          <h1 style={{ ...headingStyle, marginTop: 'var(--space-sm)' }}>Template Designer</h1>
          <div style={{ display: 'flex', gap: 'var(--space-sm)', marginTop: 'var(--space-xs)' }}>
             <span style={{ fontSize: 'var(--text-sm)', padding: '2px 8px', borderRadius: 4, background: 'var(--color-bg)', color: 'var(--color-text-secondary)' }}>ID: {template?.id}</span>
             <span style={{ fontSize: 'var(--text-sm)', padding: '2px 8px', borderRadius: 4, background: 'var(--color-bg)', color: 'var(--color-text-secondary)' }}>Version: v{template?.version ?? '-'}</span>
             <span style={{ fontSize: 'var(--text-sm)', padding: '2px 8px', borderRadius: 4, background: isLatest ? '#e8f5e9' : '#fff3e0', color: isLatest ? '#2e7d32' : '#ef6c00', fontWeight: 600 }}>{isLatest ? 'LATEST / EDITABLE' : 'READONLY'}</span>
          </div>
        </div>
        <div style={toolbarActionsStyle}>
          {!isLatest && latestTemplateId ? (
            <Link to={`/owner/tasks/${numericTaskId}/templates/${latestTemplateId}`} style={latestTemplateLinkStyle}>查看最新版本</Link>
          ) : null}
          {taskMismatch ? null : canEdit ? (
            <>
              <Button aria-label="Discard" disabled={saving} onClick={discardChanges} theme="light">重置修改</Button>
              <Button aria-label="Save as new version" disabled={saveDisabled} loading={saving} theme="solid" onClick={() => void saveTemplate()}>保存并发布新版</Button>
            </>
          ) : (
            <Button aria-label="Fork as new version" disabled={saving || !schema} loading={saving} theme="solid" onClick={() => void forkTemplate()}>Fork 为新版本</Button>
          )}
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

      <div className="template-designer-grid" style={{ marginTop: 'var(--space-lg)' }}>
        <aside className="template-designer-palette" style={panelStyle}>
          <div style={{ borderBottom: '1px solid var(--color-border-light)', paddingBottom: 'var(--space-sm)', marginBottom: 'var(--space-sm)' }}>
            <h2 style={{ ...subHeadingStyle, fontSize: 'var(--text-base)' }}>组件物料</h2>
          </div>
          <div style={paletteStyle}>
            {widgetTypes.map((widget) => (
              <Button
                key={widget}
                aria-label={`Add ${widget}`}
                disabled={!canEdit}
                draggable={canEdit}
                onClick={() => appendField(widget)}
                onDragStart={(event: DragEvent<HTMLButtonElement>) => {
                  if (!canEdit) return
                  event.dataTransfer.effectAllowed = 'copy'
                  event.dataTransfer.setData('text/plain', `${PALETTE_DRAG_PREFIX}${widget}`)
                  setDraggingFieldId(null)
                }}
                theme="light"
                style={paletteButtonStyle}
              >
                <span style={paletteButtonNodeStyle}>
                  <span style={paletteWidgetCodeStyle}>{widget}</span>
                  <span style={paletteWidgetLabelStyle}>{widgetLabels[widget]}</span>
                </span>
              </Button>
            ))}
          </div>
        </aside>

        <main className="template-designer-canvas" style={{ ...panelStyle, minHeight: 800 }}>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-lg)' }}>
            <label style={fieldStyle}>
              <span style={{ fontWeight: 600 }}>模板名称</span>
              <input aria-label="template_title" disabled={!canEdit} value={title} onChange={(event) => setTitle(event.target.value)} style={inputStyle} placeholder="输入模板标题..." />
            </label>

            <div style={{ borderTop: '1px solid var(--color-border-light)', paddingTop: 'var(--space-lg)' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 'var(--space-md)' }}>
                <div style={{ fontWeight: 600 }}>画布区域 (Canvas)</div>
                <PreviewItemStatus
                  item={previewItem}
                  loading={previewLoading}
                  loadError={previewError}
                  parseError={previewPayloadResult.error}
                />
              </div>

              <div
                aria-label="canvas drop zone"
                style={canvasStyle}
                onDragOver={(event) => {
                  if (!canEdit) return
                  event.preventDefault()
                  event.dataTransfer.dropEffect = draggingFieldId ? 'move' : 'copy'
                }}
                onDrop={(event) => {
                  // Drops that miss a specific field append to the end (covers the empty canvas
                  // and the gaps below the last node). Field-level drops stop propagation.
                  handleCanvasDrop(event, null)
                }}
              >
                {fields.length === 0 ? (
                  <EmptyCanvasDiagram />
                ) : fields.map((field, index) => (
                  <CanvasField
                    key={field._draftId}
                    field={field}
                    errors={validationErrorsByDraftId.get(field._draftId) ?? []}
                    isFirst={index === 0}
                    isLast={index === fields.length - 1}
                    previewPayload={previewPayloadResult.payload}
                    selected={field._draftId === selectedId}
                    dragging={field._draftId === draggingFieldId}
                    disabled={!canEdit}
                    onSelect={() => setSelectedId(field._draftId)}
                    onCopy={() => copyField(field._draftId)}
                    onDelete={() => deleteField(field._draftId)}
                    onMoveDown={() => moveField(field._draftId, 1)}
                    onMoveUp={() => moveField(field._draftId, -1)}
                    onDragEnd={() => setDraggingFieldId(null)}
                    onDragOver={(event) => {
                      if (!canEdit) return
                      event.preventDefault()
                      event.dataTransfer.dropEffect = draggingFieldId ? 'move' : 'copy'
                    }}
                    onDragStart={(event) => {
                      if (!canEdit) return
                      event.dataTransfer.effectAllowed = 'move'
                      event.dataTransfer.setData('text/plain', field._draftId)
                      setDraggingFieldId(field._draftId)
                    }}
                    onDrop={(event) => {
                      event.stopPropagation()
                      handleCanvasDrop(event, field._draftId)
                    }}
                  />
                ))}
              </div>
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
  dragging,
  disabled,
  onSelect,
  onCopy,
  onDelete,
  onMoveDown,
  onMoveUp,
  onDragEnd,
  onDragOver,
  onDragStart,
  onDrop,
}: {
  field: DraftField
  errors: DraftValidationError[]
  isFirst: boolean
  isLast: boolean
  previewPayload: RenderPayload
  selected: boolean
  dragging: boolean
  disabled: boolean
  onSelect: () => void
  onCopy: () => void
  onDelete: () => void
  onMoveDown: () => void
  onMoveUp: () => void
  onDragEnd: () => void
  onDragOver: (event: DragEvent<HTMLElement>) => void
  onDragStart: (event: DragEvent<HTMLButtonElement>) => void
  onDrop: (event: DragEvent<HTMLElement>) => void
}) {
  const Widget = widgetRegistry[field.widget]
  return (
    <section
      aria-label={`canvas field ${field.name}`}
      onDragOver={onDragOver}
      onDrop={onDrop}
      style={dragging ? draggingCanvasItemStyle : selected ? selectedCanvasItemStyle : canvasItemStyle}
    >
      <span aria-hidden="true" style={leftPortStyle} />
      <span aria-hidden="true" style={rightPortStyle} />
      <div style={{ ...canvasItemHeaderStyle, background: selected ? 'var(--color-bg)' : '#fafafa' }}>
        <button type="button" aria-label={`select ${field.name}`} onClick={onSelect} style={selectFieldButtonStyle}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-sm)' }}>
            <strong style={{ fontSize: 'var(--text-base)', color: selected ? 'var(--color-accent)' : 'var(--color-text)' }}>{field.name}</strong>
            <span style={{ fontSize: 11, padding: '1px 6px', background: 'white', border: '1px solid var(--color-border-light)', borderRadius: 4, color: 'var(--color-text-muted)' }}>{widgetLabels[field.widget]}</span>
          </div>
          <span style={{ fontSize: 'var(--text-sm)', color: 'var(--color-text-secondary)', marginTop: 2 }}>{field.label}</span>
        </button>
        <div style={fieldActionsStyle}>
          <Button size="small" theme="light" disabled={disabled} draggable={!disabled} onDragEnd={onDragEnd} onDragStart={onDragStart} aria-label={`drag ${field.name}`} icon={<span>⠿</span>} />
          <div style={{ display: 'flex', background: 'white', border: '1px solid var(--color-border-light)', borderRadius: 'var(--radius-sm)' }}>
            <Button size="small" theme="borderless" disabled={disabled || isFirst} onClick={onMoveUp} aria-label={`move up ${field.name}`}>↑</Button>
            <Button size="small" theme="borderless" disabled={disabled || isLast} onClick={onMoveDown} aria-label={`move down ${field.name}`}>↓</Button>
          </div>
          <Button size="small" theme="light" disabled={disabled} onClick={onCopy} aria-label={`copy ${field.name}`}>复制</Button>
          <Button size="small" theme="light" disabled={disabled} onClick={onDelete} aria-label={`delete ${field.name}`} type="danger">删除</Button>
        </div>
      </div>
      {errors.length > 0 ? (
        <div aria-label={`validation ${field.name}`} style={fieldErrorListStyle}>
          {errors.map((item) => <div key={`${item.field}-${item.message}`} style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
            {item.field}: {item.message}
          </div>)}
        </div>
      ) : null}
      {field.widget === 'Group' || field.widget === 'Tabs' ? (
        <NestedCanvasPreview field={field} />
      ) : (
        <div style={{ ...widgetPreviewStyle, opacity: selected ? 1 : 0.8 }}>
          <Widget
            field={field}
            value={field.widget === 'Tags' ? [] : ''}
            answer={{}}
            payload={previewPayload}
            readOnly
            onChange={() => undefined}
          />
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

function NestedCanvasRow({ child }: { child: FieldSchema }) {
  return (
    <div style={nestedCanvasRowStyle} aria-label={`nested field ${child.name}`}>
      <span style={nestedCanvasWidgetStyle}>{child.widget}</span>
      <strong>{child.name}</strong>
      <span>{child.label}</span>
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

const paletteButtonStyle: CSSProperties = {
  justifyContent: 'stretch',
  textAlign: 'left',
  height: 'auto',
  padding: 0,
  borderColor: 'var(--color-node-border)',
  background: 'var(--color-node-bg)',
}

const paletteButtonNodeStyle: CSSProperties = {
  display: 'grid',
  gap: 3,
  width: '100%',
  padding: 'var(--space-sm) var(--space-md)',
  borderLeft: '3px solid var(--color-rail)',
}

const paletteWidgetCodeStyle: CSSProperties = {
  fontFamily: 'var(--font-mono)',
  fontSize: 10,
  color: 'var(--color-text-muted)',
  textTransform: 'uppercase',
}

const paletteWidgetLabelStyle: CSSProperties = {
  color: 'var(--color-text)',
  fontWeight: 600,
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
  transform: 'scale(0.98)',
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
  display: 'grid',
  gridTemplateColumns: '72px minmax(90px, 1fr) minmax(90px, 1fr)',
  gap: 'var(--space-sm)',
  alignItems: 'center',
  padding: 'var(--space-xs) var(--space-sm)',
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-sm)',
  background: 'var(--color-bg)',
  color: 'var(--color-text-secondary)',
  fontSize: 'var(--text-sm)',
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
