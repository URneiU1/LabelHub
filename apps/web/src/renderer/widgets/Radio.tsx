import type { CSSProperties } from 'react'
import type { FieldOption, WidgetProps } from '../types'
import FieldFrame from './FieldFrame'

export default function RadioWidget({ field, value, readOnly, onChange }: WidgetProps) {
  const options = normalizedOptions(field.options)
  return (
    <FieldFrame label={field.label} required={field.required}>
      <div role="radiogroup" aria-label={field.label} style={optionRowStyle}>
        {options.map((option) => (
          <label key={String(option)} style={optionStyle}>
            <input
              type="radio"
              name={field.name}
              checked={value === option}
              disabled={readOnly}
              onChange={() => onChange(field.name, option)}
            />
            {String(option)}
          </label>
        ))}
      </div>
    </FieldFrame>
  )
}

function normalizedOptions(options?: FieldOption[]) {
  return options && options.length > 0 ? options : [1, 2, 3, 4, 5]
}

const optionRowStyle: CSSProperties = {
  display: 'flex',
  gap: 'var(--space-sm)',
  flexWrap: 'wrap',
}

const optionStyle: CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  gap: 4,
  padding: '4px 8px',
  border: '1px solid var(--color-border-light)',
  background: 'var(--color-surface)',
}
