import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type React from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import LabelerPlaza from './Plaza'
import { apiGet, apiPost } from '../../shared/api/client'

vi.mock('../../shared/api/client', () => ({
  apiGet: vi.fn(),
  apiPost: vi.fn(),
}))

vi.mock('@douyinfe/semi-ui', () => ({
  Button: ({ children, loading, theme, ...props }: React.ButtonHTMLAttributes<HTMLButtonElement> & { loading?: boolean, theme?: string }) => {
    void loading
    void theme
    return <button type="button" {...props}>{children}</button>
  },
  Input: ({ value, onChange, showClear, ...props }: { value?: string, onChange?: (v: string) => void, showClear?: boolean } & Record<string, unknown>) => {
    void showClear
    return <input value={value} onChange={(e) => onChange?.(e.target.value)} {...props} />
  },
  Select: ({ value, onChange, optionList, ...props }: { value?: string, onChange?: (v: string) => void, optionList?: Array<{ label: string, value: string }> } & Record<string, unknown>) => (
    <select value={value} onChange={(e) => onChange?.(e.target.value)} {...props}>
      {(optionList ?? []).map((opt) => (
        <option key={opt.value} value={opt.value}>{opt.label}</option>
      ))}
    </select>
  ),
  Toast: {
    error: vi.fn(),
    success: vi.fn(),
    info: vi.fn(),
  },
}))

const mockApiGet = vi.mocked(apiGet)
const mockApiPost = vi.mocked(apiPost)

const task = {
  id: 1,
  title: 'QA 质量标注',
  description: null,
  baselineDescription: null,
  status: 'published',
  totalItems: 1,
  finishedItems: 0,
}

const item = {
  id: 11,
  taskId: 1,
  externalId: 'Q0001',
  payload: JSON.stringify({ prompt: '光合作用发生在哪里？' }),
  status: 'claimed',
}

const itemNav = {
  taskId: 1,
  total: 1,
  items: [{ itemId: 11, externalId: 'Q0001', status: 'claimed', mine: true, submissionId: null }],
  counts: { claimed: 1 },
}

