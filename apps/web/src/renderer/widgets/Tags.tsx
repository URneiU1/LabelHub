import type { CSSProperties } from 'react'
import type { FieldOption, WidgetProps } from '../types'
import FieldFrame from './FieldFrame'

export default function TagsWidget({ field, value, readOnly, onChange }: WidgetProps) {
  const selected = Array.isArray(value) ? value.map(String) : []
  const options = normalizedOptions(field.options)

  function toggle(option: FieldOption) {
    const key = String(option)
    const next = selected.includes(key)
      ? selected.filter((item) => item !== key)
      : [...selected, key]
    onChange(field.name, next)
  }

  return (
    <FieldFrame label={field.label} required={field.required}>
      <div style={optionRowStyle}>
        {options.map((option) => (
          <label key={String(option)} style={optionStyle}>
            <input
              type="checkbox"
              checked={selected.includes(String(option))}
              disabled={readOnly}
              onChange={() => toggle(option)}
            />
            {String(option)}
          </label>
        ))}
      </div>
    </FieldFrame>
  )
}

function normalizedOptions(options?: FieldOption[]) {
  return options && options.length > 0 ? options : []
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
