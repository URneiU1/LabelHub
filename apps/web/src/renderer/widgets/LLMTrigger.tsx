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
      <div style={cardStyle}>
        <div style={headStyle}>
          <span style={titleStyle}>✦ {field.label || 'AI 建议清洗'}</span>
          {!readOnly && displayValue ? (
            <button type="button" disabled={loading} style={regenStyle} onClick={() => void run()}>
              {loading ? '生成中' : '重新生成'}
            </button>
          ) : null}
        </div>
        {error ? <span role="alert" style={errorStyle}>{error}</span> : null}
        {displayValue ? <div style={resultStyle}>{String(displayValue)}</div> : null}
        {readOnly ? null : (
          <button type="button" disabled={loading} style={acceptStyle} onClick={() => void run()}>
            {loading ? '运行中' : '运行 AI 预审'}
          </button>
        )}
      </div>
    </FieldFrame>
  )
}

const cardStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-sm)',
  background: 'var(--lh-purple-soft)',
  border: '1.5px dashed var(--lh-purple)',
  borderRadius: 'var(--lh-radius-lg)',
  padding: '16px 18px',
}

const headStyle: CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 'var(--space-sm)',
}

const titleStyle: CSSProperties = {
  color: 'var(--lh-purple)',
  fontWeight: 600,
  fontSize: 'var(--text-base)',
}

const regenStyle: CSSProperties = {
  marginLeft: 'auto',
  background: '#fff',
  border: '1px solid var(--lh-border)',
  borderRadius: 'var(--lh-radius)',
  padding: '4px 12px',
  fontSize: 12,
  color: 'var(--lh-text-2)',
  cursor: 'pointer',
}

const resultStyle: CSSProperties = {
  fontSize: 13,
  color: 'var(--lh-text-1)',
  lineHeight: 1.7,
  whiteSpace: 'pre-wrap',
}

const acceptStyle: CSSProperties = {
  justifySelf: 'start',
  background: 'var(--lh-purple)',
  color: '#fff',
  border: 'none',
  borderRadius: 'var(--lh-radius)',
  padding: '9px 18px',
  fontWeight: 500,
  fontFamily: 'var(--font-body)',
  cursor: 'pointer',
}

const errorStyle: CSSProperties = {
  color: 'var(--lh-danger)',
  fontSize: 'var(--text-sm)',
}
