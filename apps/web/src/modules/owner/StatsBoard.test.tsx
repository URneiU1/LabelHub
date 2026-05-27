import { render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import StatsBoard from './StatsBoard'
import { apiGet } from '../../shared/api/client'

vi.mock('../../shared/api/client', () => ({
  apiGet: vi.fn(),
}))
vi.mock('@douyinfe/semi-ui', () => ({
  Toast: { error: vi.fn(), success: vi.fn() },
}))
// jsdom 无 canvas: 把 VChart 换成暴露 spec 的 div,断言数据而非渲染像素。
vi.mock('@visactor/react-vchart', () => ({
  VChart: (props: { spec: unknown }) => <div data-testid="vchart" data-spec={JSON.stringify(props.spec)} />,
}))

const mockApiGet = vi.mocked(apiGet)

const stats = {
  progress: { total: 10, finished: 4 },
  statusBreakdown: { approved: 3, rejected: 1, human_reviewing: 2 },
  passRate: 0.75,
  aiVsHuman: { compared: 3, disagree: 1, rate: 0.3333 },
  dimensionAverages: [{ name: '相关性', avg: 9 }, { name: '完整性', avg: 6 }],
}

describe('StatsBoard', () => {
  beforeEach(() => mockApiGet.mockReset())

  it('renders four metric blocks and the pass-rate number', async () => {
    mockApiGet.mockResolvedValue(stats)
    render(<StatsBoard taskId={1} />)

    expect(await screen.findByLabelText('进度')).toBeInTheDocument()
    expect(screen.getByLabelText('通过率')).toBeInTheDocument()
    expect(screen.getByLabelText('AI vs 人工')).toBeInTheDocument()
    expect(screen.getByLabelText('维度均分')).toBeInTheDocument()
    expect(screen.getByText('75.0%')).toBeInTheDocument()
    expect(screen.getByText('4/10')).toBeInTheDocument()
  })

  it('passes dimension averages into the chart spec', async () => {
    mockApiGet.mockResolvedValue(stats)
    render(<StatsBoard taskId={1} />)

    await waitFor(() => expect(screen.getAllByTestId('vchart').length).toBeGreaterThan(0))
    const specs = screen.getAllByTestId('vchart').map((el) => JSON.parse(el.getAttribute('data-spec') || '{}'))
    const dimSpec = specs.find((s) => s.data?.[0]?.id === 'dim')
    expect(dimSpec).toBeTruthy()
    const values = dimSpec.data[0].values
    expect(values).toContainEqual({ name: '相关性', avg: 9 })
    expect(values).toContainEqual({ name: '完整性', avg: 6 })
  })
})
