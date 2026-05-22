import type { CSSProperties } from 'react'
import type { WidgetProps } from '../types'
import FieldFrame from './FieldFrame'
import { inputStyle } from './styles'

export default function RichTextWidget({ field, value, readOnly, onChange }: WidgetProps) {
  return (
    <FieldFrame label={field.label} required={field.required}>
      <textarea
        aria-label={field.label}
        value={typeof value === 'string' ? value : ''}
        disabled={readOnly}
        rows={5}
        style={richTextStyle}
        onChange={(event) => onChange(field.name, event.target.value)}
      />
    </FieldFrame>
  )
}

const richTextStyle: CSSProperties = {
  ...inputStyle,
  resize: 'vertical',
  lineHeight: 1.7,
  minHeight: 120,
}
