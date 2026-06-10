import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TaskManagePanel from './TaskManagePanel'
import { ApiError, importItemsFile, listAssignees, transitionTask } from '../../shared/api/client'

const mockModalConfirm = vi.hoisted(() => vi.fn())

vi.mock('../../shared/api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../shared/api/client')>()
  return {
    ...actual,
    transitionTask: vi.fn(),
    importItemsFile: vi.fn(),
    createTask: vi.fn(),
    updateTask: vi.fn(),
    listAssignees: vi.fn(),
    addAssignees: vi.fn(),
    removeAssignee: vi.fn(),
    previewItem: vi.fn(),
    importItems: vi.fn(),
    batchUpdateItems: vi.fn(),
  }
})

vi.mock('@douyinfe/semi-ui', () => ({
  Toast: { error: vi.fn(), success: vi.fn(), info: vi.fn() },
  Modal: { confirm: mockModalConfirm },
}))

const mockTransitionTask = vi.mocked(transitionTask)
const mockImportItemsFile = vi.mocked(importItemsFile)
const mockListAssignees = vi.mocked(listAssignees)

const draftTask = {
  id: 7,
  title: '商品标题清洗 v3',
  description: null,
  baselineDescription: null,
  status: 'draft',
  totalItems: 10,
  finishedItems: 4,
  distribution: 'first_come',
}

const publishedTask = {
  id: 8,
  title: 'QA 质量校验',
  description: null,
  baselineDescription: null,
  status: 'published',
  totalItems: 10,
  finishedItems: 4,
  distribution: 'first_come',
}

