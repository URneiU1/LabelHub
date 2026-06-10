import type { CSSProperties, ReactNode } from 'react'
import { isSafeURL } from './security/url'

// 轻量 markdown 渲染(无第三方依赖):标题 / 段落 / 链接 / 图片 / 视频 / 引用 +
// 连续管道行聚成表格 + 行内 **粗体** 与 `代码`。供 ShowItem 与任务验收基线复用。
// 链接 / 媒体地址一律过 isSafeURL,杜绝 javascript: 等不安全协议。
export function Markdown({ text }: { text: string }): ReactNode {
  return <>{renderBlocks(text)}</>
}

function renderBlocks(markdown: string): ReactNode[] {
  const lines = markdown.split('\n')
  const out: ReactNode[] = []
  let i = 0
  while (i < lines.length) {
    const value = lines[i].trim()
    if (!value) {
      i += 1
      continue
    }
    if (isTableRow(value)) {
      const block: string[] = []
      while (i < lines.length && isTableRow(lines[i].trim())) {
        block.push(lines[i].trim())
        i += 1
      }
      out.push(renderTable(block, out.length))
      continue
    }
    out.push(renderLine(value, i))
    i += 1
  }
  return out
}

function isTableRow(value: string): boolean {
  return value.startsWith('|') && value.endsWith('|') && value.length > 1
}

function splitCells(row: string): string[] {
  return row.replace(/^\|/, '').replace(/\|$/, '').split('|').map((cell) => cell.trim())
}

function isSeparatorRow(cells: string[]): boolean {
  return cells.length > 0 && cells.every((cell) => /^:?-+:?$/.test(cell))
}

function renderTable(block: string[], key: number): ReactNode {
  const rows = block.map(splitCells)
  const hasSeparator = rows.length > 1 && isSeparatorRow(rows[1])
  const header = rows[0]
  const body = rows.slice(hasSeparator ? 2 : 1)
  return (
    <table key={`table-${key}`} style={tableStyle}>
      <thead>
        <tr>
          {header.map((cell, index) => <th key={index} style={thStyle}>{renderInline(cell)}</th>)}
        </tr>
      </thead>
      <tbody>
        {body.map((cells, rowIndex) => (
          <tr key={rowIndex}>
            {cells.map((cell, index) => <td key={index} style={tdStyle}>{renderInline(cell)}</td>)}
          </tr>
        ))}
      </tbody>
    </table>
  )
}

function renderLine(value: string, index: number): ReactNode {
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
    return <h4 key={index} style={headingStyle}>{renderInline(value.slice(4))}</h4>
  }
  if (value.startsWith('## ')) {
    return <h3 key={index} style={headingStyle}>{renderInline(value.slice(3))}</h3>
  }
  if (value.startsWith('# ')) {
    return <h2 key={index} style={headingStyle}>{renderInline(value.slice(2))}</h2>
  }
  if (value.startsWith('> ')) {
    return <blockquote key={index} style={quoteStyle}>{renderInline(value.slice(2))}</blockquote>
  }
  return <p key={index} style={paragraphStyle}>{renderInline(value)}</p>
}

// 行内 **粗体** 与 `代码`;其余原样输出。
function renderInline(text: string): ReactNode {
  const parts = text.split(/(\*\*[^*]+\*\*|`[^`]+`)/g)
  return parts.map((part, index) => {
    if (part.startsWith('**') && part.endsWith('**')) {
      return <strong key={index}>{part.slice(2, -2)}</strong>
    }
    if (part.startsWith('`') && part.endsWith('`')) {
      return <code key={index} style={codeStyle}>{part.slice(1, -1)}</code>
    }
    return part
  })
}

const headingStyle: CSSProperties = {
  fontFamily: 'var(--font-heading)',
  margin: 'var(--space-md) 0 var(--space-sm)',
}

const paragraphStyle: CSSProperties = {
  margin: 'var(--space-sm) 0',
  lineHeight: 1.6,
}

const quoteStyle: CSSProperties = {
  margin: 'var(--space-sm) 0',
  padding: '4px var(--space-md)',
  borderLeft: '3px solid var(--color-border)',
  color: 'var(--color-text-secondary)',
}

const tableStyle: CSSProperties = {
  borderCollapse: 'collapse',
  width: '100%',
  margin: 'var(--space-sm) 0',
  fontSize: 'var(--text-sm)',
}

const thStyle: CSSProperties = {
  border: '1px solid var(--color-border)',
  padding: '6px 10px',
  textAlign: 'left',
  background: 'var(--color-bg)',
  fontWeight: 600,
}

const tdStyle: CSSProperties = {
  border: '1px solid var(--color-border-light)',
  padding: '6px 10px',
  textAlign: 'left',
}

const codeStyle: CSSProperties = {
  fontFamily: 'var(--lh-font-mono, monospace)',
  background: 'var(--color-bg)',
  padding: '1px 5px',
  borderRadius: 3,
  fontSize: '0.92em',
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
