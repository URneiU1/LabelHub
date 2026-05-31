import type { CSSProperties } from 'react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { VChart } from '@visactor/react-vchart'
import { apiGet } from '../../shared/api/client'

type ConfusionCell = { ai: string, human: string, count: number }
type ScoreBucket = { label: string, count: number }
type TrendPoint = { day: string, count: number }

type TaskStats = {
  progress: { total: number, finished: number }
  statusBreakdown: Record<string, number>
  passRate: number
  aiVsHuman: { compared: number, disagree: number, rate: number }
  dimensionAverages: { name: string, avg: number }[]
  confusion: ConfusionCell[]
  scoreBuckets: ScoreBucket[]
  completionTrend: TrendPoint[]
}

interface StatsBoardProps {
  taskId: number
}

export default function StatsBoard({ taskId }: StatsBoardProps) {
  const [stats, setStats] = useState<TaskStats | null>(null)
  const [error, setError] = useState<string | null>(null)
  const requestSeq = useRef(0)

  const load = useCallback(() => {
    const seq = requestSeq.current + 1
    requestSeq.current = seq
    setError(null)
    setStats(null)
    apiGet<TaskStats>(`/tasks/${taskId}/stats`)
      .then((data) => {
        if (requestSeq.current === seq) setStats(data)
      })
      .catch((err) => {
        if (requestSeq.current === seq) setError(err instanceof Error ? err.message : '加载看板失败')
      })
    return () => {
      requestSeq.current += 1
    }
  }, [taskId])

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- load() 内 setStats(null)/setError(null) 是切任务时清空旧看板,并返回 cleanup
    return load()
  }, [load])

  if (error) {
    return (
      <section aria-label="数据看板" style={boardStyle}>
        <h3 style={headingStyle}>数据看板</h3>
        <p style={{ color: 'var(--lh-danger)' }}>{error}</p>
        <button type="button" aria-label="重试加载看板" onClick={() => load()} style={retryButtonStyle}>重试</button>
      </section>
    )
  }

  if (!stats) {
    return (
      <section aria-label="数据看板" style={boardStyle}>
        <h3 style={headingStyle}>数据看板</h3>
        <p style={{ color: 'var(--lh-text-3)' }}>加载中…</p>
      </section>
    )
  }

  // 对新字段做防御性默认:即使后端未返回(旧版本/部分数据)也不崩。
  const confusion = stats.confusion ?? []
  const scoreBuckets = stats.scoreBuckets ?? []
  const completionTrend = stats.completionTrend ?? []
  const statusValues = Object.entries(stats.statusBreakdown).map(([name, value]) => ({ name, value }))
  const dimValues = (stats.dimensionAverages ?? []).map((d) => ({ name: d.name, avg: d.avg }))
  const progressPct = stats.progress.total > 0 ? Math.round((stats.progress.finished / stats.progress.total) * 100) : 0
  const passPct = Math.round(stats.passRate * 100)
  const agreePct = stats.aiVsHuman.compared > 0
    ? Math.round((1 - stats.aiVsHuman.disagree / stats.aiVsHuman.compared) * 100)
    : null

  const statusSpec = {
    type: 'bar',
    data: [{ id: 'status', values: statusValues }],
    xField: 'name',
    yField: 'value',
    color: ['#2f6bff'],
  }
  const scoreSpec = {
    type: 'bar',
    data: [{ id: 'score', values: scoreBuckets.map((b) => ({ label: b.label, count: b.count })) }],
    xField: 'label',
    yField: 'count',
    color: ['#2f6bff'],
  }
  const trendSpec = {
    type: 'area',
    data: [{ id: 'trend', values: completionTrend.map((p) => ({ day: p.day, count: p.count })) }],
    xField: 'day',
    yField: 'count',
    color: ['#2f6bff'],
  }
  const dimSpec = {
    type: 'radar',
    data: [{ id: 'dim', values: dimValues }],
    categoryField: 'name',
    valueField: 'avg',
    area: { visible: true },
  }

  // 混淆矩阵:固定坐标轴(AI 判定 × 人工终判)查表,缺失补 0。
  const AI_ROWS: [string, string][] = [['pass', 'AI 通过'], ['reject', 'AI 打回'], ['uncertain', 'AI 待定']]
  const HUMAN_COLS: [string, string][] = [['approve', '人工通过'], ['reject', '人工打回'], ['revise', '人工返修']]
  const confMap = new Map(confusion.map((c) => [`${c.ai}/${c.human}`, c.count]))
  const confMax = Math.max(1, ...confusion.map((c) => c.count))
  const isAgree = (ai: string, human: string) =>
    (ai === 'pass' && human === 'approve') || (ai === 'reject' && human === 'reject')

  return (
    <section aria-label="数据看板" style={boardStyle}>
      <h3 style={headingStyle}>数据看板</h3>

      <div style={kpiRowStyle}>
        <div style={cardStyle} aria-label="进度">
          <span style={cardTitleStyle}>进度</span>
          <div style={ringWrapStyle}>
            <div style={ringStyle(progressPct)} aria-hidden="true">
              <div style={ringInnerStyle}>{progressPct}%</div>
            </div>
            <div>
              <div style={bigNumberStyle}>{stats.progress.finished}/{stats.progress.total}</div>
              <div style={cardTitleStyle}>已完成 / 总题数</div>
            </div>
          </div>
        </div>

        <div style={cardStyle} aria-label="通过率">
          <span style={cardTitleStyle}>通过率</span>
          <strong style={{ ...bigNumberStyle, color: kpiColor(passPct) }}>{passPct}%</strong>
          <div style={kpiHintStyle}>approved / (approved + rejected)</div>
        </div>

        <div style={cardStyle} aria-label="AI 一致率">
          <span style={cardTitleStyle}>AI 一致率</span>
          <strong style={{ ...bigNumberStyle, color: agreePct === null ? 'var(--lh-text-3)' : kpiColor(agreePct) }}>
            {agreePct === null ? '—' : `${agreePct}%`}
          </strong>
          <div style={kpiHintStyle}>{stats.aiVsHuman.compared} 条已对比</div>
        </div>
      </div>

      <div style={gridStyle}>
        <div style={cardStyle} aria-label="状态分布">
          <span style={cardTitleStyle}>状态分布</span>
          <div style={chartBoxStyle}><VChart spec={statusSpec} /></div>
        </div>

        <div style={cardStyle} aria-label="AI 分数分布">
          <span style={cardTitleStyle}>AI 分数分布</span>
          <div style={chartBoxStyle}><VChart spec={scoreSpec} /></div>
        </div>

        <div style={cardStyle} aria-label="完成趋势">
          <span style={cardTitleStyle}>完成趋势(按天)</span>
          <div style={chartBoxStyle}><VChart spec={trendSpec} /></div>
        </div>

        <div style={cardStyle} aria-label="维度雷达">
          <span style={cardTitleStyle}>各维度均分</span>
          <div style={chartBoxStyle}><VChart spec={dimSpec} /></div>
        </div>

        <div style={{ ...cardStyle, gridColumn: '1 / -1' }} aria-label="AI 与人工混淆矩阵">
          <span style={cardTitleStyle}>AI 判定 × 人工终判(混淆矩阵)</span>
          <table style={confTableStyle}>
            <thead>
              <tr>
                <th style={confHeadStyle} />
                {HUMAN_COLS.map(([key, label]) => <th key={key} style={confHeadStyle}>{label}</th>)}
              </tr>
            </thead>
            <tbody>
              {AI_ROWS.map(([ai, aiLabel]) => (
                <tr key={ai}>
                  <th style={confRowHeadStyle}>{aiLabel}</th>
                  {HUMAN_COLS.map(([human]) => {
                    const count = confMap.get(`${ai}/${human}`) ?? 0
                    const intensity = count / confMax
                    const bg = count === 0
                      ? 'transparent'
                      : isAgree(ai, human)
                        ? `rgba(0, 180, 42, ${0.12 + intensity * 0.4})`
                        : `rgba(245, 63, 63, ${0.12 + intensity * 0.4})`
                    return <td key={human} style={{ ...confCellStyle, background: bg }}>{count}</td>
                  })}
                </tr>
              ))}
            </tbody>
          </table>
          <div style={kpiHintStyle}>绿 = AI 与人工一致,红 = 不一致;颜色越深数量越多</div>
        </div>
      </div>
    </section>
  )
}

