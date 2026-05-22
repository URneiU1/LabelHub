import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { cwd } from 'node:process'
import { useState } from 'react'
import { describe, expect, it } from 'vitest'
import SchemaRenderer from './SchemaRenderer'
import { parseTemplateSchema } from './parser'
import type { AnswerValue, TemplateSchema } from './types'

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
