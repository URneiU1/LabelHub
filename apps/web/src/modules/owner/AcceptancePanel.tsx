import { useCallback, useEffect, useState } from 'react'
import { Modal, Toast } from '@douyinfe/semi-ui'
import {
  acceptAcceptanceBatch,
  getAcceptance,
  recordAcceptanceSpotCheck,
  rejectAcceptanceBatch,
  startAcceptance,
  type AcceptanceStatus,
} from '../../shared/api/client'
import StatusBadge from '../../shared/components/StatusBadge'
import EmptyState from '../../shared/components/EmptyState'
import LoadingBlock from '../../shared/components/LoadingBlock'

interface AcceptancePanelProps {
  taskId: number
}

const batchStatusBadge: Record<string, string> = {
  pending: 'draft',
  accepted: 'approved',
  rejected: 'rejected',
}

const batchStatusLabel: Record<string, string> = {
  pending: '验收中',
  accepted: '验收通过',
  rejected: '验收不通过',
}

export default function AcceptancePanel({ taskId }: AcceptancePanelProps) {
  const [status, setStatus] = useState<AcceptanceStatus | null>(null)
  const [loading, setLoading] = useState(false)
  const [busy, setBusy] = useState(false)
  const [note, setNote] = useState('')
  const [spotSubmissionId, setSpotSubmissionId] = useState('')
  const [spotNote, setSpotNote] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    try {
      setStatus(await getAcceptance(taskId))
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '加载验收状态失败')
    } finally {
      setLoading(false)
    }
  }, [taskId])

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- 切任务清空旧表单
    setNote('')
    setSpotSubmissionId('')
    setSpotNote('')
    void load()
  }, [load])

  const batch = status?.batch ?? null
  const isPending = batch?.status === 'pending'

  async function run(label: string, fn: () => Promise<void>) {
    setBusy(true)
    try {
      await fn()
      await load()
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : `${label}失败`)
    } finally {
      setBusy(false)
    }
  }

  function onStart() {
    void run('发起验收', async () => {
      await startAcceptance(taskId)
      Toast.success('已发起验收批次')
    })
  }

  function onSpotCheck(result: 'ok' | 'flag') {
    if (!batch) return
    const submissionId = Number(spotSubmissionId)
    if (!Number.isInteger(submissionId) || submissionId <= 0) {
      Toast.warning('请输入有效的提交 ID')
      return
    }
    void run('抽检', async () => {
      await recordAcceptanceSpotCheck(taskId, { batch_id: batch.id, submission_id: submissionId, result, note: spotNote })
      setSpotSubmissionId('')
      setSpotNote('')
      Toast.success(result === 'flag' ? '已标记为不合格' : '已标记为合格')
    })
  }

  // 已通过列表里逐条直接抽检(无需手输 ID),与上面的手动入口共用接口。
  function spotCheckSubmission(submissionId: number, result: 'ok' | 'flag') {
    if (!batch) return
    void run('抽检', async () => {
      await recordAcceptanceSpotCheck(taskId, { batch_id: batch.id, submission_id: submissionId, result, note: '' })
      Toast.success(result === 'flag' ? '已标记为不合格' : '已标记为合格')
    })
  }

  function onAccept() {
    if (!batch) return
    void run('验收通过', async () => {
      await acceptAcceptanceBatch(taskId, { batch_id: batch.id, note })
      setNote('')
      Toast.success('验收通过')
    })
  }

  function onReject() {
    if (!batch) return
    const flagged = (status?.spotChecks ?? []).filter((c) => c.result === 'flag').length
    Modal.confirm({
      title: '验收不通过?',
      content: `抽检标记为不合格的 ${flagged} 条已通过数据将被打回人工复审(approved → 人工复审),重审通过后才会重新计入完成。此操作会改动审核流转。`,
      okText: '确认不通过',
      cancelText: '取消',
      onOk: () =>
        run('验收不通过', async () => {
          const res = await rejectAcceptanceBatch(taskId, { batch_id: batch.id, note })
          setNote('')
          Toast.success(`验收不通过,已打回 ${res.reopenedCount} 条重审`)
        }),
    })
  }

  if (loading && !status) {
    return <LoadingBlock title="验收状态加载中" rows={2} />
  }

  return (
    <section style={sectionStyle} aria-label="数据验收">
      <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-sm)', marginBottom: 'var(--space-sm)' }}>
        <h3 style={headingStyle}>数据验收</h3>
        {batch ? <StatusBadge status={batchStatusBadge[batch.status] ?? 'draft'} label={batchStatusLabel[batch.status] ?? batch.status} /> : null}
      </div>
      <p style={mutedStyle}>
        对该任务当前「已通过」数据做一次验收:抽检若干条,确认无误则验收通过;发现不合格的标记后「验收不通过」会把这些条目打回人工复审。验收只是质检状态,不影响导出。
      </p>

      <div style={metricRowStyle}>
        <Metric label="当前已通过" value={String(status?.approvedCount ?? 0)} />
        {batch ? <Metric label="本批快照" value={String(batch.approvedCount)} /> : null}
        {batch ? <Metric label="抽检条数" value={String(status?.spotChecks.length ?? 0)} /> : null}
      </div>

      {!isPending ? (
        <div style={{ marginTop: 'var(--space-md)' }}>
          {batch ? (
            <p style={mutedStyle}>
              最近一次验收:{batchStatusLabel[batch.status] ?? batch.status}
              {batch.note ? ` · ${batch.note}` : ''}
            </p>
          ) : (
            <EmptyState title="尚未发起验收" body="对当前已通过的数据发起一次验收批次,开始抽检与质检闭环。" variant="empty" />
          )}
          <button type="button" className="lh-btn lh-btn--primary" disabled={busy} onClick={onStart} style={{ marginTop: 'var(--space-sm)' }}>
            {busy ? '处理中…' : '发起验收'}
          </button>
        </div>
      ) : (
        <div style={{ marginTop: 'var(--space-md)', display: 'grid', gap: 'var(--space-md)' }}>
          <div style={cardStyle}>
            <div style={subHeadStyle}>抽检</div>
            <ApprovedList status={status} onSpot={spotCheckSubmission} busy={busy} />
            <div style={{ ...mutedStyle, marginTop: 'var(--space-md)', fontSize: 12 }}>或手动按提交 ID 抽检:</div>
            <div style={{ display: 'flex', gap: 'var(--space-sm)', flexWrap: 'wrap', alignItems: 'center' }}>
              <input
                aria-label="提交 ID"
                style={inputStyle}
                placeholder="提交 ID"
                value={spotSubmissionId}
                onChange={(e) => setSpotSubmissionId(e.target.value.replace(/[^0-9]/g, ''))}
              />
              <input
                aria-label="抽检备注"
                style={{ ...inputStyle, flex: 1, minWidth: 160 }}
                placeholder="备注(可选)"
                value={spotNote}
                onChange={(e) => setSpotNote(e.target.value)}
              />
              <button type="button" className="lh-btn" disabled={busy} onClick={() => onSpotCheck('ok')}>合格</button>
              <button type="button" className="lh-btn lh-btn--danger" disabled={busy} onClick={() => onSpotCheck('flag')}>不合格</button>
            </div>
            <SpotCheckList status={status} />
          </div>

          <div style={cardStyle}>
            <div style={subHeadStyle}>验收结论</div>
            <textarea
              aria-label="验收备注"
              style={textareaStyle}
              placeholder="验收备注(可选)"
              value={note}
              onChange={(e) => setNote(e.target.value)}
            />
            <div style={{ display: 'flex', gap: 'var(--space-sm)', marginTop: 'var(--space-sm)' }}>
              <button type="button" className="lh-btn lh-btn--primary" disabled={busy} onClick={onAccept}>验收通过</button>
              <button type="button" className="lh-btn lh-btn--danger" disabled={busy} onClick={onReject}>验收不通过(打回不合格项)</button>
            </div>
          </div>
        </div>
      )}
    </section>
  )
}

