import type { CSSProperties } from 'react'

type LoadingBlockProps = {
  title?: string
  rows?: number
}

export default function LoadingBlock({ title = '加载中', rows = 3 }: LoadingBlockProps) {
  return (
    <section role="status" aria-busy="true" aria-label={title} style={containerStyle}>
      <div style={titleStyle}>{title}</div>
      <div style={stackStyle}>
        {Array.from({ length: rows }).map((_, index) => (
          <div
            key={index}
            className="lh-skeleton-line"
            style={{ width: `${Math.max(42, 92 - index * 16)}%` }}
          />
        ))}
      </div>
    </section>
  )
}

const containerStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-md)',
  padding: 'var(--space-lg)',
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-lg)',
  background: 'var(--color-surface)',
}

const titleStyle: CSSProperties = {
  color: 'var(--color-text-secondary)',
  fontSize: 'var(--text-sm)',
  fontWeight: 700,
}

const stackStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-sm)',
}
