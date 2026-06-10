import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { Modal } from '@douyinfe/semi-ui'
import AcceptancePanel from './AcceptancePanel'
import { acceptAcceptanceBatch, getAcceptance, recordAcceptanceSpotCheck, rejectAcceptanceBatch, startAcceptance, type AcceptanceBatch } from '../../shared/api/client'

vi.mock('../../shared/api/client', async () => {
  const actual = await vi.importActual<typeof import('../../shared/api/client')>('../../shared/api/client')
  return {
    ...actual,
    getAcceptance: vi.fn(),
    startAcceptance: vi.fn(),
    recordAcceptanceSpotCheck: vi.fn(),
    acceptAcceptanceBatch: vi.fn(),
    rejectAcceptanceBatch: vi.fn(),
  }
})

vi.mock('@douyinfe/semi-ui', () => ({
  Toast: { error: vi.fn(), success: vi.fn(), warning: vi.fn() },
  Modal: { confirm: vi.fn() },
}))

const mockGet = vi.mocked(getAcceptance)
const mockStart = vi.mocked(startAcceptance)
const mockAccept = vi.mocked(acceptAcceptanceBatch)
const mockReject = vi.mocked(rejectAcceptanceBatch)
const mockSpotCheck = vi.mocked(recordAcceptanceSpotCheck)
const mockConfirm = vi.mocked(Modal.confirm)

const pendingBatch: AcceptanceBatch = {
  id: 42,
  taskId: 1,
  status: 'pending',
  approvedCount: 3,
  note: null,
  decidedBy: null,
  decidedAt: null,
  createdAt: '2026-06-07T00:00:00Z',
}

describe('AcceptancePanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows start action and approved count when there is no batch', async () => {
    mockGet.mockResolvedValue({ batch: null, spotChecks: [], approvedCount: 3 })
    render(<AcceptancePanel taskId={1} />)
    await waitFor(() => expect(screen.getByText('发起验收')).toBeInTheDocument())
    expect(screen.getByText('3')).toBeInTheDocument()
  })

  it('starts an acceptance batch when the start button is clicked', async () => {
    mockGet.mockResolvedValueOnce({ batch: null, spotChecks: [], approvedCount: 3 })
    mockStart.mockResolvedValue({ batch: pendingBatch })
    mockGet.mockResolvedValueOnce({ batch: pendingBatch, spotChecks: [], approvedCount: 3 })

    render(<AcceptancePanel taskId={1} />)
    const startBtn = await screen.findByText('发起验收')
    await userEvent.click(startBtn)

    await waitFor(() => expect(mockStart).toHaveBeenCalledWith(1))
  })

  it('shows accept/reject controls for a pending batch', async () => {
    mockGet.mockResolvedValue({ batch: pendingBatch, spotChecks: [], approvedCount: 3 })
    render(<AcceptancePanel taskId={1} />)
    await waitFor(() => expect(screen.getByText('验收通过')).toBeInTheDocument())
    expect(screen.getByText('验收不通过(打回不合格项)')).toBeInTheDocument()
    expect(screen.getByText('验收中')).toBeInTheDocument()
  })

  it('lists approved submissions with answers and spot-checks one inline', async () => {
    mockGet.mockResolvedValue({
      batch: pendingBatch,
      spotChecks: [],
      approvedCount: 1,
      approvedSubmissions: [
        { id: 7, itemId: 11, labelerId: 3, aiVerdict: 'pass', aiScore: 88, answer: '{"relevance_score":5,"summary":"合格"}' },
      ],
    })
    mockSpotCheck.mockResolvedValue({ spotCheck: {} } as never)
    render(<AcceptancePanel taskId={1} />)

    expect(await screen.findByText('提交 #7')).toBeInTheDocument()
    // 答案默认折叠,点「查看答案」才展开,避免列表过长。
    await userEvent.click(screen.getByRole('button', { name: '查看答案' }))
    expect(screen.getByText(/相关性/)).toBeInTheDocument()

    // 列表行的「合格」就地抽检,带上该提交 ID,不必手输。
    await userEvent.click(screen.getAllByRole('button', { name: '合格' })[0])
    await waitFor(() =>
      expect(mockSpotCheck).toHaveBeenCalledWith(1, { batch_id: 42, submission_id: 7, result: 'ok', note: '' }),
    )
  })

  it('accepts a pending batch with the entered note', async () => {
    mockGet.mockResolvedValue({ batch: pendingBatch, spotChecks: [], approvedCount: 3 })
    mockAccept.mockResolvedValue(undefined as never)
    render(<AcceptancePanel taskId={1} />)
    await userEvent.type(await screen.findByLabelText('验收备注'), '看过了')
    await userEvent.click(screen.getByText('验收通过'))
    await waitFor(() => expect(mockAccept).toHaveBeenCalledWith(1, { batch_id: 42, note: '看过了' }))
  })

  it('rejects a pending batch only after confirmation, sending the batch id', async () => {
    mockGet.mockResolvedValue({ batch: pendingBatch, spotChecks: [], approvedCount: 3 })
    mockReject.mockResolvedValue({ reopenedCount: 2 } as never)
    render(<AcceptancePanel taskId={1} />)
    await userEvent.click(await screen.findByText('验收不通过(打回不合格项)'))
    // Reject is gated behind a confirm dialog — clicking alone must not fire the destructive call.
    expect(mockReject).not.toHaveBeenCalled()
    const confirmArg = mockConfirm.mock.calls[0][0] as { onOk: () => Promise<void> | void }
    await confirmArg.onOk()
    await waitFor(() => expect(mockReject).toHaveBeenCalledWith(1, { batch_id: 42, note: '' }))
  })

  it('records a spot check with the submission id and result', async () => {
    mockGet.mockResolvedValue({ batch: pendingBatch, spotChecks: [], approvedCount: 3 })
    mockSpotCheck.mockResolvedValue(undefined as never)
    render(<AcceptancePanel taskId={1} />)
    await userEvent.type(await screen.findByLabelText('提交 ID'), '77')
    await userEvent.click(screen.getByText('不合格'))
    await waitFor(() => expect(mockSpotCheck).toHaveBeenCalledWith(1, { batch_id: 42, submission_id: 77, result: 'flag', note: '' }))
  })
})
