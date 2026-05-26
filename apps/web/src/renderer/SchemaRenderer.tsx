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
  gap: 'var(--space-md)',
}

const fieldBlockStyle: CSSProperties = {
  minWidth: 0,
}

const groupStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-md)',
  padding: 'var(--space-sm)',
  border: '1px solid var(--color-border-light)',
  background: 'var(--color-bg)',
}

const tabStackStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-sm)',
}

const tabPanelStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-sm)',
  padding: 'var(--space-sm)',
  border: '1px solid var(--color-border-light)',
  background: 'var(--color-bg)',
}

const tabHeadingStyle: CSSProperties = {
  margin: 0,
  fontFamily: 'var(--font-heading)',
  fontSize: 'var(--text-base)',
}

const errorStyle: CSSProperties = {
  marginTop: 4,
  color: 'var(--color-danger, #b42318)',
  fontSize: 'var(--text-sm)',
}
