import type { CSSProperties } from 'react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Button, Toast } from '@douyinfe/semi-ui'
import { SchemaRenderer, parseAnswer, parseTemplateSchema } from '../../renderer'
import type { AnswerValue, TemplateSchema } from '../../renderer/types'
import { apiGet, apiPost, type Submission, type TaskBundle } from '../../shared/api/client'
import EmptyState from '../../shared/components/EmptyState'
import { parsePayload } from '../../shared/components/payload'
import StatusBadge from '../../shared/components/StatusBadge'
import '../../styles/lh/humanreview.css'

// 仲裁队列:多人重复标注(overlap)答案冲突的题目会进入 needs_arbitration。
// 后端 /reviewer/submissions?status=needs_arbitration 返回所有待仲裁 submission;
// 同一 item 的多份 submission 即一组冲突。Reviewer 并排对比后采纳一份(approve →
// 该份终审通过、其余自动打回)或对该题全部打回(reject)。
// review.Apply 对仲裁态的 approve/reject 都会批量处理同 item 的 sibling,故前端只需对
// 选中的一份发一次 /submissions/:id/review。

type ConflictGroup = {
  itemId: number
  taskId: number
  submissions: Submission[]
}

type ParsedSchema =
  | { ok: true, schema: TemplateSchema }
  | { ok: false, message: string }

// groupByItem 按 itemId 聚合冲突 submission;item 升序、组内 submission 升序,保证渲染稳定。
function groupByItem(submissions: Submission[]): ConflictGroup[] {
  const byItem = new Map<number, ConflictGroup>()
  for (const submission of submissions) {
    const existing = byItem.get(submission.itemId)
    if (existing) {
      existing.submissions.push(submission)
    } else {
      byItem.set(submission.itemId, { itemId: submission.itemId, taskId: submission.taskId, submissions: [submission] })
    }
  }
  return [...byItem.values()]
    .sort((a, b) => a.itemId - b.itemId)
    .map((group) => ({ ...group, submissions: [...group.submissions].sort((a, b) => a.id - b.id) }))
}

