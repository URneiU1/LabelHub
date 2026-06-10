import { useEffect, useState, type CSSProperties } from 'react'
import type { AnswerValue, FieldSchema, RenderPayload, RenderRuntime, TemplateSchema, ValidationError, VisibleWhen } from './types'
import { widgetRegistry } from './widgets'
import FieldFrame from './widgets/FieldFrame'

type SchemaRendererProps = {
  schema: TemplateSchema
  payload?: RenderPayload
  value?: AnswerValue
  readOnly?: boolean
  errors?: ValidationError[]
  runtime?: RenderRuntime
  onChange?: (next: AnswerValue) => void
}

export default function SchemaRenderer({
  schema,
  payload = {},
  value = {},
  readOnly = false,
  errors = [],
  runtime,
  onChange,
}: SchemaRendererProps) {
  useEffect(() => {
    const next = pruneHiddenAnswerValues(schema, value)
    if (next !== value) {
      onChange?.(next)
    }
  }, [schema, value, onChange])

  function updateField(name: string, nextValue: unknown) {
    onChange?.(pruneHiddenAnswerValues(schema, { ...value, [name]: nextValue }))
  }

  return (
    <form aria-label={schema.title} style={formStyle}>
      {schema.fields.map((field) => renderField(field, value, payload, readOnly, errors, runtime, updateField))}
    </form>
  )
}

function renderField(
  field: FieldSchema,
  value: AnswerValue,
  payload: RenderPayload,
  readOnly: boolean,
  errors: ValidationError[],
  runtime: RenderRuntime | undefined,
  updateField: (name: string, nextValue: unknown) => void,
) {
  if (!visibleWhenMatches(field.visibleWhen, value)) {
    return null
  }
  if (field.widget === 'Group') {
    return (
      <div key={field.name} style={fieldBlockStyle} data-widget={field.widget}>
        <FieldFrame label={field.label} required={field.required}>
          <div style={groupStyle}>
            {(field.fields ?? []).map((child) => renderField(child, value, payload, readOnly, errors, runtime, updateField))}
          </div>
        </FieldFrame>
      </div>
    )
  }
  if (field.widget === 'Tabs') {
    return (
      <TabsField
        key={field.name}
        field={field}
        value={value}
        payload={payload}
        readOnly={readOnly}
        errors={errors}
        runtime={runtime}
        updateField={updateField}
      />
    )
  }

  const Widget = widgetRegistry[field.widget]
  const fieldErrors = errors.filter((error) => error.field === field.name)
  return (
    <div key={field.name} style={fieldBlockStyle} data-widget={field.widget}>
      <Widget
        field={field}
        value={value[field.name]}
        answer={value}
        payload={payload}
        runtime={runtime}
        readOnly={readOnly}
        onChange={updateField}
      />
      {fieldErrors.map((error) => (
        <div key={error.message} role="alert" style={errorStyle}>{error.message}</div>
      ))}
    </div>
  )
}

function pruneHiddenAnswerValues(schema: TemplateSchema, answer: AnswerValue): AnswerValue {
  const visibleNames = collectVisibleLeafFieldNames(schema.fields, answer)
  const allNames = collectLeafFieldNames(schema.fields)
  let changed = false
  const next: AnswerValue = {}
  for (const [key, value] of Object.entries(answer)) {
    if (allNames.has(key) && !visibleNames.has(key)) {
      changed = true
      continue
    }
    next[key] = value
  }
  return changed ? next : answer
}

function collectVisibleLeafFieldNames(fields: FieldSchema[], answer: AnswerValue, names = new Set<string>()) {
  for (const field of fields) {
    if (!visibleWhenMatches(field.visibleWhen, answer)) {
      continue
    }
    if (field.widget === 'Group' && field.fields) {
      collectVisibleLeafFieldNames(field.fields, answer, names)
      continue
    }
    if (field.widget === 'Tabs' && field.tabs) {
      for (const tab of field.tabs) {
        collectVisibleLeafFieldNames(tab.fields, answer, names)
      }
      continue
    }
    if (field.widget !== 'ShowItem') {
      names.add(field.name)
    }
  }
  return names
}

