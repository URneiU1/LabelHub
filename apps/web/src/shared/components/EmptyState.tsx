import type { CSSProperties, ReactNode } from 'react'

type EmptyStateVariant = 'empty' | 'error' | 'locked' | 'queue' | 'done'

type EmptyStateProps = {
  title: string
  body?: string
  action?: ReactNode
  variant?: EmptyStateVariant
}

const variantStroke: Record<EmptyStateVariant, string> = {
  empty: 'var(--color-text-muted)',
  error: 'var(--color-danger)',
  locked: 'var(--color-amber)',
  queue: 'var(--color-accent)',
  done: 'var(--color-success)',
}

export default function EmptyState({ title, body, action, variant = 'empty' }: EmptyStateProps) {
  return (
    <section role="status" aria-label={title} style={containerStyle}>
      <LineIllustration stroke={variantStroke[variant]} />
      <div style={copyStackStyle}>
        <h2 style={titleStyle}>{title}</h2>
        {body ? <p style={bodyStyle}>{body}</p> : null}
      </div>
      {action ? <div style={actionStyle}>{action}</div> : null}
    </section>
  )
}

function LineIllustration({ stroke }: { stroke: string }) {
  return (
    <svg aria-hidden="true" width="96" height="54" viewBox="0 0 96 54" fill="none">
      <path d="M12 39H84" stroke={stroke} strokeWidth="1.5" strokeLinecap="round" />
      <path d="M24 39V17H72V39" stroke={stroke} strokeWidth="1.5" />
      <path d="M34 27H62" stroke={stroke} strokeWidth="1.5" strokeLinecap="round" />
      <path d="M34 33H53" stroke={stroke} strokeWidth="1.5" strokeLinecap="round" />
      <circle cx="72" cy="17" r="6" stroke={stroke} strokeWidth="1.5" />
    </svg>
  )
}

const containerStyle: CSSProperties = {
  display: 'grid',
  justifyItems: 'center',
  gap: 'var(--space-sm)',
  padding: 'var(--space-xl)',
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-lg)',
  background: 'var(--color-surface-subtle)',
  color: 'var(--color-text)',
  textAlign: 'center',
}

const copyStackStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-xs)',
}

const titleStyle: CSSProperties = {
  margin: 0,
  fontFamily: 'var(--font-heading)',
  fontSize: 'var(--text-h1)',
  fontWeight: 600,
}

const bodyStyle: CSSProperties = {
  margin: 0,
  maxWidth: 420,
  color: 'var(--color-text-secondary)',
  fontSize: 'var(--text-base)',
  lineHeight: 1.6,
}

const actionStyle: CSSProperties = {
  marginTop: 'var(--space-xs)',
}
