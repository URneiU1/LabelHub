import type { CSSProperties } from 'react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Button, Toast } from '@douyinfe/semi-ui'
import { SchemaRenderer, parseAnswer, parseTemplateSchema } from '../../renderer'
import type { AnswerValue, TemplateSchema } from '../../renderer/types'
import { apiGet, apiPost, type AIPromptSummary, type AIReviewDetail, type AuditLog, type ReviewResult, type Submission, type TaskBundle } from '../../shared/api/client'
import EmptyState from '../../shared/components/EmptyState'
import { Icon } from '../../shared/components/Icon'
import { parsePayload } from '../../shared/components/payload'
import StatusBadge from '../../shared/components/StatusBadge'
import AIVerdictPanel from './AIVerdictPanel'
import ArbitrationQueue from './ArbitrationQueue'
import ReviewResults from './ReviewResults'
import { resolveStage } from './stage'
import '../../styles/lh/humanreview.css'
import '../../styles/lh/aireview.css'

type ReviewerView = 'workbench' | 'arbitration' | 'results'

// M-10:演示样例数据只在显式 ?demo=1 时启用,默认队列为空就展示空状态,
// 不再用假 demo 顶替真实队列(否则会掩盖仲裁队列不可见、且假"审核成功"误导演示者)。
function isDemoMode() {
  if (typeof window === 'undefined') {
    return false
  }
  return new URLSearchParams(window.location.search).has('demo')
}

type ReviewResponse = {
  submission_id: number
  status: string
}

type RetryAIReviewResponse = {
  submissionId: number
  status: string
  aiReview: AIReviewDetail
}

type BatchReviewResponse = {
  results: Array<{ submissionId: number, status?: string, error?: string }>
  summary: { total: number, succeeded: number, failed: number }
}

type ReviewerRuleConfigResponse = {
  prompts: AIPromptSummary[]
  activePromptId: number | null
  aiReviewEnabled: boolean
}

type ParsedSchema =
  | { ok: true, schema: TemplateSchema }
  | { ok: false, message: string }

type DemoReviewItem = {
  id: string
  title: string
  meta: string
  verdict: string
  score: number
  badge: string
  tone: 'pass' | 'reject' | 'manual' | 'failed'
  selected?: boolean
}

type DemoTimelineEvent = {
  time: string
  actor: string
  action: string
  tone: 'green' | 'red' | 'blue'
}

type DemoReviewSnapshot = {
  reason: string
  beforeRows: Array<[string, string]>
  afterRows: Array<[string, string]>
  scoreRows: Array<{ label: string, value: number, color?: string }>
  promptLabel: string
  promptTemplate: string
  summary: string
  jsonPreview: string
  timeline: DemoTimelineEvent[]
}

type QueueItem =
  | { kind: 'real', submission: Submission }
  | { kind: 'demo', item: DemoReviewItem }

// 工作台队列分区(对齐审核流程图独立分支):
//   AI 通过待初审(human_reviewing)与「转人工复核」(manual_review,AI 综合判定为可疑)分开展示。
type QueueFilter = 'all' | 'human_reviewing' | 'manual_review'

const MANUAL_REVIEW_STATUS = 'manual_review'
const MANUAL_REVIEW_LABEL = '转人工复核'

// 审核意见快捷标签:点击追加到审核意见文本框(复刻 ui-demo 的 quick-tags)。
const QUICK_TAGS = ['# 关键词缺失', '# 类目错误', '# 标题超长', '# 包含违禁词', '# 格式不规范']

const demoItems: DemoReviewItem[] = [
  {
    id: 'SUB-00606',
    title: '儿童学习平板电脑 8 英寸护眼大屏',
    meta: '王芳标注 · 18:00:48',
    verdict: '建议通过',
    score: 91,
    badge: 'AI 91',
    tone: 'pass',
  },
  {
    id: 'SUB-00607',
    title: '户外便携野营折叠桌椅套装 5 件套',
    meta: '李雷标注 · 18:01:02',
    verdict: '建议打回',
    score: 62,
    badge: 'AI 62',
    tone: 'reject',
    selected: true,
  },
  {
    id: 'SUB-00604',
    title: '真无线主动降噪耳机 Pro Max 2026 款',
    meta: '张敏标注 · 17:58:11',
    verdict: '建议通过',
    score: 88,
    badge: 'AI 88',
    tone: 'pass',
  },
  {
    id: 'SUB-00605',
    title: '加厚熟蓝牙智能保温杯',
    meta: '王芳标注 · 18:00:31',
    verdict: '需人工',
    score: 0,
    badge: '触发安全',
    tone: 'manual',
  },
  {
    id: 'SUB-00603',
    title: '春季新款女装 5 色可选',
    meta: '张敏标注 · 17:48:00',
    verdict: '失败',
    score: 0,
    badge: '第 3 次',
    tone: 'failed',
  },
]

