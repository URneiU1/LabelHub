import { useState, type CSSProperties } from 'react'
import type { AnswerValue, FieldSchema, RenderPayload, RenderRuntime, TemplateSchema, ValidationError } from './types'
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
  function updateField(name: string, nextValue: unknown) {
    onChange?.({ ...value, [name]: nextValue })
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
