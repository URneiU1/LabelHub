import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type React from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import LabelerPlaza from './Plaza'
import { apiGet, apiPost } from '../../shared/api/client'
import { buildDraftKey, loadLocalDraft } from './offlineDraftStore'

vi.mock('../../shared/api/client', () => {
  const apiGet = vi.fn()
  const apiPost = vi.fn()
  return {
    apiGet,
    apiPost,
    // listMyTasks 走同一个 mock 的 apiGet,测试只需 mock '/me/tasks' 的返回即可。
    listMyTasks: async () => {
      const data = await apiGet('/me/tasks')
      return data?.tasks ?? []
    },
  }
})

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

    await user.click(await screen.findByRole('button', { name: '查看任务详情 QA 质量标注' }))
    await user.click(await screen.findByRole('button', { name: '符合要求 领取任务 QA 质量标注' }))
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

    await user.click(await screen.findByRole('button', { name: '查看任务详情 QA 质量标注' }))
    await user.click(await screen.findByRole('button', { name: '符合要求 领取任务 QA 质量标注' }))
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

      fireEvent.click(await screen.findByRole('button', { name: '查看任务详情 QA 质量标注' }))
      fireEvent.click(await screen.findByRole('button', { name: '符合要求 领取任务 QA 质量标注' }))
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

    await user.click(await screen.findByRole('button', { name: '查看任务详情 QA 质量标注' }))
    await user.click(await screen.findByRole('button', { name: '符合要求 领取任务 QA 质量标注' }))

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

    render(<LabelerPlaza initialPlazaTab="mydata" />)

    // Start from 我的数据, then open the revising submission.
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

    render(<LabelerPlaza initialPlazaTab="mydata" />)

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

    await user.click(await screen.findByRole('button', { name: '查看任务详情 QA 质量标注' }))
    await user.click(await screen.findByRole('button', { name: '符合要求 领取任务 QA 质量标注' }))

    // done = submitted + approved = 2 of 4 → 50%.
    const progress = await screen.findByRole('progressbar', { name: '标注进度' })
    expect(progress).toHaveAttribute('aria-valuenow', '50')
    expect(screen.getByText('2 / 4 · 进度 50%')).toBeInTheDocument()
  })

  it('locks non-owned nav items (待标/他人) so they cannot re-claim; own items stay open-able', async () => {
    const user = userEvent.setup()
    const schema = {
      title: 'qa_nav_lock',
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
          total: 3,
          items: [
            { itemId: 11, externalId: 'Q0001', status: 'draft', mine: true, submissionId: 42 },
            { itemId: 14, externalId: 'Q0004', status: 'available', mine: false, submissionId: null },
            { itemId: 15, externalId: 'Q0005', status: 'taken', mine: false, submissionId: null },
          ],
          counts: { draft: 1, available: 1, taken: 1 },
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })
    let claimCalls = 0
    mockApiPost.mockImplementation(async (path) => {
      if (path === '/tasks/1/claim') {
        claimCalls += 1
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
    await user.click(await screen.findByRole('button', { name: '查看任务详情 QA 质量标注' }))
    await user.click(await screen.findByRole('button', { name: '符合要求 领取任务 QA 质量标注' }))
    await screen.findByLabelText('一句话总评')
    expect(claimCalls).toBe(1)

    // 待标 / 他人的题不可点(disabled);只有自己的草稿题可点。
    expect(screen.getByRole('button', { name: '第 2 题 Q0004 待标' })).toBeDisabled()
    expect(screen.getByRole('button', { name: '第 3 题 Q0005 他人' })).toBeDisabled()
    expect(screen.getByRole('button', { name: '第 1 题 Q0001 草稿' })).not.toBeDisabled()

    // 点击待标项是 no-op(按钮 disabled),不会再触发领取 → 不再狂刷「已领取题目」。
    fireEvent.click(screen.getByRole('button', { name: '第 2 题 Q0004 待标' }))
    expect(claimCalls).toBe(1)
  })
})