const demoReviewDetails: Record<string, DemoReviewSnapshot> = {
  'SUB-00606': {
    reason: '护眼屏参数、适用年龄和配件信息都补齐了，家长能直接判断是否匹配。',
    beforeRows: [
      ['cleaned_title', '儿童学习平板电脑 8 英寸护眼大屏'],
      ['category', '3C 数码'],
      ['keywords', '平板, 学习, 护眼'],
    ],
    afterRows: [
      ['cleaned_title', '儿童学习平板电脑 8 英寸护眼版'],
      ['category', '学习平板'],
      ['keywords', '护眼, 8 英寸, 学习, 教育'],
    ],
    scoreRows: [
      { label: '相关性', value: 94, color: 'var(--color-success)' },
      { label: '准确性', value: 90, color: 'var(--color-success)' },
      { label: '格式合规', value: 88, color: '#f97316' },
      { label: '安全性', value: 99, color: 'var(--color-success)' },
      { label: '综合', value: 93, color: 'var(--color-success)' },
    ],
    promptLabel: '规则：学习用品 v1',
    promptTemplate: `请基于以下维度给提交内容打分（0-100）：
[相关性] 标注结果是否与学习平板场景对齐
[准确性] 规格、年龄和功能是否与商品一致
[格式合规] 是否满足模板字段与命名规则
[安全性] 是否包含敏感或违规信息`,
    summary: '护眼屏、学习场景和规格字段都对齐，建议通过。',
    jsonPreview: JSON.stringify({
      cleaned_title: '儿童学习平板电脑 8 英寸护眼版',
      category: '学习平板',
      keywords: ['护眼', '8 英寸', '学习', '教育'],
    }, null, 2),
    timeline: [
      { time: '18:00:48', actor: '王芳', action: '第 1 轮提交', tone: 'green' },
      { time: '18:00:49', actor: 'AI Agent', action: '预审 91 分 → 建议通过', tone: 'green' },
      { time: '18:00:50', actor: '王芳 · 复审', action: '确认字段完整，直接放行', tone: 'green' },
      { time: '18:00:52', actor: '系统', action: '写入可导出队列', tone: 'blue' },
    ],
  },
  'SUB-00607': {
    reason: '关键词丰富度和类目都已经覆盖到位，和上一轮的打回意见对齐。',
    beforeRows: [
      ['cleaned_title', '户外便携野营折叠桌椅套装 5 件套'],
      ['category', '家居用品'],
      ['keywords', '折叠, 户外'],
    ],
    afterRows: [
      ['cleaned_title', '户外野营便携折叠桌椅 5 件套'],
      ['category', '户外运动'],
      ['keywords', '折叠, 户外, 5 件套, 便携, 野营'],
    ],
    scoreRows: [
      { label: '相关性', value: 92, color: 'var(--color-success)' },
      { label: '准确性', value: 84, color: '#f97316' },
      { label: '格式合规', value: 88, color: '#f97316' },
      { label: '安全性', value: 99, color: 'var(--color-success)' },
      { label: '综合', value: 86, color: '#f97316' },
    ],
    promptLabel: '规则：电商相关性 v2',
    promptTemplate: `请基于以下维度给提交内容打分（0-100）：
[相关性] 标注结果与原始数据是否对齐
[准确性] 类目 / 关键词与商品事实是否一致
[格式合规] 是否满足模板字段与正则规则
[安全性] 是否包含敏感 / 违规词`,
    summary: '关键词较第 1 轮已补充至 5 个并覆盖品类核心卖点；类目改为「户外运动」更贴合事实。建议通过。',
    jsonPreview: JSON.stringify({
      cleaned_title: '户外野营便携折叠桌椅 5 件套',
      category: '户外运动',
      keywords: ['折叠', '户外', '5 件套', '便携', '野营'],
    }, null, 2),
    timeline: [
      { time: '18:01:02', actor: '李雷', action: '第 1 轮提交', tone: 'green' },
      { time: '18:01:03', actor: 'AI Agent', action: '预审 62 分 → 建议打回', tone: 'red' },
      { time: '18:01:04', actor: '王芳 · 复审', action: '采纳 AI 结论 → 打回', tone: 'red' },
      { time: '18:01:05', actor: '李雷', action: '查看打回意见并修改', tone: 'green' },
      { time: '18:01:06', actor: 'AI Agent', action: '重跑 86 分 → 建议通过', tone: 'green' },
      { time: '18:01:07', actor: '王芳 · 复审', action: '本次决策写入终审待办', tone: 'blue' },
    ],
  },
  'SUB-00604': {
    reason: '耳机型号、蓝牙版本和降噪关键词都齐了，信息完整度更高。',
    beforeRows: [
      ['cleaned_title', '真无线主动降噪耳机 Pro Max 2026 款'],
      ['category', '3C 数码'],
      ['keywords', '耳机, 降噪'],
    ],
    afterRows: [
      ['cleaned_title', '真无线主动降噪耳机 Pro Max 2026 款'],
      ['category', '蓝牙耳机'],
      ['keywords', '主动降噪, 蓝牙, 低延迟, 续航'],
    ],
    scoreRows: [
      { label: '相关性', value: 90, color: 'var(--color-success)' },
      { label: '准确性', value: 88, color: '#f97316' },
      { label: '格式合规', value: 92, color: 'var(--color-success)' },
      { label: '安全性', value: 98, color: 'var(--color-success)' },
      { label: '综合', value: 89, color: '#f97316' },
    ],
    promptLabel: '规则：数码配件 v3',
    promptTemplate: `请基于以下维度给提交内容打分（0-100）：
[相关性] 耳机场景和商品型号是否一致
[准确性] 主动降噪、蓝牙和续航字段是否准确
[格式合规] 是否满足模板字段与命名规则
[安全性] 是否包含敏感或违规信息`,
    summary: '型号、蓝牙和降噪字段明确，结构完整，建议通过。',
    jsonPreview: JSON.stringify({
      cleaned_title: '真无线主动降噪耳机 Pro Max 2026 款',
      category: '蓝牙耳机',
      keywords: ['主动降噪', '蓝牙', '低延迟', '续航'],
    }, null, 2),
    timeline: [
      { time: '17:58:11', actor: '张敏', action: '第 1 轮提交', tone: 'green' },
      { time: '17:58:12', actor: 'AI Agent', action: '预审 88 分 → 建议通过', tone: 'green' },
      { time: '17:58:13', actor: '张敏 · 复审', action: '确认规格字段无误', tone: 'green' },
      { time: '17:58:14', actor: '系统', action: '待终审排队', tone: 'blue' },
    ],
  },
  'SUB-00605': {
    reason: '蓝牙功能项存在歧义，已转人工核对，避免把异常字段直接放行。',
    beforeRows: [
      ['cleaned_title', '加厚熟蓝牙智能保温杯'],
      ['category', '家居用品'],
      ['keywords', '保温杯, 蓝牙'],
    ],
    afterRows: [
      ['cleaned_title', '加厚蓝牙智能保温杯'],
      ['category', '家居日用'],
      ['keywords', '保温杯, 304 不锈钢, 蓝牙'],
    ],
    scoreRows: [
      { label: '相关性', value: 58, color: 'var(--color-danger)' },
      { label: '准确性', value: 52, color: 'var(--color-danger)' },
      { label: '格式合规', value: 86, color: '#f97316' },
      { label: '安全性', value: 61, color: '#f97316' },
      { label: '综合', value: 57, color: 'var(--color-danger)' },
    ],
    promptLabel: '规则：安全词审查 v1',
    promptTemplate: `请基于以下维度给提交内容打分（0-100）：
[相关性] 商品标题是否和目标类目一致
[准确性] 功能词和材质词是否存在歧义
[格式合规] 是否满足模板字段与正则规则
[安全性] 是否包含敏感、夸大或歧义描述`,
    summary: '蓝牙功能词存在歧义，优先转人工核对，不直接通过。',
    jsonPreview: JSON.stringify({
      cleaned_title: '加厚蓝牙智能保温杯',
      category: '家居日用',
      keywords: ['保温杯', '304 不锈钢', '蓝牙'],
    }, null, 2),
    timeline: [
      { time: '18:00:31', actor: '王芳', action: '第 1 轮提交', tone: 'green' },
      { time: '18:00:32', actor: 'AI Agent', action: '预审 57 分 → 转人工', tone: 'red' },
      { time: '18:00:33', actor: '王芳 · 复审', action: '保留人工复核意见', tone: 'blue' },
      { time: '18:00:34', actor: '系统', action: '进入人工审核队列', tone: 'blue' },
    ],
  },
  'SUB-00603': {
    reason: '上一轮重试后仍未稳定，当前记录保留失败态，等模型重跑后再继续。',
    beforeRows: [
      ['cleaned_title', '春季新款女装 5 色可选'],
      ['category', '服饰'],
      ['keywords', '春装, 女装'],
    ],
    afterRows: [
      ['cleaned_title', '春季新款女装 5 色可选'],
      ['category', '女装'],
      ['keywords', '春装, 轻薄, 5 色可选'],
    ],
    scoreRows: [
      { label: '相关性', value: 0, color: 'var(--color-danger)' },
      { label: '准确性', value: 0, color: 'var(--color-danger)' },
      { label: '格式合规', value: 0, color: 'var(--color-danger)' },
      { label: '安全性', value: 0, color: 'var(--color-danger)' },
      { label: '综合', value: 0, color: 'var(--color-danger)' },
    ],
    promptLabel: '规则：女装上新 v4',
    promptTemplate: `请基于以下维度给提交内容打分（0-100）：
[相关性] 女装类目是否明确
[准确性] 颜色、季节和版型是否完整
[格式合规] 是否满足模板字段和命名规则
[安全性] 是否包含敏感或违规词`,
    summary: '当前轮次 AI 预审失败，先保留失败态等待重跑结果。',
    jsonPreview: JSON.stringify({
      cleaned_title: '春季新款女装 5 色可选',
      category: '女装',
      keywords: ['春装', '轻薄', '5 色可选'],
    }, null, 2),
    timeline: [
      { time: '17:48:00', actor: '张敏', action: '第 1 轮提交', tone: 'green' },
      { time: '17:48:01', actor: 'AI Agent', action: '预审失败，等待重跑', tone: 'red' },
      { time: '17:48:02', actor: '张敏 · 复审', action: '记录失败原因', tone: 'red' },
      { time: '17:48:03', actor: '系统', action: '标记为 failed', tone: 'red' },
    ],
  },
}

function getDemoReviewDetail(id: string) {
  return demoReviewDetails[id] ?? demoReviewDetails['SUB-00607']
}

const demoRuleConfigs: AIPromptSummary[] = [{
  id: 2041,
  version: 2,
  model: 'doubao-pro-32k',
  promptTemplate: `请基于以下维度给提交内容打分（0-100）：
[相关性] 标注结果与原始数据是否对齐
[准确性] 类目 / 关键词与商品事实是否一致
[格式合规] 是否满足模板字段与正则规则
[安全性] 是否包含敏感 / 违规词`,
  dimensions: [
    { name: '相关性', weight: 0.35 },
    { name: '准确性', weight: 0.35 },
    { name: '格式合规', weight: 0.2 },
    { name: '安全性', weight: 0.1 },
  ],
  passThreshold: 80,
  uncertainMin: 60,
}]

