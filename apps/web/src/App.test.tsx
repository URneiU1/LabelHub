import { render, screen } from '@testing-library/react'
import type React from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App'

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

vi.mock('./modules/auth/Login', () => ({
  default: () => <div>login page</div>,
}))

vi.mock('./modules/owner/Dashboard', () => ({
  default: () => <div>owner page</div>,
}))

vi.mock('./modules/labeler/Plaza', () => ({
  default: () => <div>labeler page</div>,
}))

vi.mock('./modules/reviewer/Queue', () => ({
  default: () => <div>reviewer page</div>,
}))

vi.mock('./modules/template/TemplateList', () => ({
  default: () => <div>template list page</div>,
}))

vi.mock('./modules/template/Designer', () => ({
  default: () => <div>template designer page</div>,
}))

vi.mock('./modules/styleguide/StyleGuide', () => ({
  default: () => <div>style guide page</div>,
}))

describe('App role routing', () => {
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
    window.history.pushState({}, '', '/')
  })

  it('redirects to login when the current token lacks the target role', async () => {
    localStorage.setItem('labelhub_access_token', 'owner-token')
    localStorage.setItem('labelhub_current_user', JSON.stringify({
      id: 1,
      username: 'owner1',
      displayName: '任务负责人一号',
      roles: ['owner'],
    }))
    window.history.pushState({}, '', '/labeler')

    render(<App />)

    expect(await screen.findByText('login page')).toBeInTheDocument()
    expect(screen.queryByText('labeler page')).not.toBeInTheDocument()
  })

  it('allows a user with the matching role to enter that role workspace', async () => {
    localStorage.setItem('labelhub_access_token', 'labeler-token')
    localStorage.setItem('labelhub_current_user', JSON.stringify({
      id: 2,
      username: 'labeler1',
      displayName: '标注员一号',
      roles: ['labeler'],
    }))
    window.history.pushState({}, '', '/labeler')

    render(<App />)

    expect(await screen.findByText('labeler page')).toBeInTheDocument()
    expect(screen.queryByText('login page')).not.toBeInTheDocument()
  })
})
