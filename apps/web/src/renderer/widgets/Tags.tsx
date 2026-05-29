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
        {options.map((option) => {
          const isOn = selected.includes(String(option))
          return (
            <label key={String(option)} style={optionStyle(isOn, readOnly)}>
              <input
                type="checkbox"
                checked={isOn}
                disabled={readOnly}
                onChange={() => toggle(option)}
                style={visuallyHiddenInputStyle}
              />
              {String(option)}
            </label>
          )
        })}
      </div>
    </FieldFrame>
  )
}

function normalizedOptions(options?: FieldOption[]) {
  return options && options.length > 0 ? options : []
}

const optionRowStyle: CSSProperties = {
  display: 'flex',
  gap: 10,
  flexWrap: 'wrap',
}

function optionStyle(selected: boolean, readOnly?: boolean): CSSProperties {
  return {
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    padding: '7px 18px',
    borderRadius: 999,
    border: selected ? '1px solid #c3dafe' : '1px solid var(--lh-border)',
    background: selected ? 'var(--lh-primary-soft)' : '#fff',
    color: selected ? 'var(--lh-primary)' : 'var(--lh-text-1)',
    fontSize: 13,
    fontWeight: selected ? 500 : 400,
    cursor: readOnly ? 'default' : 'pointer',
    userSelect: 'none',
    transition: 'background var(--duration-fast), border-color var(--duration-fast)',
  }
}

const visuallyHiddenInputStyle: CSSProperties = {
  position: 'absolute',
  width: 1,
  height: 1,
  padding: 0,
  margin: -1,
  overflow: 'hidden',
  clip: 'rect(0 0 0 0)',
  whiteSpace: 'nowrap',
  border: 0,
}
