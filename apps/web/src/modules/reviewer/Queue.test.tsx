import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type React from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ReviewerQueue from './Queue'
import { apiGet, apiPost, listReviewResults } from '../../shared/api/client'

vi.mock('../../shared/api/client', async () => {
  const actual = await vi.importActual<typeof import('../../shared/api/client')>('../../shared/api/client')
  return {
    ...actual,
    apiGet: vi.fn(),
    apiPost: vi.fn(),
    listReviewResults: vi.fn(),
  }
})

vi.mock('@douyinfe/semi-ui', () => ({
  Button: ({ children, loading, theme, ...props }: React.ButtonHTMLAttributes<HTMLButtonElement> & { loading?: boolean, theme?: string }) => {
    void loading
    void theme
    return <button type="button" {...props}>{children}</button>
  },
  Toast: {
    error: vi.fn(),
    success: vi.fn(),
  },
}))

const mockApiGet = vi.mocked(apiGet)
const mockApiPost = vi.mocked(apiPost)
const mockListReviewResults = vi.mocked(listReviewResults)

const submission = {
  id: 501,
  taskId: 1,
  itemId: 11,
  status: 'human_reviewing',
  aiVerdict: 'pass',
  aiScore: 92.5,
  currentRevisionId: 901,
  reviewStage: 'second' as const,
  reviewLevel: 2,
  requiredLevels: 3,
}

const task = {
  id: 1,
  title: '历史模板任务',
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
  payload: JSON.stringify({ prompt: '历史题目' }),
  status: 'finished',
}

