import type { WidgetProps } from '../types'
import FieldFrame from './FieldFrame'
import { inputStyle } from './styles'

export default function InputWidget({ field, value, readOnly, onChange }: WidgetProps) {
  return (
    <FieldFrame label={field.label} required={field.required}>
      <input
        aria-label={field.label}
        value={typeof value === 'string' ? value : ''}
        maxLength={field.maxLength}
        disabled={readOnly}
        style={inputStyle}
        onChange={(event) => onChange(field.name, event.target.value)}
      />
    </FieldFrame>
  )
}
