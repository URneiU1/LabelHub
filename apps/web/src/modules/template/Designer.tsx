import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type DragEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { Button, Toast } from '@douyinfe/semi-ui'
import { apiGet, apiPost, type TaskTemplate } from '../../shared/api/client'
import SchemaErrorBanner from '../../renderer/components/SchemaErrorBanner'
import { parseTemplateSchema } from '../../renderer/parser'
import { widgetRegistry } from '../../renderer/widgets'
import { showItemModes, widgetTypes, type FieldOption, type FieldSchema, type RenderPayload, type ShowItemMode, type TemplateSchema, type WidgetType } from '../../renderer/types'

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
      const draftFields = parsed.value.fields.map((field, index) => ({
        ...field,
        _draftId: `${field.name}-${index}`,
      }))
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

  function updateSelected(patch: Partial<FieldSchema>) {
    if (!canEdit || !selectedField) return
    setFields((current) => current.map((field) => (
      field._draftId === selectedField._draftId ? normalizeDraftField({ ...field, ...patch }) : field
    )))
  }

  function discardChanges() {
    if (!schema) return
    const draftFields = schema.fields.map((field, index) => ({
      ...field,
      _draftId: `${field.name}-${index}`,
    }))
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
      <div style={toolbarStyle}>
        <div>
          <Link to={`/owner/tasks/${numericTaskId}/templates`} style={backLinkStyle}>返回模板版本</Link>
          <h1 style={headingStyle}>Template Designer</h1>
          <p style={mutedStyle}>
            {template ? `Template #${template.id} · v${template.version ?? '-'} · ${isLatest ? 'edit' : 'readonly'}` : '模板'}
          </p>
        </div>
        <div style={toolbarActionsStyle}>
          {!isLatest && latestTemplateId ? <Link to={`/owner/tasks/${numericTaskId}/templates/${latestTemplateId}`} style={backLinkStyle}>打开 latest</Link> : null}
          {taskMismatch ? null : canEdit ? (
            <>
              <Button disabled={saving} onClick={discardChanges}>Discard</Button>
              <Button disabled={saveDisabled} loading={saving} theme="solid" onClick={() => void saveTemplate()}>Save as new version</Button>
            </>
          ) : (
            <Button disabled={saving || !schema} loading={saving} theme="solid" onClick={() => void forkTemplate()}>Fork as new version</Button>
          )}
        </div>
      </div>

      {error ? <div role="alert" style={alertStyle}>{error}</div> : null}
      {schemaError ? <SchemaErrorBanner error={schemaError} role="owner" /> : null}
      {validationErrors.length > 0 ? (
        <div role="alert" style={alertStyle}>
          {validationErrors.map((item) => <div key={`${item.field}-${item.message}`}>{item.field}: {item.message}</div>)}
        </div>
      ) : null}

      <div style={designerGridStyle}>
        <aside style={panelStyle}>
          <h2 style={subHeadingStyle}>物料</h2>
          <div style={paletteStyle}>
            {widgetTypes.map((widget) => (
              <Button key={widget} disabled={!canEdit} onClick={() => appendField(widget)}>
                Add {widget}
              </Button>
            ))}
          </div>
        </aside>

        <main style={panelStyle}>
          <label style={fieldStyle}>
            title
            <input aria-label="template_title" disabled={!canEdit} value={title} onChange={(event) => setTitle(event.target.value)} style={inputStyle} />
          </label>
          <PreviewItemStatus
            item={previewItem}
            loading={previewLoading}
            loadError={previewError}
            parseError={previewPayloadResult.error}
          />
          <div style={canvasStyle}>
            {fields.length === 0 ? (
              <p style={mutedStyle}>从左侧添加一个字段开始。</p>
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
                  event.dataTransfer.dropEffect = 'move'
                }}
                onDragStart={(event) => {
                  if (!canEdit) return
                  event.dataTransfer.effectAllowed = 'move'
                  event.dataTransfer.setData('text/plain', field._draftId)
                  setDraggingFieldId(field._draftId)
                }}
                onDrop={(event) => {
                  if (!canEdit) return
                  event.preventDefault()
                  const draggedId = draggingFieldId ?? event.dataTransfer.getData('text/plain')
                  if (draggedId) {
                    moveFieldToDragTarget(draggedId, field._draftId)
                  }
                  setDraggingFieldId(null)
                }}
              />
            ))}
          </div>
        </main>

        <aside style={panelStyle}>
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
      <div style={canvasItemHeaderStyle}>
        <button type="button" aria-label={`select ${field.name}`} onClick={onSelect} style={selectFieldButtonStyle}>
          <strong>{field.name}</strong>
          <span>{widgetLabels[field.widget]} · {field.label}</span>
        </button>
        <div style={fieldActionsStyle}>
          <Button disabled={disabled} draggable={!disabled} onDragEnd={onDragEnd} onDragStart={onDragStart} aria-label={`drag ${field.name}`}>Drag</Button>
          <Button disabled={disabled || isFirst} onClick={onMoveUp} aria-label={`move up ${field.name}`}>Up</Button>
          <Button disabled={disabled || isLast} onClick={onMoveDown} aria-label={`move down ${field.name}`}>Down</Button>
          <Button disabled={disabled} onClick={onCopy} aria-label={`copy ${field.name}`}>Copy</Button>
          <Button disabled={disabled} onClick={onDelete} aria-label={`delete ${field.name}`}>Delete</Button>
        </div>
      </div>
      {errors.length > 0 ? (
        <div aria-label={`validation ${field.name}`} style={fieldErrorListStyle}>
          {errors.map((item) => <div key={`${item.field}-${item.message}`}>{item.field}: {item.message}</div>)}
        </div>
      ) : null}
      <div style={widgetPreviewStyle}>
        <Widget
          field={field}
          value={field.widget === 'Tags' ? [] : ''}
          answer={{}}
          payload={previewPayload}
          readOnly
          onChange={() => undefined}
        />
      </div>
    </section>
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

