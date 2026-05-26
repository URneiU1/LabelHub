import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type React from 'react'
import { MemoryRouter, Route, Routes, useNavigate } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TemplateDesigner from './Designer'
import { apiGet, apiPost } from '../../shared/api/client'

vi.mock('../../shared/api/client', () => ({
  apiGet: vi.fn(),
  apiPost: vi.fn(),
}))

vi.mock('@douyinfe/semi-ui', () => ({
  Button: ({ children, loading, theme, ...props }: React.ButtonHTMLAttributes<HTMLButtonElement> & { loading?: boolean, theme?: string }) => {
    void loading
    void theme
    return <button type="button" {...props}>{children}</button>
  },
  Toast: {
    error: vi.fn(),
    success: vi.fn(),
  },
}))

const mockApiGet = vi.mocked(apiGet)
const mockApiPost = vi.mocked(apiPost)

const baseSchema = {
  title: 'QA template',
  layout: 'single_page',
  fields: [
    { name: 'summary', widget: 'Input', label: 'Summary', required: true },
  ],
}

describe('TemplateDesigner', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    mockApiGet.mockReset()
    mockApiPost.mockReset()
  })

  it('appends, edits, deletes, and saves a new clean template version', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/templates/10') {
        return templateDetail(10, baseSchema, true)
      }
      if (path === '/templates/11') {
        return templateDetail(11, {
          ...baseSchema,
          fields: [
            baseSchema.fields[0],
            { name: 'fluency_score', widget: 'Radio', label: '流畅度', required: false, options: ['1', '2', '3'] },
          ],
        }, true)
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost.mockResolvedValue({
      id: 11,
      taskId: 1,
      version: 2,
      schemaJson: '',
    })

    renderDesigner('/owner/tasks/1/templates/10')

    await screen.findByText('Template Designer')
    await user.click(screen.getByRole('button', { name: 'Add Radio' }))
    await user.click(screen.getByRole('button', { name: 'Add Radio' }))
    expect(screen.getByRole('button', { name: /select radio_1/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /select radio_2/ })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /select radio_1/ }))
    await user.clear(screen.getByLabelText('field_name'))
    await user.type(screen.getByLabelText('field_name'), 'fluency_score')
    await user.clear(screen.getByLabelText('field_label'))
    await user.type(screen.getByLabelText('field_label'), '流畅度')
    fireEvent.change(screen.getByLabelText('field_options'), { target: { value: '1\n2\n3' } })
    await user.click(screen.getByRole('button', { name: 'delete radio_2' }))
    await user.click(screen.getByRole('button', { name: 'Save as new version' }))

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith('/tasks/1/templates', expect.objectContaining({
        title: 'QA template',
        layout: 'single_page',
      }))
    })
    const [, body] = mockApiPost.mock.calls[0]
    expect(JSON.stringify(body)).not.toContain('_draftId')
    expect(body).toMatchObject({
      fields: [
        { name: 'summary', widget: 'Input', label: 'Summary', required: true },
        { name: 'fluency_score', widget: 'Radio', label: '流畅度', required: false, options: ['1', '2', '3'] },
      ],
      export_fields: ['summary', 'fluency_score'],
    })
  })

  it('keeps historical templates readonly but allows fork as a new version', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/templates/9') {
        return templateDetail(9, baseSchema, false)
      }
      if (path === '/templates/12') {
        return templateDetail(12, baseSchema, true)
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost.mockResolvedValue({
      id: 12,
      taskId: 1,
      version: 3,
      schemaJson: '',
    })

    renderDesigner('/owner/tasks/1/templates/9')

    await screen.findByText('Template Designer')
    expect(screen.getByRole('button', { name: 'Add Radio' })).toBeDisabled()
    expect(screen.getByLabelText('field_name')).toBeDisabled()

    await user.click(screen.getByRole('button', { name: 'Fork as new version' }))

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith('/tasks/1/templates', expect.objectContaining({
        fields: [{ name: 'summary', widget: 'Input', label: 'Summary', required: true }],
      }))
    })
  })

  it('fails closed when route task and loaded template task do not match', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/templates/20') {
        return templateDetail(20, baseSchema, true, 2)
      }
      throw new Error(`unexpected GET ${path}`)
    })

    renderDesigner('/owner/tasks/1/templates/20')

    expect(await screen.findByRole('alert')).toHaveTextContent('模板不属于当前 URL 中的 task')
    expect(screen.getByRole('button', { name: 'Add Radio' })).toBeDisabled()
    expect(screen.queryByRole('button', { name: 'Save as new version' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Fork as new version' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Add Radio' }))

    expect(mockApiPost).not.toHaveBeenCalled()
  })

  it('ignores stale template load responses after a route switch', async () => {
    const user = userEvent.setup()
    const staleTemplateLoad = deferred<ReturnType<typeof templateDetail>>()
    const taskASchema = {
      ...baseSchema,
      fields: [
        { name: 'task_a_summary', widget: 'Input', label: 'Task A Summary', required: true },
      ],
    }
    const taskBSchema = {
      ...baseSchema,
      fields: [
        { name: 'task_b_summary', widget: 'Input', label: 'Task B Summary', required: true },
      ],
    }
    mockApiGet.mockImplementation((path) => {
      if (path === '/templates/10') {
        return staleTemplateLoad.promise
      }
      if (path === '/templates/20') {
        return Promise.resolve(templateDetail(20, taskBSchema, true, 2))
      }
      if (path === '/templates/21') {
        return Promise.resolve(templateDetail(21, taskBSchema, true, 2))
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost.mockResolvedValue({
      id: 21,
      taskId: 2,
      version: 3,
      schemaJson: '',
    })

    renderDesignerWithRouteSwitch('/owner/tasks/1/templates/10', '/owner/tasks/2/templates/20')

    await user.click(screen.getByRole('button', { name: 'Switch route' }))

    expect(await screen.findByRole('button', { name: /select task_b_summary/ })).toBeInTheDocument()

    await act(async () => {
      staleTemplateLoad.resolve(templateDetail(10, taskASchema, true, 1))
      await staleTemplateLoad.promise
    })

    expect(screen.getByRole('button', { name: /select task_b_summary/ })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /select task_a_summary/ })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Save as new version' }))

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith('/tasks/2/templates', expect.objectContaining({
        fields: [
          { name: 'task_b_summary', widget: 'Input', label: 'Task B Summary', required: true },
        ],
      }))
    })
    const [, body] = mockApiPost.mock.calls[0]
    expect(JSON.stringify(body)).not.toContain('task_a_summary')
  })

  it('clears stale editable state when route template id is invalid', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/templates/10') {
        return templateDetail(10, baseSchema, true)
      }
      throw new Error(`unexpected GET ${path}`)
    })

    renderDesignerWithRouteSwitch('/owner/tasks/1/templates/10', '/owner/tasks/2/templates/bad-id')

    expect(await screen.findByRole('button', { name: /select summary/ })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Switch route' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('template id 无效')
    expect(screen.getByRole('button', { name: 'Add Radio' })).toBeDisabled()
    expect(screen.queryByRole('button', { name: 'Save as new version' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Fork as new version' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /select summary/ })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Add Radio' }))

    expect(mockApiPost).not.toHaveBeenCalled()
  })

  it('clears stale editable state when loading a valid route template fails', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/templates/10') {
        return templateDetail(10, baseSchema, true)
      }
      if (path === '/templates/999') {
        throw new Error('template not found')
      }
      throw new Error(`unexpected GET ${path}`)
    })

    renderDesignerWithRouteSwitch('/owner/tasks/1/templates/10', '/owner/tasks/2/templates/999')

    expect(await screen.findByRole('button', { name: /select summary/ })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Switch route' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('template not found')
    expect(screen.getByRole('button', { name: 'Add Radio' })).toBeDisabled()
    expect(screen.queryByRole('button', { name: 'Save as new version' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Fork as new version' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /select summary/ })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Add Radio' }))

    expect(mockApiPost).not.toHaveBeenCalled()
  })

  it('does not navigate when save resolves after the route changes', async () => {
    const user = userEvent.setup()
    const pendingSave = deferred<{ id: number, taskId: number, version: number, schemaJson: string }>()
    const taskBSchema = {
      ...baseSchema,
      fields: [
        { name: 'task_b_summary', widget: 'Input', label: 'Task B Summary', required: true },
      ],
    }
    mockApiGet.mockImplementation((path) => {
      if (path === '/templates/10') {
        return Promise.resolve(templateDetail(10, baseSchema, true))
      }
      if (path === '/templates/20') {
        return Promise.resolve(templateDetail(20, taskBSchema, true, 2))
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost.mockReturnValue(pendingSave.promise)

    renderDesignerWithRouteSwitch('/owner/tasks/1/templates/10', '/owner/tasks/2/templates/20')

    await screen.findByRole('button', { name: /select summary/ })
    await user.click(screen.getByRole('button', { name: 'Save as new version' }))

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith('/tasks/1/templates', expect.any(Object))
    })

    await user.click(screen.getByRole('button', { name: 'Switch route' }))
    expect(await screen.findByRole('button', { name: /select task_b_summary/ })).toBeInTheDocument()

    await act(async () => {
      pendingSave.resolve({
        id: 11,
        taskId: 1,
        version: 2,
        schemaJson: '',
      })
      await pendingSave.promise
    })

    expect(screen.getByRole('button', { name: /select task_b_summary/ })).toBeInTheDocument()
    expect(mockApiGet).not.toHaveBeenCalledWith('/templates/11')
  })

  it('fails closed for historical templates whose task does not match the route', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/templates/30') {
        return templateDetail(30, baseSchema, false, 2)
      }
      throw new Error(`unexpected GET ${path}`)
    })

    renderDesigner('/owner/tasks/1/templates/30')

    expect(await screen.findByRole('alert')).toHaveTextContent('模板不属于当前 URL 中的 task')
    expect(screen.getByRole('button', { name: 'Add Radio' })).toBeDisabled()
    expect(screen.queryByRole('button', { name: 'Save as new version' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Fork as new version' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /select summary/ })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Add Radio' }))

    expect(mockApiPost).not.toHaveBeenCalled()
  })
})

function renderDesigner(initialEntry: string) {
  render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route path="/owner/tasks/:taskId/templates/:templateId" element={<TemplateDesigner />} />
      </Routes>
    </MemoryRouter>,
  )
}

function renderDesignerWithRouteSwitch(initialEntry: string, switchTo: string) {
  render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route path="/owner/tasks/:taskId/templates/:templateId" element={(
          <>
            <RouteSwitchControl to={switchTo} />
            <TemplateDesigner />
          </>
        )}
        />
      </Routes>
    </MemoryRouter>,
  )
}

function RouteSwitchControl({ to }: { to: string }) {
  const navigate = useNavigate()
  return <button type="button" onClick={() => navigate(to)}>Switch route</button>
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: unknown) => void
  const promise = new Promise<T>((innerResolve, innerReject) => {
    resolve = innerResolve
    reject = innerReject
  })
  return { promise, resolve, reject }
}

function templateDetail(id: number, schema: unknown, isLatest: boolean, templateTaskId = 1) {
  return {
    template: {
      id,
      taskId: templateTaskId,
      version: id === 9 ? 1 : 2,
      schemaJson: JSON.stringify(schema),
    },
    isLatest,
    latestTemplateId: isLatest ? id : 10,
  }
}