describe('LabelerPlaza offline draft preservation (P3)', () => {
  // claim 返回 submission id=42、revision=null → 本地草稿键为 1:11:42:new。
  const draftKey = buildDraftKey({ taskId: 1, itemId: 11, submissionId: 42, revisionNo: null })
  const schema = {
    title: 'qa_offline',
    layout: 'single_page',
    fields: [{ name: 'summary', widget: 'Input', label: '一句话总评', required: true }],
  }
  const claimBundle = {
    task,
    item,
    template: { id: 101, schemaJson: JSON.stringify(schema) },
    submission: { id: 42, taskId: 1, itemId: 11, status: 'draft' },
    revision: null,
  }

  beforeEach(() => {
    localStorage.clear()
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

  async function enterAnswerPage() {
    fireEvent.click(await screen.findByRole('button', { name: '查看任务详情 QA 质量标注' }))
    fireEvent.click(await screen.findByRole('button', { name: '符合要求 领取任务 QA 质量标注' }))
    return screen.findByLabelText('一句话总评')
  }

  it('marks the local draft synced after a successful autosave', async () => {
    mockApiPost.mockImplementation(async (path) => {
      if (path === '/tasks/1/claim') {
        return claimBundle
      }
      if (path === '/tasks/1/items/11/draft') {
        return { id: 42, taskId: 1, itemId: 11, status: 'draft' }
      }
      throw new Error(`unexpected POST ${path}`)
    })

    try {
      render(<LabelerPlaza />)
      const input = await enterAnswerPage()

      vi.useFakeTimers()
      fireEvent.change(input, { target: { value: '自动保存答案' } })
      await act(async () => {
        await vi.advanceTimersByTimeAsync(3000)
      })
      vi.useRealTimers()

      await waitFor(() => {
        expect(mockApiPost).toHaveBeenCalledWith('/tasks/1/items/11/draft', { answer: { summary: '自动保存答案' } })
      })
      await waitFor(() => {
        const draft = loadLocalDraft(draftKey)
        expect(draft).not.toBeNull()
        expect(draft!.answer).toEqual({ summary: '自动保存答案' })
        expect(draft!.synced).toBe(true)
      })
    } finally {
      vi.useRealTimers()
    }
  })

  it('preserves the local draft and surfaces a non-blocking state when autosave fails', async () => {
    mockApiPost.mockImplementation(async (path) => {
      if (path === '/tasks/1/claim') {
        return claimBundle
      }
      if (path === '/tasks/1/items/11/draft') {
        throw new Error('network down')
      }
      throw new Error(`unexpected POST ${path}`)
    })

    try {
      render(<LabelerPlaza />)
      const input = await enterAnswerPage()

      vi.useFakeTimers()
      fireEvent.change(input, { target: { value: '断网时的答案' } })
      await act(async () => {
        await vi.advanceTimersByTimeAsync(3000)
      })
      vi.useRealTimers()

      // 本地草稿保留且仍未同步。
      await waitFor(() => {
        const draft = loadLocalDraft(draftKey)
        expect(draft).not.toBeNull()
        expect(draft!.answer).toEqual({ summary: '断网时的答案' })
        expect(draft!.synced).toBe(false)
      })
      // 非阻塞提示出现(非弹窗)。
      expect(await screen.findByText('本地草稿已保存 / local draft saved')).toBeInTheDocument()
    } finally {
      vi.useRealTimers()
    }
  })

  it('offers to restore a newer local draft on reload, and restores it', async () => {
    const user = userEvent.setup()
    // 预置一份比服务端更新、未同步、模板版本匹配(null)的本地草稿。
    localStorage.setItem(draftKey, JSON.stringify({
      answer: { summary: '断网时未保存的较新答案' },
      updatedAt: Date.now(),
      templateVersion: null,
      synced: false,
    }))
    mockApiPost.mockImplementation(async (path) => {
      if (path === '/tasks/1/claim') {
        // 服务端没有这份答案(revision=null → 空答案),本地更新 → 应提示恢复。
        return claimBundle
      }
      throw new Error(`unexpected POST ${path}`)
    })

    render(<LabelerPlaza />)
    await enterAnswerPage()

    // 出现恢复横幅。
    expect(await screen.findByText('发现未同步的本地草稿。')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '恢复本地草稿' }))

    // 恢复后答案被写回输入框。
    await waitFor(() => {
      expect(screen.getByLabelText('一句话总评')).toHaveValue('断网时未保存的较新答案')
    })
  })

  it('blocks final submit while offline, keeps the local draft, and shows clear copy', async () => {
    const user = userEvent.setup()
    mockApiPost.mockImplementation(async (path) => {
      if (path === '/tasks/1/claim') {
        return claimBundle
      }
      if (path === '/tasks/1/items/11/submit') {
        throw new Error('submit should not be called while offline')
      }
      if (path === '/tasks/1/items/11/draft') {
        return { id: 42, taskId: 1, itemId: 11, status: 'draft' }
      }
      throw new Error(`unexpected POST ${path}`)
    })
    const { Toast } = await import('@douyinfe/semi-ui')

    const onlineSpy = vi.spyOn(navigator, 'onLine', 'get').mockReturnValue(false)
    try {
      render(<LabelerPlaza />)
      const input = await enterAnswerPage()
      await user.type(input, '离线作答')
      await user.click(screen.getByRole('button', { name: '提交审核' }))

      // 提交被阻断:never hits the submit endpoint, friendly copy shown.
      expect(mockApiPost).not.toHaveBeenCalledWith('/tasks/1/items/11/submit', expect.anything())
      expect(Toast.error).toHaveBeenCalledWith(expect.stringContaining('无法提交审核'))
      const draft = loadLocalDraft(draftKey)
      expect(draft).not.toBeNull()
      expect(draft!.answer).toEqual({ summary: '离线作答' })
      expect(draft!.synced).toBe(false)
    } finally {
      onlineSpy.mockRestore()
    }
  })
})

