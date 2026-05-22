import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type React from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ReviewerQueue from './Queue'
import { apiGet, apiPost } from '../../shared/api/client'

vi.mock('../../shared/api/client', () => ({
  apiGet: vi.fn(),
  apiPost: vi.fn(),
}))

vi.mock('@douyinfe/semi-ui', () => ({
  Button: ({ children, loading: _loading, theme: _theme, ...props }: React.ButtonHTMLAttributes<HTMLButtonElement> & { loading?: boolean, theme?: string }) => (
    <button type="button" {...props}>{children}</button>
  ),
  Toast: {
    error: vi.fn(),
    success: vi.fn(),
  },
}))

const mockApiGet = vi.mocked(apiGet)
const mockApiPost = vi.mocked(apiPost)

const submission = {
  id: 501,
  taskId: 1,
  itemId: 11,
  status: 'human_reviewing',
  currentRevisionId: 901,
}

const task = {
  id: 1,
  title: '历史模板任务',
  description: null,
  baselineDescription: null,
  status: 'published',
  totalItems: 1,
  finishedItems: 0,
}

const item = {
  id: 11,
  taskId: 1,
  externalId: 'Q0001',
  payload: JSON.stringify({ prompt: '历史题目' }),
  status: 'finished',
}

describe('ReviewerQueue schema runtime flow', () => {
  beforeEach(() => {
    mockApiGet.mockReset()
    mockApiPost.mockReset()
    mockApiPost.mockResolvedValue({ submission_id: 501, status: 'approved' })
  })

  it('opens submission detail and renders historical template as read-only', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/reviewer/submissions') {
        return [submission]
      }
      if (path === '/reviewer/submissions/501') {
        return {
          task,
          item,
          template: {
            id: 101,
            schemaJson: JSON.stringify({
              title: 'historical_v1',
              layout: 'single_page',
              fields: [{ name: 'summary', widget: 'Input', label: '历史字段' }],
            }),
          },
          submission,
          revision: { id: 901, answer: JSON.stringify({ summary: '旧答案' }), draft: false },
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })

    render(<ReviewerQueue />)

    await user.click(await screen.findByText('Submission #501'))

    const input = await screen.findByLabelText('历史字段')
    expect(input).toHaveValue('旧答案')
    expect(input).toBeDisabled()
  })

  it('shows schema error banner and disables review actions for bad schema', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/reviewer/submissions') {
        return [submission]
      }
      if (path === '/reviewer/submissions/501') {
        return {
          task,
          item,
          template: {
            id: 101,
            schemaJson: JSON.stringify({
              title: 'bad_schema',
              layout: 'single_page',
              fields: [{ name: 'score', widget: 'Radio' }],
            }),
          },
          submission,
          revision: { id: 901, answer: '{}', draft: false },
        }
      }
      throw new Error(`unexpected GET ${path}`)
    })

    render(<ReviewerQueue />)

    await user.click(await screen.findByText('Submission #501'))

    expect(await screen.findByRole('alert')).toHaveTextContent('fields[0].options')
    expect(screen.getByRole('button', { name: '打回修改' })).toBeDisabled()
    expect(screen.getByRole('button', { name: '拒绝' })).toBeDisabled()
    expect(screen.getByRole('button', { name: '通过' })).toBeDisabled()
  })
})
