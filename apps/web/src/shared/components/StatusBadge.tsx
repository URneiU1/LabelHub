import type { CSSProperties } from 'react'
import { normalizeStatus, statusMeta, type StatusMeta } from './status'

const toneStyle: Record<StatusMeta['tone'], CSSProperties> = {
  draft: { background: 'var(--status-draft-bg)', color: 'var(--status-draft-fg)' },
  submitted: { background: 'var(--status-submitted-bg)', color: 'var(--status-submitted-fg)' },
  ai: { background: 'var(--status-ai-bg)', color: 'var(--status-ai-fg)' },
  human: { background: 'var(--status-human-bg)', color: 'var(--status-human-fg)' },
  approved: { background: 'var(--status-approved-bg)', color: 'var(--status-approved-fg)' },
  rejected: { background: 'var(--status-rejected-bg)', color: 'var(--status-rejected-fg)' },
  revising: { background: 'var(--status-revising-bg)', color: 'var(--status-revising-fg)' },
}

export default function StatusBadge({ status, label }: { status: string | null | undefined, label?: string }) {
  const key = normalizeStatus(status)
  const meta = statusMeta[key] ?? { label: status || '未知', tone: 'draft' as const }
  return (
    <span style={{ ...baseStyle, ...toneStyle[meta.tone] }} data-status={key} data-tone={meta.tone}>
      {label ?? meta.label}
    </span>
  )
}

const baseStyle: CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  justifyContent: 'center',
  minHeight: 22,
  minWidth: 58,
  padding: '2px 8px',
  borderRadius: 'var(--radius-md)',
  fontFamily: 'var(--font-body)',
  fontSize: 'var(--text-sm)',
  fontWeight: 700,
  lineHeight: 1.2,
  whiteSpace: 'nowrap',
}
