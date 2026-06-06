import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { cwd } from 'node:process'
import { useState } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SchemaRenderer from './SchemaRenderer'
import { parseTemplateSchema } from './parser'
import type { AnswerValue, TemplateSchema, ValidationError } from './types'
import { apiPost } from '../shared/api/client'
import { buildLargeSchema, topLevelFieldNames } from './__fixtures__/largeSchema'

vi.mock('../shared/api/client', () => ({
  apiPost: vi.fn(),
}))

const mockApiPost = vi.mocked(apiPost)

const qaQualitySchema = JSON.parse(readFileSync(
  resolve(cwd(), '../../tools/seed/templates/qa_quality_review.json'),
  'utf8',
)) as unknown

const preferenceCompareSchema = JSON.parse(readFileSync(
  resolve(cwd(), '../../tools/seed/templates/preference_compare_review.json'),
  'utf8',
)) as unknown

const qaPayload = {
  id: 'Q0001',
  category: '知识问答',
  media_type: 'text',
  prompt: '光合作用主要发生在植物细胞的哪个结构中？',
  model_answer: '光合作用主要发生在叶绿体中。',
  reference: '叶绿体。',
  expected_dimensions: ['相关性', '准确性'],
}

const preferencePayload = {
  id: 'P0001',
  task_type: '知识问答',
  lang: 'zh',
  prompt: '解释什么是过拟合，并给一个通俗例子。',
  response_a: '过拟合指模型在训练集表现很好但泛化差。比如学生死记答案，考原题满分，换题就不会。',
  model_a: 'doubao-pro',
  response_b: '过拟合就是模型训练得太好了。',
  model_b: 'baseline-7b',
}

function parsedSchema(): TemplateSchema {
  const result = parseTemplateSchema(qaQualitySchema)
  if (!result.ok) {
    throw new Error(result.error.message)
  }
  return result.value
}

function parsedPreferenceSchema(): TemplateSchema {
  const result = parseTemplateSchema(preferenceCompareSchema)
  if (!result.ok) {
    throw new Error(result.error.message)
  }
  return result.value
}

function ControlledRenderer() {
  const [answer, setAnswer] = useState<AnswerValue>({})
  return (
    <>
      <SchemaRenderer schema={parsedSchema()} payload={qaPayload} value={answer} onChange={setAnswer} />
      <output aria-label="answer-json">{JSON.stringify(answer)}</output>
    </>
  )
}

