import type { CSSProperties } from 'react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Button, Toast } from '@douyinfe/semi-ui'
import { SchemaRenderer, parseAnswer, parseTemplateSchema } from '../../renderer'
import type { AnswerValue, TemplateSchema } from '../../renderer/types'
import { apiGet, apiPost, type AIPromptSummary, type AIReviewDetail, type AuditLog, type Submission, type TaskBundle } from '../../shared/api/client'
import EmptyState from '../../shared/components/EmptyState'
import { parsePayload } from '../../shared/components/payload'
import StatusBadge from '../../shared/components/StatusBadge'

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

type QueueItem =
  | { kind: 'real', submission: Submission }
  | { kind: 'demo', item: DemoReviewItem }

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
  const [reason, setReason] = useState('本轮修改已覆盖第 1 轮打回意见，关键词丰富度与类目准确性均达标。')
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

  const loadQueue = useCallback(async () => {
    try {
      const data = await apiGet<Submission[]>('/reviewer/submissions')
      setSubmissions(data)
      setSelectedSubmissionIds((current) => current.filter((id) => data.some((submission) => submission.id === id)))
      if (data.length === 0) {
        ruleLoadSeq.current += 1
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

  const queueItems = useMemo<QueueItem[]>(() => {
    if (submissions.length > 0) {
      return submissions.map((submission) => ({ kind: 'real', submission }))
    }
    return demoItems.map((item) => ({ kind: 'demo', item }))
  }, [submissions])

  const selectedDemo = demoItems.find((item) => item.id === selectedDemoId) ?? demoItems[1]
  const schema = useMemo(() => parseBundleSchema(detail), [detail])
  const payload = useMemo(() => parsePayload(detail?.item?.payload), [detail?.item?.payload])
  const answer = useMemo<AnswerValue>(() => parseAnswer(detail?.revision?.answer), [detail?.revision?.answer])
  const showingDemo = submissions.length === 0 && !selected && !detail
  const retryDisabled = !showingDemo && (!selected || !canRetryAIReview(detail?.aiReview))
  const selectedRule = useMemo(() => {
    if (ruleConfigs.length > 0) {
      return ruleConfigs.find((item) => item.id === selectedRuleId) ?? ruleConfigs[0]
    }
    return detail?.aiReview?.prompt ?? null
  }, [detail?.aiReview?.prompt, ruleConfigs, selectedRuleId])

  async function openQueueItem(item: QueueItem) {
    ruleLoadSeq.current += 1
    setRulePanelOpen(false)
    setRuleLoading(false)
    if (item.kind === 'demo') {
      setSelected(null)
      setDetail(null)
      setSelectedDemoId(item.item.id)
      setReason('本轮修改已覆盖第 1 轮打回意见，关键词丰富度与类目准确性均达标。')
      return
    }
    const submission = item.submission
    setSelected(submission)
    try {
      const data = await apiGet<TaskBundle>(`/reviewer/submissions/${submission.id}`)
      setDetail(data)
      setReason('')
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '加载详情失败')
    }
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

  return (
    <div style={pageStyle}>
      <header style={topBarStyle}>
        <div>
          <div style={breadcrumbStyle}>审核与质检 / <strong>{showingDemo ? 'AI 预审规则 · 队列' : '人工审核 / 商品标题清洗 v3'}</strong></div>
          <h1 style={pageTitleStyle}>{showingDemo ? 'AI 自动预审队列' : '人工审核工作台'}</h1>
          <p style={pageSubTitleStyle}>异步消费提交数据 → 按评分维度调用 LLM 结构化输出 → 通过 / 打回 / 转人工复核</p>
        </div>
        <div style={headerActionsStyle}>
          <span style={modelPillStyle}>Agent v2.3 · 模型 doubao-pro-32k</span>
          <Button loading={ruleLoading} onClick={() => void openRuleConfig()} theme="light">规则配置</Button>
          <Button disabled={retryDisabled} loading={retryingAI} onClick={() => void retryAIReview()} theme="light">失败重跑</Button>
        </div>
      </header>

      <div style={layoutStyle}>
        <aside style={leftPanelStyle}>
          <div style={tabRowStyle}>
            <button style={activeTabStyle}>AI 已建议通过 <strong>128</strong></button>
            <button style={tabStyle}>AI 已建议打回 <strong>47</strong></button>
            <button style={tabStyle}>转人工 <strong>9</strong></button>
          </div>
          <div style={bulkBarStyle}>
            <input
              aria-label="选择全部审核项"
              type="checkbox"
              checked={!showingDemo && submissions.length > 0 && selectedSubmissionIds.length === submissions.length}
              onChange={(event) => setSelectedSubmissionIds(event.target.checked ? submissions.map((submission) => submission.id) : [])}
            />
            <span>已选 {showingDemo ? 3 : selectedSubmissionIds.length} 条</span>
            <button disabled={!showingDemo && selectedSubmissionIds.length === 0} onClick={() => void batchReview('approve')} style={miniButtonStyle}>批量通过</button>
            <button disabled={!showingDemo && selectedSubmissionIds.length === 0} onClick={() => void batchReview('revise')} style={miniButtonStyle}>批量打回</button>
          </div>
          <div style={slaCardStyle}>
            <strong>38</strong>
            <span>/ s · 平均耗时 1.4s · 重试率 1.2%</span>
            <small>任务 T-2041 · 规则「电商相关性 v2」</small>
          </div>
          <div style={queueListStyle}>
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

        <main style={centerPanelStyle}>
          {showingDemo ? (
            <DemoReviewDetail item={selectedDemo} reason={reason} setReason={setReason} />
          ) : (
            <RealReviewDetail
              detail={detail}
              selected={selected}
              schema={schema}
              payload={payload}
              answer={answer}
              reason={reason}
              setReason={setReason}
            />
          )}

          <div style={decisionGridStyle}>
            <button style={rejectDecisionStyle} onClick={() => void review('revise')} disabled={!showingDemo && !schema.ok}>
              <strong>↩ 打回</strong>
              <span>返回标注员修改 · 第 3 轮</span>
            </button>
            <button style={fixDecisionStyle} onClick={() => void review('reject')} disabled={!showingDemo && !schema.ok}>
              <strong>✎ 拒绝</strong>
              <span>终止本条提交 · 记录拒绝原因</span>
            </button>
            <button style={passDecisionStyle} onClick={() => void review('approve')} disabled={!showingDemo && !schema.ok}>
              <strong>✓ 通过 · 入库</strong>
              <span>本条进入终审 / 可导出</span>
            </button>
          </div>

          {selected ? (
            <div style={legacyActionRowStyle}>
              <Button aria-label="打回修改" disabled={!schema.ok} loading={loading} onClick={() => void review('revise')} theme="light">打回修改</Button>
              <Button aria-label="拒绝" disabled={!schema.ok} loading={loading} onClick={() => void review('reject')} theme="light" type="danger">拒绝</Button>
              <Button aria-label="通过" disabled={!schema.ok} loading={loading} theme="solid" onClick={() => void review('approve')}>通过</Button>
            </div>
          ) : null}
        </main>

        <aside style={rightPanelStyle}>
          <MetricGrid />
          {rulePanelOpen ? (
            <RuleConfigPanel
              taskId={showingDemo ? null : detail?.task.id}
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
          <Timeline auditLogs={showingDemo ? undefined : detail?.auditLogs} submissionId={showingDemo ? selectedDemo.id : detail?.submission?.id} />
        </aside>
      </div>
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
    return (
      <div style={active ? activeQueueCardStyle : queueCardStyle}>
        <label style={queueSelectStyle}>
          <input
            aria-label={`选择 Submission #${submission.id}`}
            checked={selected}
            onChange={(event) => onToggleSelected(event.target.checked)}
            type="checkbox"
          />
          <span>选择</span>
        </label>
        <button onClick={onClick} style={queueCardButtonStyle}>
          <div style={cardMetaStyle}>Task #{submission.taskId} · Item #{submission.itemId}</div>
          <strong>Submission #{submission.id}</strong>
          <div style={pillRowStyle}>
            <span style={aiPillStyle}>预审 {formatAIReviewSummary(submission)}</span>
            <StatusBadge status={submission.status} />
          </div>
        </button>
      </div>
    )
  }
  return (
    <button onClick={onClick} style={active ? activeQueueCardStyle : queueCardStyle}>
      <div style={cardMetaStyle}>{item.item.id} · {item.item.meta}</div>
      <strong>{item.item.title}</strong>
      <div style={pillRowStyle}>
        <span style={aiPillStyle}>{item.item.badge}</span>
        <span style={verdictPillStyle(item.item.tone)}>{item.item.verdict}</span>
      </div>
    </button>
  )
}

function DemoReviewDetail({ item, reason, setReason }: { item: DemoReviewItem, reason: string, setReason: (value: string) => void }) {
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
        <CompareBox title="第 1 轮提交（已打回）" rows={[
          ['cleaned_title', '户外便携野营折叠桌椅套装 5 件套'],
          ['category', '家居用品'],
          ['keywords', '折叠, 户外'],
        ]} />
        <CompareBox title="第 2 轮提交（本轮 · 修改后）" rows={[
          ['cleaned_title', '户外野营便携折叠桌椅 5 件套'],
          ['category', '户外运动'],
          ['keywords', '折叠, 户外, 5 件套, 便携, 野营'],
        ]} highlight />
      </div>

      <section style={aiResultStyle}>
        <div style={sectionTitleRowStyle}>
          <h3 style={sectionTitleStyle}>✦ AI 预审 · 本轮重跑结果</h3>
          <span style={modelPillStyle}>v2.3 · doubao-pro-32k</span>
        </div>
        <div style={scoreLineStyle}>
          <strong>综合 <span>86</span></strong>
          <span>相关性 92</span>
          <span>准确性 84</span>
          <span>格式合规 88</span>
          <span>安全 99</span>
        </div>
        <ScoreBars />
        <p style={resultReasonStyle}>关键词较第 1 轮已补充至 5 个并覆盖品类核心卖点；类目改为「户外运动」更贴合事实。建议通过。</p>
      </section>

      <label style={labelStyle}>审核意见（打回时必填）</label>
      <textarea value={reason} onChange={(event) => setReason(event.target.value)} style={textareaStyle} />
      <div style={tagRowStyle}>
        {['# 关键词缺失', '# 类目错误', '# 标题超长', '# 包含违禁词', '# 格式不规范'].map((tag) => <span key={tag} style={neutralPillStyle}>{tag}</span>)}
      </div>

      <div style={diagnosticGridStyle}>
        <section style={diagnosticPanelStyle}>
          <div style={sectionTitleRowStyle}>
            <h3 style={sectionTitleStyle}>提交内容</h3>
            <span style={neutralPillStyle}>JSON 字段视图</span>
          </div>
          <pre style={codeBlockStyle}>{`{
  "cleaned_title": "户外便携野营折叠桌椅套装 5 件套",
  "category": "户外运动",
  "keywords": ["折叠", "户外", "5 件套", "便携", "野营"]
}`}</pre>
        </section>
        <section style={diagnosticPanelStyle}>
          <div style={sectionTitleRowStyle}>
            <h3 style={sectionTitleStyle}>审核 Prompt 模板</h3>
            <span style={modelPillStyle}>规则：电商相关性 v2</span>
          </div>
          <pre style={codeBlockStyle}>{`请基于以下维度给提交内容打分（0-100）：
[相关性] 标注结果与原始数据是否对齐
[准确性] 类目 / 关键词与商品事实是否一致
[格式合规] 是否满足模板字段与正则规则
[安全性] 是否包含敏感 / 违规词

通过 function_call 返回 JSON:
{ "scores": {...}, "verdict": "pass|reject|manual" }`}</pre>
        </section>
      </div>

      <section style={diagnosticPanelStyle}>
        <h3 style={sectionTitleStyle}>处理日志 / 审计</h3>
        <div style={processLogStyle}>
          {[
            ['18:01:02', 'queue', '进入 BullMQ 队列 ai-prereview · 优先级 5'],
            ['18:01:03', 'llm', '调用 doubao-pro-32k · tokens 1342 · 1.42s'],
            ['18:01:03', 'verdict', '结构化输出：pass (86)'],
          ].map(([time, label, text]) => (
            <div key={`${time}-${label}`} style={processLogRowStyle}>
              <span>{time}</span>
              <span style={neutralPillStyle}>{label}</span>
              <strong>{text}</strong>
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
}: {
  detail: TaskBundle | null
  selected: Submission | null
  schema: ParsedSchema
  payload: Record<string, unknown>
  answer: AnswerValue
  reason: string
  setReason: (value: string) => void
}) {
  if (!detail?.item) {
    return (
      <section style={{ ...detailShellStyle, border: '1px dashed var(--color-border-light)', minHeight: 420, alignContent: 'center', justifyItems: 'center' }}>
        <EmptyState title="等待审核" body="从左侧队列选择一条提交记录开始人工审核。" variant="queue" />
      </section>
    )
  }

  return (
    <section style={detailShellStyle}>
      <div style={detailHeaderStyle}>
        <div>
          <h2 style={detailTitleStyle}>{schema.ok ? schema.schema.title : '提交详情'}</h2>
          <p style={mutedTextStyle}>Submission #{detail.submission?.id} · Task #{detail.task.id} · Item #{detail.item.id}</p>
        </div>
        <StatusBadge status={selected?.status} />
      </div>
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
      <section style={aiResultStyle}>
        <RealAIReviewSummary aiReview={detail.aiReview} submission={detail.submission} />
      </section>
      {detail.latestHumanReview?.reason ? (
        <section style={previousReviewStyle}>
          <h3 style={sectionTitleStyle}>上一轮意见</h3>
          <p style={resultReasonStyle}>{detail.latestHumanReview.reason}</p>
        </section>
      ) : null}
      <label style={labelStyle}>审核意见</label>
      <textarea value={reason} onChange={(event) => setReason(event.target.value)} style={textareaStyle} placeholder="输入审核意见..." />
      <RealAIDiagnostics answer={detail.revision?.answer} aiReview={detail.aiReview} auditLogs={detail.auditLogs ?? []} />
    </section>
  )
}

function RealAIReviewSummary({ aiReview, submission }: { aiReview?: AIReviewDetail | null, submission?: Submission }) {
  if (aiReview) {
    const score = formatAIScore(aiReview.overallScore)
    const verdict = aiReview.verdict ?? submission?.aiVerdict ?? 'unknown'
    const dimensions = normalizeDimensions(aiReview.dimensions)
    return (
      <>
        <div style={sectionTitleRowStyle}>
          <h3 style={sectionTitleStyle}>AI 预审结论</h3>
          <span style={modelPillStyle}>{aiReview.prompt ? `v${aiReview.prompt.version} · ${aiReview.prompt.model}` : `prompt v${aiReview.promptVersion}`}</span>
        </div>
        <div>AI {verdict} · {score}</div>
        <div style={{ color: verdict === 'pass' ? 'var(--color-success)' : 'var(--color-danger)', fontWeight: 700 }}>verdict: {verdict}</div>
        <div style={{ fontWeight: 700 }}>score: {score}</div>
        {dimensions.length > 0 ? <ScoreBars rows={dimensions} /> : null}
        {aiReview.reason ? <p style={resultReasonStyle}>{aiReview.reason}</p> : null}
        <div style={scoreLineStyle}>
          <StatusBadge status={aiReview.status} />
          <span>tokens: {aiReview.tokensInput + aiReview.tokensOutput}</span>
          <span>latency: {aiReview.latencyMs}ms</span>
        </div>
      </>
    )
  }
  if (submission?.aiVerdict) {
    return (
      <>
        <h3 style={sectionTitleStyle}>AI 预审结论</h3>
        <div>AI {submission.aiVerdict} · {formatAIScore(submission.aiScore)}</div>
        <div style={{ color: submission.aiVerdict === 'pass' ? 'var(--color-success)' : 'var(--color-danger)', fontWeight: 700 }}>verdict: {submission.aiVerdict}</div>
        <div style={{ fontWeight: 700 }}>score: {formatAIScore(submission.aiScore)}</div>
      </>
    )
  }
  return (
    <>
      <h3 style={sectionTitleStyle}>AI 预审结论</h3>
      <div>AI 未预审</div>
      <p style={mutedTextStyle}>暂无 AI 预审结果</p>
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
              <span style={neutralPillStyle}>ai_review</span>
              <strong>{aiReview.status} · {aiReview.verdict ?? 'no verdict'} · {formatAIScore(aiReview.overallScore)}</strong>
            </div>
          ) : null}
          {auditLogs.map((log) => (
            <div key={log.id} style={processLogRowStyle}>
              <span>{formatTime(log.createdAt)}</span>
              <span style={neutralPillStyle}>{log.event}</span>
              <strong>{log.actorType} · {auditStateText(log)}</strong>
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

function Timeline({ auditLogs, submissionId }: { auditLogs?: AuditLog[], submissionId?: number | string }) {
  if (auditLogs && auditLogs.length > 0) {
    return (
      <section style={timelineStyle}>
        <h3 style={sectionTitleStyle}>审计时间线（{submissionId ? `SUB-${submissionId}` : '当前提交'}）</h3>
        {auditLogs.map((log) => (
          <div key={log.id} style={timelineRowStyle}>
            <span style={{ ...timelineDotStyle, background: log.actorType === 'ai_worker' || log.event.startsWith('ai_') ? '#7c3aed' : 'var(--color-success)' }} />
            <div>
              <strong>{log.actorType}</strong>
              <p>{log.event} · {auditStateText(log)}</p>
            </div>
          </div>
        ))}
      </section>
    )
  }
  const events = [
    ['李雷', '第 1 轮提交', 'green'],
    ['AI Agent', '预审 62 分 → 建议打回', 'red'],
    ['王芳 · 复审', '采纳 AI 结论 → 打回', 'red'],
    ['李雷', '查看打回意见，开始修改', 'green'],
    ['AI Agent', '重审 86 分 → 建议通过', 'green'],
    ['王芳 · 复审中', '本次决策将写入终审待办', 'blue'],
  ]
  return (
    <section style={timelineStyle}>
      <h3 style={sectionTitleStyle}>审计时间线（{submissionId ?? 'SUB-00607'}）</h3>
      {events.map(([actor, action, tone]) => (
        <div key={`${actor}-${action}`} style={timelineRowStyle}>
          <span style={{ ...timelineDotStyle, background: tone === 'green' ? 'var(--color-success)' : tone === 'red' ? 'var(--color-danger)' : 'var(--color-accent)' }} />
          <div>
            <strong>{actor}</strong>
            <p>{action}</p>
          </div>
        </div>
      ))}
    </section>
  )
}

function RuleConfigPanel({
  taskId,
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
  taskId?: number | null
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
  const editHref = taskId
    ? `/owner?taskId=${taskId}${selectedRule ? `&aiPromptId=${selectedRule.id}` : ''}#ai-prompts`
    : '/owner#ai-prompts'
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
          <div style={ruleActionRowStyle}>
            <span style={mutedTextStyle}>规则切换请在 Owner 配置页完成。</span>
            <a href={editHref} style={editRuleLinkStyle}>跳转 Owner 编辑</a>
          </div>
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

function normalizeDimensions(raw: unknown): Array<{ label: string, value: number }> {
  if (!Array.isArray(raw)) {
    return []
  }
  return raw.flatMap((item) => {
    if (!item || typeof item !== 'object') {
      return []
    }
    const record = item as Record<string, unknown>
    const label = typeof record.name === 'string' ? record.name : typeof record.label === 'string' ? record.label : ''
    const score = typeof record.score === 'number' ? record.score : typeof record.value === 'number' ? record.value : NaN
    if (!label || !Number.isFinite(score)) {
      return []
    }
    return [{ label, value: score }]
  })
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

function auditStateText(log: AuditLog) {
  const fromState = log.fromState && 'Valid' in log.fromState && log.fromState.Valid ? log.fromState.String : ''
  return fromState ? `${fromState} → ${log.toState}` : log.toState
}

function canRetryAIReview(aiReview: AIReviewDetail | null | undefined) {
  return aiReview?.status === 'failed' || aiReview?.status === 'dead'
}

function verdictPillStyle(tone: DemoReviewItem['tone']): CSSProperties {
  return {
    ...neutralPillStyle,
    color: tone === 'pass' ? 'var(--color-success)' : tone === 'reject' ? '#f97316' : tone === 'failed' ? 'var(--color-danger)' : 'var(--color-text-secondary)',
    background: tone === 'pass' ? 'var(--color-success-soft)' : tone === 'reject' ? '#fff7ed' : tone === 'failed' ? 'var(--color-danger-soft)' : 'var(--color-panel-header)',
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

const layoutStyle: CSSProperties = {
  display: 'grid',
  gridTemplateColumns: '300px minmax(0, 1fr) 240px',
  gap: 'var(--space-sm)',
  alignItems: 'start',
}

const leftPanelStyle: CSSProperties = {
  background: 'var(--color-surface)',
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-lg)',
  padding: 'var(--space-sm)',
  minHeight: 720,
}

const centerPanelStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-md)',
  minWidth: 0,
}

const rightPanelStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-md)',
  background: 'var(--color-surface)',
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-lg)',
  padding: 'var(--space-sm)',
  minHeight: 720,
}

const tabRowStyle: CSSProperties = {
  display: 'grid',
  gridTemplateColumns: '1fr 1fr 1fr',
  gap: 'var(--space-xs)',
  borderBottom: '1px solid var(--color-border-light)',
  paddingBottom: 'var(--space-sm)',
}

const tabStyle: CSSProperties = {
  border: 0,
  background: 'transparent',
  color: 'var(--color-text-secondary)',
  fontWeight: 600,
  padding: 'var(--space-sm)',
  cursor: 'pointer',
}

const activeTabStyle: CSSProperties = {
  ...tabStyle,
  color: 'var(--color-accent)',
  borderBottom: '2px solid var(--color-accent)',
}

const bulkBarStyle: CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 'var(--space-sm)',
  marginTop: 'var(--space-md)',
  padding: 'var(--space-sm)',
  background: 'var(--color-panel-header)',
  borderRadius: 'var(--radius-md)',
  fontSize: 'var(--text-sm)',
}

const miniButtonStyle: CSSProperties = {
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-sm)',
  background: 'var(--color-surface)',
  padding: '4px 8px',
  cursor: 'pointer',
}

const slaCardStyle: CSSProperties = {
  display: 'grid',
  gap: 2,
  marginTop: 'var(--space-md)',
  padding: 'var(--space-md)',
  border: '1px solid var(--color-accent-soft)',
  borderRadius: 'var(--radius-md)',
  background: 'var(--color-info-bg)',
  color: 'var(--color-text-secondary)',
}

const queueListStyle: CSSProperties = {
  display: 'grid',
  marginTop: 'var(--space-md)',
}

const queueCardStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-xs)',
  width: '100%',
  padding: 'var(--space-md)',
  border: 0,
  borderBottom: '1px solid var(--color-border-light)',
  background: 'var(--color-surface)',
  textAlign: 'left',
  cursor: 'pointer',
}

const activeQueueCardStyle: CSSProperties = {
  ...queueCardStyle,
  background: 'var(--color-info-bg)',
  borderLeft: '3px solid var(--color-accent)',
}

const queueSelectStyle: CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 'var(--space-xs)',
  color: 'var(--color-text-muted)',
  fontSize: 'var(--text-sm)',
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

const cardMetaStyle: CSSProperties = {
  color: 'var(--color-text-muted)',
  fontSize: 'var(--text-sm)',
}

const pillRowStyle: CSSProperties = {
  display: 'flex',
  gap: 'var(--space-xs)',
  flexWrap: 'wrap',
}

const aiPillStyle: CSSProperties = {
  borderRadius: 999,
  background: '#f3e8ff',
  color: '#7c3aed',
  padding: '2px 8px',
  fontSize: 'var(--text-sm)',
  fontWeight: 700,
}

const neutralPillStyle: CSSProperties = {
  borderRadius: 999,
  background: 'var(--color-panel-header)',
  color: 'var(--color-text-secondary)',
  padding: '2px 8px',
  fontSize: 'var(--text-sm)',
  fontWeight: 600,
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
  gridTemplateColumns: '72px 72px 1fr',
  gap: 'var(--space-sm)',
  alignItems: 'center',
  padding: 'var(--space-sm) 0',
  borderBottom: '1px dashed var(--color-border-light)',
  color: 'var(--color-text-secondary)',
  fontSize: 'var(--text-sm)',
}

const decisionGridStyle: CSSProperties = {
  display: 'grid',
  gridTemplateColumns: 'repeat(3, 1fr)',
  gap: 'var(--space-sm)',
}

const decisionButtonBaseStyle: CSSProperties = {
  display: 'grid',
  gap: 4,
  padding: 'var(--space-lg)',
  borderRadius: 'var(--radius-md)',
  cursor: 'pointer',
  textAlign: 'center',
}

const rejectDecisionStyle: CSSProperties = {
  ...decisionButtonBaseStyle,
  border: '1px solid #ffb4ab',
  background: '#fff1f1',
  color: 'var(--color-danger)',
}

const fixDecisionStyle: CSSProperties = {
  ...decisionButtonBaseStyle,
  border: '1px solid #f59e0b',
  background: '#fffbeb',
  color: '#b45309',
}

const passDecisionStyle: CSSProperties = {
  ...decisionButtonBaseStyle,
  border: '2px solid #4ade80',
  background: 'var(--color-success-soft)',
  color: 'var(--color-success)',
}

const legacyActionRowStyle: CSSProperties = {
  display: 'flex',
  justifyContent: 'flex-end',
  gap: 'var(--space-md)',
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

const ruleActionRowStyle: CSSProperties = {
  display: 'grid',
  gridTemplateColumns: '1fr 1fr',
  gap: 'var(--space-sm)',
}

const editRuleLinkStyle: CSSProperties = {
  display: 'inline-flex',
  justifyContent: 'center',
  alignItems: 'center',
  minHeight: 32,
  borderRadius: 'var(--radius-sm)',
  background: 'var(--color-accent)',
  color: '#fff',
  textDecoration: 'none',
  fontSize: 'var(--text-sm)',
  fontWeight: 700,
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
