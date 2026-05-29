import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Toast } from '@douyinfe/semi-ui'
import ExportPanel from './ExportPanel'
import { apiGet, apiPostRawJSON } from '../../shared/api/client'

vi.mock('../../shared/api/client', () => ({
  apiGet: vi.fn(),
  apiPostRawJSON: vi.fn(),
}))

vi.mock('@douyinfe/semi-ui', () => ({
  Toast: { error: vi.fn(), success: vi.fn() },
}))

const mockApiGet = vi.mocked(apiGet)
const mockApiPostRawJSON = vi.mocked(apiPostRawJSON)

const succeededRecord = { id: 5, format: 'csv', status: 'succeeded', rowCount: 12, errorMsg: null, createdAt: '2026-05-27T12:00:00Z' }

describe('ExportPanel', () => {
  beforeEach(() => {
    mockApiGet.mockReset()
    mockApiPostRawJSON.mockReset()
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('renders format options and export history', async () => {
    mockApiGet.mockResolvedValue({ exports: [succeededRecord] })
    render(<ExportPanel taskId={1} />)

    expect(screen.getByRole('button', { name: '格式 csv' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '格式 xlsx' })).toBeInTheDocument()
    expect(await screen.findByText('#5')).toBeInTheDocument()
    expect(screen.getByText('已完成')).toBeInTheDocument()
  })

  it('posts an export with the selected format and field map', async () => {
    const user = userEvent.setup()
    mockApiGet.mockResolvedValue({ exports: [] })
    mockApiPostRawJSON.mockResolvedValue({ id: 9, status: 'queued' })
    render(<ExportPanel taskId={1} />)

    await user.click(screen.getByRole('button', { name: '格式 json' }))
    await user.click(screen.getByLabelText('含审核记录'))
    await user.click(screen.getByRole('button', { name: '开始导出' }))

    await waitFor(() => {
      expect(mockApiPostRawJSON).toHaveBeenCalledTimes(1)
    })
    const [path, body] = mockApiPostRawJSON.mock.calls[0]
    expect(path).toBe('/tasks/1/exports')
    expect(body).toContain('"format":"json"')
    expect(body).toContain('"include_reviews":true')
  })

  it('opens the signed url on download', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path: string) => {
      if (path === '/tasks/1/exports') return { exports: [succeededRecord] }
      if (path === '/tasks/1/exports/5/download-url') return { url: '/api/v1/exports/download?token=abc', expiresIn: 600 }
      throw new Error(`unexpected GET ${path}`)
    })
    const openSpy = vi.spyOn(window, 'open').mockImplementation(() => null)
    render(<ExportPanel taskId={1} />)

    await user.click(await screen.findByRole('button', { name: '下载导出 #5' }))
    await waitFor(() => {
      expect(openSpy).toHaveBeenCalledWith('/api/v1/exports/download?token=abc', '_blank', 'noopener,noreferrer')
    })
  })

  it('rejects an unsafe download url and does not open a window', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path: string) => {
      if (path === '/tasks/1/exports') return { exports: [succeededRecord] }
      if (path === '/tasks/1/exports/5/download-url') return { url: 'javascript:alert(1)', expiresIn: 600 }
      throw new Error(`unexpected GET ${path}`)
    })
    const openSpy = vi.spyOn(window, 'open').mockImplementation(() => null)
    openSpy.mockClear() // 同一 spy 实例可能带着上一个测试的调用记录,清空后再断言本测试不应 open
    render(<ExportPanel taskId={1} />)

    await user.click(await screen.findByRole('button', { name: '下载导出 #5' }))
    await waitFor(() => {
      expect(Toast.error).toHaveBeenCalledWith('下载链接无效')
    })
    expect(openSpy).not.toHaveBeenCalled()
  })

  it('polls while there are queued or running rows', async () => {
    vi.useFakeTimers()
    let call = 0
    mockApiGet.mockImplementation(async () => {
      call += 1
      if (call === 1) return { exports: [{ ...succeededRecord, id: 7, status: 'queued', rowCount: null }] }
      return { exports: [{ ...succeededRecord, id: 7, status: 'succeeded', rowCount: 3 }] }
    })
    render(<ExportPanel taskId={1} />)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0)
    })
    expect(screen.getByText('排队中')).toBeInTheDocument()

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000)
    })
    expect(screen.getByText('已完成')).toBeInTheDocument()

    // 全部 succeeded 后轮询必须停:再推进 2s 不应再发请求。
    const callsAfterSettle = mockApiGet.mock.calls.length
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000)
    })
    expect(mockApiGet.mock.calls.length).toBe(callsAfterSettle)
  })

  it('ignores stale history after switching task', async () => {
    let resolveFirst: (v: unknown) => void = () => {}
    mockApiGet.mockImplementation((path: string) => {
      if (path === '/tasks/1/exports') {
        return new Promise((resolve) => { resolveFirst = resolve })
      }
      return Promise.resolve({ exports: [{ ...succeededRecord, id: 22, format: 'json' }] })
    })

    const { rerender } = render(<ExportPanel taskId={1} />)
    rerender(<ExportPanel taskId={2} />)
    await screen.findByText('#22') // task 2 loaded

    await act(async () => {
      resolveFirst({ exports: [{ ...succeededRecord, id: 11, format: 'csv' }] })
    })
    expect(screen.queryByText('#11')).not.toBeInTheDocument()
  })

  it('does not toast a stale error after switching task', async () => {
    let rejectFirst: (e: unknown) => void = () => {}
    mockApiGet.mockImplementation((path: string) => {
      if (path === '/tasks/1/exports') {
        return new Promise((_, reject) => { rejectFirst = reject })
      }
      return Promise.resolve({ exports: [{ ...succeededRecord, id: 22, format: 'json' }] })
    })

    const { rerender } = render(<ExportPanel taskId={1} />)
    rerender(<ExportPanel taskId={2} />)
    await screen.findByText('#22') // task 2 loaded
    vi.mocked(Toast.error).mockClear()

    await act(async () => {
      rejectFirst(new Error('stale boom'))
    })
    // task1 的失败晚到,不该弹到 task2 上下文。
    expect(vi.mocked(Toast.error)).not.toHaveBeenCalled()
  })
})
