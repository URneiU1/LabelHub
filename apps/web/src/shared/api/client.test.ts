import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  ApiError,
  addAssignees,
  batchUpdateItems,
  createTask,
  importItems,
  importItemsFile,
  listAssignees,
  listTaskItems,
  previewItem,
  removeAssignee,
  transitionTask,
  updateTask,
} from './client'

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

function okResponse(data: unknown) {
  return {
    ok: true,
    json: async () => ({ data, request_id: 'req-ok' }),
  } as unknown as Response
}

function errorResponse(code: string, message: string) {
  return {
    ok: false,
    json: async () => ({ error: { code, message }, request_id: 'req-err' }),
  } as unknown as Response
}

describe('task management client wrappers', () => {
  let fetchMock: ReturnType<typeof vi.fn>

  beforeEach(() => {
    localStorage.setItem('labelhub_access_token', 'token-123')
    fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    localStorage.clear()
  })

  it('createTask POSTs serialized JSON task info and unwraps the bare task', async () => {
    fetchMock.mockResolvedValue(okResponse({ id: 7, title: 'New', status: 'draft', totalItems: 0, finishedItems: 0, description: null, baselineDescription: null }))

    const task = await createTask({
      title: 'New',
      tags: ['a', 'b'],
      rewardConfig: { amount: 0.3, unit: '元/条' },
      distribution: 'quota',
      quotaPerUser: 100,
      overlapCount: 3,
      overlapCoveragePct: 50,
      leaseTimeoutMinutes: 45,
      reviewSamplingPct: 20,
      dailySubmissionLimitPerLabeler: 12,
      humanReviewEnabled: true,
      deadline: '2026-06-01T15:59:00.000Z',
    })

    expect(task.id).toBe(7)
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/api/v1/tasks')
    expect(init.method).toBe('POST')
    const body = JSON.parse(init.body as string)
    expect(body).toMatchObject({
      title: 'New',
      tags: ['a', 'b'],
      rewardConfig: { amount: 0.3, unit: '元/条' },
      distribution: 'quota',
      quotaPerUser: 100,
      overlapCount: 3,
      overlapCoveragePct: 50,
      leaseTimeoutMinutes: 45,
      reviewSamplingPct: 20,
      dailySubmissionLimitPerLabeler: 12,
      humanReviewEnabled: true,
      deadline: '2026-06-01T15:59:00.000Z',
    })
  })

  it('updateTask PUTs only provided fields and unwraps { task }', async () => {
    fetchMock.mockResolvedValue(okResponse({ task: { id: 7, title: 'Edited', status: 'draft', totalItems: 0, finishedItems: 0, description: null, baselineDescription: null } }))

    const task = await updateTask(7, { title: 'Edited', deadline: null })

    expect(task.title).toBe('Edited')
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/api/v1/tasks/7')
    expect(init.method).toBe('PUT')
    const body = JSON.parse(init.body as string)
    expect(body).toEqual({ title: 'Edited', deadline: null })
  })

  it('transitionTask POSTs the action path and unwraps { task }', async () => {
    fetchMock.mockResolvedValue(okResponse({ task: { id: 7, title: 'T', status: 'published', totalItems: 0, finishedItems: 0, description: null, baselineDescription: null } }))

    const task = await transitionTask(7, 'publish')

    expect(task.status).toBe('published')
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/tasks/7/publish')
  })

  it('transitionTask surfaces a friendly ApiError on 422 INVALID_STATE', async () => {
    fetchMock.mockResolvedValue(errorResponse('INVALID_STATE', 'task must have a bound template before publishing'))

    await expect(transitionTask(7, 'publish')).rejects.toMatchObject({
      code: 'INVALID_STATE',
    })
    await expect(transitionTask(7, 'publish')).rejects.toThrow('刷新')
  })

  it('importItemsFile sends multipart form with file and optional format', async () => {
    fetchMock.mockResolvedValue(okResponse({ imported: 3, format: 'jsonl' }))
    const file = new File(['{"id":"a"}\n'], 'data.jsonl', { type: 'application/jsonl' })

    const result = await importItemsFile(7, file, 'jsonl')

    expect(result).toEqual({ imported: 3, format: 'jsonl' })
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/api/v1/tasks/7/items/import-file')
    expect(init.method).toBe('POST')
    expect(init.body).toBeInstanceOf(FormData)
    const form = init.body as FormData
    expect(form.get('file')).toBeInstanceOf(File)
    expect(form.get('format')).toBe('jsonl')
  })

  it('importItems POSTs { items } JSON', async () => {
    fetchMock.mockResolvedValue(okResponse({ imported: 2 }))

    const result = await importItems(7, [{ id: 'a' }, { id: 'b' }])

    expect(result.imported).toBe(2)
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/api/v1/tasks/7/items/import')
    expect(JSON.parse(init.body as string)).toEqual({ items: [{ id: 'a' }, { id: 'b' }] })
  })

  it('batchUpdateItems POSTs { items: [{itemId, payload}] }', async () => {
    fetchMock.mockResolvedValue(okResponse({ updated: 1, requested: 1 }))

    const result = await batchUpdateItems(7, [{ itemId: 9, payload: { text: 'x' } }])

    expect(result).toEqual({ updated: 1, requested: 1 })
    expect(JSON.parse(fetchMock.mock.calls[0][1].body as string)).toEqual({ items: [{ itemId: 9, payload: { text: 'x' } }] })
  })

  it('previewItem GETs item-preview and unwraps { item }', async () => {
    fetchMock.mockResolvedValue(okResponse({ item: { id: 9, externalId: 'a', payload: { text: 'x' } } }))

    const item = await previewItem(7)

    expect(item).toEqual({ id: 9, externalId: 'a', payload: { text: 'x' } })
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/tasks/7/item-preview')
  })

  it('assignee wrappers hit the assignees endpoints', async () => {
    fetchMock.mockResolvedValueOnce(okResponse({ assignees: [{ userId: 5, assignedAt: null }] }))
    const listed = await listAssignees(7)
    expect(listed).toEqual([{ userId: 5, assignedAt: null }])
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/tasks/7/assignees')

    fetchMock.mockResolvedValueOnce(okResponse({ added: 1 }))
    await addAssignees(7, [5])
    expect(fetchMock.mock.calls[1][0]).toBe('/api/v1/tasks/7/assignees')
    expect(JSON.parse(fetchMock.mock.calls[1][1].body as string)).toEqual({ userIds: [5] })

    fetchMock.mockResolvedValueOnce(okResponse({ removed: 5 }))
    await removeAssignee(7, 5)
    expect(fetchMock.mock.calls[2][0]).toBe('/api/v1/tasks/7/assignees/5')
    expect(fetchMock.mock.calls[2][1].method).toBe('DELETE')
  })

  it('listTaskItems GETs items with limit and maps the page envelope', async () => {
    fetchMock.mockResolvedValue({
      ok: true,
      json: async () => ({
        data: [{ id: 10, externalId: 'Q10', status: 'available', priority: 0, payload: { text: 'x' } }],
        page: { next_cursor: '10', has_more: true },
        request_id: 'req-ok',
      }),
    } as unknown as Response)

    const result = await listTaskItems(7, { limit: 50 })

    expect(result.items).toEqual([{ id: 10, externalId: 'Q10', status: 'available', priority: 0, payload: { text: 'x' } }])
    expect(result.nextCursor).toBe('10')
    expect(result.hasMore).toBe(true)
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/tasks/7/items?limit=50')
  })

  it('listTaskItems forwards the cursor and defaults has_more to false', async () => {
    fetchMock.mockResolvedValue({
      ok: true,
      json: async () => ({ data: [], request_id: 'req-ok' }),
    } as unknown as Response)

    const result = await listTaskItems(7, { cursor: '12', limit: 20 })

    expect(result.items).toEqual([])
    expect(result.hasMore).toBe(false)
    expect(result.nextCursor).toBe('')
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/tasks/7/items?cursor=12&limit=20')
  })
})
