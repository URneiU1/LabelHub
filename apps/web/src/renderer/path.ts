import type { AnswerValue, RenderPayload } from './types'

type PathRoot = {
  payload: RenderPayload
  answer: AnswerValue
}

export function resolvePath(path: string | undefined, root: PathRoot): unknown {
  if (!path || path === '$payload') {
    return root.payload
  }
  if (path === '$answer') {
    return root.answer
  }
  if (!path.startsWith('$')) {
    return undefined
  }
  const parts = path.slice(1).split('.').filter(Boolean)
  let current: unknown = root
  for (const part of parts) {
    if (!isRecord(current)) {
      return undefined
    }
    current = current[part]
  }
  return current
}

export function textValue(value: unknown) {
  if (typeof value === 'string') {
    return value
  }
  if (typeof value === 'number' || typeof value === 'boolean') {
    return String(value)
  }
  return ''
}

export function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}
