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

// 对少数会让标注/审核流程卡住的错误码,给出可执行的中文提示;其余错误码沿用后端 message。
// INVALID_STATE 一个码覆盖多种业务原因(没绑模板 / 状态已变 / 字段冻结),统一文案会误导
// (如"没绑模板"被显示成"状态被他人改动,请刷新"——刷新无用)。按后端 message 内容分流。
function resolveFriendlyMessage(code: string, backendMessage: string): string | null {
  if (code === 'INVALID_STATE') {
    if (backendMessage.toLowerCase().includes('template')) {
      return '请先为该任务搭建并保存模板,再发布。'
    }
    return '该任务状态已变化(可能已发布或已被改动),请刷新后重试。'
  }
  if (code === 'LLM_PROVIDER_ERROR') {
    return 'AI 服务暂时不可用,可稍后重试,或直接转人工审核。'
  }
  return null
}

export class ApiError extends Error {
  code: string
  requestId: string

  constructor(code: string, backendMessage: string, requestId: string) {
    super(resolveFriendlyMessage(code, backendMessage) || backendMessage || '请求失败')
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
  overlapCount?: number
  overlapCoveragePct?: number
  leaseTimeoutMinutes?: number
  reviewSamplingPct?: number
  dailySubmissionLimitPerLabeler?: number
  humanReviewEnabled?: boolean
  deadline?: string | null
  publishedAt?: string | null
}

// 当前 labeler 已领取(有提交)的一个大任务 + 我在其中的进度:
// myCounts 各状态计数、myTotal 我的提交总数、myInProgress 进行中(draft/revising)数量、
// resumeItemId 最近一条进行中提交的题目 id(用于「继续标注」直接恢复),无进行中则为 null。
export type MyTask = {
  task: Task
  myCounts: Record<string, number>
  myTotal: number
  myInProgress: number
  resumeItemId: number | null
}

// 拉取「已领取的任务」(大任务粒度),按最近活跃排序。供任务广场「已领取的任务」区块与
// 标注工作台的大任务切换器使用。
export async function listMyTasks(): Promise<MyTask[]> {
  const data = await apiGet<{ tasks: MyTask[] }>('/me/tasks')
  return data?.tasks ?? []
}

// 整体领取一个大任务(first_come / assigned 独占):该任务所有题一次性锁给当前 labeler,全部解锁可做。
// quota 任务不走这里(按题抢单,用逐题领取接口)。
export async function claimTask(taskId: number): Promise<{ task: Task }> {
  return apiPost<{ task: Task }>(`/tasks/${taskId}/claim-task`, {})
}

// 「AI 审核队列」只读视图里的一条 AI 预审 + 其提交/任务/Prompt 上下文。
// 模板 schema 兼容性:保存新模板版本前与历史版本比对的结构变更报告。
export type SchemaChangeSeverity = 'safe' | 'warning' | 'breaking'
export type SchemaChange = { field: string, kind: string, severity: SchemaChangeSeverity, detail: string }
export type SchemaCompatibility = { breaking: number, warning: number, safe: number }
export type TemplateValidateResponse = {
  valid: boolean
  errors: Array<{ field: string, message: string }>
  compareVersion?: number
  changes?: SchemaChange[]
  compatibility?: SchemaCompatibility
}

export type AIReviewRow = {
  id: number
  status: string
  verdict: string | null
  overallScore: number | null
  dimensions: Array<{ name: string, score: number, reason?: string }> | null
  reason: string | null
  promptVersion: number
  model: string
  promptTemplate: string
  passThreshold: number
  uncertainMin: number
  // promptSnapshot 是本次预审真正发给 LLM 的消息全文(AI 实际看到的 prompt);promptHash 是其
  // sha256 指纹。promptDrift 为真表示该预审所用 prompt 配置已不再是任务当前生效版本,
  // activePromptVersion 是任务当前生效的 prompt 版本号。
  promptSnapshot: string | null
  promptHash: string | null
  promptDrift: boolean
  activePromptVersion: number
  tokensInput: number
  tokensOutput: number
  latencyMs: number
  retryCount: number
  idempotencyKey: string
  errorMsg: string | null
  createdAt: string
  startedAt: string | null
  finishedAt: string | null
  submissionId: number
  taskId: number
  taskTitle: string
  itemId: number
}

// 拉取 AI 预审队列(最新在前,可按 status 过滤、id 游标分页)。供审核员「AI 审核队列」只读视图。
export async function listAIReviews(params?: { status?: string, limit?: number, before?: number }): Promise<{ items: AIReviewRow[], nextBefore: number | null }> {
  const query = new URLSearchParams()
  if (params?.status) {
    query.set('status', params.status)
  }
  if (params?.limit) {
    query.set('limit', String(params.limit))
  }
  if (params?.before) {
    query.set('before', String(params.before))
  }
  const suffix = query.toString() ? `?${query.toString()}` : ''
  const data = await apiGet<{ items: AIReviewRow[], nextBefore: number | null }>(`/reviewer/ai-reviews${suffix}`)
  return { items: data?.items ?? [], nextBefore: data?.nextBefore ?? null }
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
  overlapCount?: number
  overlapCoveragePct?: number
  leaseTimeoutMinutes?: number
  reviewSamplingPct?: number
  dailySubmissionLimitPerLabeler?: number
  humanReviewEnabled?: boolean
  deadline?: string | null
  // 切换任务绑定的模板版本(仅 draft 任务,发布后后端冻结)。
  templateId?: number
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
  if (input.overlapCount !== undefined) body.overlapCount = input.overlapCount
  if (input.overlapCoveragePct !== undefined) body.overlapCoveragePct = input.overlapCoveragePct
  if (input.leaseTimeoutMinutes !== undefined) body.leaseTimeoutMinutes = input.leaseTimeoutMinutes
  if (input.reviewSamplingPct !== undefined) body.reviewSamplingPct = input.reviewSamplingPct
  if (input.dailySubmissionLimitPerLabeler !== undefined) body.dailySubmissionLimitPerLabeler = input.dailySubmissionLimitPerLabeler
  if (input.humanReviewEnabled !== undefined) body.humanReviewEnabled = input.humanReviewEnabled
  if (input.deadline !== undefined) body.deadline = input.deadline
  if (input.templateId !== undefined) body.templateId = input.templateId
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

export type AssigneeCandidate = {
  userId: number
  username: string
  displayName: string
}

// 列出可指派的标注员(role=labeler 且 active),供「指派」分发策略下从列表选人。
export async function listAssigneeCandidates(taskId: number) {
  const data = await apiGet<{ candidates: AssigneeCandidate[] }>(`/tasks/${taskId}/assignee-candidates`)
  return data.candidates
}

export async function addAssignees(taskId: number, userIds: number[]) {
  return apiPost<AddAssigneesResult>(`/tasks/${taskId}/assignees`, { userIds })
}

export async function removeAssignee(taskId: number, userId: number) {
  return apiDelete<RemoveAssigneeResult>(`/tasks/${taskId}/assignees/${userId}`)
}

// --- 审核员指派(task_reviewers)。仅 task owner / admin 可操作。 ---

export type TaskReviewerView = {
  userId: number
  assignedAt: string
}

export type ReviewerCandidate = {
  userId: number
  username: string
  displayName: string
}

export type AddReviewersResult = { added: number }
export type RemoveReviewerResult = { removed: number }

export async function listReviewers(taskId: number) {
  const data = await apiGet<{ reviewers: TaskReviewerView[] }>(`/tasks/${taskId}/reviewers`)
  return data.reviewers
}

// 列出可指派的审核员(role=reviewer 且 active)。
export async function listReviewerCandidates(taskId: number) {
  const data = await apiGet<{ candidates: ReviewerCandidate[] }>(`/tasks/${taskId}/reviewer-candidates`)
  return data.candidates
}

export async function addReviewers(taskId: number, userIds: number[]) {
  return apiPost<AddReviewersResult>(`/tasks/${taskId}/reviewers`, { userIds })
}

export async function removeReviewer(taskId: number, userId: number) {
  return apiDelete<RemoveReviewerResult>(`/tasks/${taskId}/reviewers/${userId}`)
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
  labelerId?: number | null
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
  submissionId?: number
  revisionNo?: number
  answer: string
  draft: boolean
  createdBy?: number
  createdAt?: string
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
  revisionHistory?: SubmissionRevision[]
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

// Owner「审核结果」逐条质检反馈:某任务已定稿提交的 AI 判定 vs 人工判定 + 是否一致。
export type OwnerReviewResult = {
  id: number
  itemId: number
  status: string
  aiVerdict: string | null
  aiScore: number | null
  humanVerdict: string | null
  agreed: boolean | null
  updatedAt: string
}

export type OwnerReviewResultsPage = {
  results: OwnerReviewResult[]
  nextCursor: string
  hasMore: boolean
}

// 游标分页。后端 PageOK 返回 { data, page },标准 apiGet 丢 page,这里直接读 envelope。
export async function listOwnerReviewResults(taskId: number, params?: { cursor?: string, limit?: number }): Promise<OwnerReviewResultsPage> {
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
  const response = await fetch(`/api/v1/tasks/${taskId}/review-results${suffix}`, {
    method: 'GET',
    headers: { Authorization: `Bearer ${token}` },
  })
  const payload = await response.json() as
    | { data: OwnerReviewResult[], page?: { next_cursor?: string, has_more?: boolean }, request_id?: string }
    | ApiErrorEnvelope
  if (!response.ok) {
    const errorPayload = payload as ApiErrorEnvelope
    throw new ApiError(errorPayload.error?.code ?? 'UNKNOWN', errorPayload.error?.message ?? '加载审核结果失败', errorPayload.request_id ?? '')
  }
  const pagePayload = payload as { data: OwnerReviewResult[], page?: { next_cursor?: string, has_more?: boolean } }
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

// --- Owner data-acceptance loop ---

export type AcceptanceBatch = {
  id: number
  taskId: number
  status: string // pending | accepted | rejected
  approvedCount: number
  note: string | null
  decidedBy: number | null
  decidedAt: string | null
  createdAt: string
}

export type AcceptanceSpotCheck = {
  id: number
  batchId: number
  submissionId: number
  result: string // ok | flag
  note: string | null
  checkedBy: number
  createdAt: string
}

export type AcceptanceApprovedSubmission = {
  id: number
  itemId: number
  labelerId: number
  aiVerdict: string | null
  aiScore: number | null
  answer: string
}

export type AcceptanceStatus = {
  batch: AcceptanceBatch | null
  spotChecks: AcceptanceSpotCheck[]
  approvedCount: number
  // 后端 Status 总会返回;设为可选只是为了向后兼容旧的测试 mock,前端一律用 `?? []` 兜底。
  approvedSubmissions?: AcceptanceApprovedSubmission[]
}

export function getAcceptance(taskId: number) {
  return apiGet<AcceptanceStatus>(`/tasks/${taskId}/acceptance`)
}

export function startAcceptance(taskId: number) {
  return apiPost<{ batch: AcceptanceBatch }>(`/tasks/${taskId}/acceptance`, {})
}

export function recordAcceptanceSpotCheck(
  taskId: number,
  body: { batch_id: number, submission_id: number, result: 'ok' | 'flag', note?: string },
) {
  return apiPost<{ spotCheck: AcceptanceSpotCheck }>(`/tasks/${taskId}/acceptance/spot-checks`, body)
}

export function acceptAcceptanceBatch(taskId: number, body: { batch_id: number, note?: string }) {
  return apiPost<{ batch: AcceptanceBatch }>(`/tasks/${taskId}/acceptance/accept`, body)
}

export function rejectAcceptanceBatch(taskId: number, body: { batch_id: number, note?: string }) {
  return apiPost<{ batch: AcceptanceBatch, reopenedCount: number }>(`/tasks/${taskId}/acceptance/reject`, body)
}
