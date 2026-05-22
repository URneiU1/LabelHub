import type { CSSProperties } from 'react'
import type { AnswerValue, RenderPayload, RenderRuntime, TemplateSchema, ValidationError } from './types'
import { widgetRegistry } from './widgets'

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
      {schema.fields.map((field) => {
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
      })}
    </form>
  )
}

const formStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-md)',
}

const fieldBlockStyle: CSSProperties = {
  minWidth: 0,
}

const errorStyle: CSSProperties = {
  marginTop: 4,
  color: 'var(--color-danger, #b42318)',
  fontSize: 'var(--text-sm)',
}