export default function ArbitrationQueue() {
  const [groups, setGroups] = useState<ConflictGroup[]>([])
  const [selectedItemId, setSelectedItemId] = useState<number | null>(null)
  const [bundles, setBundles] = useState<Record<number, TaskBundle>>({})
  const [detailLoading, setDetailLoading] = useState(false)
  const [resolving, setResolving] = useState(false)
  const [reason, setReason] = useState('')

  const load = useCallback(async () => {
    try {
      const data = await apiGet<Submission[]>('/reviewer/submissions?status=needs_arbitration')
      setGroups(groupByItem(data))
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '加载仲裁队列失败')
    }
  }, [])

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void load()
  }, [load])

  const selectedGroup = useMemo(
    () => groups.find((group) => group.itemId === selectedItemId) ?? null,
    [groups, selectedItemId],
  )

  async function openGroup(group: ConflictGroup) {
    setSelectedItemId(group.itemId)
    setReason('')
    setBundles({})
    setDetailLoading(true)
    try {
      const loaded = await Promise.all(
        group.submissions.map((submission) => apiGet<TaskBundle>(`/reviewer/submissions/${submission.id}`)),
      )
      const map: Record<number, TaskBundle> = {}
      loaded.forEach((bundle, index) => {
        map[group.submissions[index].id] = bundle
      })
      setBundles(map)
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '加载冲突详情失败')
    } finally {
      setDetailLoading(false)
    }
  }

  async function resolve(submissionId: number, verdict: 'approve' | 'reject') {
    if (verdict === 'reject' && reason.trim().length < 5) {
      Toast.error('全部打回必须填写至少 5 字的理由')
      return
    }
    setResolving(true)
    try {
      await apiPost(`/submissions/${submissionId}/review`, { verdict, reason: verdict === 'reject' ? reason : '' })
      Toast.success(verdict === 'approve' ? '已采纳该份并终审，其余冲突已打回' : '已打回该题全部冲突提交')
      setSelectedItemId(null)
      setBundles({})
      setReason('')
      await load()
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '仲裁失败')
    } finally {
      setResolving(false)
    }
  }

  return (
    <div className="hr-shell" style={shellStyle}>
      <aside className="hr-side" style={sideStyle}>
        <div style={sideHeadStyle}>
          <h2 style={sideTitleStyle}>仲裁队列</h2>
          <p style={sideHintStyle}>多人重复标注答案冲突的题目，选一题并排对比后终审。</p>
        </div>
        {groups.length === 0 ? (
          <EmptyState title="暂无待仲裁" body="overlap 题答案冲突时会进入这里等待终审。" variant="done" />
        ) : (
          <div>
            {groups.map((group) => (
              <button
                key={group.itemId}
                type="button"
                className={`hr-item${group.itemId === selectedItemId ? ' hr-item--selected' : ''}`}
                style={itemButtonStyle}
                onClick={() => void openGroup(group)}
              >
                <div className="hr-item__head" style={{ marginBottom: 6 }}>
                  <span className="hr-item__sub">Item #{group.itemId}</span>
                  <span>· Task #{group.taskId}</span>
                </div>
                <div className="hr-item__tags">
                  <span className="hr-tag hr-tag--purple">{group.submissions.length} 份冲突</span>
                  <StatusBadge status="needs_arbitration" />
                </div>
              </button>
            ))}
          </div>
        )}
      </aside>

      <main className="hr-main" style={mainStyle}>
        {!selectedGroup ? (
          <EmptyState title="选择一题开始仲裁" body="从左侧选择一题，并排查看冲突答案后采纳正确的一份。" variant="queue" />
        ) : (
          <>
            <div className="hr-main__head">
              <h2>仲裁 · Item #{selectedGroup.itemId}</h2>
              <div className="hr-main__head-right">
                <span className="hr-tag hr-tag--purple" aria-label={`冲突答案 ${selectedGroup.submissions.length} 份`}>
                  {selectedGroup.submissions.length} 份冲突答案
                </span>
                <span>Task #{selectedGroup.taskId}</span>
              </div>
            </div>
            <p style={mainHintStyle}>采纳其中一份即终审通过该题、其余自动打回；或对该题全部打回。</p>

            {detailLoading ? (
              <p style={mutedStyle}>加载冲突详情…</p>
            ) : (
              <div style={compareGridStyle}>
                {selectedGroup.submissions.map((submission) => (
                  <ConflictCard
                    key={submission.id}
                    submission={submission}
                    bundle={bundles[submission.id]}
                    resolving={resolving}
                    onApprove={() => void resolve(submission.id, 'approve')}
                  />
                ))}
              </div>
            )}

            <div style={rejectRowStyle}>
              <label htmlFor="arb-reject-reason" style={labelStyle}>全部打回理由（至少 5 字）</label>
              <textarea
                id="arb-reject-reason"
                className="hr-textarea"
                value={reason}
                onChange={(event) => setReason(event.target.value)}
                placeholder="输入打回理由…"
              />
              <Button
                theme="light"
                type="danger"
                disabled={resolving || selectedGroup.submissions.length === 0}
                onClick={() => void resolve(selectedGroup.submissions[0].id, 'reject')}
              >
                全部打回该题
              </Button>
            </div>
          </>
        )}
      </main>
    </div>
  )
}

function ConflictCard({
  submission,
  bundle,
  resolving,
  onApprove,
}: {
  submission: Submission
  bundle: TaskBundle | undefined
  resolving: boolean
  onApprove: () => void
}) {
  const schema = useMemo(() => parseBundleSchema(bundle), [bundle])
  const payload = useMemo(() => parsePayload(bundle?.item?.payload), [bundle?.item?.payload])
  const answer = useMemo<AnswerValue>(() => parseAnswer(bundle?.revision?.answer), [bundle?.revision?.answer])

  return (
    <section style={cardStyle} aria-label={`冲突答案 Submission #${submission.id}`}>
      <div style={cardHeadStyle}>
        <div>
          <strong>Submission #{submission.id}</strong>
          <div style={cardMetaStyle}>标注员 #{submission.labelerId ?? '—'}</div>
        </div>
        {submission.aiVerdict ? (
          <span className="hr-tag hr-tag--ai">AI {submission.aiVerdict} · {formatAIScore(submission.aiScore)}</span>
        ) : null}
      </div>
      <div style={cardBodyStyle}>
        {!bundle ? (
          <p style={mutedStyle}>加载中…</p>
        ) : schema.ok ? (
          <SchemaRenderer
            schema={schema.schema}
            payload={payload}
            value={answer}
            readOnly
            runtime={{ taskId: submission.taskId, itemId: submission.itemId, submissionId: submission.id }}
          />
        ) : (
          <div role="alert" style={errorStyle}>{schema.message}</div>
        )}
      </div>
      <Button
        theme="solid"
        disabled={resolving || !bundle || !schema.ok}
        onClick={onApprove}
        style={cardApproveStyle}
      >
        采纳此份（通过）
      </Button>
    </section>
  )
}

