import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ReviewResultsPanel from './ReviewResultsPanel'
import { listOwnerReviewResults } from '../../shared/api/client'

vi.mock('../../shared/api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../shared/api/client')>()
  return { ...actual, listOwnerReviewResults: vi.fn() }
})

vi.mock('@douyinfe/semi-ui', () => ({
  Toast: { error: vi.fn(), success: vi.fn(), info: vi.fn() },
}))

const mockList = vi.mocked(listOwnerReviewResults)

const page = {
  results: [
    { id: 20, itemId: 200, status: 'approved', aiVerdict: 'pass', aiScore: 0.9, humanVerdict: 'approve', agreed: true, updatedAt: '2026-05-31T00:00:00Z' },
    { id: 19, itemId: 199, status: 'rejected', aiVerdict: 'pass', aiScore: 0.7, humanVerdict: 'reject', agreed: false, updatedAt: '2026-05-31T00:00:00Z' },
  ],
  nextCursor: '',
  hasMore: false,
}

describe('ReviewResultsPanel', () => {
  beforeEach(() => {
    mockList.mockReset()
  })

  it('lists results with AI-vs-human agreement', async () => {
    mockList.mockResolvedValue(page)
    render(<ReviewResultsPanel taskId={5} />)

    expect(await screen.findByText('题目 #200')).toBeInTheDocument()
    expect(screen.getByText('题目 #199')).toBeInTheDocument()
    expect(screen.getByText('一致')).toBeInTheDocument()
    expect(screen.getByText('不一致')).toBeInTheDocument()
  })

  it('filters to only AI-vs-human disagreements', async () => {
    mockList.mockResolvedValue(page)
    const user = userEvent.setup()
    render(<ReviewResultsPanel taskId={5} />)

    await screen.findByText('题目 #200')
    await user.click(screen.getByLabelText('只看 AI 与人工不一致'))

    await waitFor(() => expect(screen.queryByText('题目 #200')).toBeNull())
    expect(screen.getByText('题目 #199')).toBeInTheDocument()
  })

  it('shows an empty state when there are no results', async () => {
    mockList.mockResolvedValue({ results: [], nextCursor: '', hasMore: false })
    render(<ReviewResultsPanel taskId={5} />)

    expect(await screen.findByText('暂无审核结果')).toBeInTheDocument()
  })
})
