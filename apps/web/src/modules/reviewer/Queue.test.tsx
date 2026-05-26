import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type React from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ReviewerQueue from './Queue'
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
  Toast: {
    error: vi.fn(),
    success: vi.fn(),
  },
}))

const mockApiGet = vi.mocked(apiGet)
const mockApiPost = vi.mocked(apiPost)

const submission = {
  id: 501,
  taskId: 1,
  itemId: 11,
  status: 'human_reviewing',
  aiVerdict: 'pass',
  aiScore: 92.5,
  currentRevisionId: 901,
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
    mockApiPost.mockResolvedValue({ submission_id: 501, status: 'approved' })
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
            fromState: { String: 'ai_reviewing', Valid: true },
            toState: 'human_reviewing',
            actorType: 'ai_worker',
            actorId: null,
            event: 'ai_done',
            payload: { score: 92.5 },
            createdAt: '2026-05-26T13:00:00Z',
          }],
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })

    render(<ReviewerQueue />)

    await user.click(await screen.findByText('Submission #501'))

    const input = await screen.findByLabelText('历史字段')
    expect(input).toHaveValue('旧答案')
    expect(input).toBeDisabled()
    expect(screen.getByText('AI pass · 92.5')).toBeInTheDocument()
    expect(screen.getByText('verdict: pass')).toBeInTheDocument()
    expect(screen.getByText('score: 92.5')).toBeInTheDocument()
    expect(screen.getByText('关键词覆盖充分，建议通过。')).toBeInTheDocument()
    expect(screen.getByText('请审核商品标题')).toBeInTheDocument()
    expect(screen.getAllByText('ai_done').length).toBeGreaterThan(0)
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
    expect(screen.getByRole('button', { name: '打回修改' })).toBeDisabled()
    expect(screen.getByRole('button', { name: '拒绝' })).toBeDisabled()
    expect(screen.getByRole('button', { name: '通过' })).toBeDisabled()
  })

  it('opens real rule selector and links to owner prompt editing', async () => {
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
    expect(screen.getByRole('link', { name: '跳转 Owner 编辑' })).toHaveAttribute('href', '/owner?taskId=1&aiPromptId=40#ai-prompts')
    expect(mockApiGet).toHaveBeenCalledWith('/reviewer/tasks/1/ai-prompts')
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
      expect(screen.getByRole('link', { name: '跳转 Owner 编辑' })).toHaveAttribute('href', '/owner?taskId=2&aiPromptId=51#ai-prompts')
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
