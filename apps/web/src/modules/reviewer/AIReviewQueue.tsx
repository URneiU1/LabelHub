import { useEffect, useState } from 'react'
import { Toast } from '@douyinfe/semi-ui'
import { listAIReviews, type AIReviewRow } from '../../shared/api/client'
import EmptyState from '../../shared/components/EmptyState'

const STATUS_FILTERS: Array<{ key: string, label: string }> = [
  { key: '', label: '全部' },
  { key: 'pending', label: '待预审' },
  { key: 'running', label: '预审中' },
  { key: 'succeeded', label: '已完成' },
  { key: 'dead', label: '失败' },
]

const STATUS_LABEL: Record<string, string> = {
  pending: '待预审',
  running: '预审中',
  succeeded: '已完成',
  dead: '失败',
}

function verdictView(verdict: string | null): { label: string, tone: string } {
  switch (verdict) {
    case 'pass':
      return { label: '建议通过', tone: '#16a34a' }
    case 'reject':
      return { label: '建议打回', tone: '#dc2626' }
    case 'uncertain':
      return { label: '转人工复核', tone: '#d97706' }
    default:
      return { label: '待出结论', tone: 'var(--lh-text-3)' }
  }
}

function formatTime(value: string | null): string {
  if (!value) {
    return '—'
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  }
  const pad = (n: number) => n.toString().padStart(2, '0')
  return `${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

// AI 审核队列:只读展示 AI 预审 Agent 流水线 —— 每条提交的入队、按维度结构化打分、通过/打回/转人工结论、
// 原始 Prompt、耗时/tokens/重试/幂等键。把「可配置评测标准的审核 Agent」可视化给审核员看。
export default function AIReviewQueue() {
  const [rows, setRows] = useState<AIReviewRow[]>([])
  const [status, setStatus] = useState('')
  const [loading, setLoading] = useState(false)
  const [activeId, setActiveId] = useState<number | null>(null)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    void listAIReviews({ status: status || undefined, limit: 100 })
      .then((data) => {
        if (cancelled) {
          return
        }
        setRows(data.items)
        setActiveId((current) => (current && data.items.some((row) => row.id === current) ? current : data.items[0]?.id ?? null))
      })
      .catch((error) => {
        if (!cancelled) {
          Toast.error(error instanceof Error ? error.message : '加载 AI 审核队列失败')
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false)
        }
      })
    return () => {
      cancelled = true
    }
  }, [status])

  const active = rows.find((row) => row.id === activeId) ?? null

  return (
    <div style={{ display: 'grid', gap: 'var(--space-lg)' }}>
      <header className="lh-page-header" style={{ display: 'flex', justifyContent: 'space-between', gap: 'var(--space-lg)', alignItems: 'flex-start' }}>
        <div className="lh-page-header-title">
          <div style={{ fontSize: 'var(--text-sm)', color: 'var(--color-text-secondary)', marginBottom: 'var(--space-md)' }}>审核与质检 / <strong>AI 预审队列</strong></div>
          <h1 style={{ margin: 0, fontFamily: 'var(--font-heading)', fontSize: 'var(--text-h1)', fontWeight: 700 }}>AI 自动预审队列</h1>
          <p style={{ margin: 'var(--space-xs) 0 0', color: 'var(--color-text-secondary)', fontSize: 'var(--text-sm)' }}>
            异步入队 → 按评分维度调用 LLM 结构化输出（function calling）→ 通过 / 打回 / 转人工复核（只读）
          </p>
        </div>
      </header>

      <div className="hr-side__tabs" role="tablist" style={{ maxWidth: 520 }}>
        {STATUS_FILTERS.map((filter) => (
          <button
            key={filter.key || 'all'}
            type="button"
            role="tab"
            aria-selected={status === filter.key}
            className={'hr-side__tab' + (status === filter.key ? ' hr-side__tab--active' : '')}
            onClick={() => setStatus(filter.key)}
          >
            {filter.label}
          </button>
        ))}
      </div>

      {rows.length === 0 ? (
        <EmptyState
          title={loading ? '加载中' : 'AI 预审队列为空'}
          body={loading ? '正在拉取 AI 预审记录。' : '还没有 AI 预审记录;标注员提交且任务开启 AI 预审后,这里会自动出现。'}
          variant="queue"
        />
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0, 360px) minmax(0, 1fr)', gap: 'var(--space-lg)', alignItems: 'start' }}>
          {/* 左:队列列表 */}
          <div style={{ display: 'grid', gap: 8 }}>
            {rows.map((row) => {
              const verdict = verdictView(row.verdict)
              const selected = row.id === activeId
              return (
                <button
                  key={row.id}
                  type="button"
                  aria-label={`查看 AI 预审 #${row.id} 提交 #${row.submissionId}`}
                  onClick={() => setActiveId(row.id)}
                  style={{
                    appearance: 'none', textAlign: 'left', font: 'inherit', cursor: 'pointer',
                    background: selected ? 'var(--lh-primary-soft)' : '#fff',
                    border: '1px solid ' + (selected ? 'var(--lh-primary)' : 'var(--lh-border)'),
                    borderRadius: 'var(--lh-radius)', padding: '12px 14px', display: 'grid', gap: 4,
                  }}
                >
                  <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8, alignItems: 'center' }}>
                    <span style={{ fontFamily: 'var(--lh-font-mono)', fontSize: 12, color: 'var(--lh-text-3)' }}>SUB-{row.submissionId} · 题 #{row.itemId}</span>
                    <span style={{ fontSize: 12, fontWeight: 600, color: verdict.tone }}>{verdict.label}{row.overallScore != null ? ` · ${row.overallScore}` : ''}</span>
                  </div>
                  <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--lh-text-1)', overflow: 'hidden', whiteSpace: 'nowrap', textOverflow: 'ellipsis' }}>{row.taskTitle || `任务 #${row.taskId}`}</div>
                  <div style={{ fontSize: 12, color: 'var(--lh-text-3)' }}>
                    {STATUS_LABEL[row.status] ?? row.status} · {row.model || '—'}{row.retryCount > 0 ? ` · 重试 ${row.retryCount}` : ''}
                  </div>
                </button>
              )
            })}
          </div>

          {/* 右:详情 */}
          {active ? (
            <section style={{ border: '1px solid var(--lh-border)', borderRadius: 'var(--lh-radius-lg)', background: 'var(--lh-bg-card, #fff)', padding: 'var(--space-lg)', display: 'grid', gap: 'var(--space-lg)' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'flex-start' }}>
                <div>
                  <h2 style={{ margin: 0, fontSize: 18, fontWeight: 700 }}>SUB-{active.submissionId} · {active.taskTitle || `任务 #${active.taskId}`}</h2>
                  <div style={{ fontSize: 12, color: 'var(--lh-text-3)', fontFamily: 'var(--lh-font-mono)', marginTop: 4 }}>
                    预审 #{active.id} · 题目 #{active.itemId} · 模板 v{active.promptVersion} · {STATUS_LABEL[active.status] ?? active.status}
                  </div>
                </div>
                <span style={{ fontSize: 14, fontWeight: 700, color: verdictView(active.verdict).tone, whiteSpace: 'nowrap' }}>
                  AI {verdictView(active.verdict).label}{active.overallScore != null ? ` (${active.overallScore})` : ''}
                </span>
              </div>

              {/* 维度评分 */}
              <div>
                <div style={{ fontSize: 12, fontWeight: 600, letterSpacing: '0.04em', textTransform: 'uppercase', color: 'var(--lh-text-3)', marginBottom: 8 }}>维度评分（阈值：综合 ≥ {active.passThreshold} 通过 / &lt; {active.uncertainMin} 打回）</div>
                {active.dimensions && active.dimensions.length > 0 ? (
                  <div style={{ display: 'grid', gap: 8 }}>
                    {active.dimensions.map((dim) => (
                      <div key={dim.name} style={{ display: 'grid', gridTemplateColumns: '88px 1fr 40px', gap: 10, alignItems: 'center' }} title={dim.reason}>
                        <span style={{ fontSize: 13, color: 'var(--lh-text-2)' }}>{dim.name}</span>
                        <span style={{ height: 6, borderRadius: 999, background: 'var(--lh-divider)', overflow: 'hidden' }}>
                          <span style={{ display: 'block', height: '100%', width: `${Math.max(0, Math.min(100, dim.score))}%`, background: dim.score >= active.passThreshold ? '#16a34a' : '#d97706' }} />
                        </span>
                        <span style={{ fontSize: 13, fontWeight: 700, textAlign: 'right' }}>{dim.score}</span>
                      </div>
                    ))}
                  </div>
                ) : (
                  <div style={{ fontSize: 13, color: 'var(--lh-text-3)' }}>暂无维度评分(预审尚未完成或失败)。</div>
                )}
              </div>

              {/* AI 评语 */}
              {active.reason ? (
                <div>
                  <div style={{ fontSize: 12, fontWeight: 600, letterSpacing: '0.04em', textTransform: 'uppercase', color: 'var(--lh-text-3)', marginBottom: 6 }}>AI 评语</div>
                  <div style={{ fontSize: 14, lineHeight: 1.6, color: 'var(--lh-text-1)', background: 'var(--lh-bg-elev)', borderRadius: 'var(--lh-radius)', padding: '10px 12px' }}>{active.reason}</div>
                </div>
              ) : null}

              {active.errorMsg ? (
                <div role="alert" style={{ fontSize: 13, color: '#dc2626', background: '#fef2f2', borderRadius: 'var(--lh-radius)', padding: '8px 12px' }}>失败原因：{active.errorMsg}</div>
              ) : null}

              {/* 原始 Prompt 模板 */}
              <div>
                <div style={{ fontSize: 12, fontWeight: 600, letterSpacing: '0.04em', textTransform: 'uppercase', color: 'var(--lh-text-3)', marginBottom: 6 }}>审核 Prompt 模板（{active.model || '—'}）</div>
                <pre style={{ margin: 0, fontSize: 12.5, lineHeight: 1.6, whiteSpace: 'pre-wrap', wordBreak: 'break-word', background: 'var(--lh-bg-elev)', borderRadius: 'var(--lh-radius)', padding: '10px 12px', color: 'var(--lh-text-1)' }}>{active.promptTemplate || '（未配置 Prompt 模板）'}</pre>
              </div>

              {/* 工程化元信息 */}
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(120px, 1fr))', gap: 10, fontSize: 12 }}>
                {[
                  ['模型', active.model || '—'],
                  ['Tokens', `${active.tokensInput} / ${active.tokensOutput}`],
                  ['耗时', `${active.latencyMs} ms`],
                  ['重试', String(active.retryCount)],
                  ['入队', formatTime(active.createdAt)],
                  ['完成', formatTime(active.finishedAt)],
                ].map(([label, value]) => (
                  <div key={label}>
                    <div style={{ color: 'var(--lh-text-3)' }}>{label}</div>
                    <div style={{ fontWeight: 600, color: 'var(--lh-text-1)', fontFamily: 'var(--lh-font-mono)' }}>{value}</div>
                  </div>
                ))}
              </div>
              <div style={{ fontSize: 11, color: 'var(--lh-text-3)', fontFamily: 'var(--lh-font-mono)', wordBreak: 'break-all' }}>幂等键 {active.idempotencyKey}</div>
            </section>
          ) : null}
        </div>
      )}
    </div>
  )
}
