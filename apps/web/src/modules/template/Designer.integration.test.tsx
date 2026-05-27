import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type React from 'react'
import { MemoryRouter, Route, Routes, useNavigate } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TemplateDesigner from './Designer'
import { apiGet, apiPost } from '../../shared/api/client'
import SchemaRenderer from '../../renderer/SchemaRenderer'
import { parseTemplateSchema } from '../../renderer/parser'

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

const showItemSchema = {
  title: 'Preview template',
  layout: 'single_page',
  fields: [
    { name: 'source_display', widget: 'ShowItem', label: '原始数据', path: '$payload', mode: 'auto' },
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

  it('copies and reorders fields before saving a new version', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/templates/10') {
        return templateDetail(10, baseSchema, true)
      }
      if (path === '/templates/12') {
        return templateDetail(12, {
          ...baseSchema,
          fields: [
            baseSchema.fields[0],
            { name: 'radio_1', widget: 'Radio', label: '单选', required: false, options: ['pass', 'reject', 'uncertain'] },
            { name: 'summary_copy', widget: 'Input', label: 'Summary', required: true },
          ],
        }, true)
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost.mockResolvedValue({
      id: 12,
      taskId: 1,
      version: 3,
      schemaJson: '',
    })

    renderDesigner('/owner/tasks/1/templates/10')

    await screen.findByRole('button', { name: /select summary/ })
    await user.click(screen.getByRole('button', { name: 'copy summary' }))
    expect(screen.getByRole('button', { name: /select summary_copy/ })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Add Radio' }))
    await user.click(screen.getByRole('button', { name: 'move up radio_1' }))
    await user.click(screen.getByRole('button', { name: 'Save as new version' }))

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith('/tasks/1/templates', expect.objectContaining({
        export_fields: ['summary', 'radio_1', 'summary_copy'],
      }))
    })
    const [, body] = mockApiPost.mock.calls[0]
    expect(body).toMatchObject({
      fields: [
        { name: 'summary', widget: 'Input', label: 'Summary', required: true },
        { name: 'radio_1', widget: 'Radio', label: '单选', required: false, options: ['pass', 'reject', 'uncertain'] },
        { name: 'summary_copy', widget: 'Input', label: 'Summary', required: true },
      ],
    })
    expect(JSON.stringify(body)).not.toContain('_draftId')
  })

  it('reorders fields with drag and drop before saving a new version', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/templates/10') {
        return templateDetail(10, baseSchema, true)
      }
      if (path === '/templates/13') {
        return templateDetail(13, {
          ...baseSchema,
          fields: [
            { name: 'tags_1', widget: 'Tags', label: '标签多选', required: false, options: ['pass', 'reject', 'uncertain'] },
            baseSchema.fields[0],
            { name: 'radio_1', widget: 'Radio', label: '单选', required: false, options: ['pass', 'reject', 'uncertain'] },
          ],
        }, true)
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost.mockResolvedValue({
      id: 13,
      taskId: 1,
      version: 4,
      schemaJson: '',
    })

    renderDesigner('/owner/tasks/1/templates/10')

    await screen.findByRole('button', { name: /select summary/ })
    await user.click(screen.getByRole('button', { name: 'Add Radio' }))
    await user.click(screen.getByRole('button', { name: 'Add Tags' }))
    expect(screen.getByLabelText('field_name')).toHaveValue('tags_1')

    const dataTransfer = dragDataTransfer()
    fireEvent.dragStart(screen.getByRole('button', { name: 'drag tags_1' }), { dataTransfer })
    fireEvent.dragOver(screen.getByLabelText('canvas field summary'), { dataTransfer })
    fireEvent.drop(screen.getByLabelText('canvas field summary'), { dataTransfer })

    expect(screen.getByLabelText('field_name')).toHaveValue('tags_1')

    await user.click(screen.getByRole('button', { name: 'Save as new version' }))

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith('/tasks/1/templates', expect.objectContaining({
        export_fields: ['tags_1', 'summary', 'radio_1'],
      }))
    })
    const [, body] = mockApiPost.mock.calls[0]
    expect(body).toMatchObject({
      fields: [
        { name: 'tags_1', widget: 'Tags', label: '标签多选', required: false, options: ['pass', 'reject', 'uncertain'] },
        { name: 'summary', widget: 'Input', label: 'Summary', required: true },
        { name: 'radio_1', widget: 'Radio', label: '单选', required: false, options: ['pass', 'reject', 'uncertain'] },
      ],
    })
    expect(JSON.stringify(body)).not.toContain('_draftId')
  })

  it('creates group and tabs fields with nested export fields', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/templates/10') {
        return templateDetail(10, baseSchema, true)
      }
      if (path === '/templates/14') {
        return templateDetail(14, {
          ...baseSchema,
          fields: [
            baseSchema.fields[0],
            {
              name: 'group_1',
              widget: 'Group',
              label: '字段组',
              required: false,
              fields: [{ name: 'group_1_input', widget: 'Input', label: '单行输入', required: false }],
            },
            {
              name: 'tabs_1',
              widget: 'Tabs',
              label: '分页组',
              required: false,
              tabs: [
                { label: 'Tab 1', fields: [{ name: 'tabs_1_tab1_input', widget: 'Input', label: '单行输入', required: false }] },
                { label: 'Tab 2', fields: [{ name: 'tabs_1_tab2_text', widget: 'TextArea', label: '多行文本', required: false }] },
              ],
            },
          ],
        }, true)
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost.mockResolvedValue({
      id: 14,
      taskId: 1,
      version: 5,
      schemaJson: '',
    })

    renderDesigner('/owner/tasks/1/templates/10')

    await screen.findByRole('button', { name: /select summary/ })
    await user.click(screen.getByRole('button', { name: 'Add Group' }))
    await user.clear(screen.getByLabelText('group_fields_child_name_1'))
    await user.type(screen.getByLabelText('group_fields_child_name_1'), 'group_summary')
    await user.clear(screen.getByLabelText('group_fields_child_label_1'))
    await user.type(screen.getByLabelText('group_fields_child_label_1'), '组内摘要')
    await user.click(screen.getByRole('button', { name: 'add group_fields Radio' }))
    await user.clear(screen.getByLabelText('group_fields_child_name_2'))
    await user.type(screen.getByLabelText('group_fields_child_name_2'), 'group_decision')
    await user.click(screen.getByRole('button', { name: 'Add Tabs' }))
    await user.click(screen.getByRole('button', { name: /select tabs_1/ }))
    await user.clear(screen.getByLabelText('tab_label_1'))
    await user.type(screen.getByLabelText('tab_label_1'), '基础')
    await user.click(screen.getByRole('button', { name: 'add tab_1_fields Radio' }))
    await user.clear(screen.getByLabelText('tab_1_fields_child_name_2'))
    await user.type(screen.getByLabelText('tab_1_fields_child_name_2'), 'tabs_1_decision')
    expect(screen.getByRole('button', { name: /select group_1/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /select tabs_1/ })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Save as new version' }))

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith('/tasks/1/templates', expect.objectContaining({
        export_fields: ['summary', 'group_summary', 'group_decision', 'tabs_1_tab1_input', 'tabs_1_decision', 'tabs_1_tab2_text'],
      }))
    })
    const [, body] = mockApiPost.mock.calls[0]
    expect(body).toMatchObject({
      fields: [
        { name: 'summary', widget: 'Input', label: 'Summary', required: true },
        {
          name: 'group_1',
          widget: 'Group',
          fields: [
            { name: 'group_summary', widget: 'Input', label: '组内摘要', required: false },
            { name: 'group_decision', widget: 'Radio', label: '单选', required: false, options: ['pass', 'reject', 'uncertain'] },
          ],
        },
        {
          name: 'tabs_1',
          widget: 'Tabs',
          tabs: [
            { label: '基础', fields: [
              { name: 'tabs_1_tab1_input', widget: 'Input', label: '单行输入', required: false },
              { name: 'tabs_1_decision', widget: 'Radio', label: '单选', required: false, options: ['pass', 'reject', 'uncertain'] },
            ] },
            { label: 'Tab 2', fields: [{ name: 'tabs_1_tab2_text', widget: 'TextArea', label: '多行文本', required: false }] },
          ],
        },
      ],
    })
    expect(JSON.stringify(body)).not.toContain('_draftId')

    const parsed = parseTemplateSchema(body)
    if (!parsed.ok) {
      throw new Error(parsed.error.message)
    }
    render(<SchemaRenderer schema={parsed.value} payload={{}} />)
    const roundTrip = within(screen.getByRole('form', { name: 'QA template' }))
    expect(roundTrip.getByLabelText('Summary')).toBeInTheDocument()
    expect(roundTrip.getByLabelText('组内摘要')).toBeInTheDocument()
    expect(roundTrip.getAllByRole('radiogroup', { name: '单选' })).toHaveLength(2)
    expect(roundTrip.getByLabelText('多行文本')).toBeInTheDocument()
  })

  it('shows field-level validation and blocks saving invalid fields', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/templates/10') {
        return templateDetail(10, baseSchema, true)
      }
      throw new Error(`unexpected GET ${path}`)
    })

    renderDesigner('/owner/tasks/1/templates/10')

    await screen.findByRole('button', { name: /select summary/ })
    await user.click(screen.getByRole('button', { name: 'Add Radio' }))
    fireEvent.change(screen.getByLabelText('field_options'), { target: { value: '' } })

    expect(screen.getByLabelText('validation radio_1')).toHaveTextContent('fields[1].options: options must be non-empty')
    expect(screen.getByLabelText('selected validation radio_1')).toHaveTextContent('fields[1].options: options must be non-empty')
    expect(screen.getByRole('button', { name: 'Save as new version' })).toBeDisabled()
  })

  it('renders ShowItem preview from a real task item payload without saving preview data', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/templates/10') {
        return templateDetail(10, showItemSchema, true)
      }
      if (path === '/templates/11') {
        return templateDetail(11, showItemSchema, true)
      }
      if (path === '/tasks/1/item-preview') {
        return previewItem(11, {
          id: 'row-001',
          prompt: 'Real preview prompt',
          model_answer: 'Real preview answer',
          media_type: 'text',
        })
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

    expect(await screen.findByText('Real preview prompt')).toBeInTheDocument()
    expect(screen.getByText('Real preview answer')).toBeInTheDocument()
    expect(screen.getByLabelText('preview_item_status')).toHaveTextContent('Preview item #11')

    await user.click(screen.getByRole('button', { name: 'Save as new version' }))

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith('/tasks/1/templates', expect.any(Object))
    })
    const [, body] = mockApiPost.mock.calls[0]
    expect(JSON.stringify(body)).not.toContain('Real preview prompt')
    expect(JSON.stringify(body)).not.toContain('_draftId')
  })

  it('ignores stale preview item responses after a route switch', async () => {
    const user = userEvent.setup()
    const stalePreviewLoad = deferred<ReturnType<typeof previewItem>>()
    mockApiGet.mockImplementation((path) => {
      if (path === '/templates/10') {
        return Promise.resolve(templateDetail(10, showItemSchema, true, 1))
      }
      if (path === '/templates/20') {
        return Promise.resolve(templateDetail(20, showItemSchema, true, 2))
      }
      if (path === '/tasks/1/item-preview') {
        return stalePreviewLoad.promise
      }
      if (path === '/tasks/2/item-preview') {
        return Promise.resolve(previewItem(22, {
          id: 'row-002',
          prompt: 'Task B preview prompt',
          model_answer: 'Task B preview answer',
          media_type: 'text',
        }))
      }
      throw new Error(`unexpected GET ${path}`)
    })

    renderDesignerWithRouteSwitch('/owner/tasks/1/templates/10', '/owner/tasks/2/templates/20')

    await waitFor(() => {
      expect(mockApiGet).toHaveBeenCalledWith('/tasks/1/item-preview')
    })
    await user.click(screen.getByRole('button', { name: 'Switch route' }))
    expect(await screen.findByText('Task B preview prompt')).toBeInTheDocument()

    await act(async () => {
      stalePreviewLoad.resolve(previewItem(11, {
        id: 'row-001',
        prompt: 'Task A stale preview prompt',
        model_answer: 'Task A stale preview answer',
        media_type: 'text',
      }))
      await stalePreviewLoad.promise
    })

    expect(screen.getByText('Task B preview prompt')).toBeInTheDocument()
    expect(screen.queryByText('Task A stale preview prompt')).not.toBeInTheDocument()
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
    expect(screen.getByRole('button', { name: 'drag summary' })).toBeDisabled()
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

function dragDataTransfer() {
  const data = new Map<string, string>()
  return {
    dropEffect: '',
    effectAllowed: '',
    getData: vi.fn((type: string) => data.get(type) ?? ''),
    setData: vi.fn((type: string, value: string) => data.set(type, value)),
  }
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

function previewItem(id: number, payload: unknown) {
  return {
    item: {
      id,
      externalId: `row-${id}`,
      payload,
    },
  }
}
