import { useEffect, useState, type CSSProperties } from 'react'
import { Link, useParams } from 'react-router-dom'
import { apiGet, type TaskTemplate } from '../../shared/api/client'

export default function TemplateList() {
  const { taskId } = useParams()
  const numericTaskId = Number(taskId)
  const [templates, setTemplates] = useState<TaskTemplate[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    async function loadTemplates() {
      if (!Number.isFinite(numericTaskId) || numericTaskId <= 0) {
        setError('task id 无效')
        setLoading(false)
        return
      }
      setLoading(true)
      setError('')
      try {
        const data = await apiGet<TaskTemplate[]>(`/tasks/${numericTaskId}/templates`)
        setTemplates(data)
      } catch (error) {
        setError(error instanceof Error ? error.message : '加载模板版本失败')
      } finally {
        setLoading(false)
      }
    }
    void loadTemplates()
  }, [numericTaskId])

  return (
    <div style={pageStyle}>
      <div style={{ background: 'var(--color-surface)', padding: 'var(--space-lg) var(--space-xl)', borderRadius: 'var(--radius-lg)', boxShadow: 'var(--shadow-sm)', border: '1px solid var(--color-border-light)' }}>
        <Link to="/owner" style={backLinkStyle}>← 返回 Owner 仪表盘</Link>
        <h1 style={{ ...headingStyle, marginTop: 'var(--space-sm)' }}>模板版本管理</h1>
        <p style={mutedStyle}>查看与管理当前任务的所有标注模板版本。最新的版本始终处于可编辑状态。</p>
      </div>

      {error ? <div role="alert" style={{ ...alertStyle, margin: 'var(--space-md) 0' }}>{error}</div> : null}

      <div style={{ marginTop: 'var(--space-lg)' }}>
        {loading ? (
          <p style={mutedStyle}>加载版本列表中...</p>
        ) : templates.length === 0 ? (
          <div style={{ padding: 'var(--space-2xl)', textAlign: 'center', background: 'var(--color-surface)', borderRadius: 'var(--radius-lg)', border: '2px dashed var(--color-border)' }}>
            <p style={mutedStyle}>当前任务暂无模板版本。</p>
          </div>
        ) : (
          <div style={listStyle}>
            {templates.map((template, index) => (
              <Link key={template.id} to={`/owner/tasks/${numericTaskId}/templates/${template.id}`} style={index === 0 ? activeItemStyle : itemStyle}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-md)' }}>
                    <strong style={{ fontSize: '1.25rem', color: index === 0 ? 'var(--color-accent)' : 'inherit' }}>v{template.version ?? '-'}</strong>
                    <div style={{ display: 'flex', flexDirection: 'column' }}>
                      <span style={{ fontWeight: 600 }}>Template #{template.id}</span>
                      <span style={{ fontSize: 'var(--text-sm)', color: 'var(--color-text-muted)' }}>
                        创建于: {template.createdAt ? new Date(template.createdAt).toLocaleString() : '未知'}
                      </span>
                    </div>
                  </div>
                  {index === 0 && (
                    <span style={{ padding: '4px 12px', background: '#e8f5e9', color: '#2e7d32', borderRadius: 12, fontSize: 12, fontWeight: 'bold' }}>
                      LATEST / EDITABLE
                    </span>
                  )}
                </div>
              </Link>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

const pageStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-md)',
}

const headingStyle: CSSProperties = {
  margin: 0,
  fontFamily: 'var(--font-heading)',
  fontSize: 'var(--text-h1)',
  fontWeight: 700,
}

const mutedStyle: CSSProperties = {
  margin: 'var(--space-xs) 0 0',
  color: 'var(--color-text-secondary)',
  fontSize: 'var(--text-base)',
}

const listStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-md)',
}

const itemStyle: CSSProperties = {
  display: 'block',
  padding: 'var(--space-lg) var(--space-xl)',
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-md)',
  background: 'var(--color-surface)',
  color: 'var(--color-text)',
  textDecoration: 'none',
  boxShadow: 'var(--shadow-sm)',
  transition: 'all var(--duration-fast)',
}

const activeItemStyle: CSSProperties = {
  ...itemStyle,
  borderColor: 'var(--color-accent)',
  borderWidth: '1.5px',
}

const backLinkStyle: CSSProperties = {
  color: 'var(--color-accent)',
  textDecoration: 'none',
  fontWeight: 500,
  fontSize: 'var(--text-sm)',
}

const alertStyle: CSSProperties = {
  padding: 'var(--space-md)',
  borderRadius: 'var(--radius-md)',
  border: '1px solid var(--color-danger)',
  background: '#fff1f0',
  color: 'var(--color-danger)',
}