function SpotCheckList({ status }: { status: AcceptanceStatus | null }) {
  const checks = status?.spotChecks ?? []
  if (checks.length === 0) {
    return <p style={{ ...mutedStyle, marginTop: 'var(--space-sm)' }}>还没有抽检记录。输入提交 ID 后标记合格 / 不合格。</p>
  }
  return (
    <div style={{ display: 'grid', gap: 6, marginTop: 'var(--space-sm)' }}>
      {checks.map((c) => (
        <div key={c.id} style={spotRowStyle}>
          <span style={{ color: 'var(--lh-text-3)', fontSize: 'var(--text-sm)' }}>提交 #{c.submissionId}</span>
          <StatusBadge status={c.result === 'flag' ? 'rejected' : 'approved'} label={c.result === 'flag' ? '不合格' : '合格'} />
          <span style={{ fontSize: 'var(--text-sm)', color: 'var(--lh-text-2)' }}>{c.note ?? ''}</span>
        </div>
      ))}
    </div>
  )
}

function Metric({ label, value }: { label: string, value: string }) {
  return (
    <div style={metricStyle}>
      <div style={{ fontSize: 'var(--text-sm)', color: 'var(--lh-text-3)', textTransform: 'uppercase' }}>{label}</div>
      <div style={{ fontSize: 'var(--text-lg)', fontWeight: 700, color: 'var(--lh-text-1)' }}>{value}</div>
    </div>
  )
}

