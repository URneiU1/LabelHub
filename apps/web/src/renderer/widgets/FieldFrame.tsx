import type { CSSProperties, ReactNode } from 'react'

type FieldFrameProps = {
  label: string
  required?: boolean
  children: ReactNode
}

export default function FieldFrame({ label, required, children }: FieldFrameProps) {
  return (
    <div style={frameStyle}>
      <span style={labelStyle}>
        {label}
        {required ? <span style={requiredStyle}> *</span> : null}
      </span>
      {children}
    </div>
  )
}

const frameStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-sm)',
  marginBottom: 'var(--space-sm)',
  padding: 'var(--space-sm) 0',
}

const labelStyle: CSSProperties = {
  fontFamily: 'var(--font-heading)',
  fontSize: 'var(--text-base)',
  fontWeight: 600,
  color: 'var(--color-text)',
  display: 'flex',
  alignItems: 'center',
  gap: 4,
}

const requiredStyle: CSSProperties = {
  color: 'var(--color-danger)',
  fontSize: 'var(--text-base)',
  lineHeight: 1,
}