function PropertyPanel({ field, errors, fields, disabled, onChange }: {
  field: DraftField | null
  errors: DraftValidationError[]
  fields: DraftField[]
  disabled: boolean
  onChange: (patch: Partial<FieldSchema>) => void
}) {
  if (!field) {
    return (
      <>
        <h2 style={subHeadingStyle}>属性</h2>
        <pre style={jsonPreviewStyle}>{JSON.stringify({ fields: fields.map(stripDraftField) }, null, 2)}</pre>
      </>
    )
  }
  const duplicateName = fields.some((candidate) => candidate._draftId !== field._draftId && candidate.name === field.name)
  return (
    <>
      <h2 style={subHeadingStyle}>属性</h2>
      <div style={propertyStackStyle}>
        {errors.length > 0 ? (
          <div aria-label={`selected validation ${field.name}`} style={fieldErrorListStyle}>
            {errors.map((item) => <div key={`${item.field}-${item.message}`}>{item.field}: {item.message}</div>)}
          </div>
        ) : null}
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
    </>
  )
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
        <div key={`${field.name}-tab-${index}`} style={nestedPanelStyle}>
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
          <Button disabled={disabled || tabs.length <= 1} onClick={() => deleteTab(index)} aria-label={`delete tab ${index + 1}`}>Delete tab</Button>
        </div>
      ))}
      <Button disabled={disabled} onClick={addTab} aria-label="add tab">Add tab</Button>
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
      currentIndex === index ? createNestedFieldForWidget(child.name, widget, child.label) : child
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
        <div key={`${label}-child-${index}`} style={nestedFieldRowStyle}>
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
            <Button disabled={disabled || index === 0} onClick={() => moveChild(index, -1)} aria-label={`move up ${label} child ${index + 1}`}>Up</Button>
            <Button disabled={disabled || index === fields.length - 1} onClick={() => moveChild(index, 1)} aria-label={`move down ${label} child ${index + 1}`}>Down</Button>
            <Button disabled={disabled || fields.length <= 1} onClick={() => deleteChild(index)} aria-label={`delete ${label} child ${index + 1}`}>Delete</Button>
          </div>
        </div>
      ))}
      <div style={fieldActionsStyle}>
        {nestedWidgetTypes.map((widget) => (
          <Button key={widget} disabled={disabled} onClick={() => addChild(widget)} aria-label={`add ${label} ${widget}`}>Add {widget}</Button>
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
  return normalizeDraftField({
    _draftId: createDraftId(),
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
  })
}

function createNestedDefaultField(name: string, widget: WidgetType): FieldSchema {
  return createNestedFieldForWidget(name, widget, widgetLabels[widget])
}

function createNestedFieldForWidget(name: string, widget: WidgetType, label: string): FieldSchema {
  return normalizeFieldSchema({
    name,
    widget,
    label,
    required: false,
    ...(widget === 'Radio' || widget === 'Tags' ? { options: ['pass', 'reject', 'uncertain'] } : {}),
    ...(widget === 'ShowItem' ? { path: '$payload', mode: 'auto' as ShowItemMode } : {}),
    ...(widget === 'FileUpload' ? { maxFiles: 3 } : {}),
    ...(widget === 'LLMTrigger' ? { target_field: name, prompt: '请根据 payload 和当前答案给出辅助建议。' } : {}),
  })
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
  return normalizeDraftField({
    ...field,
    _draftId: createDraftId(),
    name: nextCopyFieldName(field.name, current),
  })
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

function createDraftId() {
  return globalThis.crypto?.randomUUID?.() ?? `draft-${Date.now()}-${Math.random().toString(36).slice(2)}`
}

function normalizeDraftField(field: DraftField): DraftField {
  return normalizeFieldSchema(field) as DraftField
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

const mutedStyle: CSSProperties = {
  margin: 'var(--space-xs) 0 0',
  color: 'var(--color-text-secondary)',
}

const designerGridStyle: CSSProperties = {
  display: 'grid',
  gridTemplateColumns: '220px minmax(360px, 1fr) 320px',
  gap: 'var(--space-md)',
  alignItems: 'start',
}

const panelStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-md)',
  padding: 'var(--space-md)',
  border: '1px solid var(--color-border)',
  background: 'var(--color-surface)',
}

const paletteStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-sm)',
}

const canvasStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-sm)',
}

const previewStatusStyle: CSSProperties = {
  padding: 'var(--space-sm)',
  border: '1px solid var(--color-border-light)',
  background: 'var(--color-bg)',
  color: 'var(--color-text-secondary)',
  fontSize: 'var(--text-sm)',
}

const canvasItemStyle: CSSProperties = {
  border: '1px solid var(--color-border-light)',
  background: 'var(--color-bg)',
}

const selectedCanvasItemStyle: CSSProperties = {
  ...canvasItemStyle,
  outline: '2px solid var(--color-accent)',
  outlineOffset: 0,
}

const draggingCanvasItemStyle: CSSProperties = {
  ...selectedCanvasItemStyle,
  opacity: 0.72,
}

const canvasItemHeaderStyle: CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'space-between',
  gap: 'var(--space-sm)',
  padding: 'var(--space-sm)',
  borderBottom: '1px solid var(--color-border-light)',
}

const fieldActionsStyle: CSSProperties = {
  display: 'flex',
  gap: 4,
  alignItems: 'center',
  flexWrap: 'wrap',
  justifyContent: 'flex-end',
}

const selectFieldButtonStyle: CSSProperties = {
  display: 'grid',
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
  padding: 'var(--space-sm)',
}

const fieldErrorListStyle: CSSProperties = {
  display: 'grid',
  gap: 2,
  padding: 'var(--space-xs) var(--space-sm)',
  borderBottom: '1px solid var(--color-border-light)',
  color: 'var(--color-danger)',
  fontSize: 'var(--text-sm)',
}

const fieldStyle: CSSProperties = {
  display: 'grid',
  gap: 4,
  color: 'var(--color-text-secondary)',
  fontSize: 'var(--text-sm)',
}

const inputStyle: CSSProperties = {
  width: '100%',
  minHeight: 36,
  border: '1px solid var(--color-border-light)',
  padding: '0 var(--space-sm)',
  boxSizing: 'border-box',
  color: 'var(--color-text)',
  background: 'var(--color-surface)',
}

const textareaStyle: CSSProperties = {
  ...inputStyle,
  minHeight: 96,
  padding: 'var(--space-sm)',
  resize: 'vertical',
  fontFamily: 'var(--font-body)',
}

const propertyStackStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-sm)',
}

const nestedEditorStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-sm)',
  padding: 'var(--space-sm)',
  border: '1px solid var(--color-border-light)',
  background: 'var(--color-bg)',
}

const nestedEditorLabelStyle: CSSProperties = {
  color: 'var(--color-text-secondary)',
  fontSize: 'var(--text-sm)',
}

const nestedPanelStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-sm)',
  padding: 'var(--space-sm)',
  border: '1px solid var(--color-border-light)',
  background: 'var(--color-surface)',
}

const nestedFieldRowStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-sm)',
  padding: 'var(--space-sm)',
  border: '1px solid var(--color-border-light)',
  background: 'var(--color-surface)',
}

const twoColumnStyle: CSSProperties = {
  display: 'grid',
  gridTemplateColumns: '1fr 1fr',
  gap: 'var(--space-sm)',
}

const checkboxRowStyle: CSSProperties = {
  display: 'flex',
  gap: 'var(--space-sm)',
  alignItems: 'center',
}

const jsonPreviewStyle: CSSProperties = {
  margin: 0,
  padding: 'var(--space-sm)',
  border: '1px solid var(--color-border-light)',
  background: 'var(--color-bg)',
  maxHeight: 520,
  overflow: 'auto',
  whiteSpace: 'pre-wrap',
}

const backLinkStyle: CSSProperties = {
  color: 'var(--color-accent)',
  textDecoration: 'none',
}

const alertStyle: CSSProperties = {
  padding: 'var(--space-sm)',
  border: '1px solid var(--color-danger)',
  color: 'var(--color-danger)',
}

const errorTextStyle: CSSProperties = {
  color: 'var(--color-danger)',
  fontSize: 'var(--text-sm)',
}