describe('TaskManagePanel', () => {
  beforeEach(() => {
    mockTransitionTask.mockReset()
    mockImportItemsFile.mockReset()
    mockListAssignees.mockReset()
    mockModalConfirm.mockReset()
    mockListAssignees.mockResolvedValue([])
  })

  it('renders stat cards and a task row with status, distribution and progress', () => {
    render(
      <TaskManagePanel tasks={[draftTask]} selected={draftTask} onSelect={vi.fn()} onTaskSaved={vi.fn()} onTasksChanged={vi.fn()} />,
    )

    expect(screen.getByText('发布中任务')).toBeInTheDocument()
    expect(screen.getByText('商品标题清洗 v3')).toBeInTheDocument()
    expect(screen.getByText('先到先得')).toBeInTheDocument()
    expect(screen.getByText('4 / 10')).toBeInTheDocument()
  })

  it('selects a task row from the keyboard', async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()
    render(
      <TaskManagePanel tasks={[draftTask]} selected={null} onSelect={onSelect} onTaskSaved={vi.fn()} onTasksChanged={vi.fn()} />,
    )

    const selectButton = screen.getByRole('button', { name: '选择任务 商品标题清洗 v3' })
    selectButton.focus()
    await user.keyboard('{Enter}')

    expect(onSelect).toHaveBeenCalledWith(draftTask)
  })

  it('selects a task by clicking anywhere on the row, not only the title', async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()
    render(
      <TaskManagePanel tasks={[draftTask]} selected={null} onSelect={onSelect} onTaskSaved={vi.fn()} onTasksChanged={vi.fn()} />,
    )

    // 点分发策略列(非标题、非按钮)也应选中:整行可点。回归 2026-06-04「只有标题可点 → 点行其它地方没反应」。
    await user.click(screen.getByText('先到先得'))

    expect(onSelect).toHaveBeenCalledTimes(1)
    expect(onSelect).toHaveBeenCalledWith(draftTask)
  })

  it('publishes a draft task via the state-machine transition', async () => {
    const user = userEvent.setup()
    mockTransitionTask.mockResolvedValue({ ...draftTask, status: 'published' })
    const onTaskSaved = vi.fn()

    render(
      <TaskManagePanel tasks={[draftTask]} selected={draftTask} onSelect={vi.fn()} onTaskSaved={onTaskSaved} onTasksChanged={vi.fn()} />,
    )

    await user.click(screen.getByRole('button', { name: '发布任务' }))

    await waitFor(() => {
      expect(mockTransitionTask).toHaveBeenCalledWith(7, 'publish')
    })
    expect(onTaskSaved).toHaveBeenCalledWith(expect.objectContaining({ status: 'published' }), false)
  })

  it('surfaces the friendly ApiError when publishing without a template returns 422', async () => {
    const user = userEvent.setup()
    const { Toast } = await import('@douyinfe/semi-ui')
    mockTransitionTask.mockRejectedValue(new ApiError('INVALID_STATE', 'task must have a bound template before publishing', 'req-1'))

    render(
      <TaskManagePanel tasks={[draftTask]} selected={draftTask} onSelect={vi.fn()} onTaskSaved={vi.fn()} onTasksChanged={vi.fn()} />,
    )

    await user.click(screen.getByRole('button', { name: '发布任务' }))

    await waitFor(() => {
      // 没绑模板的 INVALID_STATE 现按后端原因分流成"先搭模板"提示(见 client.ts resolveFriendlyMessage),不再是泛化的"刷新"。
      expect(Toast.error).toHaveBeenCalledWith(expect.stringContaining('模板'))
    })
  })

  it('shows the template tab in the edit drawer', async () => {
    const user = userEvent.setup()

    render(
      <TaskManagePanel tasks={[draftTask]} selected={draftTask} onSelect={vi.fn()} onTaskSaved={vi.fn()} onTasksChanged={vi.fn()} />,
    )

    await user.click(screen.getByRole('button', { name: '编辑任务' }))

    // 编辑抽屉提供「模板」标签页(TemplateBindPanel),Owner 可显式查看/切换绑定的模板版本。
    expect(screen.getByRole('button', { name: '抽屉标签 模板' })).toBeInTheDocument()
  })

  it('uploads a dataset file via the import-file endpoint from the edit drawer', async () => {
    const user = userEvent.setup()
    mockImportItemsFile.mockResolvedValue({ imported: 5, format: 'json' })
    const onTasksChanged = vi.fn()

    render(
      <TaskManagePanel tasks={[draftTask]} selected={draftTask} onSelect={vi.fn()} onTaskSaved={vi.fn()} onTasksChanged={onTasksChanged} />,
    )

    await user.click(screen.getByRole('button', { name: '编辑任务' }))
    await user.click(screen.getByRole('button', { name: '抽屉标签 数据集' }))

    const file = new File(['[{"id":"a"}]'], 'items.json', { type: 'application/json' })
    await user.upload(screen.getByLabelText('import_file'), file)

    await waitFor(() => {
      expect(mockImportItemsFile).toHaveBeenCalledWith(7, file)
    })
    expect(onTasksChanged).toHaveBeenCalled()
  })

  it('asks for confirmation before ending a task and only transitions after the user confirms', async () => {
    const user = userEvent.setup()
    mockTransitionTask.mockResolvedValue({ ...publishedTask, status: 'ended' })

    render(
      <TaskManagePanel tasks={[publishedTask]} selected={publishedTask} onSelect={vi.fn()} onTaskSaved={vi.fn()} onTasksChanged={vi.fn()} />,
    )

    await user.click(screen.getByRole('button', { name: '下线任务' }))

    // 「下线 / 结束」是终态不可逆,先弹二次确认,确认前不应真的下线。
    expect(mockModalConfirm).toHaveBeenCalledWith(expect.objectContaining({
      title: '确定结束该任务?',
      content: expect.stringContaining('不可恢复'),
    }))
    expect(mockTransitionTask).not.toHaveBeenCalled()

    // 点「确定」(onOk)后才真正走状态机 end。
    const confirmConfig = mockModalConfirm.mock.calls[0][0]
    await act(async () => {
      await confirmConfig.onOk()
    })

    await waitFor(() => {
      expect(mockTransitionTask).toHaveBeenCalledWith(8, 'end')
    })
  })

  it('does not end the task when the confirmation is cancelled', async () => {
    const user = userEvent.setup()

    render(
      <TaskManagePanel tasks={[publishedTask]} selected={publishedTask} onSelect={vi.fn()} onTaskSaved={vi.fn()} onTasksChanged={vi.fn()} />,
    )

    await user.click(screen.getByRole('button', { name: '下线任务' }))

    // 取消 = 不触发 onOk,状态机不应被调用。
    expect(mockModalConfirm).toHaveBeenCalledTimes(1)
    expect(mockTransitionTask).not.toHaveBeenCalled()
  })

  it('pauses a published task directly without a confirmation dialog', async () => {
    const user = userEvent.setup()
    mockTransitionTask.mockResolvedValue({ ...publishedTask, status: 'paused' })

    render(
      <TaskManagePanel tasks={[publishedTask]} selected={publishedTask} onSelect={vi.fn()} onTaskSaved={vi.fn()} onTasksChanged={vi.fn()} />,
    )

    await user.click(screen.getByRole('button', { name: '暂停任务' }))

    // 只有「下线 / 结束」走确认弹窗,其它转移(暂停/恢复/发布)直接执行。
    await waitFor(() => {
      expect(mockTransitionTask).toHaveBeenCalledWith(8, 'pause')
    })
    expect(mockModalConfirm).not.toHaveBeenCalled()
  })
})
