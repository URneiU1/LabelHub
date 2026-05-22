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