describe('LabelerPlaza workbench empty-state fallback', () => {
  beforeEach(() => {
    mockApiGet.mockReset()
    mockApiPost.mockReset()
    mockApiGet.mockResolvedValue([])
  })

  it('falls back to the task plaza when the workbench has no active task', async () => {
    render(<LabelerPlaza initialView="answer" />)
    // 应回落到任务广场,而不是停在"准备开始标注"空作答页
    expect(await screen.findByText('领取题目开始标注')).toBeInTheDocument()
    expect(screen.queryByText('准备开始标注')).not.toBeInTheDocument()
  })
})

describe('LabelerPlaza workbench resumes in-progress task', () => {
  const schema = {
    title: 'qa_resume',
    layout: 'single_page',
    fields: [{ name: 'summary', widget: 'Input', label: '一句话总评', required: true }],
  }

  beforeEach(() => {
    mockApiGet.mockReset()
    mockApiPost.mockReset()
    mockApiPost.mockImplementation(async (path) => {
      throw new Error(`unexpected POST ${path}`)
    })
  })

  it('auto-resumes the most-recent draft (claimed-unsubmitted) submission on workbench mount', async () => {
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/labeler/tasks') {
        return [task]
      }
      if (path === '/me/submissions') {
        // 已领未提交的草稿应被恢复,而不是回落到任务广场(否则用户会反复重领同一题)。
        return [{ id: 42, taskId: 1, itemId: 11, status: 'draft' }]
      }
      if (path === '/tasks/1/labeler/items') {
        return itemNav
      }
      if (path === '/tasks/1/items/11') {
        return {
          task,
          item,
          template: { id: 101, schemaJson: JSON.stringify(schema) },
          submission: { id: 42, taskId: 1, itemId: 11, status: 'draft' },
          revision: null,
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })

    render(<LabelerPlaza initialView="answer" />)

    // 自动进入作答页(渲染出表单),而不是停在任务广场。
    expect(await screen.findByLabelText('一句话总评')).toBeInTheDocument()
    await waitFor(() => {
      expect(mockApiGet).toHaveBeenCalledWith('/tasks/1/items/11')
    })
    expect(screen.queryByText('领取题目开始标注')).not.toBeInTheDocument()
    // 不应触发任何领取请求(纯恢复,不消耗新题)。
    expect(mockApiPost).not.toHaveBeenCalledWith('/tasks/1/claim', expect.anything())
  })

  it('still falls back to the task plaza when no submission is resumable', async () => {
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/labeler/tasks') {
        return [task]
      }
      if (path === '/me/submissions') {
        // 仅有终态/审核中提交 → 无可恢复项 → 回落任务广场。
        return [{ id: 7, taskId: 1, itemId: 9, status: 'approved' }]
      }
      throw new Error(`unexpected GET ${path}`)
    })

    render(<LabelerPlaza initialView="answer" />)

    expect(await screen.findByText('领取题目开始标注')).toBeInTheDocument()
    expect(screen.queryByText('准备开始标注')).not.toBeInTheDocument()
  })
})