function parseBundleSchema(bundle: TaskBundle | undefined): ParsedSchema {
  if (!bundle?.template?.schemaJson) {
    return { ok: false, message: '当前提交缺少模板快照' }
  }
  const result = parseTemplateSchema(bundle.template.schemaJson)
  if (!result.ok) {
    return { ok: false, message: `${result.error.field}: ${result.error.message}` }
  }
  return { ok: true, schema: result.value }
}

function formatAIScore(score: number | null | undefined) {
  return typeof score === 'number' && Number.isFinite(score) ? String(score) : '-'
}

const shellStyle: CSSProperties = {
  minHeight: 640,
  border: '1px solid var(--lh-border)',
  borderRadius: 'var(--lh-radius-lg)',
  overflow: 'hidden',
  background: 'var(--lh-bg-card)',
}

const sideStyle: CSSProperties = {
  minWidth: 0,
}

const sideHeadStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-xs)',
  padding: 'var(--space-md)',
  borderBottom: '1px solid var(--lh-border)',
}

const sideTitleStyle: CSSProperties = {
  margin: 0,
  fontFamily: 'var(--font-heading)',
  fontSize: 'var(--text-h3)',
}

const sideHintStyle: CSSProperties = {
  margin: 0,
  color: 'var(--color-text-secondary)',
  fontSize: 'var(--text-sm)',
}

const itemButtonStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-xs)',
  width: '100%',
  textAlign: 'left',
  font: 'inherit',
  color: 'inherit',
  cursor: 'pointer',
}

const mainStyle: CSSProperties = {
  minWidth: 0,
  display: 'grid',
  gap: 'var(--space-md)',
  alignContent: 'start',
}

const mainHintStyle: CSSProperties = {
  margin: 0,
  color: 'var(--color-text-secondary)',
  fontSize: 'var(--text-sm)',
}

const compareGridStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-md)',
  gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))',
  alignItems: 'start',
}

const cardStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-sm)',
  padding: 'var(--space-md)',
  border: '1px solid var(--lh-border)',
  borderRadius: 'var(--lh-radius-md)',
  background: 'var(--color-surface)',
  alignContent: 'start',
}

const cardHeadStyle: CSSProperties = {
  display: 'flex',
  justifyContent: 'space-between',
  alignItems: 'flex-start',
  gap: 'var(--space-sm)',
}

const cardMetaStyle: CSSProperties = {
  color: 'var(--color-text-secondary)',
  fontSize: 'var(--text-sm)',
}

const cardBodyStyle: CSSProperties = {
  padding: 'var(--space-sm)',
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-md)',
  background: 'var(--color-canvas)',
}

const cardApproveStyle: CSSProperties = {
  width: '100%',
}

const rejectRowStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-xs)',
  paddingTop: 'var(--space-sm)',
  borderTop: '1px solid var(--color-border-light)',
}

const labelStyle: CSSProperties = {
  color: 'var(--color-text-secondary)',
  fontSize: 'var(--text-sm)',
  fontWeight: 600,
}

const mutedStyle: CSSProperties = {
  color: 'var(--color-text-secondary)',
  fontSize: 'var(--text-sm)',
}

const errorStyle: CSSProperties = {
  color: 'var(--color-danger)',
  fontSize: 'var(--text-sm)',
}
