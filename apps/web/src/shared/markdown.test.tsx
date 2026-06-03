import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Markdown } from './markdown'

describe('Markdown', () => {
  it('renders ## as a heading', () => {
    render(<Markdown text={'## 任务背景'} />)
    expect(screen.getByRole('heading', { name: '任务背景' })).toBeInTheDocument()
  })

  it('renders a pipe block as a table and skips the separator row', () => {
    render(<Markdown text={'| 字段 | 含义 |\n|------|------|\n| id | 编号 |'} />)
    expect(screen.getByRole('table')).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: '字段' })).toBeInTheDocument()
    expect(screen.getByRole('cell', { name: '编号' })).toBeInTheDocument()
    // 分隔行 |---| 不应作为数据行出现
    expect(screen.queryByText('------')).not.toBeInTheDocument()
  })

  it('renders **bold** inline as <strong>', () => {
    render(<Markdown text={'这是 **重点** 内容'} />)
    expect(screen.getByText('重点').tagName).toBe('STRONG')
  })

  it('renders a blockquote line', () => {
    render(<Markdown text={'> 先看素材再标注'} />)
    expect(screen.getByText('先看素材再标注')).toBeInTheDocument()
  })

  it('drops unsafe links instead of rendering an anchor', () => {
    render(<Markdown text={'[x](javascript:alert(1))'} />)
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })
})