export default function ReviewerQueue() {
  const [submissions, setSubmissions] = useState<Submission[]>([])
  const [selected, setSelected] = useState<Submission | null>(null)
  const [selectedSubmissionIds, setSelectedSubmissionIds] = useState<number[]>([])
  const [selectedDemoId, setSelectedDemoId] = useState('SUB-00607')
  const [detail, setDetail] = useState<TaskBundle | null>(null)
  const [reason, setReason] = useState(() => getDemoReviewDetail('SUB-00607').reason)
  const [loading, setLoading] = useState(false)
  const [retryingAI, setRetryingAI] = useState(false)
  const [rulePanelOpen, setRulePanelOpen] = useState(false)
  const [ruleConfigs, setRuleConfigs] = useState<AIPromptSummary[]>([])
  const [selectedRuleId, setSelectedRuleId] = useState<number | null>(null)
  const [ruleActivePromptId, setRuleActivePromptId] = useState<number | null>(null)
  const [ruleAIReviewEnabled, setRuleAIReviewEnabled] = useState(false)
  const [ruleLoading, setRuleLoading] = useState(false)
  const [ruleError, setRuleError] = useState('')
  const ruleLoadSeq = useRef(0)
  // 详情加载用独立序号,和规则面板的 ruleLoadSeq 解耦:开关「规则配置」不应误失效正在加载的详情。
  const detailLoadSeq = useRef(0)
  // /reviewer/results 已提升为外层导航入口;组件内部仍保留 results view 用来渲染该路由。
  // 工作台内的顶部切换只保留「审核工作台 / 仲裁」,避免与侧栏「审核结果」重复。
  // 用 window.location 而非 useLocation:组件在测试里不一定包 Router;路由对两条路径用不同 key 强制重挂载,首次挂载读路径即正确。
  const [view, setView] = useState<ReviewerView>(() => (window.location.pathname.endsWith('/results') ? 'results' : 'workbench'))
  // 工作台内的队列分区:全部 / AI 通过待初审 / 转人工复核。
  const [queueFilter, setQueueFilter] = useState<QueueFilter>('all')
  // 演示样例只在显式 ?demo=1 时启用,默认空队列展示空状态(M-10)。
  const demoMode = useMemo(() => isDemoMode(), [])

  const loadQueue = useCallback(async () => {
    try {
      // 两条独立分支分别拉取:AI 通过待初审(默认 human_reviewing)+ 转人工复核(manual_review)。
      // 主队列(human_reviewing)失败按错误处理;转人工复核分支失败仅降级为空,不拖垮主队列。
      const humanReviewing = await apiGet<Submission[]>('/reviewer/submissions')
      let manualReview: Submission[] = []
      try {
        manualReview = await apiGet<Submission[]>(`/reviewer/submissions?status=${MANUAL_REVIEW_STATUS}`)
      } catch {
        manualReview = []
      }
      const data = [...humanReviewing, ...(manualReview ?? [])]
      setSubmissions(data)
      setSelectedSubmissionIds((current) => current.filter((id) => data.some((submission) => submission.id === id)))
      if (data.length === 0) {
        ruleLoadSeq.current += 1
        detailLoadSeq.current += 1
        setSelected(null)
        setDetail(null)
        setRulePanelOpen(false)
      }
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '加载审核队列失败')
    }
  }, [])

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void loadQueue()
  }, [loadQueue])

  // 各分区计数(基于真实队列;转人工复核 = status manual_review)。
  const manualReviewCount = useMemo(
    () => submissions.filter((submission) => submission.status === MANUAL_REVIEW_STATUS).length,
    [submissions],
  )
  const humanReviewingCount = submissions.length - manualReviewCount

  const queueItems = useMemo<QueueItem[]>(() => {
    if (submissions.length > 0) {
      const filtered = submissions.filter((submission) => {
        if (queueFilter === 'manual_review') return submission.status === MANUAL_REVIEW_STATUS
        if (queueFilter === 'human_reviewing') return submission.status !== MANUAL_REVIEW_STATUS
        return true
      })
      return filtered.map((submission) => ({ kind: 'real', submission }))
    }
    if (demoMode) {
      return demoItems.map((item) => ({ kind: 'demo', item }))
    }
    return []
  }, [submissions, demoMode, queueFilter])

  // 当前 filter 下可见的真实 submission id(全选 / 批量只作用于可见分区)。
  const visibleSubmissionIds = useMemo(
    () => queueItems.flatMap((item) => (item.kind === 'real' ? [item.submission.id] : [])),
    [queueItems],
  )

  const selectedDemo = demoItems.find((item) => item.id === selectedDemoId) ?? demoItems[1]
  const selectedDemoDetail = useMemo(() => getDemoReviewDetail(selectedDemo.id), [selectedDemo.id])
  const schema = useMemo(() => parseBundleSchema(detail), [detail])
  const payload = useMemo(() => parsePayload(detail?.item?.payload), [detail?.item?.payload])
  const answer = useMemo<AnswerValue>(() => parseAnswer(detail?.revision?.answer), [detail?.revision?.answer])
  const showingDemo = demoMode && submissions.length === 0 && !selected && !detail
  // 全选 checkbox 的全选 / 半选态:可见项全部选中 → 勾选;仅部分选中 → indeterminate 半选。
  const selectAllRef = useRef<HTMLInputElement>(null)
  const allVisibleSelected = !showingDemo && visibleSubmissionIds.length > 0 && visibleSubmissionIds.every((id) => selectedSubmissionIds.includes(id))
  const someVisibleSelected = !showingDemo && visibleSubmissionIds.some((id) => selectedSubmissionIds.includes(id))
  useEffect(() => {
    if (selectAllRef.current) {
      selectAllRef.current.indeterminate = someVisibleSelected && !allVisibleSelected
    }
  }, [someVisibleSelected, allVisibleSelected])
  const retryDisabled = !showingDemo && (!selected || !canRetryAIReview(detail?.aiReview))
  const selectedRule = useMemo(() => {
    if (ruleConfigs.length > 0) {
      return ruleConfigs.find((item) => item.id === selectedRuleId) ?? ruleConfigs[0]
    }
    return detail?.aiReview?.prompt ?? null
  }, [detail?.aiReview?.prompt, ruleConfigs, selectedRuleId])
  // 当前详情的人工审核阶段(初审/复审/终审),供详情头部与「通过」按钮文案使用。
  const detailStage = useMemo(() => resolveStage({
    reviewStage: detail?.reviewStage ?? detail?.submission?.reviewStage,
    reviewLevel: detail?.reviewLevel ?? detail?.submission?.reviewLevel,
    requiredLevels: detail?.requiredLevels ?? detail?.submission?.requiredLevels,
  }), [detail?.reviewStage, detail?.reviewLevel, detail?.requiredLevels, detail?.submission?.reviewStage, detail?.submission?.reviewLevel, detail?.submission?.requiredLevels])

  async function openQueueItem(item: QueueItem) {
    // 切换队列项:bump ruleLoadSeq 取消正在加载的规则面板请求;另用独立的 detailLoadSeq
    // 守护本次详情加载——只有最新一次选择的响应才允许落详情(避免快速连点张冠李戴),
    // 且不会被「规则配置」开关(只动 ruleLoadSeq)误失效。
    ruleLoadSeq.current += 1
    const requestSeq = detailLoadSeq.current + 1
    detailLoadSeq.current = requestSeq
    setRulePanelOpen(false)
    setRuleLoading(false)
    if (item.kind === 'demo') {
      const demoDetail = getDemoReviewDetail(item.item.id)
      setSelected(null)
      setDetail(null)
      setSelectedDemoId(item.item.id)
      setReason(demoDetail.reason)
      return
    }
    const submission = item.submission
    setSelected(submission)
    try {
      const data = await apiGet<TaskBundle>(`/reviewer/submissions/${submission.id}`)
      if (detailLoadSeq.current !== requestSeq) return
      setDetail(data)
      setReason('')
    } catch (error) {
      if (detailLoadSeq.current !== requestSeq) return
      Toast.error(error instanceof Error ? error.message : '加载详情失败')
    }
  }

  // 从「审核结果」列表打开某条已定稿提交的只读详情:复用 openQueueItem 的详情加载与
  // 序号守护,把结果行包装成一个最小的 real submission,并切回工作台视图展示。
  async function openResultDetail(result: ReviewResult) {
    setView('workbench')
    await openQueueItem({
      kind: 'real',
      submission: {
        id: result.id,
        taskId: result.taskId,
        itemId: result.itemId,
        status: result.status,
        aiScore: result.aiScore ?? null,
      },
    })
  }

  async function review(verdict: 'approve' | 'reject' | 'revise') {
    if (!selected) {
      Toast.success('演示样例：已记录本次审核动作')
      return
    }
    if ((verdict === 'reject' || verdict === 'revise') && reason.trim().length < 5) {
      Toast.error('打回/拒绝必须填写至少 5 字的理由')
      return
    }
    setLoading(true)
    try {
      const data = await apiPost<ReviewResponse>(`/submissions/${selected.id}/review`, { verdict, reason })
      Toast.success(`审核完成，状态 ${data.status}`)
      await loadQueue()
      ruleLoadSeq.current += 1
      detailLoadSeq.current += 1
      setSelected(null)
      setDetail(null)
      setRulePanelOpen(false)
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '审核失败')
    } finally {
      setLoading(false)
    }
  }

  async function batchReview(verdict: 'approve' | 'revise') {
    if (showingDemo) {
      Toast.success('演示样例：已模拟批量审核')
      return
    }
    if (selectedSubmissionIds.length === 0) {
      Toast.error('请先选择要批量处理的提交')
      return
    }
    const batchReason = verdict === 'approve' ? '' : (reason.trim() || '批量打回：请根据上一轮意见补充修改。')
    setLoading(true)
    try {
      const data = await apiPost<BatchReviewResponse>('/reviews/batch', {
        submission_ids: selectedSubmissionIds,
        verdict,
        reason: batchReason,
      })
      Toast.success(`批量完成 ${data.summary.succeeded}/${data.summary.total}`)
      await loadQueue()
      setSelectedSubmissionIds([])
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '批量审核失败')
    } finally {
      setLoading(false)
    }
  }

  async function retryAIReview() {
    if (!selected) {
      Toast.success('演示样例：已模拟触发失败重跑')
      return
    }
    setRetryingAI(true)
    try {
      const data = await apiPost<RetryAIReviewResponse>(`/reviewer/submissions/${selected.id}/ai-review/retry`, {})
      Toast.success(`AI 重跑已入队，状态 ${data.status}`)
      await loadQueue()
      ruleLoadSeq.current += 1
      detailLoadSeq.current += 1
      setSelected(null)
      setDetail(null)
      setRulePanelOpen(false)
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : 'AI 重跑失败')
    } finally {
      setRetryingAI(false)
    }
  }

  async function openRuleConfig() {
    const requestSeq = ruleLoadSeq.current + 1
    ruleLoadSeq.current = requestSeq
    if (showingDemo) {
      setRuleConfigs(demoRuleConfigs)
      setSelectedRuleId(demoRuleConfigs[0]?.id ?? null)
      setRuleActivePromptId(demoRuleConfigs[0]?.id ?? null)
      setRuleAIReviewEnabled(true)
      setRuleError('')
      setRulePanelOpen(true)
      return
    }
    if (!detail?.task?.id) {
      Toast.error('请先选择一条真实提交')
      return
    }
    const taskId = detail.task.id
    const currentPrompt = detail.aiReview?.prompt ?? null
    setRulePanelOpen(true)
    setRuleLoading(true)
    setRuleError('')
    setRuleConfigs(currentPrompt ? [currentPrompt] : [])
    setSelectedRuleId(currentPrompt?.id ?? null)
    setRuleActivePromptId(null)
    setRuleAIReviewEnabled(false)
    try {
      const data = await apiGet<ReviewerRuleConfigResponse>(`/reviewer/tasks/${taskId}/ai-prompts`)
      if (ruleLoadSeq.current !== requestSeq) return
      const prompts = data.prompts.length === 0 && currentPrompt ? [currentPrompt] : data.prompts
      setRuleConfigs(prompts)
      setRuleActivePromptId(data.activePromptId)
      setRuleAIReviewEnabled(data.aiReviewEnabled)
      setSelectedRuleId(currentPrompt?.id ?? data.activePromptId ?? prompts[0]?.id ?? null)
    } catch (error) {
      if (ruleLoadSeq.current !== requestSeq) return
      setRuleConfigs(currentPrompt ? [currentPrompt] : [])
      setRuleError(error instanceof Error ? error.message : '加载规则配置失败')
    } finally {
      if (ruleLoadSeq.current === requestSeq) {
        setRuleLoading(false)
      }
    }
  }

  const approveLabel = !showingDemo && selected ? `通过(${detailStage.label})` : '通过'

  return (
    <div style={pageStyle}>
      <header className="lh-page-header" style={topBarStyle}>
        <div className="lh-page-header-title">
          <div style={breadcrumbStyle}>审核中心 / <strong>{showingDemo ? 'AI 预审规则 · 队列' : (detail?.task.title ?? '人工审核工作台')}</strong></div>
          <h1 style={pageTitleStyle}>{showingDemo ? 'AI 自动预审队列' : '人工审核工作台'}</h1>
          <p style={pageSubTitleStyle}>异步消费提交数据 → 按评分维度调用 LLM 结构化输出 → 通过 / 打回 / 转人工复核</p>
        </div>
        <div className="lh-page-header-actions" style={headerActionsStyle}>
          {showingDemo ? <span style={demoBadgeStyle}>演示数据 · DEMO</span> : null}
          {showingDemo ? <span style={modelPillStyle}>Agent v2.3 · 模型 doubao-pro-32k</span> : null}
          <Button loading={ruleLoading} onClick={() => void openRuleConfig()} theme="light">规则配置</Button>
          <Button disabled={retryDisabled} loading={retryingAI} onClick={() => void retryAIReview()} theme="light">失败重跑</Button>
        </div>
      </header>

      {view === 'results' ? null : (
        <div className="hr-side__tabs" role="tablist" style={viewTabsStyle}>
          <button
            type="button"
            role="tab"
            aria-selected={view === 'workbench'}
            className={`hr-side__tab${view === 'workbench' ? ' hr-side__tab--active' : ''}`}
            onClick={() => setView('workbench')}
          >
            审核工作台
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={view === 'arbitration'}
            className={`hr-side__tab${view === 'arbitration' ? ' hr-side__tab--active' : ''}`}
            onClick={() => setView('arbitration')}
          >
            仲裁
          </button>
        </div>
      )}

      {view === 'results' ? (
        <ReviewResults onOpenResult={(result) => { void openResultDetail(result) }} />
      ) : view === 'arbitration' ? (
        <ArbitrationQueue />
      ) : (
        <div className="hr-shell" style={shellStyle}>
          <aside className="hr-side" style={hrSideStyle}>
            {showingDemo ? (
              <div className="hr-side__tabs">
                <span className="hr-side__tab hr-side__tab--active">AI 已建议通过<span className="hr-side__tab-num">128</span></span>
                <span className="hr-side__tab">AI 已建议打回<span className="hr-side__tab-num">47</span></span>
                <span className="hr-side__tab">转人工<span className="hr-side__tab-num">9</span></span>
              </div>
            ) : (
              // 真实队列:把「AI 通过待初审」与「转人工复核(AI 可疑)」分成独立分区(对齐审核流程图独立分支)。
              <div className="hr-side__tabs" role="tablist" aria-label="审核队列分区">
                <button
                  type="button"
                  role="tab"
                  aria-selected={queueFilter === 'all'}
                  className={`hr-side__tab${queueFilter === 'all' ? ' hr-side__tab--active' : ''}`}
                  onClick={() => setQueueFilter('all')}
                >
                  全部<span className="hr-side__tab-num">{submissions.length}</span>
                </button>
                <button
                  type="button"
                  role="tab"
                  aria-selected={queueFilter === 'human_reviewing'}
                  className={`hr-side__tab${queueFilter === 'human_reviewing' ? ' hr-side__tab--active' : ''}`}
                  onClick={() => setQueueFilter('human_reviewing')}
                >
                  AI 通过待初审<span className="hr-side__tab-num">{humanReviewingCount}</span>
                </button>
                <button
                  type="button"
                  role="tab"
                  aria-selected={queueFilter === 'manual_review'}
                  className={`hr-side__tab${queueFilter === 'manual_review' ? ' hr-side__tab--active' : ''}`}
                  onClick={() => setQueueFilter('manual_review')}
                >
                  {MANUAL_REVIEW_LABEL}<span className="hr-side__tab-num">{manualReviewCount}</span>
                </button>
              </div>
            )}
            <div className="hr-batch">
              <label className="hr-batch__check">
                <input
                  ref={selectAllRef}
                  aria-label="选择全部审核项"
                  type="checkbox"
                  checked={allVisibleSelected}
                  onChange={(event) => setSelectedSubmissionIds(event.target.checked ? visibleSubmissionIds : [])}
                />
                已选 {showingDemo ? 3 : selectedSubmissionIds.length} 条
              </label>
              <button disabled={!showingDemo && selectedSubmissionIds.length === 0} onClick={() => void batchReview('approve')}>批量通过</button>
              <button disabled={!showingDemo && selectedSubmissionIds.length === 0} onClick={() => void batchReview('revise')}>批量打回</button>
            </div>
            {showingDemo ? (
              <div className="ai-stat-card">
                <div className="ai-stat-card__radial" />
                <div>
                  <div className="ai-stat-card__rate">38 / s</div>
                  <div className="ai-stat-card__meta">平均耗时 1.4s · 重试率 1.2%<br />任务 T-2041 · 规则「电商相关性 v2」</div>
                </div>
              </div>
            ) : null}
            <div>
              {queueItems.length === 0 && !showingDemo ? (
                <EmptyState title="队列为空" body="当前没有待人工审核的提交。" variant="empty" />
              ) : null}
              {queueItems.map((item) => (
                <QueueCard
                  key={item.kind === 'real' ? item.submission.id : item.item.id}
                  item={item}
                  active={isQueueItemActive(item, selected, selectedDemoId)}
                  selected={item.kind === 'real' ? selectedSubmissionIds.includes(item.submission.id) : false}
                  onToggleSelected={(checked) => {
                    if (item.kind !== 'real') return
                    setSelectedSubmissionIds((current) => checked
                      ? Array.from(new Set([...current, item.submission.id]))
                      : current.filter((id) => id !== item.submission.id))
                  }}
                  onClick={() => void openQueueItem(item)}
                />
              ))}
            </div>
          </aside>

          <main className="hr-main" style={hrMainStyle}>
            {showingDemo ? (
              <DemoReviewDetail item={selectedDemo} detail={selectedDemoDetail} reason={reason} setReason={setReason} />
            ) : (
              <RealReviewDetail
                detail={detail}
                selected={selected}
                schema={schema}
                payload={payload}
                answer={answer}
                reason={reason}
                setReason={setReason}
                stageLabel={detailStage.label}
                stageProgress={detailStage.progress}
              />
            )}

            <div className="hr-decisions">
              <button className="hr-decision hr-decision--reject" onClick={() => void review('revise')} disabled={loading || (!showingDemo && !schema.ok)}>
                <div className="hr-decision__title">↩ 打回</div>
                <div className="hr-decision__hint">返回标注员修改 · 重新提交</div>
              </button>
              <button className="hr-decision hr-decision--revise" onClick={() => void review('reject')} disabled={loading || (!showingDemo && !schema.ok)}>
                <div className="hr-decision__title">✎ 拒绝</div>
                <div className="hr-decision__hint">终止本条提交 · 记录拒绝原因</div>
              </button>
              <button className="hr-decision hr-decision--approve" onClick={() => void review('approve')} disabled={loading || (!showingDemo && !schema.ok)}>
                <div className="hr-decision__title">✓ {approveLabel}</div>
                <div className="hr-decision__hint">{showingDemo || !selected ? '本条进入终审 / 可导出' : `推进一级 · 审核进度 ${detailStage.progress}`}</div>
              </button>
            </div>

          </main>

          <aside className="hr-right" style={hrRightStyle}>
            {showingDemo ? <MetricGrid /> : null}
            {rulePanelOpen ? (
              <RuleConfigPanel
                prompts={ruleConfigs}
                selectedRule={selectedRule}
                selectedRuleId={selectedRuleId}
                activePromptId={ruleActivePromptId}
                aiReviewEnabled={ruleAIReviewEnabled}
                currentPromptId={detail?.aiReview?.prompt?.id ?? null}
                loading={ruleLoading}
                error={ruleError}
                onSelectRule={setSelectedRuleId}
                onClose={() => {
                  ruleLoadSeq.current += 1
                  setRulePanelOpen(false)
                  setRuleLoading(false)
                }}
              />
            ) : null}
            <Timeline
              auditLogs={showingDemo ? undefined : detail?.auditLogs}
              submissionId={showingDemo ? selectedDemo.id : detail?.submission?.id}
              demoEvents={showingDemo ? selectedDemoDetail.timeline : undefined}
              demoMode={showingDemo}
            />
          </aside>
        </div>
      )}
    </div>
  )
}