describe('ReviewerQueue schema runtime flow', () => {
  beforeEach(() => {
    mockApiGet.mockReset()
    mockApiPost.mockReset()
    mockListReviewResults.mockReset()
    mockListReviewResults.mockResolvedValue({ results: [], nextCursor: '', hasMore: false })
    mockApiPost.mockResolvedValue({ submission_id: 501, status: 'approved', stage: 'final' })
    // M-10:demo 仅在显式 ?demo=1 下启用,默认清掉 URL 上的 demo 开关。
    window.history.replaceState(null, '', '/reviewer')
  })

  it('batch approves selected submissions through the real batch endpoint', async () => {
    const user = userEvent.setup()
    const secondSubmission = { ...submission, id: 502, itemId: 12, currentRevisionId: 902 }
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/reviewer/submissions') {
        return [submission, secondSubmission]
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost.mockResolvedValue({
      results: [
        { submissionId: 501, status: 'approved' },
        { submissionId: 502, status: 'approved' },
      ],
      summary: { total: 2, succeeded: 2, failed: 0 },
    })

    render(<ReviewerQueue />)

    await user.click(await screen.findByLabelText('选择 Submission #501'))
    await user.click(screen.getByLabelText('选择 Submission #502'))
    await user.click(screen.getByRole('button', { name: '批量通过' }))

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith('/reviews/batch', {
        submission_ids: [501, 502],
        verdict: 'approve',
        reason: '',
      })
    })
  })

  it('loads the manual_review branch and exposes a dedicated 转人工复核 partition', async () => {
    const user = userEvent.setup()
    const manualSubmission = { ...submission, id: 777, itemId: 17, status: 'manual_review', aiVerdict: 'uncertain', currentRevisionId: 977 }
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/reviewer/submissions') {
        return [submission]
      }
      if (path === '/reviewer/submissions?status=manual_review') {
        return [manualSubmission]
      }
      throw new Error(`unexpected GET ${path}`)
    })

    render(<ReviewerQueue />)

    // 两条独立分支都被拉取。
    await waitFor(() => {
      expect(mockApiGet).toHaveBeenCalledWith('/reviewer/submissions')
      expect(mockApiGet).toHaveBeenCalledWith('/reviewer/submissions?status=manual_review')
    })

    // 「转人工复核」分区 tab 存在;两条 submission 默认都在「全部」分区可见。
    await screen.findByLabelText('选择 Submission #501')
    await screen.findByLabelText('选择 Submission #777')
    const manualTab = screen.getByRole('tab', { name: /转人工复核/ })

    // 切到「转人工复核」分区后,只剩 manual_review 那条。
    await user.click(manualTab)
    await waitFor(() => {
      expect(screen.queryByLabelText('选择 Submission #501')).toBeNull()
    })
    expect(screen.getByLabelText('选择 Submission #777')).toBeTruthy()
  })

  it('switches demo detail content when selecting different demo submissions (explicit ?demo=1)', async () => {
    const user = userEvent.setup()
    // M-10:demo 数据只在显式 ?demo=1 下出现,并带醒目「演示数据 · DEMO」标识。
    window.history.replaceState(null, '', '/reviewer?demo=1')
    mockApiGet.mockResolvedValue([])

    render(<ReviewerQueue />)

    expect(await screen.findByText('AI 自动预审队列')).toBeInTheDocument()
    expect(screen.getByText('演示数据 · DEMO')).toBeInTheDocument()
    expect(screen.getAllByText('重跑 86 分 → 建议通过').length).toBeGreaterThan(0)

    await user.click(screen.getByText('真无线主动降噪耳机 Pro Max 2026 款'))

    expect(screen.getAllByText('预审 88 分 → 建议通过').length).toBeGreaterThan(0)
    expect(screen.queryAllByText('重跑 86 分 → 建议通过')).toHaveLength(0)
    expect(screen.getByText(/AI 预审 · 本轮重跑结果/)).toBeInTheDocument()
  })

  it('shows an empty state instead of fake demo when the queue is empty and demo is off (M-10)', async () => {
    mockApiGet.mockResolvedValue([])

    render(<ReviewerQueue />)

    // 默认(无 ?demo=1):空队列展示空状态,绝不顶替成假 demo。
    expect(await screen.findByText('队列为空')).toBeInTheDocument()
    expect(screen.queryByText('AI 自动预审队列')).not.toBeInTheDocument()
    expect(screen.queryByText('演示数据 · DEMO')).not.toBeInTheDocument()
    expect(screen.queryByText('真无线主动降噪耳机 Pro Max 2026 款')).not.toBeInTheDocument()
  })

  it('switches reviewer views from the keyboard', async () => {
    const user = userEvent.setup()
    mockApiGet.mockResolvedValue([])

    render(<ReviewerQueue />)

    await screen.findByText('队列为空')
    const arbitrationTab = screen.getByRole('tab', { name: '仲裁' })
    arbitrationTab.focus()
    await user.keyboard('{Enter}')

    expect(arbitrationTab).toHaveAttribute('aria-selected', 'true')
    expect(await screen.findByText('仲裁队列')).toBeInTheDocument()

    const timelineTab = screen.getByRole('tab', { name: '审计时间线' })
    timelineTab.focus()
    await user.keyboard('{Enter}')

    expect(timelineTab).toHaveAttribute('aria-selected', 'true')
    expect(await screen.findByText('暂无审计记录')).toBeInTheDocument()
  })

  it('opens submission detail and renders historical template as read-only', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/reviewer/submissions') {
        return [submission]
      }
      if (path === '/reviewer/submissions/501') {
        return {
          task,
          item,
          template: {
            id: 101,
            schemaJson: JSON.stringify({
              title: 'historical_v1',
              layout: 'single_page',
              fields: [{ name: 'summary', widget: 'Input', label: '历史字段' }],
            }),
          },
          submission,
          revision: { id: 901, answer: JSON.stringify({ summary: '旧答案' }), draft: false },
          aiReview: {
            id: 31,
            submissionId: 501,
            revisionId: 901,
            idempotencyKey: 'abc',
            promptVersion: 2,
            verdict: 'pass',
            overallScore: 92.5,
            dimensions: [{ name: '相关性', score: 92 }],
            reason: '关键词覆盖充分，建议通过。',
            rawResponse: { ok: true },
            tokensInput: 100,
            tokensOutput: 20,
            latencyMs: 1420,
            status: 'succeeded',
            retryCount: 1,
            errorMsg: null,
            createdAt: '2026-05-26T13:00:00Z',
            prompt: {
              id: 41,
              version: 2,
              model: 'doubao-pro-32k',
              promptTemplate: '请审核商品标题',
              dimensions: [{ name: '相关性', weight: 1 }],
              passThreshold: 80,
              uncertainMin: 60,
            },
          },
          auditLogs: [{
            id: 71,
            entityType: 'submission',
            entityId: 501,
            fromState: 'ai_reviewing',
            toState: 'human_reviewing',
            actorType: 'ai_worker',
            actorId: null,
            event: 'ai_done',
            payload: { score: 92.5 },
            createdAt: '2026-05-26T13:00:00Z',
          }],
          reviewStage: 'second',
          reviewLevel: 2,
          requiredLevels: 3,
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })

    render(<ReviewerQueue />)

    await user.click(await screen.findByText('Submission #501'))

    const input = await screen.findByLabelText('历史字段')
    expect(input).toHaveValue('旧答案')
    expect(input).toBeDisabled()
    expect(screen.getByText('AI 判定：建议通过 · 92.5')).toBeInTheDocument()
    expect(screen.getByText('判定：建议通过')).toBeInTheDocument()
    expect(screen.getByText('综合分：92.5')).toBeInTheDocument()
    expect(screen.getByText('关键词覆盖充分，建议通过。')).toBeInTheDocument()
    expect(screen.getByText('请审核商品标题')).toBeInTheDocument()
    expect(screen.queryByText('审计时间线（SUB-501）')).not.toBeInTheDocument()

    await user.click(screen.getByRole('tab', { name: '审计时间线' }))
    expect(await screen.findByText('审计时间线（SUB-501）')).toBeInTheDocument()
    expect(screen.getAllByText(/AI 预审通过/).length).toBeGreaterThan(0)
    expect(screen.getAllByText('AI Agent').length).toBeGreaterThan(0)
    expect(screen.getAllByText(/AI 预审中 → 人工审核中/).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/^\d{2}:\d{2}:\d{2}$/).length).toBeGreaterThan(0)
    expect(screen.queryByText('ai_done')).not.toBeInTheDocument()
    expect(screen.queryByText('ai_reviewing → human_reviewing')).not.toBeInTheDocument()
  })

  it('shows the review stage (复审 / 审核进度 2/3) in the detail and labels the approve action with the stage', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/reviewer/submissions') {
        return [submission]
      }
      if (path === '/reviewer/submissions/501') {
        return {
          task,
          item,
          template: {
            id: 101,
            schemaJson: JSON.stringify({
              title: 'historical_v1',
              layout: 'single_page',
              fields: [{ name: 'summary', widget: 'Input', label: '历史字段' }],
            }),
          },
          submission,
          revision: { id: 901, answer: JSON.stringify({ summary: '旧答案' }), draft: false },
          aiReview: null,
          auditLogs: [],
          reviewStage: 'second',
          reviewLevel: 2,
          requiredLevels: 3,
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })

    render(<ReviewerQueue />)

    // 队列行先带阶段小徽标(进入详情前只有 1 个)。
    expect(await screen.findByLabelText('审核阶段 复审')).toBeInTheDocument()

    await user.click(await screen.findByText('Submission #501'))

    // 进入详情后,详情头部也显示当前阶段 + 审核进度(队列 + 详情共 2 个徽标)。
    const stageBadges = await screen.findAllByLabelText('审核阶段 复审')
    expect(stageBadges.length).toBeGreaterThanOrEqual(2)
    expect(stageBadges.some((node) => node.textContent === '复审 · 审核进度 2/3')).toBe(true)
    // 通过决策按钮标注当前阶段,体现「approve 推进一级」。
    expect(screen.getByText('✓ 通过(复审)')).toBeInTheDocument()
    expect(screen.getByText('推进一级 · 审核进度 2/3')).toBeInTheDocument()
  })

  it('lists finalized review results and opens one read-only', async () => {
    const user = userEvent.setup()
    window.history.replaceState(null, '', '/reviewer/results')
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/reviewer/submissions') {
        return [submission]
      }
      if (path === '/reviewer/submissions/777') {
        return {
          task,
          item: { ...item, id: 22 },
          template: {
            id: 101,
            schemaJson: JSON.stringify({
              title: 'historical_v1',
              layout: 'single_page',
              fields: [{ name: 'summary', widget: 'Input', label: '历史字段' }],
            }),
          },
          submission: { ...submission, id: 777, itemId: 22, status: 'approved' },
          revision: { id: 902, answer: JSON.stringify({ summary: '已定稿' }), draft: false },
          aiReview: null,
          auditLogs: [],
          reviewStage: 'final',
          reviewLevel: 3,
          requiredLevels: 3,
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockListReviewResults.mockResolvedValue({
      results: [{
        id: 777,
        taskId: 1,
        itemId: 22,
        status: 'approved',
        finalVerdict: 'approve',
        reviewerId: 9,
        aiScore: 88,
        updatedAt: '2026-05-28T10:00:00Z',
      }],
      nextCursor: '',
      hasMore: false,
    })

    render(<ReviewerQueue />)

    expect(screen.queryByRole('tab', { name: '审核结果' })).not.toBeInTheDocument()
    const resultRow = await screen.findByLabelText('查看 Submission #777 审核结果')
    expect(resultRow).toHaveTextContent('SUB-777')
    expect(resultRow).toHaveTextContent('决定 通过')
    expect(mockListReviewResults).toHaveBeenCalled()

    await user.click(resultRow)

    // 点击结果行后切回工作台并加载只读详情。
    const input = await screen.findByLabelText('历史字段')
    expect(input).toHaveValue('已定稿')
    expect(input).toBeDisabled()
    await waitFor(() => expect(mockApiGet).toHaveBeenCalledWith('/reviewer/submissions/777'))
  })

  it('shows an empty AI review state when no AI verdict exists', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/reviewer/submissions') {
        return [{ ...submission, aiVerdict: null, aiScore: null }]
      }
      if (path === '/reviewer/submissions/501') {
        return {
          task,
          item,
          template: {
            id: 101,
            schemaJson: JSON.stringify({
              title: 'historical_v1',
              layout: 'single_page',
              fields: [{ name: 'summary', widget: 'Input', label: '历史字段' }],
            }),
          },
          submission: { ...submission, aiVerdict: null, aiScore: null },
          revision: { id: 901, answer: JSON.stringify({ summary: '旧答案' }), draft: false },
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })

    render(<ReviewerQueue />)

    await user.click(await screen.findByText('Submission #501'))

    expect(screen.getByText('AI 未预审')).toBeInTheDocument()
    expect(await screen.findByText('暂无 AI 预审结果')).toBeInTheDocument()
  })

  it('shows schema error banner and disables review actions for bad schema', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/reviewer/submissions') {
        return [submission]
      }
      if (path === '/reviewer/submissions/501') {
        return {
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
          submission,
          revision: { id: 901, answer: '{}', draft: false },
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })

    render(<ReviewerQueue />)

    await user.click(await screen.findByText('Submission #501'))

    expect(await screen.findByRole('alert')).toHaveTextContent('fields[0].options')
    expect(screen.getByRole('button', { name: /返回标注员修改/ })).toBeDisabled()
    expect(screen.getByRole('button', { name: /终止本条提交/ })).toBeDisabled()
    expect(screen.getByRole('button', { name: /推进一级/ })).toBeDisabled()
  })

  it('disables review actions while a review request is pending', async () => {
    const user = userEvent.setup()
    const reviewResult = deferred<ReviewResponse>()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/reviewer/submissions') {
        return [submission]
      }
      if (path === '/reviewer/submissions/501') {
        return {
          task,
          item,
          template: {
            id: 101,
            schemaJson: JSON.stringify({
              title: 'historical_v1',
              layout: 'single_page',
              fields: [{ name: 'summary', widget: 'Input', label: '历史字段' }],
            }),
          },
          submission,
          revision: { id: 901, answer: JSON.stringify({ summary: '旧答案' }), draft: false },
          aiReview: null,
          auditLogs: [],
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost.mockReturnValue(reviewResult.promise)

    render(<ReviewerQueue />)

    await user.click(await screen.findByText('Submission #501'))
    await user.click(screen.getByRole('button', { name: /推进一级/ }))

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /返回标注员修改/ })).toBeDisabled()
      expect(screen.getByRole('button', { name: /终止本条提交/ })).toBeDisabled()
      expect(screen.getByRole('button', { name: /推进一级/ })).toBeDisabled()
    })

    await act(async () => {
      reviewResult.resolve({ submission_id: 501, status: 'approved' })
      await reviewResult.promise
    })
  })

  it('opens real rule selector and keeps prompt config read-only', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/reviewer/submissions') {
        return [submission]
      }
      if (path === '/reviewer/submissions/501') {
        return {
          task,
          item,
          template: {
            id: 101,
            schemaJson: JSON.stringify({
              title: 'historical_v1',
              layout: 'single_page',
              fields: [{ name: 'summary', widget: 'Input', label: '历史字段' }],
            }),
          },
          submission,
          revision: { id: 901, answer: JSON.stringify({ summary: '旧答案' }), draft: false },
          aiReview: {
            id: 31,
            submissionId: 501,
            revisionId: 901,
            idempotencyKey: 'abc',
            promptVersion: 2,
            verdict: 'pass',
            overallScore: 92.5,
            dimensions: [{ name: '相关性', score: 92 }],
            reason: '关键词覆盖充分，建议通过。',
            rawResponse: { ok: true },
            tokensInput: 100,
            tokensOutput: 20,
            latencyMs: 1420,
            status: 'succeeded',
            retryCount: 1,
            errorMsg: null,
            createdAt: '2026-05-26T13:00:00Z',
            prompt: {
              id: 41,
              version: 2,
              model: 'doubao-pro-32k',
              promptTemplate: '当前规则模板',
              dimensions: [{ name: '相关性', weight: 1 }],
              passThreshold: 80,
              uncertainMin: 60,
            },
          },
          auditLogs: [],
        }
      }
      if (path === '/reviewer/tasks/1/ai-prompts') {
        return {
          prompts: [
            {
              id: 41,
              version: 2,
              model: 'doubao-pro-32k',
              promptTemplate: '当前规则模板',
              dimensions: [{ name: '相关性', weight: 1 }],
              passThreshold: 80,
              uncertainMin: 60,
            },
            {
              id: 40,
              version: 1,
              model: 'mock-model',
              promptTemplate: '历史规则模板',
              dimensions: [{ name: '准确性', weight: 1 }],
              passThreshold: 75,
              uncertainMin: 55,
            },
          ],
          activePromptId: 41,
          aiReviewEnabled: true,
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })

    render(<ReviewerQueue />)

    await user.click(await screen.findByText('Submission #501'))
    await user.click(screen.getByRole('button', { name: '规则配置' }))
    await user.selectOptions(await screen.findByLabelText('reviewer_rule_select'), '40')

    expect(await screen.findByText('历史规则模板')).toBeInTheDocument()
    expect(await screen.findByText('active #41')).toBeInTheDocument()
    expect(screen.getByText('规则仅供查看。')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: '跳转 Owner 编辑' })).not.toBeInTheDocument()
    expect(mockApiGet).toHaveBeenCalledWith('/reviewer/tasks/1/ai-prompts')
    expect(mockApiPost).not.toHaveBeenCalledWith('/reviewer/tasks/1/ai-prompts/40/activate', {})
  })

  it('ignores stale rule selector responses after switching submissions', async () => {
    const user = userEvent.setup()
    let resolveTaskOneRules: (value: unknown) => void = () => {}
    const taskOneRules = new Promise((resolve) => {
      resolveTaskOneRules = resolve
    })
    const submissionTwo = { ...submission, id: 502, taskId: 2, itemId: 12, currentRevisionId: 902 }
    const taskTwo = { ...task, id: 2, title: '第二个任务' }
    const itemTwo = { ...item, id: 12, taskId: 2, externalId: 'Q0002' }

    mockApiGet.mockImplementation(async (path) => {
      if (path === '/reviewer/submissions') {
        return [submission, submissionTwo]
      }
      if (path === '/reviewer/submissions/501') {
        return {
          task,
          item,
          template: { id: 101, schemaJson: JSON.stringify({ title: 'task1', layout: 'single_page', fields: [{ name: 'summary', widget: 'Input', label: '字段' }] }) },
          submission,
          revision: { id: 901, answer: JSON.stringify({ summary: '任务一' }), draft: false },
          aiReview: {
            id: 31,
            submissionId: 501,
            revisionId: 901,
            idempotencyKey: 'abc',
            promptVersion: 2,
            verdict: 'pass',
            overallScore: 92.5,
            dimensions: [],
            reason: '',
            rawResponse: {},
            tokensInput: 0,
            tokensOutput: 0,
            latencyMs: 0,
            status: 'succeeded',
            retryCount: 0,
            errorMsg: null,
            createdAt: '2026-05-26T13:00:00Z',
            prompt: { id: 41, version: 2, model: 'model-a', promptTemplate: '任务一当前规则', dimensions: [], passThreshold: 80, uncertainMin: 60 },
          },
          auditLogs: [],
        }
      }
      if (path === '/reviewer/tasks/1/ai-prompts') {
        return taskOneRules
      }
      if (path === '/reviewer/submissions/502') {
        return {
          task: taskTwo,
          item: itemTwo,
          template: { id: 102, schemaJson: JSON.stringify({ title: 'task2', layout: 'single_page', fields: [{ name: 'summary', widget: 'Input', label: '字段' }] }) },
          submission: submissionTwo,
          revision: { id: 902, answer: JSON.stringify({ summary: '任务二' }), draft: false },
          aiReview: {
            id: 32,
            submissionId: 502,
            revisionId: 902,
            idempotencyKey: 'def',
            promptVersion: 1,
            verdict: 'pass',
            overallScore: 88,
            dimensions: [],
            reason: '',
            rawResponse: {},
            tokensInput: 0,
            tokensOutput: 0,
            latencyMs: 0,
            status: 'succeeded',
            retryCount: 0,
            errorMsg: null,
            createdAt: '2026-05-26T13:00:00Z',
            prompt: { id: 51, version: 1, model: 'model-b', promptTemplate: '任务二当前规则', dimensions: [], passThreshold: 80, uncertainMin: 60 },
          },
          auditLogs: [],
        }
      }
      if (path === '/reviewer/tasks/2/ai-prompts') {
        return {
          prompts: [{ id: 51, version: 1, model: 'model-b', promptTemplate: '任务二规则', dimensions: [], passThreshold: 80, uncertainMin: 60 }],
          activePromptId: 51,
          aiReviewEnabled: true,
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })

    render(<ReviewerQueue />)

    await user.click(await screen.findByText('Submission #501'))
    await user.click(screen.getByRole('button', { name: '规则配置' }))
    await waitFor(() => expect(mockApiGet).toHaveBeenCalledWith('/reviewer/tasks/1/ai-prompts'))

    await user.click(await screen.findByText('Submission #502'))
    await user.click(screen.getByRole('button', { name: '规则配置' }))
    expect(await screen.findByText('任务二规则')).toBeInTheDocument()

    resolveTaskOneRules({
      prompts: [{ id: 40, version: 1, model: 'model-a', promptTemplate: '任务一晚到规则', dimensions: [], passThreshold: 75, uncertainMin: 55 }],
      activePromptId: 40,
      aiReviewEnabled: true,
    })

    await waitFor(() => {
      expect(screen.getByText('规则仅供查看。')).toBeInTheDocument()
      expect(screen.queryByRole('link', { name: '跳转 Owner 编辑' })).not.toBeInTheDocument()
    })
    expect(screen.queryByText('任务一晚到规则')).not.toBeInTheDocument()
  })

  it('posts failed AI review retry for the selected submission', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/reviewer/submissions') {
        return [submission]
      }
      if (path === '/reviewer/submissions/501') {
        return {
          task,
          item,
          template: {
            id: 101,
            schemaJson: JSON.stringify({
              title: 'historical_v1',
              layout: 'single_page',
              fields: [{ name: 'summary', widget: 'Input', label: '历史字段' }],
            }),
          },
          submission,
          revision: { id: 901, answer: JSON.stringify({ summary: '旧答案' }), draft: false },
          aiReview: {
            id: 31,
            submissionId: 501,
            revisionId: 901,
            idempotencyKey: 'abc',
            promptVersion: 2,
            verdict: null,
            overallScore: null,
            dimensions: [],
            reason: null,
            rawResponse: null,
            tokensInput: 0,
            tokensOutput: 0,
            latencyMs: 0,
            status: 'dead',
            retryCount: 5,
            errorMsg: 'provider failed',
            createdAt: '2026-05-26T13:00:00Z',
            prompt: null,
          },
          auditLogs: [],
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost.mockResolvedValueOnce({
      submissionId: 501,
      status: 'ai_reviewing',
      aiReview: {
        id: 31,
        submissionId: 501,
        revisionId: 901,
        idempotencyKey: 'abc',
        promptVersion: 2,
        verdict: null,
        overallScore: null,
        dimensions: [],
        reason: null,
        rawResponse: null,
        tokensInput: 0,
        tokensOutput: 0,
        latencyMs: 0,
        status: 'pending',
        retryCount: 0,
        errorMsg: null,
        createdAt: '2026-05-26T13:00:00Z',
        prompt: null,
      },
    })

    render(<ReviewerQueue />)

    await user.click(await screen.findByText('Submission #501'))
    await user.click(screen.getByRole('button', { name: '失败重跑' }))

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith('/reviewer/submissions/501/ai-review/retry', {})
    })
  })
})

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}
