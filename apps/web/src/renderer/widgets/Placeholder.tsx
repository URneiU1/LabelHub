import type { CSSProperties } from 'react'
import type { WidgetProps } from '../types'
import FieldFrame from './FieldFrame'

export default function PlaceholderWidget({ field, value }: WidgetProps) {
  return (
    <FieldFrame label={field.label} required={field.required}>
      <pre style={placeholderStyle}>{formatValue(value)}</pre>
    </FieldFrame>
  )
}

function formatValue(value: unknown) {
  if (value === undefined || value === null || value === '') {
    return `${fieldPendingText}`
  }
  if (typeof value === 'string') {
    return value
  }
  return JSON.stringify(value, null, 2)
}

const fieldPendingText = 'Day 3 widget implementation pending'

const placeholderStyle: CSSProperties = {
  margin: 0,
  padding: 'var(--space-sm)',
  border: '1px dashed var(--color-border-light)',
  background: 'var(--color-bg)',
  color: 'var(--color-text-secondary)',
  whiteSpace: 'pre-wrap',
}
