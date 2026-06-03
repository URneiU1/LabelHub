import type { CSSProperties, ReactNode } from 'react'
import { Markdown } from '../../shared/markdown'
import { isSafeURL } from '../../shared/security/url'
import { isRecord, resolvePath, textValue } from '../path'
import type { ShowItemMode, WidgetProps } from '../types'

export default function ShowItemWidget({ field, answer, payload }: WidgetProps) {
  const value = resolvePath(field.path, { payload, answer })
  const mode = resolveMode(field.mode, value)

  if (mode === 'json') {
    return <ValuePanel label={field.label}><pre style={preStyle}>{formatJSON(value)}</pre></ValuePanel>
  }
  if (mode === 'text') {
    return <ValuePanel label={field.label}><pre style={preStyle}>{textValue(value)}</pre></ValuePanel>
  }
  if (mode === 'video') {
    const src = mediaURL(value)
    return <ValuePanel label={field.label}>{isSafeURL(src) ? <video controls src={src} style={mediaStyle} /> : <EmptyMedia />}</ValuePanel>
  }
  if (mode === 'image') {
    const src = mediaURL(value)
    return <ValuePanel label={field.label}>{isSafeURL(src) ? <img src={src} alt={field.label} style={imageStyle} /> : <EmptyMedia />}</ValuePanel>
  }
  if (mode === 'markdown') {
    return <ValuePanel label={field.label}><div style={markdownStyle}><Markdown text={markdownText(value)} /></div></ValuePanel>
  }

  return <AutoPayloadView label={field.label} value={value} />
}

function AutoPayloadView({ label, value }: { label: string, value: unknown }) {
  if (!isRecord(value)) {
    return <ValuePanel label={label}><pre style={preStyle}>{textValue(value) || formatJSON(value)}</pre></ValuePanel>
  }

  const dimensions = Array.isArray(value.expected_dimensions) ? value.expected_dimensions.map(String) : []
  return (
    <section style={panelStyle}>
      <div style={metaRowStyle}>
        <span>{textValue(value.id) || '未命名题目'}</span>
        <span>{textValue(value.category) || '未分类'}</span>
        <span>{textValue(value.media_type) || 'text'}</span>
      </div>

      <Field label="Prompt">
        <pre style={preStyle}>{textValue(value.prompt) || textValue(value.question) || '暂无 prompt'}</pre>
      </Field>

      {renderMediaFromPayload(value)}

      <Field label="模型回答">
        <pre style={preStyle}>{textValue(value.model_answer) || '暂无回答'}</pre>
      </Field>

      {textValue(value.reference) ? (
        <Field label="参考答案">
          <pre style={preStyle}>{textValue(value.reference)}</pre>
        </Field>
      ) : null}

      {dimensions.length > 0 ? (
        <div style={tagRowStyle}>
          {dimensions.map((dimension) => <span key={dimension} style={tagStyle}>{dimension}</span>)}
        </div>
      ) : null}
    </section>
  )
}

function ValuePanel({ label, children }: { label: string, children: ReactNode }) {
  return (
    <section style={panelStyle}>
      <Field label={label}>{children}</Field>
    </section>
  )
}

function Field({ label, children }: { label: string, children: ReactNode }) {
  return (
    <div style={{ marginTop: 'var(--space-md)' }}>
      <div style={labelStyle}>{label}</div>
      {children}
    </div>
  )
}

function resolveMode(mode: ShowItemMode | undefined, value: unknown): ShowItemMode {
  if (mode && mode !== 'auto') {
    return mode
  }
  if (isRecord(value)) {
    const mediaType = textValue(value.media_type)
    if (mediaType === 'video' || mediaType === 'image' || mediaType === 'markdown') {
      return mediaType
    }
    return 'auto'
  }
  return typeof value === 'string' ? 'text' : 'json'
}

function renderMediaFromPayload(payload: Record<string, unknown>) {
  const mediaType = textValue(payload.media_type)
  const src = textValue(payload.media_url)
  const markdown = textValue(payload.content_markdown)
  if (mediaType === 'video' && src) {
    return <Field label="视频素材">{isSafeURL(src) ? <video controls src={src} style={mediaStyle} /> : <EmptyMedia />}</Field>
  }
  if (mediaType === 'image' && src) {
    return <Field label="图片素材">{isSafeURL(src) ? <img src={src} alt="任务素材" style={imageStyle} /> : <EmptyMedia />}</Field>
  }
  if (mediaType === 'markdown' && markdown) {
    return <Field label="Markdown 素材"><div style={markdownStyle}><Markdown text={markdown} /></div></Field>
  }
  return null
}

function mediaURL(value: unknown) {
  if (typeof value === 'string') {
    return value
  }
  if (isRecord(value)) {
    return textValue(value.media_url) || textValue(value.url)
  }
  return ''
}

function markdownText(value: unknown) {
  if (typeof value === 'string') {
    return value
  }
  if (isRecord(value)) {
    return textValue(value.content_markdown) || textValue(value.markdown)
  }
  return ''
}

function formatJSON(value: unknown) {
  if (value === undefined) {
    return ''
  }
  return JSON.stringify(value, null, 2)
}

function EmptyMedia() {
  return <div style={emptyMediaStyle}>未配置素材</div>
}

const panelStyle: CSSProperties = {
  background: 'var(--color-surface)',
  border: '1px solid var(--color-border)',
  padding: 'var(--space-lg)',
}

const metaRowStyle: CSSProperties = {
  display: 'flex',
  gap: 'var(--space-sm)',
  flexWrap: 'wrap',
  color: 'var(--color-text-secondary)',
  fontFamily: 'var(--font-body)',
  fontSize: 'var(--text-sm)',
}

const labelStyle: CSSProperties = {
  fontFamily: 'var(--font-heading)',
  fontWeight: 600,
  marginBottom: 'var(--space-xs)',
}

const preStyle: CSSProperties = {
  margin: 0,
  padding: 'var(--space-md)',
  background: 'var(--color-bg)',
  border: '1px solid var(--color-border-light)',
  whiteSpace: 'pre-wrap',
  overflow: 'auto',
  fontFamily: 'var(--font-body)',
  lineHeight: 1.6,
}

const mediaStyle: CSSProperties = {
  display: 'block',
  width: '100%',
  maxHeight: 360,
  background: '#000',
  border: '1px solid var(--color-border-light)',
}

const imageStyle: CSSProperties = {
  display: 'block',
  width: '100%',
  maxHeight: 360,
  objectFit: 'contain',
  border: '1px solid var(--color-border-light)',
  background: 'var(--color-bg)',
}

const markdownStyle: CSSProperties = {
  padding: 'var(--space-md)',
  background: 'var(--color-bg)',
  border: '1px solid var(--color-border-light)',
  lineHeight: 1.6,
}

const tagRowStyle: CSSProperties = {
  display: 'flex',
  gap: 'var(--space-sm)',
  flexWrap: 'wrap',
  marginTop: 'var(--space-md)',
}

const tagStyle: CSSProperties = {
  padding: '2px 8px',
  border: '1px solid var(--color-border-light)',
  background: 'var(--color-bg)',
  fontSize: 'var(--text-sm)',
}

const emptyMediaStyle: CSSProperties = {
  padding: 'var(--space-md)',
  border: '1px dashed var(--color-border-light)',
  color: 'var(--color-text-secondary)',
}
