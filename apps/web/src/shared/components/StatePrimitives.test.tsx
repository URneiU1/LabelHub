import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import EmptyState from './EmptyState'
import LoadingBlock from './LoadingBlock'
import StatusBadge from './StatusBadge'
import { statusLabel } from './status'

describe('StatusBadge', () => {
  it('maps workflow states to stable labels and tones', () => {
    const statuses = [
      ['draft', '草稿', 'draft'],
      ['submitted', '已提交', 'submitted'],
      ['ai_reviewing', 'AI 预审', 'ai'],
      ['human_reviewing', '人工审核', 'human'],
      ['approved', '已通过', 'approved'],
      ['rejected', '已拒绝', 'rejected'],
      ['revising', '待修改', 'revising'],
    ] as const

    for (const [status, label, tone] of statuses) {
      const { container, unmount } = render(<StatusBadge status={status} />)
      expect(screen.getByText(label)).toBeInTheDocument()
      expect(container.querySelector('[data-tone]')).toHaveAttribute('data-tone', tone)
      unmount()
    }
  })

  it('keeps unknown status visible', () => {
    render(<StatusBadge status="archived" />)
    expect(screen.getByText('archived')).toBeInTheDocument()
    expect(statusLabel('queued')).toBe('排队中')
  })
})

describe('EmptyState', () => {
  it('renders copy and optional action', () => {
    render(<EmptyState title="暂无任务" body="当前筛选下没有可处理数据。" action={<button type="button">刷新</button>} />)

    expect(screen.getByRole('status', { name: '暂无任务' })).toBeInTheDocument()
    expect(screen.getByText('当前筛选下没有可处理数据。')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '刷新' })).toBeInTheDocument()
  })
})

describe('LoadingBlock', () => {
  it('renders accessible busy state', () => {
    render(<LoadingBlock title="加载任务" rows={2} />)

    expect(screen.getByRole('status', { name: '加载任务' })).toHaveAttribute('aria-busy', 'true')
    expect(document.querySelectorAll('.lh-skeleton-line')).toHaveLength(2)
  })
})
