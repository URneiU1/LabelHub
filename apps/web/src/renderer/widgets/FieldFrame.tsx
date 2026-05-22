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
  gap: 'var(--space-xs)',
}

const labelStyle: CSSProperties = {
  fontFamily: 'var(--font-heading)',
  fontWeight: 600,
}

const requiredStyle: CSSProperties = {
  color: '#b42318',
}
