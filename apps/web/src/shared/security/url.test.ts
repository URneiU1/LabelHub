import { describe, expect, it } from 'vitest'
import { isSafeURL } from './url'

describe('isSafeURL', () => {
  it('allows http, https, and relative URLs', () => {
    expect(isSafeURL('https://example.com/a.png')).toBe(true)
    expect(isSafeURL('http://example.com/a.mp4')).toBe(true)
    expect(isSafeURL('/uploads/task/file.png')).toBe(true)
  })

  it('blocks scriptable and local-only schemes', () => {
    expect(isSafeURL('javascript:alert(1)')).toBe(false)
    expect(isSafeURL('data:text/html,<script>alert(1)</script>')).toBe(false)
    expect(isSafeURL('file:///etc/passwd')).toBe(false)
  })
})
