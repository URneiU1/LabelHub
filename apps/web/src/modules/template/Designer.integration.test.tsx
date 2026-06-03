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

// @dnd-kit 在 jsdom 下无法靠 getBoundingClientRect 驱动(rect 全为 0),
// 故 mock DndContext 捕获 onDragEnd,测试直接派发拖拽结束事件来精确驱动重排/插入,
// 仍走真实组件的 handleDragEnd → setFields → buildTemplatePayload → 保存全链路。
const dndState = vi.hoisted(() => ({ onDragEnd: undefined as ((event: unknown) => void) | undefined }))

vi.mock('@dnd-kit/core', () => ({
  DndContext: ({ children, onDragEnd }: { children: React.ReactNode, onDragEnd?: (event: unknown) => void }) => {
    dndState.onDragEnd = onDragEnd
    return children
  },
  DragOverlay: ({ children }: { children?: React.ReactNode }) => children ?? null,
  PointerSensor: function PointerSensor() {},
  KeyboardSensor: function KeyboardSensor() {},
  useSensor: () => ({}),
  useSensors: () => [],
  useDraggable: () => ({ attributes: {}, listeners: {}, setNodeRef: () => {}, transform: null, isDragging: false }),
  useDroppable: () => ({ setNodeRef: () => {}, isOver: false }),
  closestCenter: () => [],
}))

vi.mock('@dnd-kit/sortable', () => ({
  SortableContext: ({ children }: { children?: React.ReactNode }) => children ?? null,
  verticalListSortingStrategy: {},
  sortableKeyboardCoordinates: () => {},
  useSortable: () => ({ attributes: {}, listeners: {}, setNodeRef: () => {}, setActivatorNodeRef: () => {}, transform: null, transition: undefined, isDragging: false }),
}))

