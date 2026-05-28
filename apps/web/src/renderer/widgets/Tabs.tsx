import type { CSSProperties } from 'react'
import type { WidgetProps } from '../types'
import FieldFrame from './FieldFrame'

export default function TabsWidget({ field }: WidgetProps) {
  return (
    <FieldFrame label={field.label} required={field.required}>
      <div style={containerStyle}>
        {(field.tabs ?? []).map((tab) => (
          <section key={tab.label} style={tabStyle}>
            <strong>{tab.label}</strong>
            <span>{tab.fields.map((child) => child.name).join(', ')}</span>
          </section>
        ))}
      </div>
    </FieldFrame>
  )
}

const containerStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-xs)',
}

const tabStyle: CSSProperties = {
  display: 'grid',
  gap: 2,
  padding: 'var(--space-xs) var(--space-sm)',
  border: '1px solid var(--color-border-light)',
  background: 'var(--color-bg)',
  color: 'var(--color-text-secondary)',
  fontSize: 'var(--text-sm)',
}
