export type RawPayload = Record<string, unknown>

export function parsePayload(raw?: string): RawPayload {
  if (!raw) {
    return {}
  }
  try {
    const value = JSON.parse(raw)
    return typeof value === 'object' && value !== null ? value as RawPayload : { value }
  } catch {
    return { raw }
  }
}
