import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type React from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import LabelerPlaza from './Plaza'
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

const task = {
  id: 1,
  title: 'QA 质量标注',
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
  payload: JSON.stringify({ prompt: '光合作用发生在哪里？' }),
  status: 'claimed',
}

describe('LabelerPlaza schema runtime flow', () => {
  beforeEach(() => {
    mockApiGet.mockReset()
    mockApiPost.mockReset()
    mockApiGet.mockResolvedValue([task])
  })

  it('claims a task bundle, renders schema fields, and submits collected answer', async () => {
    const user = userEvent.setup()
    const schema = {
      title: 'qa_runtime',
      layout: 'single_page',
      fields: [
        { name: 'summary', widget: 'Input', label: '一句话总评', required: true },
      ],
    }
    mockApiPost.mockImplementation(async (path) => {
      if (path === '/tasks/1/claim') {
        return {
          task,
          item,
          template: { id: 101, schemaJson: JSON.stringify(schema) },
          submission: { id: 42, taskId: 1, itemId: 11, status: 'draft' },
          revision: null,
        }
      }
      if (path === '/tasks/1/items/11/submit') {
        return { id: 42, taskId: 1, itemId: 11, status: 'human_reviewing' }
      }
      throw new Error(`unexpected POST ${path}`)
    })

    render(<LabelerPlaza />)

    expect(await screen.findByText('QA 质量标注')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '领取题目' }))
    await user.type(await screen.findByLabelText('一句话总评'), '回答准确')
    await user.click(screen.getByRole('button', { name: '提交审核' }))

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith('/tasks/1/items/11/submit', { answer: { summary: '回答准确' } })
    })
  })

  it('shows schema error banner and disables actions for bad schema', async () => {
    const user = userEvent.setup()
    mockApiPost.mockResolvedValue({
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
      submission: { id: 42, taskId: 1, itemId: 11, status: 'draft' },
      revision: null,
    })

    render(<LabelerPlaza />)

    await user.click(await screen.findByRole('button', { name: '领取题目' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('fields[0].options')
    expect(screen.getByRole('button', { name: '保存草稿' })).toBeDisabled()
    expect(screen.getByRole('button', { name: '提交审核' })).toBeDisabled()
  })
})
