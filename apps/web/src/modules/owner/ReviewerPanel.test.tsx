import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ReviewerPanel from './ReviewerPanel'
import { addReviewers, listReviewerCandidates, listReviewers, removeReviewer } from '../../shared/api/client'

vi.mock('../../shared/api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../shared/api/client')>()
  return {
    ...actual,
    listReviewers: vi.fn(),
    listReviewerCandidates: vi.fn(),
    addReviewers: vi.fn(),
    removeReviewer: vi.fn(),
  }
})

vi.mock('@douyinfe/semi-ui', () => ({
  Toast: { error: vi.fn(), success: vi.fn(), info: vi.fn() },
}))

const mockListReviewers = vi.mocked(listReviewers)
const mockListCandidates = vi.mocked(listReviewerCandidates)
const mockAddReviewers = vi.mocked(addReviewers)
const mockRemoveReviewer = vi.mocked(removeReviewer)

const candidates = [
  { userId: 201, username: 'reviewer1', displayName: '审核员一号' },
  { userId: 202, username: 'reviewer2', displayName: '审核员二号' },
]

describe('ReviewerPanel', () => {
  beforeEach(() => {
    mockListReviewers.mockReset()
    mockListCandidates.mockReset()
    mockAddReviewers.mockReset()
    mockRemoveReviewer.mockReset()
  })

  it('shows reviewer names and lists only unassigned reviewers in the picker', async () => {
    mockListReviewers.mockResolvedValue([{ userId: 201, assignedAt: '' }])
    mockListCandidates.mockResolvedValue(candidates)

    render(<ReviewerPanel taskId={5} />)

    // 已指派的用 displayName 展示
    expect(await screen.findByText('审核员一号')).toBeInTheDocument()
    // 选择器只列未指派的审核员(202),不含已指派的 201
    const picker = screen.getByLabelText('reviewer_candidate') as HTMLSelectElement
    const optionValues = [...picker.options].map((option) => option.value)
    expect(optionValues).toContain('202')
    expect(optionValues).not.toContain('201')
  })

  it('assigns the selected reviewer by userId', async () => {
    mockListReviewers.mockResolvedValue([])
    mockListCandidates.mockResolvedValue(candidates)
    mockAddReviewers.mockResolvedValue({ added: 1 })

    const user = userEvent.setup()
    render(<ReviewerPanel taskId={5} />)

    await screen.findByLabelText('reviewer_candidate')
    await user.selectOptions(screen.getByLabelText('reviewer_candidate'), '202')
    await user.click(screen.getByLabelText('添加审核员'))

    await waitFor(() => expect(mockAddReviewers).toHaveBeenCalledWith(5, [202]))
  })

  it('removes a reviewer', async () => {
    mockListReviewers.mockResolvedValue([{ userId: 201, assignedAt: '' }])
    mockListCandidates.mockResolvedValue(candidates)
    mockRemoveReviewer.mockResolvedValue({ removed: 201 })

    const user = userEvent.setup()
    render(<ReviewerPanel taskId={5} />)

    await user.click(await screen.findByLabelText('移除审核员 201'))
    expect(mockRemoveReviewer).toHaveBeenCalledWith(5, 201)
  })
})
