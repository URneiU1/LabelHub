import type { CSSProperties } from 'react'
import type { FieldOption, WidgetProps } from '../types'
import FieldFrame from './FieldFrame'

export default function RadioWidget({ field, value, readOnly, onChange }: WidgetProps) {
  const options = normalizedOptions(field.options)
  return (
    <FieldFrame label={field.label} required={field.required}>
      <div role="radiogroup" aria-label={field.label} style={optionRowStyle}>
        {options.map((option) => {
          const selected = value === option
          return (
            <label key={String(option)} style={optionStyle(selected, readOnly)}>
              <input
                type="radio"
                name={field.name}
                checked={selected}
                disabled={readOnly}
                onChange={() => onChange(field.name, option)}
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
  return options && options.length > 0 ? options : [1, 2, 3, 4, 5]
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
    border: selected ? '1px solid var(--lh-primary)' : '1px solid var(--lh-border)',
    background: selected ? 'var(--lh-primary)' : '#fff',
    color: selected ? '#fff' : 'var(--lh-text-1)',
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
