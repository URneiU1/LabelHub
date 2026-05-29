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

// 对少数会让标注/审核流程卡住的错误码,给出可执行的中文提示;
// 其余错误码沿用后端 message,避免覆盖已有的具体说明。
const FRIENDLY_ERROR_BY_CODE: Record<string, string> = {
  INVALID_STATE: '该记录的状态已被其他人改动,请刷新后再操作。',
  LLM_PROVIDER_ERROR: 'AI 服务暂时不可用,可稍后重试,或直接转人工审核。',
}

export class ApiError extends Error {
  code: string
  requestId: string

  constructor(code: string, backendMessage: string, requestId: string) {
    super(FRIENDLY_ERROR_BY_CODE[code] || backendMessage || '请求失败')
    this.name = 'ApiError'
    this.code = code
    this.requestId = requestId
  }
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
    throw new ApiError(errorPayload.error?.code ?? 'UNKNOWN', errorPayload.error?.message ?? '上传失败', errorPayload.request_id ?? '')
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
    throw new ApiError(errorPayload.error?.code ?? 'UNKNOWN', errorPayload.error?.message ?? '请求失败', errorPayload.request_id ?? '')
  }
  return (payload as ApiEnvelope<T>).data
}

export type DemoUser = {
  id: number
  username: string
  displayName: string
  roles: string[]
}

export type TaskDistribution = 'first_come' | 'assigned' | 'quota'

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
  richDescription?: string | null
  tags?: string | null
  rewardConfig?: string | null
  distribution?: string
  quotaPerUser?: number
  deadline?: string | null
  publishedAt?: string | null
}

// 任务基础信息 create/update 的输入。后端会把 JSON 字段(richDescription/tags/rewardConfig)
// 原样存进 json 列,所以这里用结构化值,提交前序列化成 JSON。
export type TaskInfoInput = {
  title?: string
  description?: string | null
  richDescription?: unknown
  tags?: unknown
  rewardConfig?: unknown
  distribution?: TaskDistribution
  quotaPerUser?: number
  deadline?: string | null
}

export type TaskAssigneeView = {
  userId: number
  assignedAt: string | null
}

export type ImportItemsFileResult = { imported: number, format: string }
export type ImportItemsResult = { imported: number }
export type BatchUpdateItemsResult = { updated: number, requested: number }
export type AddAssigneesResult = { added: number }
export type RemoveAssigneeResult = { removed: number }

export type PreviewItem = {
  id: number
  externalId: string | null
  payload: unknown
}

// --- 4.1 任务管理后台的端点封装 ---

// 创建任务:返回裸 task(后端 CreateTask 直接 httpx.OK(task))。
export async function createTask(input: TaskInfoInput) {
  return apiPost<Task>('/tasks', buildTaskInfoBody(input))
}

