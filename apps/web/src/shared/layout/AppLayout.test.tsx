import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it } from 'vitest'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import AppLayout from './AppLayout'

describe('AppLayout navigation', () => {
  beforeEach(() => {
    const storage = new Map<string, string>()
    Object.defineProperty(globalThis, 'localStorage', {
      configurable: true,
      value: {
        clear: () => storage.clear(),
        getItem: (key: string) => storage.get(key) ?? null,
        removeItem: (key: string) => storage.delete(key),
        setItem: (key: string, value: string) => storage.set(key, value),
      },
    })
    localStorage.clear()
    localStorage.setItem('labelhub_access_token', 'owner-token')
    localStorage.setItem('labelhub_current_user', JSON.stringify({
      id: 1,
      username: 'owner1',
      displayName: '任务负责人一号',
      roles: ['owner'],
    }))
  })

  it('does not expose other roles navigation to owner users', () => {
    render(
      <MemoryRouter initialEntries={['/owner']}>
        <Routes>
          <Route element={<AppLayout />}>
            <Route path="/owner" element={<div>owner page</div>} />
          </Route>
        </Routes>
      </MemoryRouter>,
    )

    expect(screen.getAllByText('任务管理').length).toBeGreaterThan(0)
    expect(screen.queryByText('标注工作台')).not.toBeInTheDocument()
    expect(screen.queryByText('审核中心')).not.toBeInTheDocument()
  })

  it('collapses and expands the shared side navigation', async () => {
    const user = userEvent.setup()
    const { container } = render(
      <MemoryRouter initialEntries={['/owner']}>
        <Routes>
          <Route element={<AppLayout />}>
            <Route path="/owner" element={<div>owner page</div>} />
          </Route>
        </Routes>
      </MemoryRouter>,
    )

    const shell = container.querySelector('.lh-shell')
    expect(shell).not.toHaveClass('lh-shell--side-collapsed')

    await user.click(screen.getByRole('button', { name: '收起侧边导航' }))
    expect(shell).toHaveClass('lh-shell--side-collapsed')
    expect(screen.getByRole('button', { name: '展开侧边导航' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '展开侧边导航' }))
    expect(shell).not.toHaveClass('lh-shell--side-collapsed')
  })

  function renderAt(path: string) {
    return render(
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route element={<AppLayout />}>
            <Route path="/owner/:section" element={<div>section page</div>} />
            <Route path="/owner/:section/:sub" element={<div>sub page</div>} />
          </Route>
        </Routes>
      </MemoryRouter>,
    )
  }

  it('shows in-page sub-tabs only for sections that split into sub-pages (AI yes, tasks no)', () => {
    const { unmount } = renderAt('/owner/ai')
    // AI 分节有子页 → 顶部出现「全部」+ 子页标签
    expect(screen.getByText('全部')).toBeInTheDocument()
    expect(screen.getByText('Prompt 配置')).toBeInTheDocument()
    expect(screen.getByText('试跑历史')).toBeInTheDocument()
    unmount()

    // 任务管理是单一页面 → 不出现子标签条
    renderAt('/owner/tasks')
    expect(screen.queryByText('全部')).not.toBeInTheDocument()
    expect(screen.queryByText('Prompt 配置')).not.toBeInTheDocument()
  })

  it('shows only the two sub-tabs for 数据导出 (no 全部 overview tab)', () => {
    renderAt('/owner/export/config')
    expect(screen.getByText('导出配置')).toBeInTheDocument()
    expect(screen.getByText('导出历史')).toBeInTheDocument()
    expect(screen.queryByText('全部')).not.toBeInTheDocument()
  })
})
