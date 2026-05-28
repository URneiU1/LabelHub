import type { CSSProperties } from 'react'
import type { WidgetProps } from '../types'
import FieldFrame from './FieldFrame'

export default function GroupWidget({ field }: WidgetProps) {
  return (
    <FieldFrame label={field.label} required={field.required}>
      <div style={containerStyle}>
        {(field.fields ?? []).map((child) => (
          <div key={child.name} style={itemStyle}>
            <strong>{child.name}</strong>
            <span>{child.widget} · {child.label}</span>
          </div>
        ))}
      </div>
    </FieldFrame>
  )
}

const containerStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-xs)',
}

const itemStyle: CSSProperties = {
  display: 'flex',
  justifyContent: 'space-between',
  gap: 'var(--space-sm)',
  padding: 'var(--space-xs) var(--space-sm)',
  border: '1px solid var(--color-border-light)',
  background: 'var(--color-bg)',
  color: 'var(--color-text-secondary)',
  fontSize: 'var(--text-sm)',
}
