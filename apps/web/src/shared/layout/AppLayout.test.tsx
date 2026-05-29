import { render, screen } from '@testing-library/react'
import type React from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import AppLayout from './AppLayout'

vi.mock('@douyinfe/semi-ui', () => ({
  Layout: Object.assign(
    ({ children, ...props }: React.HTMLAttributes<HTMLDivElement>) => <div {...props}>{children}</div>,
    {
      Header: ({ children, ...props }: React.HTMLAttributes<HTMLElement>) => <header {...props}>{children}</header>,
      Sider: ({ children, ...props }: React.HTMLAttributes<HTMLElement>) => <aside {...props}>{children}</aside>,
      Content: ({ children, ...props }: React.HTMLAttributes<HTMLElement>) => <main {...props}>{children}</main>,
    },
  ),
  Nav: ({ children, ...props }: React.HTMLAttributes<HTMLElement>) => <nav {...props}>{children}</nav>,
  Typography: {
    Text: ({ children, strong, ...props }: React.HTMLAttributes<HTMLSpanElement> & { strong?: boolean }) => {
      void strong
      return <span {...props}>{children}</span>
    },
  },
}))

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

  it('does not expose reviewer navigation to owner users', () => {
    render(
      <MemoryRouter initialEntries={['/owner']}>
        <Routes>
          <Route element={<AppLayout />}>
            <Route path="/owner" element={<div>owner page</div>} />
          </Route>
        </Routes>
      </MemoryRouter>,
    )

    expect(screen.getByText('任务负责人')).toBeInTheDocument()
    expect(screen.queryByText('标注工作台')).not.toBeInTheDocument()
    expect(screen.queryByText('审核中心')).not.toBeInTheDocument()
  })
})
