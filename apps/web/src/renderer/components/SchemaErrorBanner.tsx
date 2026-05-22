import type { CSSProperties } from 'react'
import type { SchemaParseError } from '../types'

type SchemaErrorBannerProps = {
  error: SchemaParseError
  role: 'owner' | 'labeler' | 'reviewer'
}

export default function SchemaErrorBanner({ error, role }: SchemaErrorBannerProps) {
  const message = role === 'owner'
    ? `Schema 解析失败: ${error.field} ${error.message}`
    : '标注模板配置异常,请联系任务管理员。'
  return (
    <div role="alert" style={bannerStyle}>
      {message}
    </div>
  )
}

const bannerStyle: CSSProperties = {
  padding: 'var(--space-md)',
  border: '1px solid #f5b5ac',
  background: '#fff1ef',
  color: '#9f1d14',
}