const fieldLabels: Record<string, string> = {
  relevance_score: '相关性', accuracy_score: '准确性', format_score: '格式合规', safety_score: '安全性',
  summary: '摘要', comment: '评语', issue_tags: '问题标签', ai_precheck: 'AI 预评分',
}

// ApprovedList 列出本任务全部「已通过」提交,每条平铺答案 + 就地标记合格 / 不合格,
// 让 Owner 不必切到 Reviewer 视图或手输提交 ID 就能抽检。
function ApprovedList({ status, onSpot, busy }: {
  status: AcceptanceStatus | null
  onSpot: (submissionId: number, result: 'ok' | 'flag') => void
  busy: boolean
}) {
  const [openIds, setOpenIds] = useState<Set<number>>(new Set())
  const subs = status?.approvedSubmissions ?? []
  const checks = status?.spotChecks ?? []
  if (subs.length === 0) {
    return <p style={{ ...mutedStyle, marginTop: 'var(--space-sm)' }}>当前没有「已通过」数据可抽检。</p>
  }
  function toggle(id: number) {
    setOpenIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }
  return (
    <div style={{ display: 'grid', gap: 'var(--space-sm)', marginTop: 'var(--space-sm)' }}>
      {subs.map((sub) => {
        const check = checks.find((c) => c.submissionId === sub.id)
        const open = openIds.has(sub.id)
        return (
          <div key={sub.id} style={approvedRowStyle}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-sm)', flexWrap: 'wrap' }}>
              <strong style={{ fontSize: 13 }}>提交 #{sub.id}</strong>
              <span style={{ ...mutedStyle, fontSize: 12 }}>题 #{sub.itemId} · 标注员 #{sub.labelerId}</span>
              {sub.aiVerdict ? <span style={{ ...mutedStyle, fontSize: 12 }}>AI {sub.aiVerdict}{sub.aiScore != null ? ` · ${sub.aiScore}` : ''}</span> : null}
              {check ? <StatusBadge status={check.result === 'flag' ? 'rejected' : 'approved'} label={check.result === 'flag' ? '抽检不合格' : '抽检合格'} /> : null}
              <button type="button" className="lh-btn lh-btn--sm" onClick={() => toggle(sub.id)} style={{ marginLeft: 'auto' }}>{open ? '收起' : '查看答案'}</button>
            </div>
            {open ? <AnswerSummary answer={sub.answer} /> : null}
            <div style={{ display: 'flex', gap: 'var(--space-sm)' }}>
              <button type="button" className="lh-btn lh-btn--sm" disabled={busy} onClick={() => onSpot(sub.id, 'ok')}>合格</button>
              <button type="button" className="lh-btn lh-btn--sm lh-btn--danger" disabled={busy} onClick={() => onSpot(sub.id, 'flag')}>不合格</button>
            </div>
          </div>
        )
      })}
    </div>
  )
}

