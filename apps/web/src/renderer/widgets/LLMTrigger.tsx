import type { CSSProperties } from 'react'
import { useState } from 'react'
import { apiPost } from '../../shared/api/client'
import type { WidgetProps } from '../types'
import FieldFrame from './FieldFrame'

type InlineLLMResponse = {
  text: string
  provider: string
}

export default function LLMTriggerWidget({ field, value, answer, payload, readOnly, onChange }: WidgetProps) {
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const targetField = field.target_field || field.name
  const displayValue = targetField === field.name ? value : answer[targetField]

  async function run() {
    setLoading(true)
    setError('')
    try {
      const data = await apiPost<InlineLLMResponse>('/llm/inline', {
        prompt: field.prompt || '请根据任务原始数据和当前标注答案给出预审建议。',
        input: { payload, answer },
      })
      onChange(targetField, data.text)
    } catch (error) {
      setError(error instanceof Error ? error.message : 'AI 预审失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <FieldFrame label={field.label} required={field.required}>
      {readOnly ? null : (
        <button type="button" disabled={loading} style={buttonStyle} onClick={() => void run()}>
          {loading ? '运行中' : '运行 AI 预审'}
        </button>
      )}
      {error ? <span role="alert" style={errorStyle}>{error}</span> : null}
      {displayValue ? <pre style={resultStyle}>{String(displayValue)}</pre> : null}
    </FieldFrame>
  )
}

const buttonStyle: CSSProperties = {
  width: 'fit-content',
  minHeight: 36,
  padding: '0 var(--space-md)',
  border: '1px solid var(--color-border)',
  background: 'var(--color-surface)',
  fontFamily: 'var(--font-body)',
  cursor: 'pointer',
}

const resultStyle: CSSProperties = {
  margin: 'var(--space-sm) 0 0',
  padding: 'var(--space-sm)',
  border: '1px solid var(--color-border-light)',
  background: 'var(--color-bg)',
  whiteSpace: 'pre-wrap',
  lineHeight: 1.6,
}

const errorStyle: CSSProperties = {
  color: 'var(--color-danger, #b42318)',
  fontSize: 'var(--text-sm)',
}