function TabsField({
  field,
  value,
  payload,
  readOnly,
  errors,
  runtime,
  updateField,
}: {
  field: FieldSchema
  value: AnswerValue
  payload: RenderPayload
  readOnly: boolean
  errors: ValidationError[]
  runtime: RenderRuntime | undefined
  updateField: (name: string, nextValue: unknown) => void
}) {
  const tabs = field.tabs ?? []
  const [activeIndex, setActiveIndex] = useState(0)
  const safeIndex = tabs.length === 0 ? 0 : Math.min(activeIndex, tabs.length - 1)
  const activeTab = tabs[safeIndex]
  const tabErrorCounts = tabs.map((tab) => countFieldErrors(tab.fields, errors))
  const firstErrorTabIndex = tabErrorCounts.findIndex((count) => count > 0)

  useEffect(() => {
    // Validation errors are passed in from the parent submit flow; jump to the first errored tab
    // so the inline message is visible instead of leaving it hidden on an inactive tab.
    if (firstErrorTabIndex >= 0) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setActiveIndex(firstErrorTabIndex)
    }
  }, [firstErrorTabIndex])

  return (
    <div style={fieldBlockStyle} data-widget={field.widget}>
      <FieldFrame label={field.label} required={field.required}>
        {tabs.length > 0 ? (
          <>
            <div role="tablist" aria-label={field.label} style={tabListStyle}>
              {tabs.map((tab, index) => {
                const selected = index === safeIndex
                const tabId = `${field.name}-tab-${index}`
                const panelId = `${field.name}-panel-${index}`
                return (
                  <button
                    key={tab.label}
                    id={tabId}
                    type="button"
                    role="tab"
                    aria-selected={selected}
                    aria-controls={panelId}
                    onClick={() => setActiveIndex(index)}
                    style={selected ? tabButtonActiveStyle : tabButtonStyle}
                  >
                    {tab.label}
                    {tabErrorCounts[index] > 0 ? (
                      <span aria-hidden="true" style={tabErrorBadgeStyle}>{tabErrorCounts[index]}</span>
                    ) : null}
                  </button>
                )
              })}
            </div>
            <section
              id={`${field.name}-panel-${safeIndex}`}
              role="tabpanel"
              aria-labelledby={`${field.name}-tab-${safeIndex}`}
              style={tabPanelStyle}
            >
              <div style={groupStyle}>
                {activeTab.fields.map((child) => renderField(child, value, payload, readOnly, errors, runtime, updateField))}
              </div>
            </section>
          </>
        ) : (
          <div style={emptyTabsStyle}>未配置 tab</div>
        )}
      </FieldFrame>
    </div>
  )
}

const formStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-lg)',
}

const fieldBlockStyle: CSSProperties = {
  minWidth: 0,
}

const groupStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-md)',
  padding: 'var(--space-lg)',
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-md)',
  background: 'var(--color-canvas)',
  borderLeft: '3px solid var(--color-rail)',
}

const tabPanelStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-md)',
  padding: 'var(--space-lg)',
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-md)',
  background: 'var(--color-canvas)',
}

const tabListStyle: CSSProperties = {
  display: 'flex',
  flexWrap: 'wrap',
  gap: 'var(--space-xs)',
  marginBottom: 'var(--space-md)',
}

const tabButtonStyle: CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  gap: 'var(--space-xs)',
  minHeight: 32,
  padding: '0 var(--space-md)',
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-md)',
  background: 'var(--color-surface)',
  color: 'var(--color-text-secondary)',
  cursor: 'pointer',
  fontWeight: 600,
}

const tabButtonActiveStyle: CSSProperties = {
  ...tabButtonStyle,
  border: '1px solid var(--color-accent)',
  background: 'var(--color-accent-soft)',
  color: 'var(--color-accent)',
}

const emptyTabsStyle: CSSProperties = {
  padding: 'var(--space-md)',
  border: '1px dashed var(--color-border-light)',
  color: 'var(--color-text-muted)',
}

const errorStyle: CSSProperties = {
  marginTop: 6,
  color: 'var(--color-danger)',
  fontSize: 'var(--text-sm)',
  display: 'flex',
  alignItems: 'center',
  gap: 4,
}

const tabErrorBadgeStyle: CSSProperties = {
  minWidth: 18,
  height: 18,
  padding: '0 6px',
  borderRadius: 999,
  background: 'var(--color-danger-soft)',
  color: 'var(--color-danger)',
  fontSize: 'var(--text-sm)',
  lineHeight: '18px',
  textAlign: 'center',
}

function visibleWhenMatches(condition: VisibleWhen | undefined, answer: AnswerValue) {
  if (!condition) {
    return true
  }
  const value = answer[condition.field]
  if ('equals' in condition) {
    return value === condition.equals
  }
  return condition.notEmpty === true && !isEmptyValue(value)
}

function isEmptyValue(value: unknown) {
  if (value === undefined || value === null) {
    return true
  }
  if (typeof value === 'string') {
    return value.trim() === ''
  }
  if (Array.isArray(value)) {
    return value.length === 0
  }
  return false
}

function countFieldErrors(fields: FieldSchema[], errors: ValidationError[]) {
  const fieldNames = collectLeafFieldNames(fields)
  return errors.reduce((count, error) => count + (fieldNames.has(error.field) ? 1 : 0), 0)
}

function collectLeafFieldNames(fields: FieldSchema[], names = new Set<string>()) {
  for (const field of fields) {
    if (field.widget === 'Group' && field.fields) {
      collectLeafFieldNames(field.fields, names)
      continue
    }
    if (field.widget === 'Tabs' && field.tabs) {
      for (const tab of field.tabs) {
        collectLeafFieldNames(tab.fields, names)
      }
      continue
    }
    names.add(field.name)
  }
  return names
}
