import { describe, expect, it } from 'vitest'
import type { TemplateSchema } from './types'
import { validateAnswer } from './validator'

const schema: TemplateSchema = {
  title: 'qa_quality_review',
  layout: 'single_page',
  fields: [
    { name: 'source_display', widget: 'ShowItem', label: '原始数据' },
    { name: 'summary', widget: 'Input', label: '一句话总评', required: true, maxLength: 8 },
    { name: 'comment', widget: 'TextArea', label: '详细评语', required: true, minLength: 4 },
    { name: 'issue_tags', widget: 'Tags', label: '问题类型', required: true },
  ],
}

describe('validateAnswer', () => {
  it('checks required values and length constraints', () => {
    const errors = validateAnswer(schema, {
      summary: '这是一段超过最大长度的总结',
      comment: '短',
      issue_tags: [],
    })

    expect(errors).toEqual([
      { field: 'summary', message: '一句话总评 must be at most 8 characters' },
      { field: 'comment', message: '详细评语 must be at least 4 characters' },
      { field: 'issue_tags', message: '问题类型 is required' },
    ])
  })

  it('ignores ShowItem and accepts complete answers', () => {
    expect(validateAnswer(schema, {
      summary: '很好',
      comment: '详细评语',
      issue_tags: ['无明显问题'],
    })).toEqual([])
  })
})
