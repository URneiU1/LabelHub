import type { CSSProperties } from 'react'
import { isSafeURL } from '../security/url'

export type RawPayload = Record<string, unknown>

export default function ShowItem({ payload }: { payload: RawPayload }) {
  const mediaType = textValue(payload.media_type)
  const mediaURL = textValue(payload.media_url)
  const markdown = textValue(payload.content_markdown)
  const dimensions = Array.isArray(payload.expected_dimensions) ? payload.expected_dimensions.map(String) : []

  return (
    <section style={panelStyle}>
      <div style={metaRowStyle}>
        <span>{textValue(payload.id) || '未命名题目'}</span>
        <span>{textValue(payload.category) || '未分类'}</span>
        <span>{mediaType || 'text'}</span>
      </div>

      <Field label="Prompt">
        <pre style={valueBlockStyle}>{textValue(payload.prompt) || textValue(payload.question) || '暂无 prompt'}</pre>
      </Field>

      {renderMedia(mediaType, mediaURL, markdown)}

      <Field label="模型回答">
        <pre style={valueBlockStyle}>{textValue(payload.model_answer) || '暂无回答'}</pre>
      </Field>

      {textValue(payload.reference) ? (
        <Field label="参考答案">
          <pre style={valueBlockStyle}>{textValue(payload.reference)}</pre>
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

function Field({ label, children }: { label: string, children: React.ReactNode }) {
  return (
    <div style={{ marginTop: 'var(--space-md)' }}>
      <div style={labelStyle}>{label}</div>
      {children}
    </div>
  )
}

function renderMedia(mediaType: string, mediaURL: string, markdown: string) {
  if (mediaType === 'video' && mediaURL) {
    return (
      <Field label="视频素材">
        {isSafeURL(mediaURL) ? <video controls src={mediaURL} style={mediaStyle} /> : <EmptyMedia />}
      </Field>
    )
  }
  if (mediaType === 'image' && mediaURL) {
    return (
      <Field label="图片素材">
        {isSafeURL(mediaURL) ? <img src={mediaURL} alt="任务素材" style={imageStyle} /> : <EmptyMedia />}
      </Field>
    )
  }
  if (mediaType === 'markdown' && markdown) {
    return (
      <Field label="Markdown 素材">
        <div style={markdownStyle}>{renderMarkdown(markdown)}</div>
      </Field>
    )
  }
  return null
}

function renderMarkdown(markdown: string) {
  return markdown.split('\n').map((line, index) => {
    const value = line.trim()
    if (!value) {
      return null
    }

    const video = value.match(/<video[^>]*src=["']([^"']+)["'][^>]*>/i)
    if (video && isSafeURL(video[1])) {
      return <video key={index} controls src={video[1]} style={mediaStyle} />
    }

    const image = value.match(/!\[([^\]]*)\]\(([^)]+)\)/)
    if (image && isSafeURL(image[2])) {
      return <img key={index} src={image[2]} alt={image[1] || 'markdown 图片'} style={imageStyle} />
    }

    const link = value.match(/\[([^\]]+)\]\(([^)]+)\)/)
    if (link && isSafeURL(link[2]) && link[2].endsWith('.mp4')) {
      return <video key={index} controls src={link[2]} style={mediaStyle} />
    }
    if (link && isSafeURL(link[2])) {
      return <a key={index} href={link[2]} target="_blank" rel="noreferrer">{link[1]}</a>
    }

    if (value.startsWith('### ')) {
      return <h4 key={index} style={markdownHeadingStyle}>{value.slice(4)}</h4>
    }
    if (value.startsWith('## ')) {
      return <h3 key={index} style={markdownHeadingStyle}>{value.slice(3)}</h3>
    }
    if (value.startsWith('# ')) {
      return <h2 key={index} style={markdownHeadingStyle}>{value.slice(2)}</h2>
    }
    return <p key={index} style={{ margin: 'var(--space-sm) 0' }}>{value}</p>
  })
}

function textValue(value: unknown) {
  return typeof value === 'string' ? value : ''
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

const valueBlockStyle: CSSProperties = {
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

const emptyMediaStyle: CSSProperties = {
  minHeight: 96,
  display: 'grid',
  placeItems: 'center',
  color: 'var(--color-text-secondary)',
  border: '1px dashed var(--color-border)',
  background: 'var(--color-bg)',
}

const markdownStyle: CSSProperties = {
  padding: 'var(--space-md)',
  background: 'var(--color-bg)',
  border: '1px solid var(--color-border-light)',
  lineHeight: 1.6,
}

const markdownHeadingStyle: CSSProperties = {
  fontFamily: 'var(--font-heading)',
  margin: 'var(--space-sm) 0',
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
