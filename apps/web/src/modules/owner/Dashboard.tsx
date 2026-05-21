import { useCallback, useEffect, useState } from 'react'
import { Button, Toast } from '@douyinfe/semi-ui'
import { apiGet, type Task } from '../../shared/api/client'

type TaskListResponse = Task[]
type ExportResponse = {
  task: Task
  rows: Array<Record<string, unknown>>
}

export default function OwnerDashboard() {
  const [tasks, setTasks] = useState<Task[]>([])
  const [selected, setSelected] = useState<Task | null>(null)
  const [exportRows, setExportRows] = useState<Array<Record<string, unknown>>>([])

  const loadTasks = useCallback(async () => {
    try {
      const data = await apiGet<TaskListResponse>('/tasks')
      setTasks(data)
      setSelected(data[0] ?? null)
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '加载任务失败')
    }
  }, [])

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void loadTasks()
  }, [loadTasks])

  async function exportJSON(taskId: number) {
    try {
      const data = await apiGet<ExportResponse>(`/tasks/${taskId}/export/json`)
      setExportRows(data.rows)
      Toast.success(`导出 ${data.rows.length} 条 approved 数据`)
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '导出失败')
    }
  }

  return (
    <div>
      <h1 style={{ fontFamily: 'var(--font-heading)', fontSize: 'var(--text-h1)' }}>Owner 任务负责人</h1>
      <p style={{ fontFamily: 'var(--font-body)', color: 'var(--color-text-secondary)' }}>任务管理 · baseline · 数据导出</p>

      <div style={{ display: 'grid', gridTemplateColumns: '280px minmax(0, 1fr)', gap: 'var(--space-lg)', marginTop: 'var(--space-lg)' }}>
        <section style={panelStyle}>
          <h2 style={headingStyle}>任务</h2>
          {tasks.map((task) => (
            <button key={task.id} onClick={() => setSelected(task)} style={task.id === selected?.id ? activeListButtonStyle : listButtonStyle}>
              <strong>{task.title}</strong>
              <span>{task.finishedItems}/{task.totalItems} · {task.status}</span>
            </button>
          ))}
        </section>

        <section style={panelStyle}>
          {selected ? (
            <>
              <h2 style={headingStyle}>{selected.title}</h2>
              <p style={{ color: 'var(--color-text-secondary)' }}>
                官方 qa_quality 主线任务。AI 预审在 Sprint 1 关闭，提交后直接进入人工审核。
              </p>
              <div style={{ marginTop: 'var(--space-md)', padding: 'var(--space-md)', border: '1px solid var(--color-border-light)', maxHeight: 260, overflow: 'auto', whiteSpace: 'pre-wrap' }}>
                {selected.baselineDescription?.String || '暂无 baseline'}
              </div>
              <Button onClick={() => void exportJSON(selected.id)} style={{ marginTop: 'var(--space-md)' }}>
                导出 approved JSON
              </Button>
              {exportRows.length > 0 ? (
                <pre style={{ marginTop: 'var(--space-md)', padding: 'var(--space-md)', background: 'var(--color-bg)', overflow: 'auto', maxHeight: 260 }}>
                  {JSON.stringify(exportRows.slice(0, 3), null, 2)}
                </pre>
              ) : null}
            </>
          ) : (
            <p>暂无任务</p>
          )}
        </section>
      </div>
    </div>
  )
}

const panelStyle: React.CSSProperties = {
  background: 'var(--color-surface)',
  border: '1px solid var(--color-border)',
  padding: 'var(--space-lg)',
}

const headingStyle: React.CSSProperties = {
  fontFamily: 'var(--font-heading)',
  fontSize: 'var(--text-h2)',
  margin: 0,
}

const listButtonStyle: React.CSSProperties = {
  display: 'grid',
  gap: 4,
  width: '100%',
  marginTop: 'var(--space-sm)',
  padding: 'var(--space-sm)',
  textAlign: 'left',
  background: 'var(--color-surface)',
  border: '1px solid var(--color-border-light)',
}

const activeListButtonStyle: React.CSSProperties = {
  ...listButtonStyle,
  borderColor: 'var(--color-accent)',
  background: 'var(--color-bg)',
}