describe('LabelerPlaza schema runtime flow', () => {
  beforeEach(() => {
    mockApiGet.mockReset()
    mockApiPost.mockReset()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/labeler/tasks') {
        return [task]
      }
      if (path === '/me/submissions') {
        return []
      }
      if (path === '/tasks/1/labeler/items') {
        return itemNav
      }
      throw new Error(`unexpected GET ${path}`)
    })
  })

  it('claims a task bundle, renders schema fields, and submits collected answer', async () => {
    const user = userEvent.setup()
    const schema = {
      title: 'qa_runtime',
      layout: 'single_page',
      fields: [
        { name: 'summary', widget: 'Input', label: '一句话总评', required: true },
      ],
    }
    mockApiPost.mockImplementation(async (path) => {
      if (path === '/tasks/1/claim') {
        return {
          task,
          item,
          template: { id: 101, schemaJson: JSON.stringify(schema) },
          submission: { id: 42, taskId: 1, itemId: 11, status: 'draft' },
          revision: null,
        }
      }
      if (path === '/tasks/1/items/11/submit') {
        return { id: 42, taskId: 1, itemId: 11, status: 'human_reviewing' }
      }
      throw new Error(`unexpected POST ${path}`)
    })

    render(<LabelerPlaza />)

    await user.click(await screen.findByRole('button', { name: '领取题目 QA 质量标注' }))
    await user.type(await screen.findByLabelText('一句话总评'), '回答准确')
    await user.click(screen.getByRole('button', { name: '提交审核' }))

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith('/tasks/1/items/11/submit', { answer: { summary: '回答准确' } })
    })
  })

  it('submits the active answer via Ctrl/Cmd+Enter', async () => {
    const user = userEvent.setup()
    const schema = {
      title: 'qa_runtime',
      layout: 'single_page',
      fields: [
        { name: 'summary', widget: 'Input', label: '一句话总评', required: true },
      ],
    }
    mockApiPost.mockImplementation(async (path) => {
      if (path === '/tasks/1/claim') {
        return {
          task,
          item,
          template: { id: 101, schemaJson: JSON.stringify(schema) },
          submission: { id: 42, taskId: 1, itemId: 11, status: 'draft' },
          revision: null,
        }
      }
      if (path === '/tasks/1/items/11/submit') {
        return { id: 42, taskId: 1, itemId: 11, status: 'human_reviewing' }
      }
      throw new Error(`unexpected POST ${path}`)
    })

    render(<LabelerPlaza />)

    await user.click(await screen.findByRole('button', { name: '领取题目 QA 质量标注' }))
    await user.type(await screen.findByLabelText('一句话总评'), '回答准确')
    fireEvent.keyDown(window, { key: 'Enter', ctrlKey: true })

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith('/tasks/1/items/11/submit', { answer: { summary: '回答准确' } })
    })
  })

  it('auto-saves changed answers after a 3s debounce', async () => {
    const schema = {
      title: 'qa_autosave',
      layout: 'single_page',
      fields: [
        { name: 'summary', widget: 'Input', label: '一句话总评', required: true },
      ],
    }
    mockApiPost.mockImplementation(async (path) => {
      if (path === '/tasks/1/claim') {
        return {
          task,
          item,
          template: { id: 101, schemaJson: JSON.stringify(schema) },
          submission: { id: 42, taskId: 1, itemId: 11, status: 'draft' },
          revision: null,
        }
      }
      if (path === '/tasks/1/items/11/draft') {
        return { id: 42, taskId: 1, itemId: 11, status: 'draft' }
      }
      throw new Error(`unexpected POST ${path}`)
    })

    try {
      render(<LabelerPlaza />)

      fireEvent.click(await screen.findByRole('button', { name: '领取题目 QA 质量标注' }))
      const input = await screen.findByLabelText('一句话总评')

      vi.useFakeTimers()
      fireEvent.change(input, { target: { value: '自动保存答案' } })

      await act(async () => {
        await vi.advanceTimersByTimeAsync(2999)
      })
      expect(mockApiPost).not.toHaveBeenCalledWith('/tasks/1/items/11/draft', { answer: { summary: '自动保存答案' } })

      await act(async () => {
        await vi.advanceTimersByTimeAsync(1)
      })
      vi.useRealTimers()

      await waitFor(() => {
        expect(mockApiPost).toHaveBeenCalledWith('/tasks/1/items/11/draft', { answer: { summary: '自动保存答案' } })
      })
      expect(screen.getByText('已自动保存')).toBeInTheDocument()
    } finally {
      vi.useRealTimers()
    }
  })

  it('shows schema error banner and disables actions for bad schema', async () => {
    const user = userEvent.setup()
    mockApiPost.mockResolvedValue({
      task,
      item,
      template: {
        id: 101,
        schemaJson: JSON.stringify({
          title: 'bad_schema',
          layout: 'single_page',
          fields: [{ name: 'score', widget: 'Radio' }],
        }),
      },
      submission: { id: 42, taskId: 1, itemId: 11, status: 'draft' },
      revision: null,
    })

    render(<LabelerPlaza />)

    await user.click(await screen.findByRole('button', { name: '领取题目 QA 质量标注' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('fields[0].options')
    expect(screen.getByRole('button', { name: '保存草稿' })).toBeDisabled()
    expect(screen.getByRole('button', { name: '提交审核' })).toBeDisabled()
  })

  it('opens a revising submission from 我的数据 and resubmits it', async () => {
    const user = userEvent.setup()
    const schema = {
      title: 'qa_revision',
      layout: 'single_page',
      fields: [
        { name: 'summary', widget: 'Input', label: '一句话总评', required: true },
      ],
    }
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/labeler/tasks') {
        return [task]
      }
      if (path === '/me/submissions') {
        return [{ id: 42, taskId: 1, itemId: 11, status: 'revising', currentRevisionId: 901 }]
      }
      if (path === '/tasks/1/labeler/items') {
        return itemNav
      }
      if (path === '/tasks/1/items/11') {
        return {
          task,
          item,
          template: { id: 101, schemaJson: JSON.stringify(schema) },
          submission: { id: 42, taskId: 1, itemId: 11, status: 'revising', currentRevisionId: 901 },
          revision: { id: 901, answer: JSON.stringify({ summary: '旧答案' }), draft: false },
          latestHumanReview: {
            verdict: 'revise',
            reason: '上一轮原因：关键词缺失',
          },
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost.mockImplementation(async (path) => {
      if (path === '/tasks/1/items/11/submit') {
        return { id: 42, taskId: 1, itemId: 11, status: 'human_reviewing' }
      }
      throw new Error(`unexpected POST ${path}`)
    })

    render(<LabelerPlaza />)

    // Switch to 我的数据 tab, then open the revising submission.
    await user.click(await screen.findByRole('tab', { name: '我的数据' }))
    await user.click(await screen.findByRole('button', { name: '打开提交 #42' }))

    expect(await screen.findByText('上一轮原因：关键词缺失')).toBeInTheDocument()
    await user.clear(screen.getByLabelText('一句话总评'))
    await user.type(screen.getByLabelText('一句话总评'), '补充关键词后的答案')
    await user.click(screen.getByRole('button', { name: '提交审核' }))

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith('/tasks/1/items/11/submit', { answer: { summary: '补充关键词后的答案' } })
    })
  })

  it('aggregates my submission counts by bucket in 我的数据', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/labeler/tasks') {
        return []
      }
      if (path === '/me/submissions') {
        return [
          { id: 1, taskId: 1, itemId: 11, status: 'submitted' },
          { id: 2, taskId: 1, itemId: 12, status: 'human_reviewing' },
          { id: 3, taskId: 1, itemId: 13, status: 'approved' },
          { id: 4, taskId: 1, itemId: 14, status: 'rejected' },
          { id: 5, taskId: 1, itemId: 15, status: 'revising' },
        ]
      }
      throw new Error(`unexpected GET ${path}`)
    })

    render(<LabelerPlaza />)

    await user.click(await screen.findByRole('tab', { name: '我的数据' }))

    // 已提交 = submitted + human_reviewing = 2; 通过 = 1; 打回 = 1; 待修改 = 1.
    // "已提交" 同时出现在 stat 卡片标签和 StatusBadge 中,故把查询限定在 .lh-stats 容器内。
    const statsRegion = (await screen.findByRole('button', { name: '打开提交 #5' }))
      .closest('.lz-mydata')!
      .querySelector('.lh-stats') as HTMLElement
    const stats = within(statsRegion)
    const submitted = stats.getByText('已提交').closest('.lh-stat')
    expect(submitted).toHaveTextContent('2')
    const approved = stats.getByText('通过').closest('.lh-stat')
    expect(approved).toHaveTextContent('1')
  })

  it('renders item-nav progress from the labeler items endpoint', async () => {
    const user = userEvent.setup()
    const schema = {
      title: 'qa_progress',
      layout: 'single_page',
      fields: [{ name: 'summary', widget: 'Input', label: '一句话总评', required: true }],
    }
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/labeler/tasks') {
        return [task]
      }
      if (path === '/me/submissions') {
        return []
      }
      if (path === '/tasks/1/labeler/items') {
        return {
          taskId: 1,
          total: 4,
          items: [
            { itemId: 11, externalId: 'Q0001', status: 'claimed', mine: true, submissionId: null },
            { itemId: 12, externalId: 'Q0002', status: 'submitted', mine: true, submissionId: 51 },
            { itemId: 13, externalId: 'Q0003', status: 'approved', mine: true, submissionId: 52 },
            { itemId: 14, externalId: 'Q0004', status: 'available', mine: false, submissionId: null },
          ],
          counts: { claimed: 1, submitted: 1, approved: 1, available: 1 },
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost.mockImplementation(async (path) => {
      if (path === '/tasks/1/claim') {
        return {
          task,
          item,
          template: { id: 101, schemaJson: JSON.stringify(schema) },
          submission: { id: 42, taskId: 1, itemId: 11, status: 'draft' },
          revision: null,
        }
      }
      throw new Error(`unexpected POST ${path}`)
    })

    render(<LabelerPlaza />)

    await user.click(await screen.findByRole('button', { name: '领取题目 QA 质量标注' }))

    // done = submitted + approved = 2 of 4 → 50%.
    const progress = await screen.findByRole('progressbar', { name: '标注进度' })
    expect(progress).toHaveAttribute('aria-valuenow', '50')
    expect(screen.getByText('2 / 4 · 进度 50%')).toBeInTheDocument()
  })
})