function QueueCard({
  item,
  active,
  selected,
  onToggleSelected,
  onClick,
}: {
  item: QueueItem
  active: boolean
  selected: boolean
  onToggleSelected: (checked: boolean) => void
  onClick: () => void
}) {
  if (item.kind === 'real') {
    const submission = item.submission
    const stage = resolveStage(submission)
    // 转人工复核(AI 可疑):独立中文标签 + 中文 StatusBadge,与「AI 通过待初审」区分。
    const isManual = submission.status === MANUAL_REVIEW_STATUS
    return (
      <div className={`hr-item${active ? ' hr-item--selected' : ''}`}>
        <div className="hr-item__head">
          <input
            aria-label={`选择 Submission #${submission.id}`}
            checked={selected}
            onChange={(event) => onToggleSelected(event.target.checked)}
            type="checkbox"
          />
        </div>
        <button onClick={onClick} style={queueCardButtonStyle}>
          <div className="hr-item__head" style={{ marginBottom: 6 }}>
            <span className="hr-item__sub">Submission #{submission.id}</span>
            <span>· Task #{submission.taskId} · Item #{submission.itemId}</span>
          </div>
          <div className="hr-item__tags">
            {isManual ? <span className="hr-tag hr-tag--warning">{MANUAL_REVIEW_LABEL}</span> : null}
            <span className="hr-tag hr-tag--ai">预审 {formatAIReviewSummary(submission)}</span>
            <span className="hr-tag hr-tag--purple" aria-label={`审核阶段 ${stage.label}`}>{stage.label} {stage.progress}</span>
            <StatusBadge status={submission.status} label={isManual ? MANUAL_REVIEW_LABEL : undefined} />
          </div>
        </button>
      </div>
    )
  }
  return (
    <button onClick={onClick} className={`hr-item${active ? ' hr-item--selected' : ''}`} style={demoQueueButtonStyle}>
      <div className="hr-item__head">
        <span className="hr-item__sub">{item.item.id}</span>
        <span>· {item.item.meta}</span>
      </div>
      <div className="hr-item__title">{item.item.title}</div>
      <div className="hr-item__tags">
        <span className="hr-tag hr-tag--ai">{item.item.badge}</span>
        <span className={demoVerdictTagClass(item.item.tone)}>{item.item.verdict}</span>
      </div>
    </button>
  )
}

