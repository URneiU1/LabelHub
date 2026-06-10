import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type React from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ArbitrationQueue from './ArbitrationQueue'
import { apiGet, apiPost } from '../../shared/api/client'

vi.mock('../../shared/api/client', async () => {
  const actual = await vi.importActual<typeof import('../../shared/api/client')>('../../shared/api/client')
  return {
    ...actual,
    apiGet: vi.fn(),
    apiPost: vi.fn(),
  }
})

vi.mock('@douyinfe/semi-ui', () => ({
  Button: ({ children, loading, theme, type, ...props }: React.ButtonHTMLAttributes<HTMLButtonElement> & { loading?: boolean, theme?: string, type?: string }) => {
    void loading
    void theme
    void type
    return <button type="button" {...props}>{children}</button>
  },
  Toast: {
    error: vi.fn(),
    success: vi.fn(),
  },
}))

const mockApiGet = vi.mocked(apiGet)
const mockApiPost = vi.mocked(apiPost)

const itemSchema = JSON.stringify({
  title: 'arbitration',
  layout: 'single_page',
  fields: [{ name: 'summary', widget: 'Input', label: '答案字段' }],
})

function bundle(submissionId: number, itemId: number, labelerId: number, answer: string) {
  return {
    task: { id: 1, title: '仲裁任务', description: null, baselineDescription: null, status: 'published', totalItems: 1, finishedItems: 0 },
    item: { id: itemId, taskId: 1, externalId: 'Q', payload: '{}', status: 'needs_arbitration' },
    template: { id: 101, schemaJson: itemSchema },
    submission: { id: submissionId, taskId: 1, itemId, labelerId, status: 'needs_arbitration' },
    revision: { id: submissionId * 10, answer: JSON.stringify({ summary: answer }), draft: false },
    aiReview: null,
    auditLogs: [],
  }
}

describe('ArbitrationQueue', () => {
  beforeEach(() => {
    mockApiGet.mockReset()
    mockApiPost.mockReset()
    mockApiPost.mockResolvedValue({})
  })

  it('shows an empty state when there is nothing to arbitrate', async () => {
    mockApiGet.mockResolvedValue([])

    render(<ArbitrationQueue />)

    expect(await screen.findByText('暂无待仲裁')).toBeInTheDocument()
  })

  it('groups conflicting submissions by item, shows them side by side, and adopts one as approved', async () => {
    const user = userEvent.setup()
    const submissions = [
      { id: 100, taskId: 1, itemId: 10, labelerId: 2, status: 'needs_arbitration' },
      { id: 101, taskId: 1, itemId: 10, labelerId: 3, status: 'needs_arbitration' },
    ]
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/reviewer/submissions?status=needs_arbitration') {
        return submissions
      }
      if (path === '/reviewer/submissions/100') {
        return bundle(100, 10, 2, '答案甲')
      }
      if (path === '/reviewer/submissions/101') {
        return bundle(101, 10, 3, '答案乙')
      }
      throw new Error(`unexpected GET ${path}`)
    })

    render(<ArbitrationQueue />)

    // 同一 item 的两份冲突聚成一组,显示冲突份数。
    expect(await screen.findByText('2 份冲突')).toBeInTheDocument()
    await user.click(screen.getByText('Item #10'))

    // 并排展示两份冲突答案(各自的作答内容)。
    const inputs = await screen.findAllByLabelText('答案字段')
    expect(inputs).toHaveLength(2)
    expect(inputs[0]).toHaveValue('答案甲')
    expect(inputs[1]).toHaveValue('答案乙')

    // 采纳第一份 → 对该 submission 发 approve;后端会自动打回 sibling。
    const approveButtons = screen.getAllByRole('button', { name: '采纳此份（通过）' })
    await user.click(approveButtons[0])

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith('/submissions/100/review', { verdict: 'approve', reason: '' })
    })
  })
})