describe('SchemaRenderer', () => {
  beforeEach(() => {
    mockApiPost.mockReset()
  })

  it('renders qa_quality fixture fields without missing registry entries', () => {
    render(<SchemaRenderer schema={parsedSchema()} payload={qaPayload} />)

    expect(screen.getByText('光合作用主要发生在植物细胞的哪个结构中？')).toBeInTheDocument()
    expect(screen.getByLabelText('一句话总评')).toBeInTheDocument()
    expect(screen.getByLabelText('详细评语')).toBeInTheDocument()
    expect(screen.getByText('修订建议')).toBeInTheDocument()
    expect(document.querySelectorAll('[data-widget]')).toHaveLength(12)
  })

  it('renders preference_compare fixture with grouped source data and tabbed annotation fields', async () => {
    const user = userEvent.setup()
    render(<SchemaRenderer schema={parsedPreferenceSchema()} payload={preferencePayload} />)

    expect(screen.getByText('解释什么是过拟合，并给一个通俗例子。')).toBeInTheDocument()
    expect(screen.getByText('过拟合指模型在训练集表现很好但泛化差。比如学生死记答案，考原题满分，换题就不会。')).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: '判定' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('radiogroup', { name: '偏好结论' })).toBeInTheDocument()
    expect(screen.queryByLabelText('判断理由')).not.toBeInTheDocument()

    await user.click(screen.getByRole('tab', { name: '说明' }))

    expect(screen.getByLabelText('判断理由')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '运行 AI 预审' })).toBeInTheDocument()
  })

  it('updates radio, tags, input, and textarea values', async () => {
    const user = userEvent.setup()
    render(<ControlledRenderer />)

    const relevanceGroup = screen.getByRole('radiogroup', { name: '相关性' })
    await user.click(within(relevanceGroup).getByLabelText('5'))
    await user.click(screen.getByLabelText('无明显问题'))
    await user.type(screen.getByLabelText('一句话总评'), '回答准确')
    await user.type(screen.getByLabelText('详细评语'), '内容完整且安全')

    const answer = JSON.parse(screen.getByLabelText('answer-json').textContent || '{}') as AnswerValue
    expect(answer).toMatchObject({
      relevance_score: 5,
      issue_tags: ['无明显问题'],
      summary: '回答准确',
      comment: '内容完整且安全',
    })
  })

  it('updates rich text and JSON editor widgets', async () => {
    const user = userEvent.setup()
    render(<ControlledRenderer />)

    await user.type(screen.getByLabelText('修订建议'), '补充引用来源')
    fireEvent.change(screen.getByLabelText('修正后答案'), {
      target: { value: '{"corrected_answer":"叶绿体"}' },
    })
    fireEvent.blur(screen.getByLabelText('修正后答案'))

    const answer = JSON.parse(screen.getByLabelText('answer-json').textContent || '{}') as AnswerValue
    expect(answer.revision_suggestion).toBe('补充引用来源')
    expect(answer.corrected_answer).toEqual({ corrected_answer: '叶绿体' })
  })

  it('renders group and tabs fields with flat answer updates', async () => {
    const user = userEvent.setup()
    const result = parseTemplateSchema({
      title: 'structured',
      fields: [
        {
          name: 'quality_group',
          widget: 'Group',
          label: '质量判断',
          fields: [
            { name: 'summary', widget: 'Input', label: '一句话总评' },
          ],
        },
        {
          name: 'review_tabs',
          widget: 'Tabs',
          label: '分步审核',
          tabs: [
            { label: '基础', fields: [{ name: 'decision', widget: 'Radio', label: '结论', options: ['pass', 'reject'] }] },
            { label: '备注', fields: [{ name: 'comment', widget: 'TextArea', label: '备注' }] },
          ],
        },
      ],
    })
    if (!result.ok) {
      throw new Error(result.error.message)
    }

    function StructuredRenderer() {
      const [answer, setAnswer] = useState<AnswerValue>({})
      return (
        <>
          <SchemaRenderer schema={result.value} value={answer} onChange={setAnswer} />
          <output aria-label="structured-answer-json">{JSON.stringify(answer)}</output>
        </>
      )
    }

    render(<StructuredRenderer />)

    await user.type(screen.getByLabelText('一句话总评'), '可以通过')
    expect(screen.getByRole('tab', { name: '基础' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.queryByLabelText('备注')).not.toBeInTheDocument()
    await user.click(within(screen.getByRole('radiogroup', { name: '结论' })).getByLabelText('pass'))
    await user.click(screen.getByRole('tab', { name: '备注' }))
    expect(screen.getByRole('tab', { name: '备注' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.queryByRole('radiogroup', { name: '结论' })).not.toBeInTheDocument()
    const commentInput = screen.getAllByLabelText('备注').find((element) => element.tagName === 'TEXTAREA')
    if (!commentInput) {
      throw new Error('comment textarea not found')
    }
    await user.type(commentInput, '结构完整')
    await user.click(screen.getByRole('tab', { name: '基础' }))
    expect(within(screen.getByRole('radiogroup', { name: '结论' })).getByLabelText('pass')).toBeChecked()

    const answer = JSON.parse(screen.getByLabelText('structured-answer-json').textContent || '{}') as AnswerValue
    expect(answer).toMatchObject({
      summary: '可以通过',
      decision: 'pass',
      comment: '结构完整',
    })
    expect(document.querySelectorAll('[data-widget="Group"]')).toHaveLength(1)
    expect(document.querySelectorAll('[data-widget="Tabs"]')).toHaveLength(1)
  })

  it('switches to the first tab with validation errors', async () => {
    const user = userEvent.setup()
    const result = parseTemplateSchema({
      title: 'structured',
      fields: [
        {
          name: 'review_tabs',
          widget: 'Tabs',
          label: '分步审核',
          tabs: [
            { label: '基础', fields: [{ name: 'decision', widget: 'Radio', label: '结论', options: ['pass', 'reject'] }] },
            { label: '备注', fields: [{ name: 'comment', widget: 'TextArea', label: '备注', required: true }] },
          ],
        },
      ],
    })
    if (!result.ok) {
      throw new Error(result.error.message)
    }

    function StructuredRenderer() {
      const [answer, setAnswer] = useState<AnswerValue>({ decision: 'pass' })
      const [errors, setErrors] = useState<ValidationError[]>([])
      return (
        <>
          <button type="button" onClick={() => setErrors([{ field: 'comment', message: '备注 is required' }])}>
            inject errors
          </button>
          <SchemaRenderer schema={result.value} value={answer} errors={errors} onChange={setAnswer} />
        </>
      )
    }

    render(<StructuredRenderer />)

    await user.click(screen.getByRole('button', { name: 'inject errors' }))

    await waitFor(() => {
      expect(screen.getByRole('tab', { name: '备注' })).toHaveAttribute('aria-selected', 'true')
    })
    expect(screen.getByRole('alert')).toHaveTextContent('备注 is required')
  })

  it('hides and shows fields based on visibleWhen against the current answer', async () => {
    const user = userEvent.setup()
    const result = parseTemplateSchema({
      title: 'conditional',
      fields: [
        { name: 'decision', widget: 'Radio', label: '结论', options: ['pass', 'reject'] },
        {
          name: 'reject_reason',
          widget: 'Input',
          label: '打回原因',
          visibleWhen: { field: 'decision', equals: 'reject' },
        },
        {
          name: 'reject_group',
          widget: 'Group',
          label: '打回详情',
          visibleWhen: { field: 'decision', equals: 'reject' },
          fields: [{ name: 'severity', widget: 'Input', label: '严重程度' }],
        },
      ],
    })
    if (!result.ok) {
      throw new Error(result.error.message)
    }

    function ConditionalRenderer() {
      const [answer, setAnswer] = useState<AnswerValue>({ decision: 'pass' })
      return <SchemaRenderer schema={result.value} value={answer} onChange={setAnswer} />
    }

    render(<ConditionalRenderer />)

    // Hidden while decision === pass (covers both leaf and Group-child fields).
    expect(screen.queryByLabelText('打回原因')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('严重程度')).not.toBeInTheDocument()

    await user.click(within(screen.getByRole('radiogroup', { name: '结论' })).getByLabelText('reject'))

    // Shown once decision === reject.
    expect(screen.getByLabelText('打回原因')).toBeInTheDocument()
    expect(screen.getByLabelText('严重程度')).toBeInTheDocument()
  })

  it('prunes hidden leaf values when conditional fields become invisible', async () => {
    const user = userEvent.setup()
    const result = parseTemplateSchema({
      title: 'conditional',
      fields: [
        { name: 'decision', widget: 'Radio', label: '结论', options: ['pass', 'reject'] },
        {
          name: 'reject_reason',
          widget: 'Input',
          label: '打回原因',
          visibleWhen: { field: 'decision', equals: 'reject' },
        },
        {
          name: 'reject_group',
          widget: 'Group',
          label: '打回详情',
          visibleWhen: { field: 'decision', equals: 'reject' },
          fields: [{ name: 'severity', widget: 'Input', label: '严重程度' }],
        },
      ],
    })
    if (!result.ok) {
      throw new Error(result.error.message)
    }

    function ConditionalRenderer() {
      const [answer, setAnswer] = useState<AnswerValue>({
        decision: 'reject',
        reject_reason: '证据不足',
        severity: 'high',
      })
      return (
        <>
          <SchemaRenderer schema={result.value} value={answer} onChange={setAnswer} />
          <output aria-label="conditional-answer-json">{JSON.stringify(answer)}</output>
        </>
      )
    }

    render(<ConditionalRenderer />)

    expect(screen.getByLabelText('打回原因')).toHaveValue('证据不足')
    expect(screen.getByLabelText('严重程度')).toHaveValue('high')

    await user.click(within(screen.getByRole('radiogroup', { name: '结论' })).getByLabelText('pass'))

    await waitFor(() => {
      const answer = JSON.parse(screen.getByLabelText('conditional-answer-json').textContent || '{}') as AnswerValue
      expect(answer).toEqual({ decision: 'pass' })
    })
    expect(screen.queryByLabelText('打回原因')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('严重程度')).not.toBeInTheDocument()
  })

  it('renders ShowItem media modes from path', () => {
    const result = parseTemplateSchema({
      title: 'media',
      fields: [
        { name: 'image', widget: 'ShowItem', label: '图片', path: '$payload.media_url', mode: 'image' },
        { name: 'markdown', widget: 'ShowItem', label: 'Markdown', path: '$payload.content_markdown', mode: 'markdown' },
      ],
    })
    if (!result.ok) {
      throw new Error(result.error.message)
    }

    render(<SchemaRenderer schema={result.value} payload={{
      media_url: 'https://example.com/a.png',
      content_markdown: '![alt](https://example.com/b.png)',
    }} />)

    expect(screen.getByAltText('图片')).toHaveAttribute('src', 'https://example.com/a.png')
    expect(screen.getByAltText('alt')).toHaveAttribute('src', 'https://example.com/b.png')
  })

  it('renders ShowItem text, video, and JSON modes from paths', () => {
    const result = parseTemplateSchema({
      title: 'show-item-modes',
      fields: [
        { name: 'prompt_text', widget: 'ShowItem', label: '题目文本', path: '$payload.prompt', mode: 'text' },
        { name: 'video_asset', widget: 'ShowItem', label: '视频', path: '$payload.video_url', mode: 'video' },
        { name: 'metadata', widget: 'ShowItem', label: '元数据', path: '$payload.meta', mode: 'json' },
      ],
    })
    if (!result.ok) {
      throw new Error(result.error.message)
    }

    render(<SchemaRenderer schema={result.value} payload={{
      prompt: '请判断回答是否完整',
      video_url: 'https://example.com/demo.mp4',
      meta: { source: 'seed', score: 91 },
    }} />)

    expect(screen.getByText('请判断回答是否完整')).toBeInTheDocument()
    expect(document.querySelector('video')).toHaveAttribute('src', 'https://example.com/demo.mp4')
    expect(screen.getByText(/"source": "seed"/)).toBeInTheDocument()
    expect(screen.getByText(/"score": 91/)).toBeInTheDocument()
  })

  it('runs LLMTrigger and writes the response into its target field', async () => {
    const user = userEvent.setup()
    mockApiPost.mockResolvedValueOnce({ text: 'AI 建议：补充引用', provider: 'mock' })
    const result = parseTemplateSchema({
      title: 'llm-trigger',
      fields: [
        { name: 'summary', widget: 'Input', label: '摘要' },
        { name: 'ai_button', widget: 'LLMTrigger', label: 'AI 辅助', prompt: '检查摘要', target_field: 'summary' },
      ],
    })
    if (!result.ok) {
      throw new Error(result.error.message)
    }

    function LLMRenderer() {
      const [answer, setAnswer] = useState<AnswerValue>({ summary: '旧摘要' })
      return (
        <>
          <SchemaRenderer schema={result.value} payload={{ prompt: '题目' }} value={answer} onChange={setAnswer} />
          <output aria-label="llm-answer-json">{JSON.stringify(answer)}</output>
        </>
      )
    }

    render(<LLMRenderer />)
    await user.click(screen.getByRole('button', { name: '运行 AI 预审' }))

    expect(mockApiPost).toHaveBeenCalledWith('/llm/inline', {
      prompt: '检查摘要',
      input: { payload: { prompt: '题目' }, answer: { summary: '旧摘要' } },
    })
    expect(await screen.findByText('AI 建议：补充引用')).toBeInTheDocument()
    const answer = JSON.parse(screen.getByLabelText('llm-answer-json').textContent || '{}') as AnswerValue
    expect(answer.summary).toBe('AI 建议：补充引用')
  })

  it('does not render unsafe markdown URLs as links', () => {
    const result = parseTemplateSchema({
      title: 'unsafe',
      fields: [
        { name: 'markdown', widget: 'ShowItem', label: 'Markdown', path: '$payload.content_markdown', mode: 'markdown' },
        { name: 'image', widget: 'ShowItem', label: '图片', path: '$payload.media_url', mode: 'image' },
      ],
    })
    if (!result.ok) {
      throw new Error(result.error.message)
    }

    render(<SchemaRenderer schema={result.value} payload={{
      content_markdown: '[bad](javascript:alert(1))',
      media_url: 'javascript:alert(1)',
    }} />)

    expect(screen.queryByRole('link', { name: 'bad' })).not.toBeInTheDocument()
    expect(screen.queryByAltText('图片')).not.toBeInTheDocument()
  })

  describe('large schema (P2 performance fixture)', () => {
    it('parses a 300+ field schema without corrupting field order or keys', () => {
      const raw = buildLargeSchema(300)
      const result = parseTemplateSchema(raw)
      if (!result.ok) {
        throw new Error(result.error.message)
      }
      // 顶层字段的顺序与键必须与输入一致(Designer 加载/保存不应打乱)。
      expect(result.value.fields.map((field) => field.name)).toEqual(topLevelFieldNames(raw))
      // Group / Tabs 容器存在,确认大 schema 覆盖嵌套结构。
      expect(result.value.fields.some((field) => field.widget === 'Group')).toBe(true)
      expect(result.value.fields.some((field) => field.widget === 'Tabs')).toBe(true)
    })

    it('renders a large schema without throwing and keeps only visible leaf values', async () => {
      const raw = buildLargeSchema(300)
      const result = parseTemplateSchema(raw)
      if (!result.ok) {
        throw new Error(result.error.message)
      }

      function LargeHarness() {
        // f_1 依赖 gate_0 == 'show';gate_0 设为 'hide' → f_1 隐藏,其残留值应被裁剪。
        const [answer, setAnswer] = useState<AnswerValue>({ gate_0: 'hide', f_1: 'orphan', f_2: 'kept' })
        return (
          <>
            <SchemaRenderer schema={result.value} value={answer} onChange={setAnswer} />
            <output aria-label="large-answer-json">{JSON.stringify(answer)}</output>
          </>
        )
      }

      render(<LargeHarness />)

      await waitFor(() => {
        const answer = JSON.parse(screen.getByLabelText('large-answer-json').textContent || '{}') as AnswerValue
        expect(answer.f_1).toBeUndefined()
      })
      const answer = JSON.parse(screen.getByLabelText('large-answer-json').textContent || '{}') as AnswerValue
      expect(answer.f_2).toBe('kept')
      expect(answer.gate_0).toBe('hide')
    })
  })
})
