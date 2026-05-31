import type { CSSProperties } from 'react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { VChart } from '@visactor/react-vchart'
import { apiGet } from '../../shared/api/client'

type TaskStats = {
  progress: { total: number, finished: number }
  statusBreakdown: Record<string, number>
  passRate: number
  aiVsHuman: { compared: number, disagree: number, rate: number }
  dimensionAverages: { name: string, avg: number }[]
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

  const statusValues = Object.entries(stats.statusBreakdown).map(([name, value]) => ({ name, value }))
  const aiValues = [
    { type: '一致', value: Math.max(0, stats.aiVsHuman.compared - stats.aiVsHuman.disagree) },
    { type: '不一致', value: stats.aiVsHuman.disagree },
  ]
  const dimValues = stats.dimensionAverages.map((d) => ({ name: d.name, avg: d.avg }))
  const progressPct = stats.progress.total > 0 ? Math.round((stats.progress.finished / stats.progress.total) * 100) : 0

  const statusSpec = {
    type: 'bar',
    data: [{ id: 'status', values: statusValues }],
    xField: 'name',
    yField: 'value',
  }
  const aiSpec = {
    type: 'pie',
    data: [{ id: 'ai', values: aiValues }],
    valueField: 'value',
    categoryField: 'type',
  }
  const dimSpec = {
    type: 'bar',
    data: [{ id: 'dim', values: dimValues }],
    xField: 'avg',
    yField: 'name',
    direction: 'horizontal',
  }

  return (
    <section aria-label="数据看板" style={boardStyle}>
      <h3 style={headingStyle}>数据看板</h3>
      <div style={gridStyle}>
        <div id="st-progress" style={cardStyle} aria-label="进度">
          <span style={cardTitleStyle}>进度</span>
          <strong style={bigNumberStyle}>{stats.progress.finished}/{stats.progress.total}</strong>
          <div style={progressTrackStyle} aria-hidden="true">
            <div style={{ ...progressFillStyle, width: `${progressPct}%` }} />
          </div>
        </div>

        <div id="st-pass" style={cardStyle} aria-label="通过率">
          <span style={cardTitleStyle}>通过率</span>
          <strong style={bigNumberStyle}>{(stats.passRate * 100).toFixed(1)}%</strong>
        </div>

        <div id="st-status" style={cardStyle} aria-label="状态分布">
          <span style={cardTitleStyle}>状态分布</span>
          <div style={chartBoxStyle}><VChart spec={statusSpec} /></div>
        </div>

        <div style={cardStyle} aria-label="AI vs 人工">
          <span style={cardTitleStyle}>AI vs 人工差异(差异率 {(stats.aiVsHuman.rate * 100).toFixed(1)}%)</span>
          <div style={chartBoxStyle}><VChart spec={aiSpec} /></div>
        </div>

        <div id="st-dim" style={{ ...cardStyle, gridColumn: '1 / -1' }} aria-label="维度均分">
          <span style={cardTitleStyle}>各维度均分</span>
          <div style={chartBoxStyle}><VChart spec={dimSpec} /></div>
        </div>
      </div>
    </section>
  )
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
const progressTrackStyle: CSSProperties = { height: 6, background: 'var(--lh-border)', borderRadius: 99, overflow: 'hidden' }
const progressFillStyle: CSSProperties = { height: '100%', background: 'var(--lh-primary)' }
const retryButtonStyle: CSSProperties = { marginTop: 'var(--space-sm)', padding: '6px 16px', border: '1px solid var(--lh-border)', borderRadius: 'var(--radius-md)', background: 'var(--lh-bg)', cursor: 'pointer' }