function kpiColor(pct: number): string {
  if (pct >= 90) return 'var(--lh-success)'
  if (pct >= 70) return 'var(--lh-warning)'
  return 'var(--lh-danger)'
}

function ringStyle(pct: number): CSSProperties {
  return {
    width: 64,
    height: 64,
    borderRadius: '50%',
    background: `conic-gradient(var(--lh-primary) ${pct}%, var(--lh-border) 0)`,
    display: 'grid',
    placeItems: 'center',
    flexShrink: 0,
  }
}

const boardStyle: CSSProperties = {
  background: 'var(--lh-bg-card)',
  border: '1px solid var(--lh-border)',
  borderRadius: 'var(--radius-lg)',
  padding: 'var(--space-lg)',
  marginTop: 'var(--space-lg)',
}
const headingStyle: CSSProperties = { fontFamily: 'var(--lh-font-sans)', fontSize: 'var(--text-h2)', margin: 0, marginBottom: 'var(--space-md)' }
const gridStyle: CSSProperties = { display: 'grid', gridTemplateColumns: 'repeat(2, minmax(0, 1fr))', gap: 'var(--space-md)' }
const cardStyle: CSSProperties = { border: '1px solid var(--lh-border)', borderRadius: 'var(--radius-md)', padding: 'var(--space-md)', display: 'flex', flexDirection: 'column', gap: 'var(--space-xs)' }
const cardTitleStyle: CSSProperties = { fontSize: 'var(--text-sm)', color: 'var(--lh-text-2)' }
const bigNumberStyle: CSSProperties = { fontFamily: 'var(--lh-font-sans)', fontSize: 'var(--text-h1)', color: 'var(--lh-primary)' }
const chartBoxStyle: CSSProperties = { height: 220 }
const kpiRowStyle: CSSProperties = { display: 'grid', gridTemplateColumns: 'repeat(3, minmax(0, 1fr))', gap: 'var(--space-md)', marginBottom: 'var(--space-md)' }
const kpiHintStyle: CSSProperties = { fontSize: 'var(--text-xs)', color: 'var(--lh-text-3)' }
const ringWrapStyle: CSSProperties = { display: 'flex', alignItems: 'center', gap: 'var(--space-md)' }
const ringInnerStyle: CSSProperties = { width: 46, height: 46, borderRadius: '50%', background: 'var(--lh-bg-card)', display: 'grid', placeItems: 'center', fontWeight: 700, fontSize: 13, color: 'var(--lh-text-1)' }
const confTableStyle: CSSProperties = { borderCollapse: 'collapse', width: '100%', marginTop: 'var(--space-sm)', tableLayout: 'fixed' }
const confHeadStyle: CSSProperties = { fontSize: 'var(--text-sm)', color: 'var(--lh-text-2)', fontWeight: 600, padding: '6px 8px', textAlign: 'center', borderBottom: '1px solid var(--lh-border)' }
const confRowHeadStyle: CSSProperties = { fontSize: 'var(--text-sm)', color: 'var(--lh-text-2)', fontWeight: 600, padding: '6px 8px', textAlign: 'left', whiteSpace: 'nowrap' }
const confCellStyle: CSSProperties = { padding: '10px 8px', textAlign: 'center', fontWeight: 600, fontVariantNumeric: 'tabular-nums', border: '1px solid var(--lh-divider)', borderRadius: 'var(--radius-sm)' }
const retryButtonStyle: CSSProperties = { marginTop: 'var(--space-sm)', padding: '6px 16px', border: '1px solid var(--lh-border)', borderRadius: 'var(--radius-md)', background: 'var(--lh-bg)', cursor: 'pointer' }
