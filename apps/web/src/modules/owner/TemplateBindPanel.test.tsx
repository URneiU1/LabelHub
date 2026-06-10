import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TemplateBindPanel from './TemplateBindPanel'
import { apiGet, updateTask, type Task } from '../../shared/api/client'

vi.mock('../../shared/api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../shared/api/client')>()
  return {
    ...actual,
    apiGet: vi.fn(),
    updateTask: vi.fn(),
  }
})

vi.mock('@douyinfe/semi-ui', () => ({
  Toast: { error: vi.fn(), success: vi.fn(), info: vi.fn() },
}))

const mockApiGet = vi.mocked(apiGet)
const mockUpdateTask = vi.mocked(updateTask)

const templates = [
  { id: 12, taskId: 7, version: 2, schemaJson: '{}', createdAt: '2026-06-01T00:00:00Z' },
  { id: 11, taskId: 7, version: 1, schemaJson: '{}', createdAt: '2026-05-30T00:00:00Z' },
]

const draftTask: Task = {
  id: 7,
  title: '商品标题清洗 v3',
  description: null,
  baselineDescription: null,
  status: 'draft',
  templateId: 12,
  totalItems: 0,
  finishedItems: 0,
}

function renderPanel(task: Task, onTaskSaved = vi.fn()) {
  render(
    <MemoryRouter>
      <TemplateBindPanel task={task} onTaskSaved={onTaskSaved} />
    </MemoryRouter>,
  )
  return onTaskSaved
}

describe('TemplateBindPanel', () => {
  beforeEach(() => {
    mockApiGet.mockReset()
    mockUpdateTask.mockReset()
  })

  it('lists template versions and marks the currently bound one', async () => {
    mockApiGet.mockResolvedValue(templates)
    renderPanel(draftTask)

    expect(await screen.findByText('当前绑定')).toBeInTheDocument()
    expect(screen.getByText('v2')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '绑定模板版本 v1' })).toBeEnabled()
    expect(mockApiGet).toHaveBeenCalledWith('/tasks/7/templates')
  })

  it('switches the bound version via updateTask and bubbles the saved task', async () => {
    const user = userEvent.setup()
    mockApiGet.mockResolvedValue(templates)
    mockUpdateTask.mockResolvedValue({ ...draftTask, templateId: 11 })
    const onTaskSaved = renderPanel(draftTask)

    await user.click(await screen.findByRole('button', { name: '绑定模板版本 v1' }))

    await waitFor(() => {
      expect(mockUpdateTask).toHaveBeenCalledWith(7, { templateId: 11 })
    })
    expect(onTaskSaved).toHaveBeenCalledWith(expect.objectContaining({ templateId: 11 }), false)
  })

  it('freezes switching once the task has left draft', async () => {
    mockApiGet.mockResolvedValue(templates)
    renderPanel({ ...draftTask, status: 'published' })

    // 发布后模板冻结(与后端 TaskPoliciesFrozen 一致):按钮禁用 + 冻结提示。
    expect(await screen.findByRole('button', { name: '绑定模板版本 v1' })).toBeDisabled()
    expect(screen.getByText(/模板版本已冻结/)).toBeInTheDocument()
    expect(mockUpdateTask).not.toHaveBeenCalled()
  })

  it('allows switching the bound version when the task is paused', async () => {
    const user = userEvent.setup()
    mockApiGet.mockResolvedValue(templates)
    mockUpdateTask.mockResolvedValue({ ...draftTask, status: 'paused', templateId: 11 })
    renderPanel({ ...draftTask, status: 'paused' })

    // paused 不冻结:按钮可用、点击走 updateTask 绑定。
    const btn = await screen.findByRole('button', { name: '绑定模板版本 v1' })
    expect(btn).not.toBeDisabled()
    await user.click(btn)
    await waitFor(() => expect(mockUpdateTask).toHaveBeenCalledWith(7, { templateId: 11 }))
  })

  it('shows an empty state with a designer link when no version exists', async () => {
    mockApiGet.mockResolvedValue([])
    renderPanel(draftTask)

    expect(await screen.findByText('暂无模板版本')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '前往模板搭建页' })).toHaveAttribute('href', '/owner/tasks/7/templates')
  })
})
