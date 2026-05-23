import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type React from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import OwnerDashboard from './Dashboard'
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

const task = {
  id: 1,
  title: 'QA 质量标注',
  description: null,
  baselineDescription: 'baseline',
  status: 'published',
  totalItems: 1,
  finishedItems: 0,
  aiPromptId: null,
  aiReviewEnabled: false,
}

describe('OwnerDashboard AI prompt flow', () => {
  beforeEach(() => {
    mockApiGet.mockReset()
    mockApiPost.mockReset()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/tasks') {
        return [task]
      }
      if (path === '/tasks/1/ai-prompts') {
        return { prompts: [], activePromptId: null, aiReviewEnabled: false }
      }
      throw new Error(`unexpected GET ${path}`)
    })
  })

  it('saves an AI prompt config for the selected owner task', async () => {
    const user = userEvent.setup()
    mockApiPost.mockResolvedValue({
      prompt: {
        id: 33,
        version: 1,
        promptTemplate: '请预审',
        dimensions: '[{"name":"相关性","description":"是否相关","weight":0.8}]',
        passThreshold: 80,
        uncertainMin: 60,
        model: 'doubao-seed-2.0-lite',
      },
      activePromptId: 33,
    })

    render(<OwnerDashboard />)

    await user.clear(await screen.findByLabelText('prompt_template'))
    await user.type(screen.getByLabelText('prompt_template'), '请预审')
    await user.click(screen.getByRole('button', { name: '保存 AI Prompt' }))

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith('/tasks/1/ai-prompts', expect.objectContaining({
        prompt_template: '请预审',
        model: '',
      }))
    })
  })

  it('preserves dimension description and weight when saving an existing prompt', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/tasks') {
        return [{ ...task, aiPromptId: 33 }]
      }
      if (path === '/tasks/1/ai-prompts') {
        return {
          prompts: [{
            id: 33,
            version: 1,
            promptTemplate: '请预审',
            dimensions: '[{"name":"相关性","description":"是否相关","weight":0.8}]',
            passThreshold: 80,
            uncertainMin: 60,
            model: 'doubao-seed-2.0-lite',
          }],
          activePromptId: 33,
          aiReviewEnabled: false,
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost.mockResolvedValue({
      prompt: {
        id: 34,
        version: 2,
        promptTemplate: '请预审',
        dimensions: '[{"name":"相关性","description":"是否相关","weight":0.8}]',
        passThreshold: 80,
        uncertainMin: 60,
        model: 'doubao-seed-2.0-lite',
      },
      activePromptId: 34,
    })

    render(<OwnerDashboard />)

    await screen.findByDisplayValue(/是否相关/)
    await user.click(screen.getByRole('button', { name: '保存 AI Prompt' }))

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith('/tasks/1/ai-prompts', expect.objectContaining({
        dimensions: [{ name: '相关性', description: '是否相关', weight: 0.8 }],
      }))
    })
  })

  it('resets prompt form when switching to a task without prompts', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/tasks') {
        return [
          { ...task, id: 1, title: 'Task A', aiPromptId: 33 },
          { ...task, id: 2, title: 'Task B', aiPromptId: null },
        ]
      }
      if (path === '/tasks/1/ai-prompts') {
        return {
          prompts: [{
            id: 33,
            version: 1,
            promptTemplate: 'Task A prompt',
            dimensions: '[{"name":"Task A dimension","description":"copy risk","weight":0.4}]',
            passThreshold: 91,
            uncertainMin: 71,
            model: 'task-a-model',
          }],
          activePromptId: 33,
          aiReviewEnabled: false,
        }
      }
      if (path === '/tasks/2/ai-prompts') {
        return { prompts: [], activePromptId: null, aiReviewEnabled: false }
      }
      throw new Error(`unexpected GET ${path}`)
    })

    render(<OwnerDashboard />)

    expect(await screen.findByDisplayValue('Task A prompt')).toBeInTheDocument()
    expect(screen.getByDisplayValue(/Task A dimension/)).toBeInTheDocument()
    expect(screen.getByLabelText('pass_threshold')).toHaveValue(91)
    expect(screen.getByLabelText('uncertain_min')).toHaveValue(71)
    expect(screen.getByLabelText('model')).toHaveValue('task-a-model')

    await user.click(screen.getByRole('button', { name: /Task B/ }))

    expect(await screen.findByDisplayValue('请根据 payload 和 answer 完成结构化预审。')).toBeInTheDocument()
    expect(screen.getByDisplayValue(/格式合规/)).toBeInTheDocument()
    expect(screen.getByLabelText('pass_threshold')).toHaveValue(80)
    expect(screen.getByLabelText('uncertain_min')).toHaveValue(60)
    expect(screen.getByLabelText('model')).toHaveValue('')
  })

  it('ignores out-of-order prompt loads when switching tasks', async () => {
    const user = userEvent.setup()
    const taskAPrompts = deferred<AIPromptsResponse>()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/tasks') {
        return [
          { ...task, id: 1, title: 'Task A', aiPromptId: 33 },
          { ...task, id: 2, title: 'Task B', aiPromptId: null },
        ]
      }
      if (path === '/tasks/1/ai-prompts') {
        return taskAPrompts.promise
      }
      if (path === '/tasks/2/ai-prompts') {
        return { prompts: [], activePromptId: null, aiReviewEnabled: false }
      }
      throw new Error(`unexpected GET ${path}`)
    })

    render(<OwnerDashboard />)

    await user.click(await screen.findByRole('button', { name: /Task B/ }))
    expect(await screen.findByDisplayValue('请根据 payload 和 answer 完成结构化预审。')).toBeInTheDocument()

    await act(async () => {
      taskAPrompts.resolve({
        prompts: [{
          id: 33,
          version: 1,
          promptTemplate: 'Task A prompt',
          dimensions: '[{"name":"Task A dimension"}]',
          passThreshold: 91,
          uncertainMin: 71,
          model: 'task-a-model',
        }],
        activePromptId: 33,
        aiReviewEnabled: true,
      })
      await taskAPrompts.promise
    })

    expect(screen.queryByDisplayValue('Task A prompt')).not.toBeInTheDocument()
    expect(screen.getByLabelText('pass_threshold')).toHaveValue(80)
    expect(screen.getByRole('button', { name: '启用 AI review' })).toBeDisabled()
  })

  it('runs dry-run and displays structured result with provider mark', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/tasks') {
        return [{ ...task, aiPromptId: 33 }]
      }
      if (path === '/tasks/1/ai-prompts') {
        return {
          prompts: [{
            id: 33,
            version: 1,
            promptTemplate: '请预审',
            dimensions: '[{"name":"相关性"}]',
            passThreshold: 80,
            uncertainMin: 60,
            model: 'mock-model',
          }],
          activePromptId: 33,
          aiReviewEnabled: false,
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost.mockResolvedValue({
      provider: 'mock',
      result: {
        verdict: 'uncertain',
        overall_score: 75,
        dimensions: [{ name: '相关性', score: 75, reason: 'ok' }],
        reason: 'needs human review',
      },
    })

    render(<OwnerDashboard />)

    await user.click(await screen.findByRole('button', { name: '运行 dry-run' }))

    expect(await screen.findByText('mock')).toBeInTheDocument()
    await screen.findByText('needs human review')
    expect(screen.getAllByText(/uncertain/).some((element) => element.textContent?.includes('75'))).toBe(true)
    expect(screen.getByText('needs human review')).toBeInTheDocument()
  })

  it('shows dry-run errors inline', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/tasks') {
        return [{ ...task, aiPromptId: 33 }]
      }
      if (path === '/tasks/1/ai-prompts') {
        return {
          prompts: [{
            id: 33,
            version: 1,
            promptTemplate: '请预审',
            dimensions: '[{"name":"相关性"}]',
            passThreshold: 80,
            uncertainMin: 60,
            model: 'mock-model',
          }],
          activePromptId: 33,
          aiReviewEnabled: false,
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost.mockRejectedValue(new Error('provider failed'))

    render(<OwnerDashboard />)

    await user.click(await screen.findByRole('button', { name: '运行 dry-run' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('provider failed')
  })

  it('clears stale dry-run result when a new dry-run fails', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/tasks') {
        return [{ ...task, aiPromptId: 33 }]
      }
      if (path === '/tasks/1/ai-prompts') {
        return {
          prompts: [{
            id: 33,
            version: 1,
            promptTemplate: '请预审',
            dimensions: '[{"name":"相关性"}]',
            passThreshold: 80,
            uncertainMin: 60,
            model: 'mock-model',
          }],
          activePromptId: 33,
          aiReviewEnabled: false,
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost
      .mockResolvedValueOnce({
        provider: 'mock',
        result: {
          verdict: 'uncertain',
          overall_score: 75,
          dimensions: [{ name: '相关性', score: 75, reason: 'ok' }],
          reason: 'first result',
        },
      })
      .mockRejectedValueOnce(new Error('provider failed'))

    render(<OwnerDashboard />)

    await user.click(await screen.findByRole('button', { name: '运行 dry-run' }))
    expect(await screen.findByText('first result')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '运行 dry-run' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('provider failed')
    expect(screen.queryByText('first result')).not.toBeInTheDocument()
  })

  it('disables AI review enable action when no active prompt exists', async () => {
    render(<OwnerDashboard />)

    expect(await screen.findByRole('button', { name: '启用 AI review' })).toBeDisabled()
    expect(screen.getByText('保存 AI Prompt 后才能启用 AI review')).toBeInTheDocument()
  })

  it('enables AI review for a task with active prompt', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/tasks') {
        return [{ ...task, aiPromptId: 33, aiReviewEnabled: false }]
      }
      if (path === '/tasks/1/ai-prompts') {
        return {
          prompts: [{
            id: 33,
            version: 1,
            promptTemplate: '请预审',
            dimensions: '[{"name":"相关性"}]',
            passThreshold: 80,
            uncertainMin: 60,
            model: 'mock-model',
          }],
          activePromptId: 33,
          aiReviewEnabled: false,
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost.mockResolvedValue({ aiReviewEnabled: true, activePromptId: 33 })

    render(<OwnerDashboard />)

    await user.click(await screen.findByRole('button', { name: '启用 AI review' }))

    expect(mockApiPost).toHaveBeenCalledWith('/tasks/1/ai-review-settings', { enabled: true })
    expect(await screen.findByText('AI review: 已启用')).toBeInTheDocument()
  })

  it('disables AI review for a task while keeping the active prompt', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/tasks') {
        return [{ ...task, aiPromptId: 33, aiReviewEnabled: true }]
      }
      if (path === '/tasks/1/ai-prompts') {
        return {
          prompts: [{
            id: 33,
            version: 1,
            promptTemplate: '请预审',
            dimensions: '[{"name":"相关性"}]',
            passThreshold: 80,
            uncertainMin: 60,
            model: 'mock-model',
          }],
          activePromptId: 33,
          aiReviewEnabled: true,
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost.mockResolvedValue({ aiReviewEnabled: false, activePromptId: 33 })

    render(<OwnerDashboard />)

    await user.click(await screen.findByRole('button', { name: '关闭 AI review' }))

    expect(mockApiPost).toHaveBeenCalledWith('/tasks/1/ai-review-settings', { enabled: false })
    expect(await screen.findByText('AI review: 已关闭')).toBeInTheDocument()
  })
})

type AIPromptConfig = {
  id: number
  version: number
  promptTemplate: string
  dimensions: string
  passThreshold: number
  uncertainMin: number
  model: string
}

type AIPromptsResponse = {
  prompts: AIPromptConfig[]
  activePromptId: number | null
  aiReviewEnabled: boolean
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}
