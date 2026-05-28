const SAFE_URL_PROTOCOLS = new Set(['http:', 'https:'])

export function isSafeURL(value: string) {
  const trimmed = value.trim()
  if (!trimmed) {
    return false
  }

  // 协议相对 URL(//host/path)会被解析成当前协议指向外部主机,
  // 据此可加载外链资源、泄漏 Referer —— 直接拒绝。
  if (trimmed.startsWith('//')) {
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