vi.mock('@dnd-kit/utilities', () => ({
  CSS: { Transform: { toString: () => '' }, Translate: { toString: () => '' } },
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

    await screen.findByRole('heading', { name: '模板搭建器（Designer）' })
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

  it('renders the org-style toolbar and creates a real Tabs layout from the canvas sub-nav', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/templates/10') {
        return templateDetail(10, baseSchema, true)
      }
      throw new Error(`unexpected GET ${path}`)
    })

    renderDesigner('/owner/tasks/1/templates/10')

    expect(await screen.findByRole('heading', { name: '模板搭建器（Designer）' })).toBeInTheDocument()
    expect(screen.getByText('任务负责人后台')).toBeInTheDocument()
    expect(screen.getByText('当前版本 r2')).toBeInTheDocument()
    expect(screen.getByText('绑定任务 T-1')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '预览 Schema' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '导出 Schema JSON' })).toBeInTheDocument()
    expect(screen.getByText('物料')).toBeInTheDocument()
    expect(screen.getByText('布局')).toBeInTheDocument()

    const canvasTabs = screen.getByRole('tablist', { name: 'canvas tabs' })
    expect(within(canvasTabs).getByRole('tab', { name: '基础信息' })).toHaveAttribute('aria-selected', 'true')

    await user.click(within(canvasTabs).getByRole('button', { name: '新增画布 Tab' }))

    expect(screen.getByRole('button', { name: /select tabs_1/ })).toBeInTheDocument()
    expect(within(screen.getByRole('tablist', { name: 'canvas tabs' })).getByRole('tab', { name: 'Tab 1' })).toHaveAttribute('aria-selected', 'true')
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

    // 把 tags_1 拖到 summary 之前(canvas 重排,保留「插到目标前」语义)。
    const tagsDraftId = draftIdOf('tags_1')
    await act(async () => {
      dndState.onDragEnd?.({
        active: { id: tagsDraftId, data: { current: { source: 'canvas', draftId: tagsDraftId } } },
        over: { id: draftIdOf('summary') },
      })
    })

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

    const groupPreview = within(screen.getByLabelText('nested preview group_1'))
    expect(groupPreview.getByLabelText('nested field group_summary')).toBeInTheDocument()
    expect(groupPreview.getByLabelText('nested field group_decision')).toBeInTheDocument()
    const tabsPreview = within(screen.getByLabelText('nested preview tabs_1'))
    expect(tabsPreview.getByLabelText('nested field tabs_1_decision')).toBeInTheDocument()
    expect(tabsPreview.getByLabelText('nested field tabs_1_tab2_text')).toBeInTheDocument()

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
    await user.click(roundTrip.getByRole('tab', { name: 'Tab 2' }))
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
    // 新行为:校验未通过时保存按钮不再禁用,点击会被 focusFirstError 拦截并定位到出错字段,而不保存。
    const saveButton = screen.getByRole('button', { name: 'Save as new version' })
    expect(saveButton).not.toBeDisabled()
    await user.click(saveButton)
    expect(mockApiPost).not.toHaveBeenCalled()
    expect(screen.getByLabelText('selected validation radio_1')).toBeInTheDocument()
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

    await screen.findByRole('heading', { name: '模板搭建器（Designer）' })
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

  it('configures visibleWhen and customRule and round-trips them through the saved schema', async () => {
    const user = userEvent.setup()
    const decisionSchema = {
      title: 'QA template',
      layout: 'single_page',
      fields: [
        { name: 'decision', widget: 'Radio', label: 'Decision', required: true, options: ['pass', 'reject'] },
        { name: 'reason', widget: 'TextArea', label: 'Reason', required: false },
      ],
    }
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/templates/40') {
        return templateDetail(40, decisionSchema, true)
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost.mockResolvedValue({ id: 41, taskId: 1, version: 2, schemaJson: '' })

    renderDesigner('/owner/tasks/1/templates/40')

    await screen.findByRole('button', { name: /select reason/ })
    await user.click(screen.getByRole('button', { name: /select reason/ }))

    // 联动 tab: only show "reason" when decision == reject.
    await user.click(screen.getByLabelText('property_tab_logic'))
    fireEvent.change(screen.getByLabelText('visible_when_field'), { target: { value: 'decision' } })
    fireEvent.change(screen.getByLabelText('visible_when_equals'), { target: { value: 'reject' } })

    // 校验 tab: reason must be at most 35 chars.
    await user.click(screen.getByLabelText('property_tab_validation'))
    fireEvent.change(screen.getByLabelText('custom_rule_expr'), { target: { value: 'len(value) <= 35' } })
    fireEvent.change(screen.getByLabelText('custom_rule_message'), { target: { value: '不能超过 35 个字符' } })

    await user.click(screen.getByRole('button', { name: 'Save as new version' }))

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith('/tasks/1/templates', expect.any(Object))
    })
    const [, body] = mockApiPost.mock.calls[0]
    expect(body).toMatchObject({
      fields: [
        { name: 'decision', widget: 'Radio' },
        {
          name: 'reason',
          widget: 'TextArea',
          visibleWhen: { field: 'decision', equals: 'reject' },
          customRule: { expr: 'len(value) <= 35', message: '不能超过 35 个字符' },
        },
      ],
    })
    expect(JSON.stringify(body)).not.toContain('_draftId')

    const parsed = parseTemplateSchema(body)
    if (!parsed.ok) {
      throw new Error(parsed.error.message)
    }
    const reasonField = parsed.value.fields.find((item) => item.name === 'reason')
    expect(reasonField?.visibleWhen).toEqual({ field: 'decision', equals: 'reject' })
    expect(reasonField?.customRule).toEqual({ expr: 'len(value) <= 35', message: '不能超过 35 个字符' })
  })

  it('inserts a palette widget at the drop position via drag-to-place', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      if (path === '/templates/10') {
        return templateDetail(10, baseSchema, true)
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost.mockResolvedValue({ id: 11, taskId: 1, version: 2, schemaJson: '' })

    renderDesigner('/owner/tasks/1/templates/10')

    await screen.findByRole('button', { name: /select summary/ })

    // 把 Radio 物料拖到已有的 summary 字段上 => 插到它之前。
    await act(async () => {
      dndState.onDragEnd?.({
        active: { id: 'palette-Radio', data: { current: { source: 'palette', widget: 'Radio' } } },
        over: { id: draftIdOf('summary') },
      })
    })

    expect(screen.getByRole('button', { name: /select radio_1/ })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Save as new version' }))

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith('/tasks/1/templates', expect.objectContaining({
        export_fields: ['radio_1', 'summary'],
      }))
    })
  })

  it('creates a brand-new template from the /new route without loading an existing one', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      // 新建阶段不应拉取 /templates/new;navigate 到新建版本后才加载 /templates/21。
      if (path === '/templates/21') return templateDetail(21, baseSchema, true)
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost.mockResolvedValue({ id: 21, taskId: 1, version: 1, schemaJson: '' })

    renderDesigner('/owner/tasks/1/templates/new')

    // 新建态:空白可编辑画布,显示「新建模板」而非某个版本号。
    await screen.findByText('新建模板')
    await user.click(screen.getByRole('button', { name: 'Add Input' }))
    await user.click(screen.getByRole('button', { name: 'Save as new version' }))

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith('/tasks/1/templates', expect.objectContaining({
        fields: expect.arrayContaining([expect.objectContaining({ widget: 'Input' })]),
      }))
    })
    // 新建阶段没有以 'new' 拉取任何模板。
    expect(mockApiGet).not.toHaveBeenCalledWith('/templates/new')
  })

  it('seeds a new template by cloning the source task latest template via copyFrom', async () => {
    const user = userEvent.setup()
    mockApiGet.mockImplementation(async (path) => {
      // copyFrom=9:克隆源任务 9 的最新模板 schema;不拉取 /templates/new。
      if (path === '/tasks/9/templates') {
        return [{ id: 77, taskId: 9, version: 3, schemaJson: JSON.stringify(baseSchema), createdAt: '2026-06-03T00:00:00Z' }]
      }
      throw new Error(`unexpected GET ${path}`)
    })
    mockApiPost.mockResolvedValue({ id: 30, taskId: 2, version: 1, schemaJson: '' })

    renderDesigner('/owner/tasks/2/templates/new?copyFrom=9')

    // 仍是新建态(显示「新建模板」),但画布已带源任务字段(baseSchema 的 summary)。
    await screen.findByText('新建模板')
    expect(await screen.findByRole('button', { name: /select summary/ })).toBeInTheDocument()

    // 保存 → POST 到本任务 /tasks/2/templates,克隆字段一并带上。
    await user.click(screen.getByRole('button', { name: 'Save as new version' }))
    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith('/tasks/2/templates', expect.objectContaining({
        fields: expect.arrayContaining([expect.objectContaining({ name: 'summary' })]),
      }))
    })
    expect(mockApiGet).not.toHaveBeenCalledWith('/templates/new')
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

// 从画布上某字段的「select <name>」按钮回溯到它所在卡片的 data-draft-id,
// 用作 @dnd-kit 拖拽事件里的稳定 id(组件内部生成,测试无法预知)。
function draftIdOf(name: string): string {
  const selectButton = screen.getByRole('button', { name: `select ${name}` })
  const card = selectButton.closest('[data-draft-id]')
  if (!card) throw new Error(`no canvas card found for field "${name}"`)
  return card.getAttribute('data-draft-id') as string
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
