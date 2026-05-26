import type { CSSProperties } from 'react'

export const inputStyle: CSSProperties = {
  width: '100%',
  minHeight: 40,
  padding: 'var(--space-sm) var(--space-md)',
  border: '1px solid var(--color-border)',
  borderRadius: 'var(--radius-sm)',
  background: 'var(--color-surface)',
  fontFamily: 'var(--font-body)',
  fontSize: 'var(--text-base)',
  boxSizing: 'border-box',
  transition: 'border-color var(--duration-fast)',
}
