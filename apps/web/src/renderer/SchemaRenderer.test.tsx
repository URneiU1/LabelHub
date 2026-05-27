import { fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { cwd } from 'node:process'
import { useState } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SchemaRenderer from './SchemaRenderer'
import { parseTemplateSchema } from './parser'
import type { AnswerValue, TemplateSchema } from './types'
import { apiPost } from '../shared/api/client'

vi.mock('../shared/api/client', () => ({
  apiPost: vi.fn(),
}))

const mockApiPost = vi.mocked(apiPost)

const qaQualitySchema = JSON.parse(readFileSync(
  resolve(cwd(), '../../tools/seed/templates/qa_quality_review.json'),
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

function parsedSchema(): TemplateSchema {
  const result = parseTemplateSchema(qaQualitySchema)
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
    await user.click(within(screen.getByRole('radiogroup', { name: '结论' })).getByLabelText('pass'))
    const commentInput = screen.getAllByLabelText('备注').find((element) => element.tagName === 'TEXTAREA')
    if (!commentInput) {
      throw new Error('comment textarea not found')
    }
    await user.type(commentInput, '结构完整')

    const answer = JSON.parse(screen.getByLabelText('structured-answer-json').textContent || '{}') as AnswerValue
    expect(answer).toMatchObject({
      summary: '可以通过',
      decision: 'pass',
      comment: '结构完整',
    })
    expect(document.querySelectorAll('[data-widget="Group"]')).toHaveLength(1)
    expect(document.querySelectorAll('[data-widget="Tabs"]')).toHaveLength(1)
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
})