// 更新任务基础信息:后端返回 { task }。
export async function updateTask(taskId: number, input: TaskInfoInput) {
  const data = await request<{ task: Task }>(`/tasks/${taskId}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(buildTaskInfoBody(input)),
  })
  return data.task
}

function buildTaskInfoBody(input: TaskInfoInput): Record<string, unknown> {
  const body: Record<string, unknown> = {}
  if (input.title !== undefined) body.title = input.title
  if (input.description !== undefined) body.description = input.description
  if (input.richDescription !== undefined) body.richDescription = input.richDescription
  if (input.tags !== undefined) body.tags = input.tags
  if (input.rewardConfig !== undefined) body.rewardConfig = input.rewardConfig
  if (input.distribution !== undefined) body.distribution = input.distribution
  if (input.quotaPerUser !== undefined) body.quotaPerUser = input.quotaPerUser
  if (input.deadline !== undefined) body.deadline = input.deadline
  return body
}

// 任务状态机迁移:draft→published→paused→published/ended。非法迁移后端返回 422 INVALID_STATE。
export async function transitionTask(taskId: number, action: 'publish' | 'pause' | 'resume' | 'end') {
  const data = await apiPost<{ task: Task }>(`/tasks/${taskId}/${action}`, {})
  return data.task
}

// 文件导入题目:multipart form(file + 可选 format)。复用 apiUpload。
export async function importItemsFile(taskId: number, file: File, format?: 'json' | 'jsonl' | 'xlsx') {
  const form = new FormData()
  form.append('file', file)
  if (format) {
    form.append('format', format)
  }
  return apiUpload<ImportItemsFileResult>(`/tasks/${taskId}/items/import-file`, form)
}

// JSON 直接导入:{ items: [...] }。
export async function importItems(taskId: number, items: Array<Record<string, unknown>>) {
  return apiPost<ImportItemsResult>(`/tasks/${taskId}/items/import`, { items })
}

// 批量覆盖题目 payload。
export async function batchUpdateItems(taskId: number, items: Array<{ itemId: number, payload: unknown }>) {
  return apiPost<BatchUpdateItemsResult>(`/tasks/${taskId}/items/batch-update`, { items })
}

// 随机/首条可用题目预览。无可用题目时后端返回 { item: null }。
export async function previewItem(taskId: number) {
  const data = await apiGet<{ item: PreviewItem | null }>(`/tasks/${taskId}/item-preview`)
  return data.item
}

export type OwnerTaskItem = {
  id: number
  externalId: string | null
  status: string
  priority: number
  payload: unknown
}

export type OwnerTaskItemsPage = {
  items: OwnerTaskItem[]
  nextCursor: string
  hasMore: boolean
}

// Owner 批量编辑列表:游标分页列出某任务全部题目(含状态/payload)。后端用 PageOK 返回
// { data, page:{ next_cursor, has_more } },标准 apiGet 会丢掉 page,这里直接读 envelope 保留游标。
export async function listTaskItems(taskId: number, params?: { cursor?: string, limit?: number }): Promise<OwnerTaskItemsPage> {
  const query = new URLSearchParams()
  if (params?.cursor) {
    query.set('cursor', params.cursor)
  }
  query.set('limit', String(params?.limit ?? 50))
  const token = getToken()
  if (!token) {
    throw new Error('请先登录')
  }
  const response = await fetch(`/api/v1/tasks/${taskId}/items?${query.toString()}`, {
    method: 'GET',
    headers: { Authorization: `Bearer ${token}` },
  })
  const payload = await response.json() as
    | { data: OwnerTaskItem[], page?: { next_cursor?: string, has_more?: boolean }, request_id?: string }
    | ApiErrorEnvelope
  if (!response.ok) {
    const errorPayload = payload as ApiErrorEnvelope
    throw new ApiError(errorPayload.error?.code ?? 'UNKNOWN', errorPayload.error?.message ?? '加载题目列表失败', errorPayload.request_id ?? '')
  }
  const pagePayload = payload as { data: OwnerTaskItem[], page?: { next_cursor?: string, has_more?: boolean } }
  return {
    items: pagePayload.data ?? [],
    nextCursor: pagePayload.page?.next_cursor ?? '',
    hasMore: Boolean(pagePayload.page?.has_more),
  }
}

// 指派管理(distribution='assigned')。
export async function listAssignees(taskId: number) {
  const data = await apiGet<{ assignees: TaskAssigneeView[] }>(`/tasks/${taskId}/assignees`)
  return data.assignees
}

export async function addAssignees(taskId: number, userIds: number[]) {
  return apiPost<AddAssigneesResult>(`/tasks/${taskId}/assignees`, { userIds })
}

export async function removeAssignee(taskId: number, userId: number) {
  return apiDelete<RemoveAssigneeResult>(`/tasks/${taskId}/assignees/${userId}`)
}

export type TaskItem = {
  id: number
  taskId: number
  externalId: string | null
  payload: string
  status: string
}

// 人工审核多级阶段:后端按当前 revision 的 approve 计数派生。
// reviewStage first/second/final 对应 reviewLevel 1/2/3,requiredLevels 固定 3。
export type ReviewStage = 'first' | 'second' | 'final'

export type Submission = {
  id: number
  taskId: number
  itemId: number
  status: string
  aiVerdict?: string | null
  aiScore?: number | null
  humanVerdict?: string
  currentRevisionId?: number
  reviewStage?: ReviewStage
  reviewLevel?: number
  requiredLevels?: number
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
  fromState?: string | null
  toState: string
  actorType: string
  actorId?: number | null
  event: string
  payload?: unknown
  createdAt: string
}

export type HumanReviewSummary = {
  verdict: string
  reason?: string | null
  createdAt?: string
}

export type TaskBundle = {
  task: Task
  item?: TaskItem
  template?: TaskTemplate
  submission?: Submission
  revision?: SubmissionRevision | null
  aiReview?: AIReviewDetail | null
  latestHumanReview?: HumanReviewSummary | null
  auditLogs?: AuditLog[]
  reviewStage?: ReviewStage
  reviewLevel?: number
  requiredLevels?: number
}

// /reviewer/results 的单行:已定稿(approved/rejected)的 submission。
export type ReviewResult = {
  id: number
  taskId: number
  itemId: number
  status: string
  finalVerdict?: string | null
  reviewerId?: number | null
  aiScore?: number | null
  updatedAt: string
}

export type ReviewResultsPage = {
  results: ReviewResult[]
  nextCursor: string
  hasMore: boolean
}

// 审核结果列表:游标分页。后端用 PageOK 返回 { data, page:{ next_cursor, has_more } },
// 标准 apiGet 会丢掉 page,这里直接读 envelope 以保留游标信息。
export async function listReviewResults(params?: { cursor?: string, limit?: number }): Promise<ReviewResultsPage> {
  const query = new URLSearchParams()
  if (params?.cursor) {
    query.set('cursor', params.cursor)
  }
  if (params?.limit) {
    query.set('limit', String(params.limit))
  }
  const suffix = query.toString() ? `?${query.toString()}` : ''
  const token = getToken()
  if (!token) {
    throw new Error('请先登录')
  }
  const response = await fetch(`/api/v1/reviewer/results${suffix}`, {
    method: 'GET',
    headers: { Authorization: `Bearer ${token}` },
  })
  const payload = await response.json() as
    | { data: ReviewResult[], page?: { next_cursor?: string, has_more?: boolean }, request_id?: string }
    | ApiErrorEnvelope
  if (!response.ok) {
    const errorPayload = payload as ApiErrorEnvelope
    throw new ApiError(errorPayload.error?.code ?? 'UNKNOWN', errorPayload.error?.message ?? '加载审核结果失败', errorPayload.request_id ?? '')
  }
  const pagePayload = payload as { data: ReviewResult[], page?: { next_cursor?: string, has_more?: boolean } }
  return {
    results: pagePayload.data ?? [],
    nextCursor: pagePayload.page?.next_cursor ?? '',
    hasMore: Boolean(pagePayload.page?.has_more),
  }
}

// 作答页左侧"题目导航"用:某任务下一题对当前 labeler 的状态。
// status: available 待标 / claimed 进行中 / taken 被他人领走 / 或其 submission 状态(draft/submitted/.../approved/rejected/revising)。
export type LabelerTaskItem = {
  itemId: number
  externalId: string | null
  status: string
  mine: boolean
  submissionId: number | null
}

export type LabelerTaskItems = {
  taskId: number
  total: number
  items: LabelerTaskItem[]
  counts: Record<string, number>
}
