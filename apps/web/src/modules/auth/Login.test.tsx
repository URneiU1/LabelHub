import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type React from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import Login from './Login'
import { login } from '../../shared/api/client'

const mockNavigate = vi.hoisted(() => vi.fn())

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom')
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  }
})

vi.mock('../../shared/api/client', () => ({
  login: vi.fn(),
}))

vi.mock('@douyinfe/semi-ui', () => ({
  Button: ({ children, loading, theme, ...props }: React.ButtonHTMLAttributes<HTMLButtonElement> & { loading?: boolean, theme?: string }) => {
    void loading
    void theme
    return <button type="button" {...props}>{children}</button>
  },
  Toast: {
    error: vi.fn(),
  },
}))

const mockLogin = vi.mocked(login)

describe('Login demo roles', () => {
  beforeEach(() => {
    mockLogin.mockReset()
    mockNavigate.mockReset()
    mockLogin.mockResolvedValue({
      id: 1,
      username: 'owner1',
      displayName: '任务负责人一号',
      roles: ['owner'],
    })
  })

  it.each([
    ['任务负责人 · owner1', 'owner1', '/owner'],
    ['标注员 · labeler1', 'labeler1', '/labeler'],
    ['人工审核员 · reviewer1', 'reviewer1', '/reviewer'],
  ])('logs in %s with the shared demo password', async (buttonName, username, targetPath) => {
    const user = userEvent.setup()

    render(<Login />)

    await user.click(screen.getByRole('button', { name: buttonName }))

    await waitFor(() => {
      expect(mockLogin).toHaveBeenCalledWith(username, '123456')
      expect(mockNavigate).toHaveBeenCalledWith(targetPath)
    })
  })

  it('logs in with a typed username and password and routes by role', async () => {
    const user = userEvent.setup()
    mockLogin.mockResolvedValue({ id: 9, username: 'reviewer1', displayName: '审核员', roles: ['reviewer'] })

    render(<Login />)

    await user.type(screen.getByLabelText('用户名'), 'reviewer1')
    await user.type(screen.getByLabelText('密码'), 'secret-pass')
    await user.click(screen.getByRole('button', { name: '登录' }))

    await waitFor(() => {
      expect(mockLogin).toHaveBeenCalledWith('reviewer1', 'secret-pass')
      expect(mockNavigate).toHaveBeenCalledWith('/reviewer')
    })
  })
})
