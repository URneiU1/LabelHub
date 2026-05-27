import type { CSSProperties } from 'react'
import { useEffect, useState } from 'react'
import { VChart } from '@visactor/react-vchart'
import { Toast } from '@douyinfe/semi-ui'
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

  useEffect(() => {
    let active = true
    // eslint-disable-next-line react-hooks/set-state-in-effect -- 切任务时先清空旧看板数据
    setStats(null)
    apiGet<TaskStats>(`/tasks/${taskId}/stats`)
      .then((data) => {
        if (active) setStats(data)
      })
      .catch((error) => {
        if (active) Toast.error(error instanceof Error ? error.message : '加载看板失败')
      })
    return () => {
      active = false
    }
  }, [taskId])

  if (!stats) {
    return (
      <section aria-label="数据看板" style={boardStyle}>
        <h3 style={headingStyle}>数据看板</h3>
        <p style={{ color: 'var(--color-text-muted)' }}>加载中…</p>
      </section>
    )
  }

  const statusValues = Object.entries(stats.statusBreakdown).map(([name, value]) => ({ name, value }))
  const aiValues = [
    { type: '一致', value: stats.aiVsHuman.compared - stats.aiVsHuman.disagree },
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
        <div style={cardStyle} aria-label="进度">
          <span style={cardTitleStyle}>进度</span>
          <strong style={bigNumberStyle}>{stats.progress.finished}/{stats.progress.total}</strong>
          <div style={progressTrackStyle} aria-hidden="true">
            <div style={{ ...progressFillStyle, width: `${progressPct}%` }} />
          </div>
        </div>

        <div style={cardStyle} aria-label="通过率">
          <span style={cardTitleStyle}>通过率</span>
          <strong style={bigNumberStyle}>{(stats.passRate * 100).toFixed(1)}%</strong>
        </div>

        <div style={cardStyle} aria-label="状态分布">
          <span style={cardTitleStyle}>状态分布</span>
          <div style={chartBoxStyle}><VChart spec={statusSpec} /></div>
        </div>

        <div style={cardStyle} aria-label="AI vs 人工">
          <span style={cardTitleStyle}>AI vs 人工差异(差异率 {(stats.aiVsHuman.rate * 100).toFixed(1)}%)</span>
          <div style={chartBoxStyle}><VChart spec={aiSpec} /></div>
        </div>

        <div style={{ ...cardStyle, gridColumn: '1 / -1' }} aria-label="维度均分">
          <span style={cardTitleStyle}>各维度均分</span>
          <div style={chartBoxStyle}><VChart spec={dimSpec} /></div>
        </div>
      </div>
    </section>
  )
}

const boardStyle: CSSProperties = {
  background: 'var(--color-surface)',
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-lg)',
  padding: 'var(--space-lg)',
  marginTop: 'var(--space-lg)',
}
const headingStyle: CSSProperties = { fontFamily: 'var(--font-heading)', fontSize: 'var(--text-h2)', margin: 0, marginBottom: 'var(--space-md)' }
const gridStyle: CSSProperties = { display: 'grid', gridTemplateColumns: 'repeat(2, minmax(0, 1fr))', gap: 'var(--space-md)' }
const cardStyle: CSSProperties = { border: '1px solid var(--color-border-light)', borderRadius: 'var(--radius-md)', padding: 'var(--space-md)', display: 'flex', flexDirection: 'column', gap: 'var(--space-xs)' }
const cardTitleStyle: CSSProperties = { fontSize: 'var(--text-sm)', color: 'var(--color-text-secondary)' }
const bigNumberStyle: CSSProperties = { fontFamily: 'var(--font-heading)', fontSize: 'var(--text-h1)', color: 'var(--color-accent)' }
const chartBoxStyle: CSSProperties = { height: 220 }
const progressTrackStyle: CSSProperties = { height: 6, background: 'var(--color-border-light)', borderRadius: 99, overflow: 'hidden' }
const progressFillStyle: CSSProperties = { height: '100%', background: 'var(--color-accent)' }