// AnswerSummary 把标注答案 JSON 按「字段名 → 中文标签」平铺,供 Owner 抽检时快速判断标注质量。
function AnswerSummary({ answer }: { answer: string }) {
  let parsed: Record<string, unknown>
  try {
    parsed = JSON.parse(answer) as Record<string, unknown>
  } catch {
    return <span style={{ ...mutedStyle, fontSize: 12 }}>（答案无法解析）</span>
  }
  const entries = Object.entries(parsed).filter(([, v]) => v !== null && v !== '' && !(Array.isArray(v) && v.length === 0))
  if (entries.length === 0) {
    return <span style={{ ...mutedStyle, fontSize: 12 }}>（空答案）</span>
  }
  return (
    <div style={answerGridStyle}>
      {entries.map(([key, value]) => (
        <div key={key} style={{ fontSize: 12 }}>
          <span style={{ color: 'var(--lh-text-3)' }}>{fieldLabels[key] ?? key}：</span>
          <span style={{ color: 'var(--lh-text-1)' }}>{Array.isArray(value) ? value.join('、') : String(value)}</span>
        </div>
      ))}
    </div>
  )
}

const approvedRowStyle: React.CSSProperties = {
  display: 'grid',
  gap: 'var(--space-sm)',
  padding: 'var(--space-sm) var(--space-md)',
  border: '1px solid var(--lh-border)',
  borderRadius: 'var(--radius-md)',
  background: 'var(--lh-bg-card)',
}
const answerGridStyle: React.CSSProperties = {
  display: 'grid',
  gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))',
  gap: '2px 12px',
  padding: 'var(--space-sm)',
  background: 'var(--lh-bg)',
  borderRadius: 'var(--radius-sm)',
}

const sectionStyle: React.CSSProperties = {
  background: 'var(--lh-bg-card)',
  border: '1px solid var(--lh-border)',
  borderRadius: 'var(--radius-md)',
  padding: 'var(--space-lg)',
}

const headingStyle: React.CSSProperties = { margin: 0, fontSize: 'var(--text-lg)', color: 'var(--lh-text-1)' }
const mutedStyle: React.CSSProperties = { fontSize: 'var(--text-sm)', color: 'var(--lh-text-3)', lineHeight: 1.6 }
const subHeadStyle: React.CSSProperties = { fontSize: 'var(--text-sm)', fontWeight: 600, color: 'var(--lh-text-2)', marginBottom: 'var(--space-sm)' }
const metricRowStyle: React.CSSProperties = { display: 'flex', gap: 'var(--space-md)', marginTop: 'var(--space-sm)' }
const metricStyle: React.CSSProperties = {
  flex: '0 0 auto',
  minWidth: 96,
  padding: 'var(--space-sm) var(--space-md)',
  border: '1px solid var(--lh-border)',
  borderRadius: 'var(--radius-md)',
  background: 'var(--lh-bg)',
}
const cardStyle: React.CSSProperties = {
  border: '1px solid var(--lh-border)',
  borderRadius: 'var(--radius-md)',
  padding: 'var(--space-md)',
  background: 'var(--lh-bg)',
}
const inputStyle: React.CSSProperties = {
  height: 32,
  padding: '0 var(--space-sm)',
  border: '1px solid var(--lh-border)',
  borderRadius: 'var(--radius-md)',
  fontSize: 'var(--text-sm)',
  width: 120,
}
const textareaStyle: React.CSSProperties = {
  width: '100%',
  minHeight: 60,
  padding: 'var(--space-sm)',
  border: '1px solid var(--lh-border)',
  borderRadius: 'var(--radius-md)',
  fontSize: 'var(--text-sm)',
  fontFamily: 'inherit',
}
const spotRowStyle: React.CSSProperties = {
  display: 'grid',
  gridTemplateColumns: '120px 90px 1fr',
  alignItems: 'center',
  gap: 'var(--space-sm)',
  padding: '6px var(--space-sm)',
  border: '1px solid var(--lh-border)',
  borderRadius: 'var(--radius-md)',
  background: 'var(--lh-bg-card)',
}
