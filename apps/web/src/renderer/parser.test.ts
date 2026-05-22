import { describe, expect, it } from 'vitest'
import { parseAnswer, parseTemplateSchema } from './parser'

describe('parseTemplateSchema', () => {
  it('parses a valid minimal schema', () => {
    const result = parseTemplateSchema(`{
      "title": "qa_quality_review",
      "layout": "single_page",
      "fields": [
        { "name": "summary", "widget": "Input", "label": "一句话总评", "required": true }
      ]
    }`)

    expect(result.ok).toBe(true)
    if (result.ok) {
      expect(result.value.fields[0]).toMatchObject({
        name: 'summary',
        widget: 'Input',
        label: '一句话总评',
        required: true,
      })
    }
  })

  it('reports malformed JSON', () => {
    const result = parseTemplateSchema('{not json')

    expect(result.ok).toBe(false)
    if (!result.ok) {
      expect(result.error.field).toBe('$')
      expect(result.error.message).toContain('invalid JSON')
    }
  })

  it('requires fields', () => {
    const result = parseTemplateSchema({ title: 'bad' })

    expect(result.ok).toBe(false)
    if (!result.ok) {
      expect(result.error.field).toBe('fields')
    }
  })

  it('rejects non-array fields', () => {
    const result = parseTemplateSchema({ title: 'bad', fields: {} })

    expect(result.ok).toBe(false)
    if (!result.ok) {
      expect(result.error.message).toContain('array')
    }
  })

  it('rejects unsupported layout', () => {
    const result = parseTemplateSchema({
      title: 'bad',
      layout: 'tabs',
      fields: [{ name: 'summary', widget: 'Input' }],
    })

    expect(result.ok).toBe(false)
    if (!result.ok) {
      expect(result.error.field).toBe('layout')
    }
  })

  it('rejects unknown widget', () => {
    const result = parseTemplateSchema({
      title: 'bad',
      fields: [{ name: 'x', widget: 'Slider' }],
    })

    expect(result.ok).toBe(false)
    if (!result.ok) {
      expect(result.error.field).toBe('fields[0].widget')
    }
  })

  it('detects duplicate names after trim', () => {
    const result = parseTemplateSchema({
      title: 'bad',
      fields: [
        { name: 'score', widget: 'Input' },
        { name: ' score ', widget: 'TextArea' },
      ],
    })

    expect(result.ok).toBe(false)
    if (!result.ok) {
      expect(result.error.field).toBe('fields[1].name')
    }
  })

  it('rejects invalid radio options instead of silently filtering them', () => {
    const result = parseTemplateSchema({
      title: 'bad',
      fields: [{ name: 'score', widget: 'Radio', options: [{ label: 'A', value: 'a' }] }],
    })

    expect(result.ok).toBe(false)
    if (!result.ok) {
      expect(result.error.field).toBe('fields[0].options[0]')
    }
  })

  it('requires non-empty options for radio and tags', () => {
    const result = parseTemplateSchema({
      title: 'bad',
      fields: [{ name: 'issue_tags', widget: 'Tags' }],
    })

    expect(result.ok).toBe(false)
    if (!result.ok) {
      expect(result.error.field).toBe('fields[0].options')
    }
  })

  it('rejects invalid FileUpload maxFiles', () => {
    const result = parseTemplateSchema({
      title: 'bad',
      fields: [{ name: 'evidence', widget: 'FileUpload', maxFiles: 0 }],
    })

    expect(result.ok).toBe(false)
    if (!result.ok) {
      expect(result.error.field).toBe('fields[0].maxFiles')
    }
  })

  it('requires LLMTrigger target_field to reference an existing field unless external is explicit', () => {
    const bad = parseTemplateSchema({
      title: 'bad',
      fields: [{ name: 'ai', widget: 'LLMTrigger', target_field: 'missing' }],
    })
    expect(bad.ok).toBe(false)
    if (!bad.ok) {
      expect(bad.error.field).toBe('fields[0].target_field')
    }

    const good = parseTemplateSchema({
      title: 'good',
      fields: [{ name: 'ai', widget: 'LLMTrigger', target_field: 'external.score', 'x-allow-external-target': true }],
    })
    expect(good.ok).toBe(true)
  })

  it('preserves export_fields and x-* extensions round-trip', () => {
    const result = parseTemplateSchema({
      title: 'exportable',
      layout: 'single_page',
      export_fields: ['payload', 'answer'],
      'x-owner-note': { source: 'designer' },
      fields: [
        {
          name: 'summary',
          widget: 'Input',
          label: 'Summary',
          'x-field-note': 'kept',
        },
      ],
    })

    expect(result.ok).toBe(true)
    if (result.ok) {
      expect(result.value.export_fields).toEqual(['payload', 'answer'])
      expect(result.value['x-owner-note']).toEqual({ source: 'designer' })
      expect(result.value.fields[0]['x-field-note']).toBe('kept')
    }
  })
})

describe('parseAnswer', () => {
  it('parses object answers and wraps primitive values', () => {
    expect(parseAnswer('{"summary":"ok"}')).toEqual({ summary: 'ok' })
    expect(parseAnswer('"raw text"')).toEqual({ value: 'raw text' })
  })
})
