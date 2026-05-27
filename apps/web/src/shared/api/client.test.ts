import { describe, expect, it } from 'vitest'
import { ApiError } from './client'

describe('ApiError', () => {
  it('is an Error subclass carrying code and requestId', () => {
    const err = new ApiError('INVALID_STATE', 'invalid transition', 'req-1')
    expect(err).toBeInstanceOf(Error)
    expect(err.code).toBe('INVALID_STATE')
    expect(err.requestId).toBe('req-1')
  })

  it('maps INVALID_STATE to a refresh-and-retry hint', () => {
    const err = new ApiError('INVALID_STATE', 'invalid transition', 'req-1')
    expect(err.message).toContain('刷新')
  })

  it('maps LLM_PROVIDER_ERROR to a retry-or-human-review hint', () => {
    const err = new ApiError('LLM_PROVIDER_ERROR', 'upstream 503', 'req-2')
    expect(err.message).toContain('AI 服务')
    expect(err.message).toContain('人工')
  })

  it('keeps the backend message for unmapped codes', () => {
    const err = new ApiError('VALIDATION_ERROR', 'title is required', 'req-3')
    expect(err.message).toBe('title is required')
  })

  it('falls back to a default message when the backend message is empty', () => {
    const err = new ApiError('UNKNOWN', '', 'req-4')
    expect(err.message).toBe('请求失败')
  })
})
