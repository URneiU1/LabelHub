import type { CSSProperties } from 'react'
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
      <div key={field.name} style={fieldBlockStyle} data-widget={field.widget}>
        <FieldFrame label={field.label} required={field.required}>
          <div style={tabStackStyle}>
            {(field.tabs ?? []).map((tab) => (
              <section key={tab.label} style={tabPanelStyle} aria-label={tab.label}>
                <h3 style={tabHeadingStyle}>{tab.label}</h3>
                <div style={groupStyle}>
                  {tab.fields.map((child) => renderField(child, value, payload, readOnly, errors, runtime, updateField))}
                </div>
              </section>
            ))}
          </div>
        </FieldFrame>
      </div>
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

const tabStackStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-md)',
}

const tabPanelStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-md)',
  padding: 'var(--space-lg)',
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-md)',
  background: 'var(--color-canvas)',
}

const tabHeadingStyle: CSSProperties = {
  margin: 0,
  fontFamily: 'var(--font-heading)',
  fontSize: 'var(--text-base)',
  fontWeight: 600,
  color: 'var(--color-text)',
  borderBottom: '2px solid var(--color-accent)',
  width: 'fit-content',
  paddingBottom: 4,
}

const errorStyle: CSSProperties = {
  marginTop: 6,
  color: 'var(--color-danger)',
  fontSize: 'var(--text-sm)',
  display: 'flex',
  alignItems: 'center',
  gap: 4,
}