function DemoReviewDetail({
  item,
  detail,
  reason,
  setReason,
}: {
  item: DemoReviewItem
  detail: DemoReviewSnapshot
  reason: string
  setReason: (value: string) => void
}) {
  const headlineScore = detail.scoreRows.find((row) => row.label === '综合')?.value ?? item.score
  return (
    <section style={detailShellStyle}>
      <div style={detailHeaderStyle}>
        <div>
          <h2 style={detailTitleStyle}>{item.id} · {item.title}</h2>
          <p style={mutedTextStyle}>题目 Q-2041-007 · 模板 r12 · 第 2 轮审核（上一轮标注员修改后重审）</p>
        </div>
        <span style={orangePillStyle}>第 2 轮 · 复审中</span>
      </div>

      <div style={compareGridStyle}>
        <CompareBox title="第 1 轮提交（已打回）" rows={detail.beforeRows} />
        <CompareBox title="第 2 轮提交（本轮 · 修改后）" rows={detail.afterRows} highlight />
      </div>

      <section style={aiResultStyle}>
        <div style={sectionTitleRowStyle}>
          <h3 style={sectionTitleStyle}><span className="lh-icon-text"><Icon name="sparkle" size={15} />AI 预审 · 本轮重跑结果</span></h3>
          <span style={modelPillStyle}>{detail.promptLabel}</span>
        </div>
        <div style={scoreLineStyle}>
          <strong>综合 <span>{headlineScore}</span></strong>
          {detail.scoreRows.filter((row) => row.label !== '综合').map((row) => (
            <span key={row.label}>{row.label} {row.value}</span>
          ))}
        </div>
        <ScoreBars rows={detail.scoreRows} />
        <p style={resultReasonStyle}>{detail.summary}</p>
      </section>

      <label style={labelStyle}>审核意见（打回时必填）</label>
      <textarea value={reason} onChange={(event) => setReason(event.target.value)} style={textareaStyle} />
      <div style={tagRowStyle}>
        {[
          detail.afterRows.find(([key]) => key === 'category')?.[1] ?? '未分类',
          ...((detail.afterRows.find(([key]) => key === 'keywords')?.[1] ?? '').split(',').map((value) => value.trim()).filter(Boolean)),
        ].map((tag) => <span key={tag} style={neutralPillStyle}>{tag}</span>)}
      </div>

      <div style={diagnosticGridStyle}>
        <section style={diagnosticPanelStyle}>
          <div style={sectionTitleRowStyle}>
            <h3 style={sectionTitleStyle}>提交内容</h3>
            <span style={neutralPillStyle}>JSON 字段视图</span>
          </div>
          <pre style={codeBlockStyle}>{detail.jsonPreview}</pre>
        </section>
        <section style={diagnosticPanelStyle}>
          <div style={sectionTitleRowStyle}>
            <h3 style={sectionTitleStyle}>审核 Prompt 模板</h3>
            <span style={modelPillStyle}>{detail.promptLabel}</span>
          </div>
          <pre style={codeBlockStyle}>{detail.promptTemplate}</pre>
        </section>
      </div>

      <section style={diagnosticPanelStyle}>
        <h3 style={sectionTitleStyle}>处理日志 / 审计</h3>
        <div style={processLogStyle}>
          {detail.timeline.map((log) => (
            <div key={`${log.time}-${log.actor}`} style={processLogRowStyle}>
              <span>{log.time}</span>
              <span style={neutralPillStyle}>{log.actor}</span>
              <strong>{log.action}</strong>
            </div>
          ))}
        </div>
      </section>
    </section>
  )
}

function ScoreBars({ rows }: { rows?: Array<{ label: string, value: number, color?: string }> }) {
  const displayRows = rows ?? [
    { label: '相关性', value: 92, color: 'var(--color-success)' },
    { label: '准确性', value: 84, color: '#f97316' },
    { label: '格式合规', value: 88, color: '#f97316' },
    { label: '安全性', value: 99, color: 'var(--color-success)' },
    { label: '综合', value: 86, color: '#f97316' },
  ]
  return (
    <div style={scoreBarsStyle}>
      {displayRows.map(({ label, value, color }) => (
        <div key={label} style={scoreBarRowStyle}>
          <span>{label}</span>
          <div style={scoreTrackStyle}>
            <div style={{ ...scoreFillStyle, width: `${clampScore(value)}%`, background: color ?? scoreColor(value) }} />
          </div>
          <strong>{value}</strong>
        </div>
      ))}
    </div>
  )
}

