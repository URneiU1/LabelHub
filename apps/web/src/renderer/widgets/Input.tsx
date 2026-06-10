import type { CSSProperties } from 'react'
import type { WidgetProps } from '../types'
import FieldFrame from './FieldFrame'
import { inputStyle } from './styles'

export default function InputWidget({ field, value, readOnly, onChange }: WidgetProps) {
  const text = typeof value === 'string' ? value : ''
  const hasCounter = typeof field.maxLength === 'number'
  return (
    <FieldFrame label={field.label} required={field.required}>
      <input
        aria-label={field.label}
        value={text}
        maxLength={field.maxLength}
        disabled={readOnly}
        style={inputStyle}
        onChange={(event) => onChange(field.name, event.target.value)}
      />
      {hasCounter ? (
        <div style={counterStyle}>
          {text.length} / {field.maxLength}
        </div>
      ) : null}
    </FieldFrame>
  )
}

const counterStyle: CSSProperties = {
  textAlign: 'right',
  fontSize: 12,
  color: 'var(--lh-text-3)',
  marginTop: -4,
}
