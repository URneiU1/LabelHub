const SAFE_URL_PROTOCOLS = new Set(['http:', 'https:'])

export function isSafeURL(value: string) {
  const trimmed = value.trim()
  if (!trimmed) {
    return false
  }

  try {
    const base = typeof window === 'undefined' ? 'http://localhost' : window.location.origin
    const url = new URL(trimmed, base)
    return SAFE_URL_PROTOCOLS.has(url.protocol)
  } catch {
    return false
  }
}
