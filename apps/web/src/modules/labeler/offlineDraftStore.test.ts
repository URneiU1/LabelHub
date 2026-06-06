import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  buildDraftKey,
  discardLocalDraft,
  isLocalDraftNewer,
  loadLocalDraft,
  markSynced,
  saveLocalDraft,
} from './offlineDraftStore'

describe('offlineDraftStore', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.useRealTimers()
  })

  it('builds a stable key from the four-part coordinate, with "new" for missing ids', () => {
    expect(buildDraftKey({ taskId: 1, itemId: 11, submissionId: 42, revisionNo: 3 }))
      .toBe('labelhub_offline_draft:1:11:42:3')
    expect(buildDraftKey({ taskId: 1, itemId: 11 }))
      .toBe('labelhub_offline_draft:1:11:new:new')
    expect(buildDraftKey({ taskId: 1, itemId: 11, submissionId: null, revisionNo: null }))
      .toBe('labelhub_offline_draft:1:11:new:new')
  })

  it('saves a draft as unsynced and loads it back', () => {
    const key = buildDraftKey({ taskId: 1, itemId: 11 })
    saveLocalDraft(key, { summary: '本地答案' }, 7)

    const draft = loadLocalDraft(key)
    expect(draft).not.toBeNull()
    expect(draft!.answer).toEqual({ summary: '本地答案' })
    expect(draft!.templateVersion).toBe(7)
    expect(draft!.synced).toBe(false)
    expect(typeof draft!.updatedAt).toBe('number')
  })

  it('returns null when loading a missing key', () => {
    expect(loadLocalDraft(buildDraftKey({ taskId: 9, itemId: 9 }))).toBeNull()
  })

  it('markSynced flips synced to true without mutating other fields', () => {
    const key = buildDraftKey({ taskId: 1, itemId: 11 })
    saveLocalDraft(key, { summary: 'a' }, 7)
    const before = loadLocalDraft(key)!
    markSynced(key)
    const after = loadLocalDraft(key)!

    expect(after.synced).toBe(true)
    expect(after.answer).toEqual(before.answer)
    expect(after.templateVersion).toBe(before.templateVersion)
    expect(after.updatedAt).toBe(before.updatedAt)
  })

  it('markSynced is a no-op when the draft does not exist', () => {
    const key = buildDraftKey({ taskId: 1, itemId: 11 })
    expect(() => markSynced(key)).not.toThrow()
    expect(loadLocalDraft(key)).toBeNull()
  })

  it('discardLocalDraft removes the stored draft', () => {
    const key = buildDraftKey({ taskId: 1, itemId: 11 })
    saveLocalDraft(key, { summary: 'a' }, 7)
    discardLocalDraft(key)
    expect(loadLocalDraft(key)).toBeNull()
  })

  describe('isLocalDraftNewer', () => {
    it('returns false for a missing draft', () => {
      expect(isLocalDraftNewer(null, 7)).toBe(false)
    })

    it('returns false when the draft is already synced', () => {
      const key = buildDraftKey({ taskId: 1, itemId: 11 })
      saveLocalDraft(key, { summary: 'a' }, 7)
      markSynced(key)
      expect(isLocalDraftNewer(loadLocalDraft(key), 7)).toBe(false)
    })

    it('returns false when template versions differ', () => {
      const key = buildDraftKey({ taskId: 1, itemId: 11 })
      saveLocalDraft(key, { summary: 'a' }, 7)
      expect(isLocalDraftNewer(loadLocalDraft(key), 8)).toBe(false)
    })

    it('returns true for an unsynced draft on the matching template version', () => {
      const key = buildDraftKey({ taskId: 1, itemId: 11 })
      saveLocalDraft(key, { summary: 'a' }, 7)
      expect(isLocalDraftNewer(loadLocalDraft(key), 7)).toBe(true)
    })

    it('respects an explicit server timestamp when provided', () => {
      const key = buildDraftKey({ taskId: 1, itemId: 11 })
      vi.useFakeTimers()
      vi.setSystemTime(1_000)
      saveLocalDraft(key, { summary: 'a' }, 7)
      const draft = loadLocalDraft(key)
      // 本地 updatedAt=1000:服务端更晚 → 非更新;服务端更早 → 更新。
      expect(isLocalDraftNewer(draft, 7, 2_000)).toBe(false)
      expect(isLocalDraftNewer(draft, 7, 500)).toBe(true)
    })
  })

  describe('localStorage failure tolerance', () => {
    it('saveLocalDraft does not throw when setItem rejects (quota / disabled)', () => {
      vi.spyOn(window.localStorage, 'setItem').mockImplementation(() => {
        throw new Error('QuotaExceededError')
      })
      const key = buildDraftKey({ taskId: 1, itemId: 11 })
      expect(() => saveLocalDraft(key, { summary: 'a' }, 7)).not.toThrow()
    })

    it('loadLocalDraft returns null when getItem throws', () => {
      vi.spyOn(window.localStorage, 'getItem').mockImplementation(() => {
        throw new Error('storage disabled')
      })
      expect(loadLocalDraft(buildDraftKey({ taskId: 1, itemId: 11 }))).toBeNull()
    })

    it('loadLocalDraft returns null for corrupt JSON', () => {
      const key = buildDraftKey({ taskId: 1, itemId: 11 })
      window.localStorage.setItem(key, '{not valid json')
      expect(loadLocalDraft(key)).toBeNull()
    })

    it('discardLocalDraft does not throw when removeItem rejects', () => {
      vi.spyOn(window.localStorage, 'removeItem').mockImplementation(() => {
        throw new Error('storage disabled')
      })
      expect(() => discardLocalDraft(buildDraftKey({ taskId: 1, itemId: 11 }))).not.toThrow()
    })
  })
})