function RealReviewDetail({
  detail,
  selected,
  schema,
  payload,
  answer,
  reason,
  setReason,
  stageLabel,
  stageProgress,
}: {
  detail: TaskBundle | null
  selected: Submission | null
  schema: ParsedSchema
  payload: Record<string, unknown>
  answer: AnswerValue
  reason: string
  setReason: (value: string) => void
  stageLabel: string
  stageProgress: string
}) {
  if (!detail?.item) {
    return (
      <EmptyState title="等待审核" body="从左侧队列选择一条提交记录开始人工审核。" variant="queue" />
    )
  }

  return (
    <>
      <div className="hr-main__head">
        <h2>{schema.ok ? schema.schema.title : '提交详情'}</h2>
        <div className="hr-main__head-right">
          <span className="lh-tag lh-tag--purple" aria-label={`审核阶段 ${stageLabel}`}>{stageLabel} · 审核进度 {stageProgress}</span>
          <StatusBadge status={selected?.status} label={selected?.status === MANUAL_REVIEW_STATUS ? MANUAL_REVIEW_LABEL : undefined} />
        </div>
      </div>
      <div className="hr-main__meta">Submission #{detail.submission?.id} · Task #{detail.task.id} · Item #{detail.item.id}</div>

      <div style={rendererShellStyle}>
        {schema.ok ? (
          <SchemaRenderer
            schema={schema.schema}
            payload={payload}
            value={answer}
            readOnly
            runtime={{ taskId: detail.task.id, itemId: detail.item.id, submissionId: detail.submission?.id }}
          />
        ) : (
          <div role="alert" style={errorBannerStyle}>{schema.message}</div>
        )}
      </div>

      <AIVerdictPanel aiReview={detail.aiReview} submission={detail.submission} />

      {detail.latestHumanReview?.reason ? (
        <section className="ai-section" style={previousReviewStyle}>
          <div className="ai-section__head"><span className="ai-section__title">上一轮意见</span></div>
          <p style={resultReasonStyle}>{detail.latestHumanReview.reason}</p>
        </section>
      ) : null}

      <div className="hr-review-label">审核意见 <span className="lh-muted">（打回时必填）</span></div>
      <textarea className="hr-textarea" value={reason} onChange={(event) => setReason(event.target.value)} placeholder="输入审核意见..." />
      <div className="hr-quick-tags">
        {QUICK_TAGS.map((tag) => (
          <span key={tag} className="hr-quick-tag" onClick={() => setReason(reason ? `${reason} ${tag}` : tag)}>{tag}</span>
        ))}
      </div>

      <RealAIDiagnostics answer={detail.revision?.answer} aiReview={detail.aiReview} auditLogs={detail.auditLogs ?? []} />
    </>
  )
}

function RealAIDiagnostics({ answer, aiReview, auditLogs }: { answer?: string, aiReview?: AIReviewDetail | null, auditLogs: AuditLog[] }) {
  if (!aiReview && auditLogs.length === 0) {
    return null
  }
  return (
    <>
      <div style={diagnosticGridStyle}>
        <section style={diagnosticPanelStyle}>
          <div style={sectionTitleRowStyle}>
            <h3 style={sectionTitleStyle}>提交内容</h3>
            <span style={neutralPillStyle}>JSON 字段视图</span>
          </div>
          <pre style={codeBlockStyle}>{formatJSONForDisplay(answer)}</pre>
        </section>
        <section style={diagnosticPanelStyle}>
          <div style={sectionTitleRowStyle}>
            <h3 style={sectionTitleStyle}>审核 Prompt 模板</h3>
            <span style={modelPillStyle}>{aiReview?.prompt ? `规则：v${aiReview.prompt.version}` : '未绑定规则'}</span>
          </div>
          <pre style={codeBlockStyle}>{aiReview?.prompt?.promptTemplate ?? '当前提交没有可展示的 AI Prompt 模板。'}</pre>
        </section>
      </div>
      <section style={diagnosticPanelStyle}>
        <h3 style={sectionTitleStyle}>处理日志 / 审计</h3>
        <div style={processLogStyle}>
          {aiReview ? (
            <div style={processLogRowStyle}>
              <span>{formatTime(aiReview.createdAt)}</span>
              <span style={neutralPillStyle}>AI 预审</span>
              <strong>{aiReviewStatusLabel(aiReview.status)} · {aiVerdictLabel(aiReview.verdict)} · {formatAIScore(aiReview.overallScore)}</strong>
            </div>
          ) : null}
          {auditLogs.map((log) => (
            <div key={log.id} style={processLogRowStyle}>
              <span>{formatTime(log.createdAt)}</span>
              <span style={neutralPillStyle}>{auditEventLabel(log.event)}</span>
              <strong>{auditActorLabel(log.actorType)} · {auditStateText(log)}</strong>
            </div>
          ))}
        </div>
      </section>
    </>
  )
}

function CompareBox({ title, rows, highlight = false }: { title: string, rows: Array<[string, string]>, highlight?: boolean }) {
  return (
    <div style={compareBoxStyle}>
      <h3 style={sectionTitleStyle}>{title}</h3>
      <div style={compareRowsStyle}>
        {rows.map(([key, value]) => (
          <div key={key} style={compareRowStyle}>
            <span>{key}</span>
            <strong style={highlight && key !== 'cleaned_title' ? highlightValueStyle : undefined}>{value}</strong>
          </div>
        ))}
      </div>
    </div>
  )
}

function MetricGrid() {
  return (
    <div style={metricGridStyle}>
      <Metric label="我今日已审" value="214" tone="blue" />
      <Metric label="我今日通过率" value="87%" tone="green" />
      <Metric label="待我审核" value="47" tone="orange" />
      <Metric label="SLA 剩余" value="02:14:00" tone="blue" />
    </div>
  )
}

function Metric({ label, value, tone }: { label: string, value: string, tone: 'blue' | 'green' | 'orange' }) {
  return (
    <div style={metricStyle}>
      <span>{label}</span>
      <strong style={{ color: tone === 'green' ? 'var(--color-success)' : tone === 'orange' ? '#f97316' : 'var(--color-accent)' }}>{value}</strong>
    </div>
  )
}

function Timeline({
  auditLogs,
  submissionId,
  demoMode = false,
  demoEvents,
}: {
  auditLogs?: AuditLog[]
  submissionId?: number | string
  demoMode?: boolean
  demoEvents?: DemoTimelineEvent[]
}) {
  if (auditLogs && auditLogs.length > 0) {
    return (
      <section style={timelineStyle}>
        <h3 style={sectionTitleStyle}>审计时间线（{submissionId ? `SUB-${submissionId}` : '当前提交'}）</h3>
        {auditLogs.map((log) => (
          <div key={log.id} style={timelineRowStyle}>
            <span style={{ ...timelineDotStyle, background: log.actorType === 'ai_worker' || log.event.startsWith('ai_') ? '#7c3aed' : 'var(--color-success)' }} />
            <div>
              <strong>{auditActorLabel(log.actorType)}</strong>
              <p>{auditEventLabel(log.event)} · {auditStateText(log)}</p>
            </div>
          </div>
        ))}
      </section>
    )
  }
  if (!demoMode) {
    return (
      <section style={timelineStyle}>
        <h3 style={sectionTitleStyle}>审计时间线{submissionId ? `（SUB-${submissionId}）` : ''}</h3>
        <EmptyState title="暂无审计记录" body="提交进入审核流程后,事件会自动写入审计时间线。" variant="empty" />
      </section>
    )
  }
  return (
    <section style={timelineStyle}>
      <h3 style={sectionTitleStyle}>审计时间线（{submissionId ?? 'SUB-00607'}）</h3>
      {(demoEvents ?? []).map((event) => (
        <div key={`${event.time}-${event.actor}`} style={timelineRowStyle}>
          <span style={{ ...timelineDotStyle, background: event.tone === 'green' ? 'var(--color-success)' : event.tone === 'red' ? 'var(--color-danger)' : 'var(--color-accent)' }} />
          <div>
            <strong>{event.actor}</strong>
            <p>{event.action}</p>
          </div>
        </div>
      ))}
    </section>
  )
}

