type ApiEnvelope<T> = {
  data: T
  request_id: string
}

type ApiErrorEnvelope = {
  error: {
    code: string
    message: string
  }
  request_id: string
}

const TOKEN_KEY = 'labelhub_access_token'
const USER_KEY = 'labelhub_current_user'

export function getToken() {
  return localStorage.getItem(TOKEN_KEY)
}

export function getCurrentUser(): DemoUser | null {
  const raw = localStorage.getItem(USER_KEY)
  if (!raw) {
    return null
  }

  try {
    const user = JSON.parse(raw) as DemoUser
    return Array.isArray(user.roles) ? user : null
  } catch {
    return null
  }
}

export function hasAnyRole(allowedRoles: string[]) {
  const user = getCurrentUser()
  return Boolean(user?.roles.some((role) => allowedRoles.includes(role)))
}

export function clearToken() {
  localStorage.removeItem(TOKEN_KEY)
  localStorage.removeItem(USER_KEY)
}

export async function login(username: string, password: string) {
  const data = await apiPost<{ tokens: { accessToken: string }, user: DemoUser }>('/auth/login', { username, password }, false)
  localStorage.setItem(TOKEN_KEY, data.tokens.accessToken)
  localStorage.setItem(USER_KEY, JSON.stringify(data.user))
  return data.user
}

// JWT 无服务端撤销,登出仅清本地 token + 通知后端记录(后端 Sprint 5 后会做真撤销)
export async function logout() {
  try {
    await apiPost('/auth/logout', {})
  } catch {
    // 即使后端不可达也要清本地 token
  }
  clearToken()
}

export async function apiGet<T>(path: string) {
  return request<T>(path, { method: 'GET' })
}

export async function apiPost<T>(path: string, body: Record<string, unknown> | FormData, auth = true) {
  return request<T>(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: body instanceof FormData ? body : JSON.stringify(body),
  }, auth)
}

export async function apiPostRawJSON<T>(path: string, body: string, auth = true) {
  return request<T>(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body,
  }, auth)
}

export async function apiDelete<T>(path: string) {
  return request<T>(path, { method: 'DELETE' })
}

export async function apiUpload<T>(path: string, body: FormData) {
  const headers = new Headers()
  const token = getToken()
  if (!token) {
    throw new Error('请先登录')
  }
  headers.set('Authorization', `Bearer ${token}`)

  const response = await fetch(`/api/v1${path}`, { method: 'POST', headers, body })
  const payload = await response.json() as ApiEnvelope<T> | ApiErrorEnvelope
  if (!response.ok) {
    const errorPayload = payload as ApiErrorEnvelope
    throw new Error(errorPayload.error?.message || '上传失败')
  }
  return (payload as ApiEnvelope<T>).data
}

async function request<T>(path: string, init: RequestInit, auth = true): Promise<T> {
  const headers = new Headers(init.headers)
  if (auth) {
    const token = getToken()
    if (!token) {
      throw new Error('请先登录')
    }
    headers.set('Authorization', `Bearer ${token}`)
  }

  const response = await fetch(`/api/v1${path}`, { ...init, headers })
  const payload = await response.json() as ApiEnvelope<T> | ApiErrorEnvelope
  if (!response.ok) {
    const errorPayload = payload as ApiErrorEnvelope
    throw new Error(errorPayload.error?.message || '请求失败')
  }
  return (payload as ApiEnvelope<T>).data
}

export type DemoUser = {
  id: number
  username: string
  displayName: string
  roles: string[]
}

export type Task = {
  id: number
  title: string
  description: string | null
  baselineDescription: string | null
  status: string
  templateId?: number | null
  totalItems: number
  finishedItems: number
  aiPromptId?: number | null
  aiReviewEnabled?: boolean
}

export type TaskItem = {
  id: number
  taskId: number
  externalId: string | null
  payload: string
  status: string
}

export type Submission = {
  id: number
  taskId: number
  itemId: number
  status: string
  aiVerdict?: string | null
  aiScore?: number | null
  humanVerdict?: string
  currentRevisionId?: number
}

export type TaskTemplate = {
  id: number
  taskId?: number
  version?: number
  schemaJson: string
  schemaHash?: string
  createdBy?: number
  createdAt?: string
}

export type SubmissionRevision = {
  id: number
  answer: string
  draft: boolean
}

export type AIPromptSummary = {
  id: number
  version: number
  model: string
  promptTemplate: string
  dimensions: unknown
  passThreshold: number
  uncertainMin: number
}

export type AIReviewDetail = {
  id: number
  submissionId: number
  revisionId: number
  idempotencyKey: string
  promptVersion: number
  verdict?: string | null
  overallScore?: number | null
  dimensions?: unknown
  reason?: string | null
  rawResponse?: unknown
  tokensInput: number
  tokensOutput: number
  latencyMs: number
  status: string
  retryCount: number
  errorMsg?: string | null
  createdAt: string
  finishedAt?: unknown
  prompt?: AIPromptSummary | null
}

export type AuditLog = {
  id: number
  entityType: string
  entityId: number
  fromState?: { String?: string, Valid?: boolean } | null
  toState: string
  actorType: string
  actorId?: number | null
  event: string
  payload?: unknown
  createdAt: string
}

export type TaskBundle = {
  task: Task
  item?: TaskItem
  template?: TaskTemplate
  submission?: Submission
  revision?: SubmissionRevision | null
  aiReview?: AIReviewDetail | null
  auditLogs?: AuditLog[]
}
