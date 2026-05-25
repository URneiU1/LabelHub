import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type React from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import OwnerDashboard from './Dashboard'
import { apiDelete, apiGet, apiPost, apiPostRawJSON } from '../../shared/api/client'

vi.mock('../../shared/api/client', () => ({
  apiDelete: vi.fn(),
  apiGet: vi.fn(),
  apiPost: vi.fn(),
  apiPostRawJSON: vi.fn(),
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
const mockApiPostRawJSON = vi.mocked(apiPostRawJSON)
const mockApiDelete = vi.mocked(apiDelete)

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
    vi.restoreAllMocks()
    mockApiGet.mockReset()
    mockApiPost.mockReset()
    mockApiPostRawJSON.mockReset()
    mockApiDelete.mockReset()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/tasks') {
        return [task]
      }
      if (path === '/tasks/1/ai-prompts') {
        return { prompts: [], activePromptId: null, aiReviewEnabled: false }
      }
      if (path === '/tasks/1/golden-samples') {
        return { samples: [] }
      }
      if (path.startsWith('/tasks/1/ai-dry-runs')) {
        return { dryRuns: [] }
      }
      if (path.endsWith('/golden-samples')) {
        return { samples: [] }
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
      if (path.endsWith('/golden-samples')) {
        return { samples: [] }
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
      if (path.endsWith('/golden-samples')) {
        return { samples: [] }
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
      if (path.endsWith('/golden-samples')) {
        return { samples: [] }
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

  it('ignores stale dry-run responses after switching tasks', async () => {
    const user = userEvent.setup()
    const dryRunResult = deferred<unknown>()
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
            dimensions: '[{"name":"相关性"}]',
            passThreshold: 80,
            uncertainMin: 60,
            model: 'mock-model',
          }],
          activePromptId: 33,
          aiReviewEnabled: false,
        }
      }
      if (path === '/tasks/2/ai-prompts') {
        return { prompts: [], activePromptId: null, aiReviewEnabled: false }
      }
      if (path.endsWith('/golden-samples')) {
        return { samples: [] }
      }

      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPostRawJSON.mockReturnValue(dryRunResult.promise)

    render(<OwnerDashboard />)

    await user.click(await screen.findByRole('button', { name: '运行 dry-run' }))
    await user.click(screen.getByRole('button', { name: /Task B/ }))
    expect(await screen.findByDisplayValue('请根据 payload 和 answer 完成结构化预审。')).toBeInTheDocument()

    await act(async () => {
      dryRunResult.resolve({
        provider: 'mock',
        result: {
          verdict: 'pass',
          overall_score: 90,
          dimensions: [{ name: '相关性', score: 90, reason: 'late' }],
          reason: 'late result',
        },
      })
      await dryRunResult.promise
    })

    expect(screen.queryByText('late result')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '启用 AI review' })).toBeDisabled()
  })

  it('keeps same-task dry-run response current after clicking selected task again', async () => {
    const user = userEvent.setup()
    const dryRunResult = deferred<unknown>()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/tasks') {
        return [{ ...task, id: 1, title: 'Task A', aiPromptId: 33 }]
      }
      if (path === '/tasks/1/ai-prompts') {
        return {
          prompts: [{
            id: 33,
            version: 1,
            promptTemplate: 'Task A prompt',
            dimensions: '[{"name":"相关性"}]',
            passThreshold: 80,
            uncertainMin: 60,
            model: 'mock-model',
          }],
          activePromptId: 33,
          aiReviewEnabled: false,
        }
      }
      if (path.endsWith('/golden-samples')) {
        return { samples: [] }
      }

      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPostRawJSON.mockReturnValue(dryRunResult.promise)

    render(<OwnerDashboard />)

    await screen.findByDisplayValue('Task A prompt')
    await user.click(screen.getByRole('button', { name: '运行 dry-run' }))
    await user.click(screen.getByRole('button', { name: /Task A/ }))

    await act(async () => {
      dryRunResult.resolve({
        provider: 'mock',
        result: {
          verdict: 'pass',
          overall_score: 90,
          dimensions: [{ name: '相关性', score: 90, reason: 'same task' }],
          reason: 'same task result',
        },
      })
      await dryRunResult.promise
    })

    expect(await screen.findByText('same task result')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '运行 dry-run' })).not.toBeDisabled()
  })

  it('ignores stale save prompt responses after switching tasks', async () => {
    const user = userEvent.setup()
    const saveResult = deferred<unknown>()
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
            dimensions: '[{"name":"相关性"}]',
            passThreshold: 80,
            uncertainMin: 60,
            model: 'mock-model',
          }],
          activePromptId: 33,
          aiReviewEnabled: false,
        }
      }
      if (path === '/tasks/2/ai-prompts') {
        return { prompts: [], activePromptId: null, aiReviewEnabled: false }
      }
      if (path.endsWith('/golden-samples')) {
        return { samples: [] }
      }

      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost.mockReturnValue(saveResult.promise)

    render(<OwnerDashboard />)

    await screen.findByDisplayValue('Task A prompt')
    await user.click(screen.getByRole('button', { name: '保存 AI Prompt' }))
    await user.click(screen.getByRole('button', { name: /Task B/ }))
    expect(await screen.findByDisplayValue('请根据 payload 和 answer 完成结构化预审。')).toBeInTheDocument()

    await act(async () => {
      saveResult.resolve({
        prompt: {
          id: 34,
          version: 2,
          promptTemplate: 'Late saved prompt',
          dimensions: '[{"name":"迟到"}]',
          passThreshold: 90,
          uncertainMin: 70,
          model: 'late-model',
        },
        activePromptId: 34,
      })
      await saveResult.promise
    })

    expect(screen.queryByDisplayValue('Late saved prompt')).not.toBeInTheDocument()
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
      if (path.endsWith('/golden-samples')) {
        return { samples: [] }
      }

      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPostRawJSON.mockResolvedValue({
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

  it('sends ad-hoc dry-run as raw JSON to preserve large integer spelling', async () => {
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
      if (path.endsWith('/golden-samples')) {
        return { samples: [] }
      }

      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPostRawJSON.mockResolvedValue({
      provider: 'mock',
      result: {
        verdict: 'uncertain',
        overall_score: 75,
        dimensions: [{ name: '相关性', score: 75, reason: 'ok' }],
        reason: 'needs human review',
      },
    })

    render(<OwnerDashboard />)

    fireEvent.change(await screen.findByLabelText('sample_payload'), { target: { value: '{"external_id":9007199254740993123}' } })
    fireEvent.change(screen.getByLabelText('sample_answer'), { target: { value: '{"selected_id":9007199254740993124}' } })
    await user.click(screen.getByRole('button', { name: '运行 dry-run' }))

    expect(mockApiPost).not.toHaveBeenCalled()
    expect(mockApiPostRawJSON).toHaveBeenCalledWith(
      '/tasks/1/ai-prompts/33/dry-run',
      expect.stringContaining('9007199254740993123'),
    )
    const body = mockApiPostRawJSON.mock.calls[0][1]
    expect(body).toContain('9007199254740993124')
    expect(body).not.toContain('9007199254740993000')
    expect(body).not.toContain('9.007199254740993')
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
      if (path.endsWith('/golden-samples')) {
        return { samples: [] }
      }

      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPostRawJSON.mockRejectedValue(new Error('provider failed'))

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
      if (path.endsWith('/golden-samples')) {
        return { samples: [] }
      }

      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPostRawJSON
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

  it('rejects null ad-hoc dry-run payload before request and clears stale result', async () => {
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
      if (path.endsWith('/golden-samples')) {
        return { samples: [] }
      }

      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPostRawJSON.mockResolvedValue({
      provider: 'mock',
      result: {
        verdict: 'uncertain',
        overall_score: 75,
        dimensions: [{ name: '相关性', score: 75, reason: 'ok' }],
        reason: 'first result',
      },
    })

    render(<OwnerDashboard />)

    await user.click(await screen.findByRole('button', { name: '运行 dry-run' }))
    expect(await screen.findByText('first result')).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText('sample_payload'), { target: { value: 'null' } })
    await user.click(screen.getByRole('button', { name: '运行 dry-run' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('payload is required')
    expect(screen.queryByText('first result')).not.toBeInTheDocument()
    expect(mockApiPostRawJSON).toHaveBeenCalledTimes(1)
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
      if (path.endsWith('/golden-samples')) {
        return { samples: [] }
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
      if (path.endsWith('/golden-samples')) {
        return { samples: [] }
      }

      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost.mockResolvedValue({ aiReviewEnabled: false, activePromptId: 33 })

    render(<OwnerDashboard />)

    await user.click(await screen.findByRole('button', { name: '关闭 AI review' }))

    expect(mockApiPost).toHaveBeenCalledWith('/tasks/1/ai-review-settings', { enabled: false })
    expect(await screen.findByText('AI review: 已关闭')).toBeInTheDocument()
  })

  it('loads golden samples for the selected task', async () => {
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/tasks') {
        return [task]
      }
      if (path === '/tasks/1/ai-prompts') {
        return { prompts: [], activePromptId: null, aiReviewEnabled: false }
      }
      if (path === '/tasks/1/golden-samples') {
        return {
          samples: [{
            id: 11,
            taskId: 1,
            aiPromptId: null,
            payload: { text: 'a' },
            payloadHash: 'hash',
            expectedAnswer: { label: 'ok' },
            expectedVerdict: 'pass',
            notes: 'baseline sample',
            createdBy: 7,
            createdAt: '2026-05-23T12:00:00Z',
          }],
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })

    render(<OwnerDashboard />)

    expect(await screen.findByText('#11 · pass')).toBeInTheDocument()
    expect(screen.getByText('baseline sample')).toBeInTheDocument()
    expect(screen.getByText(/"text": "a"/)).toBeInTheDocument()
    expect(screen.getByText(/"label": "ok"/)).toBeInTheDocument()
  })

  it('creates a golden sample with raw JSON object body', async () => {
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
      if (path.endsWith('/golden-samples')) {
        return { samples: [] }
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPostRawJSON.mockResolvedValue({
      sample: {
        id: 11,
        taskId: 1,
        aiPromptId: 33,
        payload: { prompt: '示例题目' },
        payloadHash: 'hash',
        expectedAnswer: { summary: '示例答案' },
        expectedVerdict: 'pass',
        notes: 'owner baseline',
        createdBy: 7,
        createdAt: '2026-05-23T12:00:00Z',
      },
    })

    render(<OwnerDashboard />)

    await screen.findByDisplayValue('请预审')
    await user.type(screen.getByLabelText('golden_sample_notes'), 'owner baseline')
    await user.click(screen.getByRole('button', { name: '创建 golden sample' }))

    await waitFor(() => {
      expect(mockApiPostRawJSON).toHaveBeenCalledWith('/tasks/1/golden-samples', expect.any(String))
    })
    const body = mockApiPostRawJSON.mock.calls[0][1] as string
    expect(body).toContain('"payload":{"prompt":"示例题目"}')
    expect(body).toContain('"expected_answer":{"summary":"示例答案"}')
    expect(body).toContain('"ai_prompt_id":33')
    expect(body).not.toContain('\\"prompt\\"')
    expect(await screen.findByText('#11 · pass')).toBeInTheDocument()
  })

  it('shows golden sample create errors without keeping stale success state', async () => {
    const user = userEvent.setup()
    mockApiPostRawJSON.mockRejectedValue(new Error('duplicate golden sample'))

    render(<OwnerDashboard />)

    const createButton = await screen.findByRole('button', { name: '创建 golden sample' })
    await waitFor(() => expect(createButton).not.toBeDisabled())
    await user.click(createButton)

    expect(await screen.findByRole('alert')).toHaveTextContent('duplicate golden sample')
    expect(screen.queryByText('#11 · pass')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '创建 golden sample' })).not.toBeDisabled()
  })

  it('ignores stale golden sample create responses after switching tasks', async () => {
    const user = userEvent.setup()
    const createResult = deferred<unknown>()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/tasks') {
        return [
          { ...task, id: 1, title: 'Task A' },
          { ...task, id: 2, title: 'Task B' },
        ]
      }
      if (path === '/tasks/1/ai-prompts') {
        return { prompts: [], activePromptId: null, aiReviewEnabled: false }
      }
      if (path === '/tasks/2/ai-prompts') {
        return { prompts: [], activePromptId: null, aiReviewEnabled: false }
      }
      if (path.endsWith('/golden-samples')) {
        return { samples: [] }
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPostRawJSON.mockReturnValue(createResult.promise)

    render(<OwnerDashboard />)

    const createButton = await screen.findByRole('button', { name: '创建 golden sample' })
    await waitFor(() => expect(createButton).not.toBeDisabled())
    await user.click(createButton)
    await user.click(screen.getByRole('button', { name: /Task B/ }))
    expect(await screen.findByText('暂无 golden samples')).toBeInTheDocument()

    await act(async () => {
      createResult.resolve({
        sample: {
          id: 11,
          taskId: 1,
          aiPromptId: null,
          payload: { text: 'late' },
          payloadHash: 'hash',
          expectedAnswer: { label: 'late' },
          expectedVerdict: 'pass',
          notes: 'late create',
          createdBy: 7,
          createdAt: '2026-05-23T12:00:00Z',
        },
      })
      await createResult.promise
    })

    expect(screen.queryByText('late create')).not.toBeInTheDocument()
  })

  it('ignores stale golden sample loads after switching tasks', async () => {
    const user = userEvent.setup()
    const taskAGoldenSamples = deferred<unknown>()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/tasks') {
        return [
          { ...task, id: 1, title: 'Task A' },
          { ...task, id: 2, title: 'Task B' },
        ]
      }
      if (path === '/tasks/1/ai-prompts') {
        return { prompts: [], activePromptId: null, aiReviewEnabled: false }
      }
      if (path === '/tasks/2/ai-prompts') {
        return { prompts: [], activePromptId: null, aiReviewEnabled: false }
      }
      if (path === '/tasks/1/golden-samples') {
        return taskAGoldenSamples.promise
      }
      if (path === '/tasks/2/golden-samples') {
        return { samples: [] }
      }
      throw new Error(`unexpected GET ${path}`)
    })

    render(<OwnerDashboard />)

    await user.click(await screen.findByRole('button', { name: /Task B/ }))
    expect(await screen.findByDisplayValue('请根据 payload 和 answer 完成结构化预审。')).toBeInTheDocument()

    await act(async () => {
      taskAGoldenSamples.resolve({
        samples: [{
          id: 11,
          taskId: 1,
          aiPromptId: null,
          payload: { text: 'late' },
          payloadHash: 'hash',
          expectedAnswer: { label: 'late' },
          expectedVerdict: 'pass',
          notes: 'late sample',
          createdBy: 7,
          createdAt: '2026-05-23T12:00:00Z',
        }],
      })
      await taskAGoldenSamples.promise
    })

    expect(screen.queryByText('late sample')).not.toBeInTheDocument()
    expect(screen.getByText('暂无 golden samples')).toBeInTheDocument()
  })

  it('clears golden samples synchronously when switching tasks', async () => {
    const user = userEvent.setup()
    const taskBGoldenSamples = deferred<unknown>()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/tasks') {
        return [
          { ...task, id: 1, title: 'Task A' },
          { ...task, id: 2, title: 'Task B' },
        ]
      }
      if (path === '/tasks/1/ai-prompts' || path === '/tasks/2/ai-prompts') {
        return { prompts: [], activePromptId: null, aiReviewEnabled: false }
      }
      if (path === '/tasks/1/golden-samples') {
        return {
          samples: [{
            id: 11,
            taskId: 1,
            aiPromptId: null,
            payload: { text: 'a' },
            payloadHash: 'hash',
            expectedAnswer: { label: 'ok' },
            expectedVerdict: 'pass',
            notes: 'task a sample',
            createdBy: 7,
            createdAt: '2026-05-23T12:00:00Z',
          }],
        }
      }
      if (path === '/tasks/2/golden-samples') {
        return taskBGoldenSamples.promise
      }
      throw new Error(`unexpected GET ${path}`)
    })

    render(<OwnerDashboard />)

    expect(await screen.findByText('task a sample')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /Task B/ }))

    expect(screen.queryByText('task a sample')).not.toBeInTheDocument()
    expect(screen.getByText('加载 golden samples...')).toBeInTheDocument()
  })

  it('deletes a golden sample from the current task list', async () => {
    const user = userEvent.setup()
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/tasks') {
        return [task]
      }
      if (path === '/tasks/1/ai-prompts') {
        return { prompts: [], activePromptId: null, aiReviewEnabled: false }
      }
      if (path === '/tasks/1/golden-samples') {
        return {
          samples: [{
            id: 11,
            taskId: 1,
            aiPromptId: null,
            payload: { text: 'a' },
            payloadHash: 'hash',
            expectedAnswer: { label: 'ok' },
            expectedVerdict: 'pass',
            notes: 'delete me',
            createdBy: 7,
            createdAt: '2026-05-23T12:00:00Z',
          }],
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiDelete.mockResolvedValue({ deleted: true })

    render(<OwnerDashboard />)

    expect(await screen.findByText('delete me')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '删除 golden sample 11' }))

    await waitFor(() => {
      expect(mockApiDelete).toHaveBeenCalledWith('/tasks/1/golden-samples/11')
    })
    expect(screen.queryByText('delete me')).not.toBeInTheDocument()
    confirmSpy.mockRestore()
  })

  it('runs a golden sample and displays the result row', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/tasks') {
        return [task]
      }
      if (path === '/tasks/1/ai-prompts') {
        return { prompts: [], activePromptId: null, aiReviewEnabled: false }
      }
      if (path === '/tasks/1/golden-samples') {
        return {
          samples: [{
            id: 11,
            taskId: 1,
            aiPromptId: null,
            payload: { text: 'a' },
            payloadHash: 'hash',
            expectedAnswer: { label: 'ok' },
            expectedVerdict: 'pass',
            notes: null,
            createdBy: 7,
            createdAt: '2026-05-23T12:00:00Z',
          }],
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost.mockResolvedValue({
      provider: 'mock',
      dryRunId: 44,
      matchedExpected: false,
      result: {
        verdict: 'reject',
        overall_score: 30,
        dimensions: [],
        reason: 'not enough evidence',
        model: 'mock-model',
      },
    })

    render(<OwnerDashboard />)

    await user.click(await screen.findByRole('button', { name: 'Run golden sample 11' }))

    expect(mockApiPost).toHaveBeenCalledWith('/tasks/1/golden-samples/11/dry-run', {})
    expect(await screen.findByText('mismatch')).toBeInTheDocument()
    expect(screen.getByText('not enough evidence')).toBeInTheDocument()
    expect(screen.getByText('44')).toBeInTheDocument()
  })

  it('runs all visible golden samples through the batch endpoint and maps partial results', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/tasks') {
        return [task]
      }
      if (path === '/tasks/1/ai-prompts') {
        return { prompts: [], activePromptId: null, aiReviewEnabled: false }
      }
      if (path === '/tasks/1/golden-samples') {
        return {
          samples: [
            { id: 11, taskId: 1, aiPromptId: null, payload: { text: 'a' }, payloadHash: 'hash-a', expectedAnswer: { label: 'a' }, expectedVerdict: 'pass', notes: null, createdBy: 7, createdAt: '2026-05-23T12:00:00Z' },
            { id: 12, taskId: 1, aiPromptId: null, payload: { text: 'b' }, payloadHash: 'hash-b', expectedAnswer: { label: 'b' }, expectedVerdict: 'uncertain', notes: null, createdBy: 7, createdAt: '2026-05-23T12:00:00Z' },
          ],
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost.mockResolvedValue({
      summary: { total: 2, succeeded: 1, failed: 1 },
      results: [
        {
          goldenSampleId: 11,
          status: 'failed',
          dryRunId: 44,
          error: 'provider failed',
        },
        {
          goldenSampleId: 12,
          status: 'succeeded',
          provider: 'mock',
          dryRunId: 45,
          matchedExpected: true,
          result: {
            verdict: 'uncertain',
            overall_score: 75,
            dimensions: [],
            reason: 'second sample done',
            model: 'mock-model',
          },
        },
      ],
    })

    render(<OwnerDashboard />)

    await user.click(await screen.findByRole('button', { name: 'Run all visible samples' }))

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith('/tasks/1/golden-samples/dry-runs', { sample_ids: [11, 12] })
    })
    expect(mockApiPost).toHaveBeenCalledTimes(1)
    expect(await screen.findByRole('alert')).toHaveTextContent('provider failed')
    expect(screen.getByText('second sample done')).toBeInTheDocument()
    expect(screen.getAllByText('matched')).toHaveLength(2)
    expect(screen.getByText('44')).toBeInTheDocument()
    expect(screen.getByText('45')).toBeInTheDocument()
  })

  it('ignores stale batch golden sample run responses after switching tasks', async () => {
    const user = userEvent.setup()
    const runResult = deferred<unknown>()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/tasks') {
        return [
          { ...task, id: 1, title: 'Task A' },
          { ...task, id: 2, title: 'Task B' },
        ]
      }
      if (path === '/tasks/1/ai-prompts') {
        return { prompts: [], activePromptId: null, aiReviewEnabled: false }
      }
      if (path === '/tasks/2/ai-prompts') {
        return { prompts: [], activePromptId: null, aiReviewEnabled: false }
      }
      if (path === '/tasks/1/golden-samples') {
        return {
          samples: [{
            id: 11,
            taskId: 1,
            aiPromptId: null,
            payload: { text: 'a' },
            payloadHash: 'hash',
            expectedAnswer: { label: 'ok' },
            expectedVerdict: 'pass',
            notes: null,
            createdBy: 7,
            createdAt: '2026-05-23T12:00:00Z',
          }],
        }
      }
      if (path === '/tasks/2/golden-samples') {
        return { samples: [] }
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost.mockReturnValue(runResult.promise)

    render(<OwnerDashboard />)

    await user.click(await screen.findByRole('button', { name: 'Run all visible samples' }))
    await user.click(screen.getByRole('button', { name: /Task B/ }))
    expect(await screen.findByText('暂无 golden samples')).toBeInTheDocument()

    await act(async () => {
      runResult.resolve({
        summary: { total: 1, succeeded: 1, failed: 0 },
        results: [{
          goldenSampleId: 11,
          status: 'succeeded',
          provider: 'mock',
          dryRunId: 44,
          matchedExpected: true,
          result: {
            verdict: 'pass',
            overall_score: 90,
            dimensions: [],
            reason: 'late batch golden run',
          },
        }],
      })
      await runResult.promise
    })

    expect(screen.queryByText('late batch golden run')).not.toBeInTheDocument()
  })

  it('keeps same-task batch golden sample run response current after clicking selected task again', async () => {
    const user = userEvent.setup()
    const runResult = deferred<unknown>()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/tasks') {
        return [{ ...task, title: 'Task A' }]
      }
      if (path === '/tasks/1/ai-prompts') {
        return { prompts: [], activePromptId: null, aiReviewEnabled: false }
      }
      if (path === '/tasks/1/golden-samples') {
        return {
          samples: [{
            id: 11,
            taskId: 1,
            aiPromptId: null,
            payload: { text: 'a' },
            payloadHash: 'hash',
            expectedAnswer: { label: 'ok' },
            expectedVerdict: 'pass',
            notes: null,
            createdBy: 7,
            createdAt: '2026-05-23T12:00:00Z',
          }],
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost.mockReturnValue(runResult.promise)

    render(<OwnerDashboard />)

    await user.click(await screen.findByRole('button', { name: 'Run all visible samples' }))
    await user.click(screen.getByRole('button', { name: /Task A/ }))

    await act(async () => {
      runResult.resolve({
        summary: { total: 1, succeeded: 1, failed: 0 },
        results: [{
          goldenSampleId: 11,
          status: 'succeeded',
          provider: 'mock',
          dryRunId: 44,
          matchedExpected: true,
          result: {
            verdict: 'pass',
            overall_score: 90,
            dimensions: [],
            reason: 'same task batch golden run',
          },
        }],
      })
      await runResult.promise
    })

    expect(await screen.findByText('same task batch golden run')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Run all visible samples' })).not.toBeDisabled()
  })

  it('ignores stale golden sample run responses after switching tasks', async () => {
    const user = userEvent.setup()
    const runResult = deferred<unknown>()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/tasks') {
        return [
          { ...task, id: 1, title: 'Task A' },
          { ...task, id: 2, title: 'Task B' },
        ]
      }
      if (path === '/tasks/1/ai-prompts') {
        return { prompts: [], activePromptId: null, aiReviewEnabled: false }
      }
      if (path === '/tasks/2/ai-prompts') {
        return { prompts: [], activePromptId: null, aiReviewEnabled: false }
      }
      if (path === '/tasks/1/golden-samples') {
        return {
          samples: [{
            id: 11,
            taskId: 1,
            aiPromptId: null,
            payload: { text: 'a' },
            payloadHash: 'hash',
            expectedAnswer: { label: 'ok' },
            expectedVerdict: 'pass',
            notes: null,
            createdBy: 7,
            createdAt: '2026-05-23T12:00:00Z',
          }],
        }
      }
      if (path === '/tasks/2/golden-samples') {
        return { samples: [] }
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost.mockReturnValue(runResult.promise)

    render(<OwnerDashboard />)

    await user.click(await screen.findByRole('button', { name: 'Run golden sample 11' }))
    await user.click(screen.getByRole('button', { name: /Task B/ }))
    expect(await screen.findByText('暂无 golden samples')).toBeInTheDocument()

    await act(async () => {
      runResult.resolve({
        provider: 'mock',
        dryRunId: 44,
        matchedExpected: true,
        result: {
          verdict: 'pass',
          overall_score: 90,
          dimensions: [],
          reason: 'late golden run',
        },
      })
      await runResult.promise
    })

    expect(screen.queryByText('late golden run')).not.toBeInTheDocument()
  })

  it('keeps same-task golden sample run response current after clicking selected task again', async () => {
    const user = userEvent.setup()
    const runResult = deferred<unknown>()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/tasks') {
        return [{ ...task, title: 'Task A' }]
      }
      if (path === '/tasks/1/ai-prompts') {
        return { prompts: [], activePromptId: null, aiReviewEnabled: false }
      }
      if (path === '/tasks/1/golden-samples') {
        return {
          samples: [{
            id: 11,
            taskId: 1,
            aiPromptId: null,
            payload: { text: 'a' },
            payloadHash: 'hash',
            expectedAnswer: { label: 'ok' },
            expectedVerdict: 'pass',
            notes: null,
            createdBy: 7,
            createdAt: '2026-05-23T12:00:00Z',
          }],
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost.mockReturnValue(runResult.promise)

    render(<OwnerDashboard />)

    await user.click(await screen.findByRole('button', { name: 'Run golden sample 11' }))
    await user.click(screen.getByRole('button', { name: /Task A/ }))

    await act(async () => {
      runResult.resolve({
        provider: 'mock',
        dryRunId: 44,
        matchedExpected: true,
        result: {
          verdict: 'pass',
          overall_score: 90,
          dimensions: [],
          reason: 'same task golden run',
        },
      })
      await runResult.promise
    })

    expect(await screen.findByText('same task golden run')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Run golden sample 11' })).not.toBeDisabled()
  })

  it('loads dry-run history and shows failed errors', async () => {
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/tasks') {
        return [task]
      }
      if (path === '/tasks/1/ai-prompts') {
        return { prompts: [], activePromptId: null, aiReviewEnabled: false }
      }
      if (path === '/tasks/1/golden-samples') {
        return {
          samples: [{
            id: 11,
            taskId: 1,
            aiPromptId: null,
            payload: { text: 'a' },
            payloadHash: 'hash',
            expectedAnswer: { label: 'ok' },
            expectedVerdict: 'pass',
            notes: null,
            createdBy: 7,
            createdAt: '2026-05-23T12:00:00Z',
          }],
        }
      }
      if (path === '/tasks/1/ai-dry-runs?limit=10') {
        return {
          dryRuns: [
            { id: 44, taskId: 1, aiPromptId: 33, goldenSampleId: 11, promptVersion: 3, expectedVerdict: 'pass', actualVerdict: 'pass', matchedExpected: true, status: 'succeeded', errorMsg: null, createdAt: '2026-05-25T12:00:00Z', finishedAt: '2026-05-25T12:01:00Z' },
            { id: 45, taskId: 1, aiPromptId: 33, goldenSampleId: 11, promptVersion: 3, expectedVerdict: 'pass', actualVerdict: null, matchedExpected: null, status: 'failed', errorMsg: 'provider timeout', createdAt: '2026-05-25T12:02:00Z', finishedAt: '2026-05-25T12:03:00Z' },
          ],
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })

    render(<OwnerDashboard />)

    expect(await screen.findByText('#44')).toBeInTheDocument()
    expect(screen.getByText('#45')).toBeInTheDocument()
    expect(screen.getByText('failed · provider timeout')).toBeInTheDocument()
    expect(screen.getAllByText('v3 #33')).toHaveLength(2)
    expect(screen.getAllByText('matched').length).toBeGreaterThanOrEqual(1)
  })

  it('filters dry-run history by golden sample', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/tasks') {
        return [task]
      }
      if (path === '/tasks/1/ai-prompts') {
        return { prompts: [], activePromptId: null, aiReviewEnabled: false }
      }
      if (path === '/tasks/1/golden-samples') {
        return {
          samples: [
            { id: 11, taskId: 1, aiPromptId: null, payload: { text: 'a' }, payloadHash: 'hash-a', expectedAnswer: { label: 'a' }, expectedVerdict: 'pass', notes: null, createdBy: 7, createdAt: '2026-05-23T12:00:00Z' },
            { id: 12, taskId: 1, aiPromptId: null, payload: { text: 'b' }, payloadHash: 'hash-b', expectedAnswer: { label: 'b' }, expectedVerdict: 'reject', notes: null, createdBy: 7, createdAt: '2026-05-23T12:00:00Z' },
          ],
        }
      }
      if (path === '/tasks/1/ai-dry-runs?limit=10') {
        return { dryRuns: [] }
      }
      if (path === '/tasks/1/ai-dry-runs?golden_sample_id=12&limit=10') {
        return {
          dryRuns: [
            { id: 77, taskId: 1, aiPromptId: 34, goldenSampleId: 12, promptVersion: 4, expectedVerdict: 'reject', actualVerdict: 'pass', matchedExpected: false, status: 'succeeded', errorMsg: null, createdAt: '2026-05-25T12:02:00Z', finishedAt: '2026-05-25T12:03:00Z' },
          ],
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })

    render(<OwnerDashboard />)

    await user.click(await screen.findByRole('button', { name: '查看 golden sample 12 history' }))

    await waitFor(() => {
      expect(mockApiGet).toHaveBeenCalledWith('/tasks/1/ai-dry-runs?golden_sample_id=12&limit=10')
    })
    expect(await screen.findByText('#77')).toBeInTheDocument()
    expect(screen.getByLabelText('dry_run_history_sample_filter')).toHaveValue('12')
    expect(screen.getByText('mismatch')).toBeInTheDocument()
    expect(screen.getByText('v4 #34')).toBeInTheDocument()
  })

  it('ignores stale dry-run history responses after switching tasks', async () => {
    const user = userEvent.setup()
    const taskAHistory = deferred<unknown>()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/tasks') {
        return [
          { ...task, id: 1, title: 'Task A' },
          { ...task, id: 2, title: 'Task B' },
        ]
      }
      if (path === '/tasks/1/ai-prompts' || path === '/tasks/2/ai-prompts') {
        return { prompts: [], activePromptId: null, aiReviewEnabled: false }
      }
      if (path === '/tasks/1/golden-samples' || path === '/tasks/2/golden-samples') {
        return { samples: [] }
      }
      if (path === '/tasks/1/ai-dry-runs?limit=10') {
        return taskAHistory.promise
      }
      if (path === '/tasks/2/ai-dry-runs?limit=10') {
        return { dryRuns: [] }
      }
      throw new Error(`unexpected GET ${path}`)
    })

    render(<OwnerDashboard />)

    await user.click(await screen.findByRole('button', { name: /Task B/ }))
    expect(await screen.findByText('暂无 dry-run history')).toBeInTheDocument()

    await act(async () => {
      taskAHistory.resolve({
        dryRuns: [{
          id: 88,
          taskId: 1,
          aiPromptId: 33,
          goldenSampleId: 11,
          promptVersion: 3,
          expectedVerdict: 'pass',
          actualVerdict: null,
          matchedExpected: null,
          status: 'failed',
          errorMsg: 'late history error',
          createdAt: '2026-05-25T12:00:00Z',
          finishedAt: '2026-05-25T12:01:00Z',
        }],
      })
      await taskAHistory.promise
    })

    expect(screen.queryByText('late history error')).not.toBeInTheDocument()
    expect(screen.queryByText('#88')).not.toBeInTheDocument()
  })

  it('resets golden sample draft only when switching to a different task', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/tasks') {
        return [
          { ...task, id: 1, title: 'Task A' },
          { ...task, id: 2, title: 'Task B' },
        ]
      }
      if (path === '/tasks/1/ai-prompts' || path === '/tasks/2/ai-prompts') {
        return { prompts: [], activePromptId: null, aiReviewEnabled: false }
      }
      if (path === '/tasks/1/golden-samples' || path === '/tasks/2/golden-samples') {
        return { samples: [] }
      }
      throw new Error(`unexpected GET ${path}`)
    })

    render(<OwnerDashboard />)

    const payloadInput = await screen.findByLabelText('golden_sample_payload')
    const answerInput = screen.getByLabelText('golden_sample_expected_answer')
    const verdictInput = screen.getByLabelText('golden_sample_expected_verdict')
    const promptChoiceInput = screen.getByLabelText('golden_sample_prompt')
    const notesInput = screen.getByLabelText('golden_sample_notes')

    fireEvent.change(payloadInput, { target: { value: '{"prompt":"Task A custom"}' } })
    fireEvent.change(answerInput, { target: { value: '{"summary":"Task A answer"}' } })
    await user.selectOptions(verdictInput, 'reject')
    await user.selectOptions(promptChoiceInput, 'none')
    fireEvent.change(notesInput, { target: { value: 'Task A note' } })

    await user.click(screen.getByRole('button', { name: /Task B/ }))

    expect(await screen.findByLabelText('golden_sample_payload')).toHaveValue('{"prompt":"示例题目"}')
    expect(screen.getByLabelText('golden_sample_expected_answer')).toHaveValue('{"summary":"示例答案"}')
    expect(screen.getByLabelText('golden_sample_expected_verdict')).toHaveValue('pass')
    expect(screen.getByLabelText('golden_sample_prompt')).toHaveValue('active')
    expect(screen.getByLabelText('golden_sample_notes')).toHaveValue('')

    fireEvent.change(screen.getByLabelText('golden_sample_payload'), { target: { value: '{"prompt":"Task B draft"}' } })
    await user.selectOptions(screen.getByLabelText('golden_sample_expected_verdict'), 'uncertain')
    await user.selectOptions(screen.getByLabelText('golden_sample_prompt'), 'none')
    fireEvent.change(screen.getByLabelText('golden_sample_notes'), { target: { value: 'Task B note' } })
    await user.click(screen.getByRole('button', { name: /Task B/ }))

    expect(screen.getByLabelText('golden_sample_payload')).toHaveValue('{"prompt":"Task B draft"}')
    expect(screen.getByLabelText('golden_sample_expected_verdict')).toHaveValue('uncertain')
    expect(screen.getByLabelText('golden_sample_prompt')).toHaveValue('none')
    expect(screen.getByLabelText('golden_sample_notes')).toHaveValue('Task B note')
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