function RuleConfigPanel({
  prompts,
  selectedRule,
  selectedRuleId,
  activePromptId,
  aiReviewEnabled,
  currentPromptId,
  loading,
  error,
  onSelectRule,
  onClose,
}: {
  prompts: AIPromptSummary[]
  selectedRule: AIPromptSummary | null
  selectedRuleId: number | null
  activePromptId: number | null
  aiReviewEnabled: boolean
  currentPromptId: number | null
  loading: boolean
  error: string
  onSelectRule: (id: number | null) => void
  onClose: () => void
}) {
  return (
    <section style={rulePanelStyle}>
      <div style={sectionTitleRowStyle}>
        <h3 style={sectionTitleStyle}>规则配置</h3>
        <button type="button" onClick={onClose} style={iconButtonStyle} aria-label="关闭规则配置">×</button>
      </div>
      {error ? <div role="alert" style={ruleErrorStyle}>{error}</div> : null}
      {loading ? <p style={mutedTextStyle}>加载规则版本...</p> : null}
      <div style={ruleMetaStyle}>
        <span style={aiReviewEnabled ? successPillStyle : neutralPillStyle}>AI review {aiReviewEnabled ? 'ON' : 'OFF'}</span>
        {activePromptId ? <span style={neutralPillStyle}>active #{activePromptId}</span> : <span style={neutralPillStyle}>no active rule</span>}
      </div>
      <label style={fieldSelectStyle}>
        <span>规则版本</span>
        <select
          aria-label="reviewer_rule_select"
          disabled={loading || prompts.length === 0}
          value={selectedRuleId ?? ''}
          onChange={(event) => onSelectRule(event.target.value ? Number(event.target.value) : null)}
          style={selectInputStyle}
        >
          {prompts.length === 0 ? <option value="">暂无可选规则</option> : null}
          {prompts.map((prompt) => (
            <option key={prompt.id} value={prompt.id}>
              v{prompt.version} · {prompt.model}{prompt.id === activePromptId ? ' · active' : ''}{prompt.id === currentPromptId ? ' · current' : ''}
            </option>
          ))}
        </select>
      </label>
      {selectedRule ? (
        <>
          <div style={ruleSummaryGridStyle}>
            <span>v{selectedRule.version}</span>
            <span>{selectedRule.model}</span>
            <span>pass {selectedRule.passThreshold}</span>
            <span>min {selectedRule.uncertainMin}</span>
          </div>
          <pre style={ruleCodeBlockStyle}>{selectedRule.promptTemplate}</pre>
          <pre style={ruleCodeBlockStyle}>{formatRuleDimensions(selectedRule.dimensions)}</pre>
          <p style={mutedTextStyle}>规则仅供查看。</p>
        </>
      ) : (
        <p style={mutedTextStyle}>当前任务还没有 AI Prompt 规则。</p>
      )}
    </section>
  )
}

function parseBundleSchema(bundle: TaskBundle | null): ParsedSchema {
  if (!bundle?.template?.schemaJson) {
    return { ok: false, message: '当前提交缺少模板快照' }
  }
  const result = parseTemplateSchema(bundle.template.schemaJson)
  if (!result.ok) {
    return { ok: false, message: `${result.error.field}: ${result.error.message}` }
  }
  return { ok: true, schema: result.value }
}

function isQueueItemActive(item: QueueItem, selected: Submission | null, selectedDemoId: string) {
  return item.kind === 'real' ? item.submission.id === selected?.id : item.item.id === selectedDemoId
}

function formatAIReviewSummary(submission: Submission) {
  if (!submission.aiVerdict) return 'AI 未预审'
  return `AI ${submission.aiVerdict} · ${formatAIScore(submission.aiScore)}`
}

function formatAIScore(score: number | null | undefined) {
  return typeof score === 'number' && Number.isFinite(score) ? String(score) : '-'
}

function clampScore(value: number) {
  return Math.max(0, Math.min(100, value))
}

function scoreColor(value: number) {
  return value >= 90 ? 'var(--color-success)' : value >= 70 ? '#f97316' : 'var(--color-danger)'
}

function formatJSONForDisplay(raw: string | undefined) {
  if (!raw) {
    return '{}'
  }
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}

function formatTime(raw: string | undefined) {
  if (!raw) {
    return '--:--'
  }
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) {
    return raw
  }
  return date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })
}

function formatRuleDimensions(raw: unknown) {
  if (typeof raw === 'string') {
    try {
      return JSON.stringify(JSON.parse(raw), null, 2)
    } catch {
      return raw
    }
  }
  return JSON.stringify(raw ?? [], null, 2)
}

const AI_REVIEW_STATUS_LABELS: Record<string, string> = {
  pending: '待处理',
  running: '处理中',
  succeeded: '成功',
  failed: '失败',
  dead: '失败终止',
}

const AI_VERDICT_LABELS: Record<string, string> = {
  pass: '通过',
  reject: '打回',
  uncertain: '需人工复核',
  manual: '需人工复核',
}

const AUDIT_EVENT_LABELS: Record<string, string> = {
  save: '保存草稿',
  submit: '标注提交',
  enqueue: '进入 AI 预审队列',
  skip_ai: '跳过 AI 预审',
  ai_done: 'AI 预审通过',
  ai_uncertain: 'AI 转人工复核',
  ai_reject: 'AI 预审打回',
  ai_fail_max: 'AI 失败转人工',
  ai_retry: 'AI 预审重试',
  consensus_conflict: '重叠仲裁冲突',
  consensus_evidence: '重叠证据归档',
  sampling_auto_approved: '抽样免审通过',
  approve: '人工通过',
  reject: '人工拒绝',
  revise: '打回修改',
  human_approve_stage: '人工审核通过一级',
  arbitration_sibling_rejected: '仲裁同题提交拒绝',
  acceptance_reopen: '验收不通过返审',
  queued: '已入队',
  exported: '已导出',
}

const AUDIT_STATE_LABELS: Record<string, string> = {
  draft: '草稿',
  submitted: '已提交',
  ai_reviewing: 'AI 预审中',
  manual_review: '人工复核',
  human_reviewing: '人工审核中',
  revising: '返修中',
  approved: '已通过',
  rejected: '已拒绝',
  needs_arbitration: '待仲裁',
  consensus_evidence: '共识证据',
  queued: '排队中',
  pending: '待处理',
  running: '处理中',
  succeeded: '成功',
  failed: '失败',
  dead: '失败终止',
}

const AUDIT_ACTOR_LABELS: Record<string, string> = {
  user: '人工',
  system: '系统',
  ai_worker: 'AI Agent',
  owner: '任务负责人',
  reviewer: '审核员',
  labeler: '标注员',
}

function aiReviewStatusLabel(status: string | null | undefined) {
  return labelFromMap(AI_REVIEW_STATUS_LABELS, status, '未知状态')
}

function aiVerdictLabel(verdict: string | null | undefined) {
  return labelFromMap(AI_VERDICT_LABELS, verdict, '暂无结论')
}

function auditEventLabel(event: string) {
  return labelFromMap(AUDIT_EVENT_LABELS, event, event)
}

function auditStateLabel(state: string) {
  return labelFromMap(AUDIT_STATE_LABELS, state, state)
}

function auditActorLabel(actorType: string) {
  return labelFromMap(AUDIT_ACTOR_LABELS, actorType, actorType)
}

function labelFromMap(labels: Record<string, string>, value: string | null | undefined, fallback: string) {
  const key = (value ?? '').trim()
  return key ? labels[key] ?? key : fallback
}

function auditStateText(log: AuditLog) {
  const fromState = log.fromState ?? ''
  return fromState ? `${auditStateLabel(fromState)} → ${auditStateLabel(log.toState)}` : auditStateLabel(log.toState)
}

function canRetryAIReview(aiReview: AIReviewDetail | null | undefined) {
  return aiReview?.status === 'failed' || aiReview?.status === 'dead'
}

function demoVerdictTagClass(tone: DemoReviewItem['tone']): string {
  switch (tone) {
    case 'pass':
      return 'hr-tag hr-tag--success'
    case 'reject':
      return 'hr-tag hr-tag--warning'
    case 'failed':
      return 'hr-tag hr-tag--danger'
    default:
      return 'hr-tag hr-tag--purple'
  }
}

const pageStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-lg)',
}

const topBarStyle: CSSProperties = {
  display: 'flex',
  justifyContent: 'space-between',
  gap: 'var(--space-lg)',
  alignItems: 'flex-start',
}

const breadcrumbStyle: CSSProperties = {
  fontSize: 'var(--text-sm)',
  color: 'var(--color-text-secondary)',
  marginBottom: 'var(--space-md)',
}

const pageTitleStyle: CSSProperties = {
  margin: 0,
  fontFamily: 'var(--font-heading)',
  fontSize: 'var(--text-h1)',
  fontWeight: 700,
}

const pageSubTitleStyle: CSSProperties = {
  margin: 'var(--space-xs) 0 0',
  color: 'var(--color-text-secondary)',
  fontSize: 'var(--text-sm)',
}

const headerActionsStyle: CSSProperties = {
  display: 'flex',
  gap: 'var(--space-md)',
  alignItems: 'center',
}

// 顶部视图切换 tab(审核工作台 / 仲裁)沿用 .hr-side__tabs 样式但收窄。
const viewTabsStyle: CSSProperties = {
  maxWidth: 320,
}

// hr-shell 默认是 flex 全高布局,这里包在卡片内,允许内容区收缩。
const shellStyle: CSSProperties = {
  minHeight: 640,
  border: '1px solid var(--lh-border)',
  borderRadius: 'var(--lh-radius-lg)',
  overflow: 'hidden',
  background: 'var(--lh-bg-card)',
}

const hrSideStyle: CSSProperties = {
  minWidth: 0,
}

const hrMainStyle: CSSProperties = {
  minWidth: 0,
}

const hrRightStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-md)',
  alignContent: 'start',
}

const demoQueueButtonStyle: CSSProperties = {
  display: 'block',
  width: '100%',
  textAlign: 'left',
  font: 'inherit',
  color: 'inherit',
}

const queueCardButtonStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-xs)',
  width: '100%',
  padding: 0,
  border: 0,
  background: 'transparent',
  textAlign: 'left',
  color: 'inherit',
  cursor: 'pointer',
}

const neutralPillStyle: CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  justifyContent: 'center',
  flex: '0 0 auto',
  width: 'fit-content',
  maxWidth: '100%',
  borderRadius: 999,
  background: 'var(--color-panel-header)',
  color: 'var(--color-text-secondary)',
  padding: '2px 8px',
  fontSize: 'var(--text-sm)',
  fontWeight: 600,
  lineHeight: 1.3,
  whiteSpace: 'nowrap',
}

const successPillStyle: CSSProperties = {
  ...neutralPillStyle,
  color: 'var(--color-success)',
  background: 'var(--color-success-soft)',
}

