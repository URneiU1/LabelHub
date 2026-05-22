import type { CSSProperties } from 'react'
import type { WidgetProps } from '../types'
import FieldFrame from './FieldFrame'
import { inputStyle } from './styles'

export default function TextAreaWidget({ field, value, readOnly, onChange }: WidgetProps) {
  return (
    <FieldFrame label={field.label} required={field.required}>
      <textarea
        aria-label={field.label}
        value={typeof value === 'string' ? value : ''}
        maxLength={field.maxLength}
        disabled={readOnly}
        rows={4}
        style={textareaStyle}
        onChange={(event) => onChange(field.name, event.target.value)}
      />
    </FieldFrame>
  )
}

const textareaStyle: CSSProperties = {
  ...inputStyle,
  resize: 'vertical',
  lineHeight: 1.6,
}
