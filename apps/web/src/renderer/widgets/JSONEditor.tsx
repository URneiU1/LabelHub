import { useState, type CSSProperties } from 'react'
import type { WidgetProps } from '../types'
import FieldFrame from './FieldFrame'
import { inputStyle } from './styles'

export default function JSONEditorWidget({ field, value, readOnly, onChange }: WidgetProps) {
  // 解析失败时仍把原文回写(不丢用户输入),但显示内联错误提示,避免「非法 JSON 被当成
  // 普通字符串静默提交」——非空字符串能绕过 required 校验,用户却毫无察觉。
  const [parseError, setParseError] = useState(false)

  function normalize(next: string) {
    if (next.trim() === '') {
      setParseError(false)
      onChange(field.name, '')
      return
    }
    try {
      onChange(field.name, JSON.parse(next) as unknown)
      setParseError(false)
    } catch {
      onChange(field.name, next)
      setParseError(true)
    }
  }

  return (
    <FieldFrame label={field.label} required={field.required}>
      <textarea
        aria-label={field.label}
        value={stringifyValue(value)}
        disabled={readOnly}
        rows={7}
        spellCheck={false}
        style={jsonStyle}
        onBlur={(event) => normalize(event.target.value)}
        onChange={(event) => onChange(field.name, event.target.value)}
      />
      {parseError ? <div role="alert" style={errorStyle}>不是合法 JSON，请修正后再提交</div> : null}
    </FieldFrame>
  )
}

function stringifyValue(value: unknown) {
  if (value === undefined || value === null || value === '') {
    return '{\n  \n}'
  }
  if (typeof value === 'string') {
    return value
  }
  return JSON.stringify(value, null, 2)
}

const jsonStyle: CSSProperties = {
  ...inputStyle,
  resize: 'vertical',
  lineHeight: 1.6,
  minHeight: 160,
  fontFamily: 'var(--font-mono)',
}

const errorStyle: CSSProperties = {
  marginTop: 4,
  fontSize: 12,
  color: 'var(--lh-danger)',
}