const orangePillStyle: CSSProperties = {
  ...neutralPillStyle,
  color: '#f97316',
  background: '#fff7ed',
  whiteSpace: 'nowrap',
}

const modelPillStyle: CSSProperties = {
  ...neutralPillStyle,
  color: '#7c3aed',
  background: '#f3e8ff',
}

// M-10:演示模式醒目标识,提醒看到的是样例数据而非真实队列。
const demoBadgeStyle: CSSProperties = {
  ...neutralPillStyle,
  color: '#f97316',
  background: '#fff7ed',
  border: '1px solid #f97316',
}

const detailShellStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-md)',
  background: 'var(--color-surface)',
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-lg)',
  padding: 'var(--space-md)',
}

const detailHeaderStyle: CSSProperties = {
  display: 'flex',
  justifyContent: 'space-between',
  gap: 'var(--space-md)',
  alignItems: 'flex-start',
  borderBottom: '1px solid var(--color-border-light)',
  paddingBottom: 'var(--space-md)',
}

const detailTitleStyle: CSSProperties = {
  margin: 0,
  fontSize: '1.05rem',
  fontWeight: 700,
  lineHeight: 1.35,
}

const mutedTextStyle: CSSProperties = {
  margin: '4px 0 0',
  color: 'var(--color-text-muted)',
  fontSize: 'var(--text-sm)',
}

const compareGridStyle: CSSProperties = {
  display: 'grid',
  gridTemplateColumns: 'minmax(0, 1fr) minmax(0, 1fr)',
  gap: 'var(--space-sm)',
}

const compareBoxStyle: CSSProperties = {
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-md)',
  padding: 'var(--space-sm)',
  minWidth: 0,
}

const compareRowsStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-xs)',
  marginTop: 'var(--space-md)',
}

const compareRowStyle: CSSProperties = {
  display: 'grid',
  gridTemplateColumns: '82px minmax(0, 1fr)',
  gap: 'var(--space-sm)',
  borderBottom: '1px dashed var(--color-border-light)',
  paddingBottom: 'var(--space-xs)',
  color: 'var(--color-text-secondary)',
  fontSize: 'var(--text-sm)',
}

const highlightValueStyle: CSSProperties = {
  background: 'var(--color-success-soft)',
  color: 'var(--color-success)',
  padding: '1px 4px',
  borderRadius: 4,
}

const aiResultStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-sm)',
  border: '1px solid #c084fc',
  borderRadius: 'var(--radius-md)',
  background: '#fbf7ff',
  padding: 'var(--space-md)',
}

const previousReviewStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-xs)',
  border: '1px solid var(--color-warning-soft)',
  borderRadius: 'var(--radius-md)',
  background: '#fff8e1',
  padding: 'var(--space-md)',
}

const sectionTitleRowStyle: CSSProperties = {
  display: 'flex',
  justifyContent: 'space-between',
  alignItems: 'center',
  gap: 'var(--space-md)',
}

const sectionTitleStyle: CSSProperties = {
  margin: 0,
  fontSize: 'var(--text-base)',
  fontWeight: 700,
}

const scoreLineStyle: CSSProperties = {
  display: 'flex',
  gap: 'var(--space-lg)',
  flexWrap: 'wrap',
  color: 'var(--color-text-secondary)',
}

const scoreBarsStyle: CSSProperties = {
  display: 'grid',
  gridTemplateColumns: '1fr 1fr',
  gap: 'var(--space-sm) var(--space-lg)',
}

const scoreBarRowStyle: CSSProperties = {
  display: 'grid',
  gridTemplateColumns: '64px 1fr 36px',
  gap: 'var(--space-sm)',
  alignItems: 'center',
  color: 'var(--color-text-secondary)',
}

const scoreTrackStyle: CSSProperties = {
  height: 6,
  background: 'var(--color-border-light)',
  borderRadius: 999,
  overflow: 'hidden',
}

const scoreFillStyle: CSSProperties = {
  height: '100%',
  borderRadius: 999,
}

const resultReasonStyle: CSSProperties = {
  margin: 0,
  color: 'var(--color-text)',
  lineHeight: 1.6,
}

const labelStyle: CSSProperties = {
  fontWeight: 600,
  color: 'var(--color-text-secondary)',
  fontSize: 'var(--text-sm)',
}

const textareaStyle: CSSProperties = {
  width: '100%',
  minHeight: 84,
  padding: 'var(--space-md)',
  border: '1px solid var(--color-border)',
  borderRadius: 'var(--radius-sm)',
  fontFamily: 'var(--font-body)',
  fontSize: 'var(--text-base)',
  resize: 'vertical',
}

const tagRowStyle: CSSProperties = {
  display: 'flex',
  flexWrap: 'wrap',
  gap: 'var(--space-sm)',
}

const diagnosticGridStyle: CSSProperties = {
  display: 'grid',
  gridTemplateColumns: '1fr 1fr',
  gap: 'var(--space-md)',
}

const diagnosticPanelStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-sm)',
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-md)',
  padding: 'var(--space-md)',
  background: 'var(--color-surface)',
}

const codeBlockStyle: CSSProperties = {
  margin: 0,
  padding: 'var(--space-md)',
  border: '1px dashed var(--color-border)',
  borderRadius: 'var(--radius-sm)',
  background: 'var(--color-panel-header)',
  color: 'var(--color-text-secondary)',
  fontSize: 'var(--text-sm)',
  lineHeight: 1.6,
  whiteSpace: 'pre-wrap',
}

const processLogStyle: CSSProperties = {
  display: 'grid',
}

const processLogRowStyle: CSSProperties = {
  display: 'grid',
  gridTemplateColumns: '72px minmax(112px, max-content) minmax(0, 1fr)',
  gap: 'var(--space-sm)',
  alignItems: 'center',
  padding: 'var(--space-sm) 0',
  borderBottom: '1px dashed var(--color-border-light)',
  color: 'var(--color-text-secondary)',
  fontSize: 'var(--text-sm)',
}

const rendererShellStyle: CSSProperties = {
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-md)',
  padding: 'var(--space-md)',
}

const errorBannerStyle: CSSProperties = {
  padding: 'var(--space-lg)',
  borderRadius: 'var(--radius-md)',
  border: '1px solid var(--color-danger)',
  color: 'var(--color-danger)',
  background: '#fff1f0',
}

const metricGridStyle: CSSProperties = {
  display: 'grid',
  gridTemplateColumns: '1fr 1fr',
  gap: 'var(--space-sm)',
}

const metricStyle: CSSProperties = {
  display: 'grid',
  gap: 4,
  background: 'var(--color-panel-header)',
  borderRadius: 'var(--radius-md)',
  padding: 'var(--space-md)',
  fontSize: 'var(--text-sm)',
  color: 'var(--color-text-muted)',
}

const rulePanelStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-sm)',
  padding: 'var(--space-md)',
  border: '1px solid var(--color-accent-soft)',
  borderRadius: 'var(--radius-md)',
  background: 'var(--color-info-bg)',
}

const iconButtonStyle: CSSProperties = {
  width: 24,
  height: 24,
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-sm)',
  background: 'var(--color-surface)',
  color: 'var(--color-text-secondary)',
  cursor: 'pointer',
}

const ruleErrorStyle: CSSProperties = {
  padding: 'var(--space-sm)',
  borderRadius: 'var(--radius-sm)',
  background: 'var(--color-danger-soft)',
  color: 'var(--color-danger)',
  fontSize: 'var(--text-sm)',
}

const ruleMetaStyle: CSSProperties = {
  display: 'flex',
  gap: 'var(--space-xs)',
  flexWrap: 'wrap',
}

const fieldSelectStyle: CSSProperties = {
  display: 'grid',
  gap: 6,
  color: 'var(--color-text-secondary)',
  fontSize: 'var(--text-sm)',
  fontWeight: 600,
}

const selectInputStyle: CSSProperties = {
  width: '100%',
  minHeight: 36,
  border: '1px solid var(--color-border)',
  borderRadius: 'var(--radius-sm)',
  background: 'var(--color-surface)',
  color: 'var(--color-text)',
  padding: '0 var(--space-sm)',
}

const ruleSummaryGridStyle: CSSProperties = {
  display: 'grid',
  gridTemplateColumns: '1fr 1fr',
  gap: 'var(--space-xs)',
  color: 'var(--color-text-secondary)',
  fontSize: 'var(--text-sm)',
}

const ruleCodeBlockStyle: CSSProperties = {
  ...codeBlockStyle,
  maxHeight: 180,
  overflow: 'auto',
  background: 'var(--color-surface)',
}

const timelineStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-md)',
}

const timelineRowStyle: CSSProperties = {
  display: 'grid',
  gridTemplateColumns: '14px 1fr',
  gap: 'var(--space-sm)',
  alignItems: 'start',
  color: 'var(--color-text-secondary)',
  fontSize: 'var(--text-sm)',
}

const timelineDotStyle: CSSProperties = {
  width: 8,
  height: 8,
  borderRadius: 99,
  marginTop: 5,
}
