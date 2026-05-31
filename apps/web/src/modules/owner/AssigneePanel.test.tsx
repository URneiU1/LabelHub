import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AssigneePanel from './AssigneePanel'
import { addAssignees, listAssigneeCandidates, listAssignees, removeAssignee } from '../../shared/api/client'

vi.mock('../../shared/api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../shared/api/client')>()
  return {
    ...actual,
    listAssignees: vi.fn(),
    listAssigneeCandidates: vi.fn(),
    addAssignees: vi.fn(),
    removeAssignee: vi.fn(),
  }
})

vi.mock('@douyinfe/semi-ui', () => ({
  Toast: { error: vi.fn(), success: vi.fn(), info: vi.fn() },
}))

const mockListAssignees = vi.mocked(listAssignees)
const mockListCandidates = vi.mocked(listAssigneeCandidates)
const mockAddAssignees = vi.mocked(addAssignees)
const mockRemoveAssignee = vi.mocked(removeAssignee)

const candidates = [
  { userId: 101, username: 'labeler1', displayName: '标注员一号' },
  { userId: 102, username: 'labeler2', displayName: '标注员二号' },
]

describe('AssigneePanel', () => {
  beforeEach(() => {
    mockListAssignees.mockReset()
    mockListCandidates.mockReset()
    mockAddAssignees.mockReset()
    mockRemoveAssignee.mockReset()
  })

  it('shows assignee names and lists only unassigned labelers in the picker', async () => {
    mockListAssignees.mockResolvedValue([{ userId: 101, assignedAt: null }])
    mockListCandidates.mockResolvedValue(candidates)

    render(<AssigneePanel taskId={5} />)

    // 已指派的用 displayName 展示,而不是裸 #id
    expect(await screen.findByText('标注员一号')).toBeInTheDocument()
    // 选择器只列未指派的标注员(102),不含已指派的 101
    const picker = screen.getByLabelText('assignee_candidate') as HTMLSelectElement
    const optionValues = [...picker.options].map((option) => option.value)
    expect(optionValues).toContain('102')
    expect(optionValues).not.toContain('101')
  })

  it('assigns the selected labeler by userId', async () => {
    mockListAssignees.mockResolvedValue([])
    mockListCandidates.mockResolvedValue(candidates)
    mockAddAssignees.mockResolvedValue({ added: 1 })

    const user = userEvent.setup()
    render(<AssigneePanel taskId={5} />)

    await screen.findByLabelText('assignee_candidate')
    await user.selectOptions(screen.getByLabelText('assignee_candidate'), '102')
    await user.click(screen.getByLabelText('添加指派'))

    await waitFor(() => expect(mockAddAssignees).toHaveBeenCalledWith(5, [102]))
  })

  it('removes an assignee', async () => {
    mockListAssignees.mockResolvedValue([{ userId: 101, assignedAt: null }])
    mockListCandidates.mockResolvedValue(candidates)
    mockRemoveAssignee.mockResolvedValue({ removed: 101 })

    const user = userEvent.setup()
    render(<AssigneePanel taskId={5} />)

    await user.click(await screen.findByLabelText('移除指派 101'))
    expect(mockRemoveAssignee).toHaveBeenCalledWith(5, 101)
  })
})
