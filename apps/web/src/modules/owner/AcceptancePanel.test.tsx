import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AcceptancePanel from './AcceptancePanel'
import { getAcceptance, startAcceptance, type AcceptanceBatch } from '../../shared/api/client'

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
})
