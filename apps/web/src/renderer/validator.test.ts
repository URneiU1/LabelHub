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

  it('checks nested fields, regex, and requiredWhen conditions', () => {
    const advancedSchema: TemplateSchema = {
      title: 'advanced',
      layout: 'single_page',
      fields: [
        { name: 'decision', widget: 'Radio', label: '结论', options: ['pass', 'reject'] },
        {
          name: 'detail_group',
          widget: 'Group',
          label: '详情',
          fields: [
            {
              name: 'reject_reason',
              widget: 'Input',
              label: '打回原因',
              regex: '^.{4,}$',
              requiredWhen: { field: 'decision', equals: 'reject' },
            },
          ],
        },
      ],
    }

    expect(validateAnswer(advancedSchema, { decision: 'reject', reject_reason: '' })).toEqual([
      { field: 'reject_reason', message: '打回原因 is required' },
    ])
    expect(validateAnswer(advancedSchema, { decision: 'reject', reject_reason: '短' })).toEqual([
      { field: 'reject_reason', message: '打回原因 format is invalid' },
    ])
    expect(validateAnswer(advancedSchema, { decision: 'pass', reject_reason: '' })).toEqual([])
  })

  it('skips validation for fields hidden by visibleWhen', () => {
    const visibilitySchema: TemplateSchema = {
      title: 'visibility',
      layout: 'single_page',
      fields: [
        { name: 'decision', widget: 'Radio', label: '结论', options: ['pass', 'reject'] },
        {
          name: 'reject_reason',
          widget: 'Input',
          label: '打回原因',
          required: true,
          visibleWhen: { field: 'decision', equals: 'reject' },
        },
      ],
    }

    // Hidden because decision !== reject -> required is skipped.
    expect(validateAnswer(visibilitySchema, { decision: 'pass' })).toEqual([])
    // Shown because decision === reject -> required is enforced.
    expect(validateAnswer(visibilitySchema, { decision: 'reject', reject_reason: '' })).toEqual([
      { field: 'reject_reason', message: '打回原因 is required' },
    ])
    expect(validateAnswer(visibilitySchema, { decision: 'reject', reject_reason: '理由充分' })).toEqual([])
  })

  it('applies customRule on non-empty visible values and skips empty ones', () => {
    const customRuleSchema: TemplateSchema = {
      title: 'custom-rule',
      layout: 'single_page',
      fields: [
        {
          name: 'score',
          widget: 'Input',
          label: '评分',
          customRule: { expr: 'value >= "60"', message: '评分至少为 60' },
        },
      ],
    }

    // Empty value -> customRule skipped.
    expect(validateAnswer(customRuleSchema, { score: '' })).toEqual([])
    // expr false on a non-empty value -> error with the configured message.
    expect(validateAnswer(customRuleSchema, { score: '50' })).toEqual([
      { field: 'score', message: '评分至少为 60' },
    ])
    // expr true -> no error.
    expect(validateAnswer(customRuleSchema, { score: '90' })).toEqual([])
  })

  it('composes customRule with len helper and sibling answers', () => {
    const composedSchema: TemplateSchema = {
      title: 'composed',
      layout: 'single_page',
      fields: [
        { name: 'min_len', widget: 'Input', label: '最小长度' },
        {
          name: 'comment',
          widget: 'TextArea',
          label: '评语',
          customRule: { expr: 'len(value) >= 4', message: '评语至少 4 个字符' },
        },
      ],
    }

    expect(validateAnswer(composedSchema, { comment: '太短' })).toEqual([
      { field: 'comment', message: '评语至少 4 个字符' },
    ])
    expect(validateAnswer(composedSchema, { comment: '足够长的评语' })).toEqual([])
  })

  it('does not run customRule on fields hidden by visibleWhen', () => {
    const hiddenRuleSchema: TemplateSchema = {
      title: 'hidden-rule',
      layout: 'single_page',
      fields: [
        { name: 'decision', widget: 'Radio', label: '结论', options: ['pass', 'reject'] },
        {
          name: 'score',
          widget: 'Input',
          label: '评分',
          visibleWhen: { field: 'decision', equals: 'reject' },
          customRule: { expr: 'len(value) >= 4', message: '评分太短' },
        },
      ],
    }

    // score would fail its customRule, but it is hidden -> skipped entirely.
    expect(validateAnswer(hiddenRuleSchema, { decision: 'pass', score: '1' })).toEqual([])
  })
})