describe('LabelerPlaza claimed-tasks (已领取的任务) + 大任务切换', () => {
  const schema = {
    title: 'qa_claimed',
    layout: 'single_page',
    fields: [{ name: 'summary', widget: 'Input', label: '一句话总评', required: true }],
  }
  const task2 = { ...task, id: 2, title: '第二个任务' }

  beforeEach(() => {
    mockApiGet.mockReset()
    mockApiPost.mockReset()
  })

  it('任务广场 shows 已领取的任务 and 继续标注 resumes the in-progress item without claiming', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/labeler/tasks') {
        return [task]
      }
      if (path === '/me/submissions') {
        return []
      }
      if (path === '/me/tasks') {
        return { tasks: [{ task, myCounts: { draft: 1 }, myTotal: 1, myInProgress: 1, resumeItemId: 11 }] }
      }
      if (path === '/tasks/1/labeler/items') {
        return itemNav
      }
      if (path === '/tasks/1/items/11') {
        return {
          task,
          item,
          template: { id: 101, schemaJson: JSON.stringify(schema) },
          submission: { id: 42, taskId: 1, itemId: 11, status: 'draft' },
          revision: null,
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })
    let claimCalls = 0
    mockApiPost.mockImplementation(async (path) => {
      if (path === '/tasks/1/claim') {
        claimCalls += 1
        return {}
      }
      throw new Error(`unexpected POST ${path}`)
    })

    render(<LabelerPlaza />)

    // 「已领取的任务」区块出现,带「继续标注」入口。
    expect(await screen.findByText('已领取的任务')).toBeInTheDocument()
    await user.click(await screen.findByRole('button', { name: '继续标注 QA 质量标注' }))

    // 直接恢复进行中的题(openByItem GET /tasks/1/items/11),进入作答页;不触发领取。
    expect(await screen.findByLabelText('一句话总评')).toBeInTheDocument()
    await waitFor(() => expect(mockApiGet).toHaveBeenCalledWith('/tasks/1/items/11'))
    expect(claimCalls).toBe(0)
  })

  it('工作台 shows a 大任务 switcher and switches between my claimed tasks', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/labeler/tasks') {
        return [task, task2]
      }
      if (path === '/me/submissions') {
        // 自动恢复最近的进行中草稿 → 任务1。
        return [{ id: 42, taskId: 1, itemId: 11, status: 'draft' }]
      }
      if (path === '/me/tasks') {
        return {
          tasks: [
            { task, myCounts: { draft: 1 }, myTotal: 1, myInProgress: 1, resumeItemId: 11 },
            { task: task2, myCounts: { draft: 1 }, myTotal: 1, myInProgress: 1, resumeItemId: 21 },
          ],
        }
      }
      if (path === '/tasks/1/labeler/items') {
        return itemNav
      }
      if (path === '/tasks/2/labeler/items') {
        return { taskId: 2, total: 1, items: [{ itemId: 21, externalId: 'P0001', status: 'draft', mine: true, submissionId: 50 }], counts: { draft: 1 } }
      }
      if (path === '/tasks/1/items/11') {
        return {
          task,
          item,
          template: { id: 101, schemaJson: JSON.stringify(schema) },
          submission: { id: 42, taskId: 1, itemId: 11, status: 'draft' },
          revision: null,
        }
      }
      if (path === '/tasks/2/items/21') {
        return {
          task: task2,
          item: { id: 21, taskId: 2, externalId: 'P0001', payload: JSON.stringify({ prompt: 't2' }), status: 'claimed' },
          template: { id: 102, schemaJson: JSON.stringify(schema) },
          submission: { id: 50, taskId: 2, itemId: 21, status: 'draft' },
          revision: null,
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost.mockImplementation(async (path) => {
      throw new Error(`unexpected POST ${path}`)
    })

    render(<LabelerPlaza initialView="answer" />)

    // 自动恢复任务1 → 作答页出现;切换器列出我的两个大任务。
    await screen.findByLabelText('一句话总评')
    const switcher = await screen.findByLabelText('切换标注任务')

    // 切到任务2 → 加载任务2 导航并恢复其进行中题(GET /tasks/2/items/21)。
    await user.selectOptions(switcher, '2')
    await waitFor(() => expect(mockApiGet).toHaveBeenCalledWith('/tasks/2/items/21'))
  })
})
