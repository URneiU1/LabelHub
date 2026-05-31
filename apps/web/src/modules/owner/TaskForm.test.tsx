import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TaskForm from './TaskForm'
import { createTask, updateTask } from '../../shared/api/client'

vi.mock('../../shared/api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../shared/api/client')>()
  return {
    ...actual,
    createTask: vi.fn(),
    updateTask: vi.fn(),
  }
})

vi.mock('@douyinfe/semi-ui', () => ({
  Toast: { error: vi.fn(), success: vi.fn(), info: vi.fn() },
}))

const mockCreateTask = vi.mocked(createTask)
const mockUpdateTask = vi.mocked(updateTask)

const savedTask = {
  id: 7,
  title: '商品标题清洗 v3',
  description: '清洗电商标题',
  baselineDescription: null,
  status: 'draft',
  totalItems: 0,
  finishedItems: 0,
  distribution: 'quota',
}

describe('TaskForm', () => {
  beforeEach(() => {
    mockCreateTask.mockReset()
    mockUpdateTask.mockReset()
  })

  it('creates a task with content, distribution and quality policies', async () => {
    const user = userEvent.setup()
    mockCreateTask.mockResolvedValue(savedTask)
    const onSaved = vi.fn()

    render(<TaskForm task={null} onSaved={onSaved} />)

    await user.type(screen.getByLabelText('task_title'), '商品标题清洗 v3')
    await user.type(screen.getByLabelText('task_description'), '清洗电商标题')
    await user.type(screen.getByLabelText('task_tag_input'), '电商')
    await user.click(screen.getByRole('button', { name: '添加标签' }))
    await user.type(screen.getByLabelText('task_reward_amount'), '0.3')
    await user.type(screen.getByLabelText('task_quota_per_user'), '100')
    await user.clear(screen.getByLabelText('task_overlap_count'))
    await user.type(screen.getByLabelText('task_overlap_count'), '3')
    await user.clear(screen.getByLabelText('task_overlap_coverage_pct'))
    await user.type(screen.getByLabelText('task_overlap_coverage_pct'), '50')
    await user.clear(screen.getByLabelText('task_lease_timeout_minutes'))
    await user.type(screen.getByLabelText('task_lease_timeout_minutes'), '45')
    await user.clear(screen.getByLabelText('task_review_sampling_pct'))
    await user.type(screen.getByLabelText('task_review_sampling_pct'), '20')
    await user.clear(screen.getByLabelText('task_daily_submission_limit'))
    await user.type(screen.getByLabelText('task_daily_submission_limit'), '12')
    await user.click(screen.getByRole('button', { name: '分发策略 配额抢单' }))
    await user.click(screen.getByRole('button', { name: '创建任务' }))

    await waitFor(() => {
      expect(mockCreateTask).toHaveBeenCalledTimes(1)
    })
    const input = mockCreateTask.mock.calls[0][0]
    expect(input.title).toBe('商品标题清洗 v3')
    expect(input.description).toBe('清洗电商标题')
    expect(input.tags).toEqual(['电商'])
    expect(input.rewardConfig).toEqual({ amount: 0.3, unit: '元/条' })
    expect(input.distribution).toBe('quota')
    expect(input.quotaPerUser).toBe(100)
    expect(input.overlapCount).toBe(3)
    expect(input.overlapCoveragePct).toBe(50)
    expect(input.leaseTimeoutMinutes).toBe(45)
    expect(input.reviewSamplingPct).toBe(20)
    expect(input.dailySubmissionLimitPerLabeler).toBe(12)
    expect(input.humanReviewEnabled).toBe(true)
    expect(onSaved).toHaveBeenCalledWith(savedTask, true)
  })

  it('blocks submit and shows an error when the title is empty', async () => {
    const user = userEvent.setup()
    const onSaved = vi.fn()

    render(<TaskForm task={null} onSaved={onSaved} />)

    await user.click(screen.getByRole('button', { name: '创建任务' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('任务标题不能为空')
    expect(mockCreateTask).not.toHaveBeenCalled()
  })

  it('prefills fields from an existing task and updates only the title path', async () => {
    const user = userEvent.setup()
    mockUpdateTask.mockResolvedValue({ ...savedTask, title: '商品标题清洗 v4' })
    const onSaved = vi.fn()

    render(<TaskForm task={{ ...savedTask, tags: '["电商","清洗"]', rewardConfig: '{"amount":0.5,"unit":"元/条"}' }} onSaved={onSaved} />)

    expect(screen.getByLabelText('task_title')).toHaveValue('商品标题清洗 v3')
    expect(screen.getByText('电商')).toBeInTheDocument()
    expect(screen.getByText('清洗')).toBeInTheDocument()
    expect(screen.getByLabelText('task_reward_amount')).toHaveValue(0.5)

    await user.clear(screen.getByLabelText('task_title'))
    await user.type(screen.getByLabelText('task_title'), '商品标题清洗 v4')
    await user.click(screen.getByRole('button', { name: '保存任务' }))

    await waitFor(() => {
      expect(mockUpdateTask).toHaveBeenCalledWith(7, expect.objectContaining({ title: '商品标题清洗 v4' }))
    })
    expect(onSaved).toHaveBeenCalledWith(expect.objectContaining({ title: '商品标题清洗 v4' }), false)
  })
})
