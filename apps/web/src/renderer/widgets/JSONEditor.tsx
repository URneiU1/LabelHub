import type { CSSProperties } from 'react'
import type { WidgetProps } from '../types'
import FieldFrame from './FieldFrame'
import { inputStyle } from './styles'

export default function JSONEditorWidget({ field, value, readOnly, onChange }: WidgetProps) {
  function normalize(next: string) {
    try {
      onChange(field.name, JSON.parse(next) as unknown)
    } catch {
      onChange(field.name, next)
    }
  }

  return (
    <FieldFrame label={field.label} required={field.required}>
      <textarea
        aria-label={field.label}
        value={stringifyValue(value)}
        disabled={readOnly}
        rows={7}
        spellCheck={false}
        style={jsonStyle}
        onBlur={(event) => normalize(event.target.value)}
        onChange={(event) => onChange(field.name, event.target.value)}
      />
    </FieldFrame>
  )
}

function stringifyValue(value: unknown) {
  if (value === undefined || value === null || value === '') {
    return '{\n  \n}'
  }
  if (typeof value === 'string') {
    return value
  }
  return JSON.stringify(value, null, 2)
}

const jsonStyle: CSSProperties = {
  ...inputStyle,
  resize: 'vertical',
  lineHeight: 1.6,
  minHeight: 160,
  fontFamily: 'var(--font-mono)',
}
