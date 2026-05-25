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
      <div>
        <Link to="/owner" style={backLinkStyle}>返回 Owner</Link>
        <h1 style={headingStyle}>模板版本</h1>
        <p style={mutedStyle}>选择最新版本进入编辑；历史版本可只读查看或 Fork。</p>
      </div>
      {error ? <div role="alert" style={alertStyle}>{error}</div> : null}
      {loading ? <p style={mutedStyle}>加载模板版本...</p> : null}
      {!loading && templates.length === 0 ? <p style={mutedStyle}>当前任务暂无模板版本。</p> : null}
      <div style={listStyle}>
        {templates.map((template, index) => (
          <Link key={template.id} to={`/owner/tasks/${numericTaskId}/templates/${template.id}`} style={itemStyle}>
            <strong>v{template.version ?? '-'}</strong>
            <span>Template #{template.id}{index === 0 ? ' · latest' : ''}</span>
            <span>{template.createdAt ? new Date(template.createdAt).toLocaleString() : 'created time unknown'}</span>
          </Link>
        ))}
      </div>
    </div>
  )
}

const pageStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-lg)',
}

const headingStyle: CSSProperties = {
  margin: 'var(--space-xs) 0 0',
  fontFamily: 'var(--font-heading)',
  fontSize: 'var(--text-h1)',
}

const mutedStyle: CSSProperties = {
  margin: 'var(--space-xs) 0 0',
  color: 'var(--color-text-secondary)',
}

const listStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-sm)',
}

const itemStyle: CSSProperties = {
  display: 'grid',
  gap: 4,
  padding: 'var(--space-md)',
  border: '1px solid var(--color-border)',
  background: 'var(--color-surface)',
  color: 'var(--color-text)',
  textDecoration: 'none',
}

const backLinkStyle: CSSProperties = {
  color: 'var(--color-accent)',
  textDecoration: 'none',
}

const alertStyle: CSSProperties = {
  padding: 'var(--space-sm)',
  border: '1px solid var(--color-danger)',
  color: 'var(--color-danger)',
}
