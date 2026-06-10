import type { CSSProperties } from 'react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { Toast } from '@douyinfe/semi-ui'
import { apiGet, apiPostRawJSON } from '../../shared/api/client'
import EmptyState from '../../shared/components/EmptyState'
import StatusBadge from '../../shared/components/StatusBadge'
import { isSafeURL } from '../../shared/security/url'

type ExportFormat = 'json' | 'jsonl' | 'csv' | 'xlsx' | 'md' | 'coco'

type ExportRecord = {
  id: number
  format: string
  status: 'queued' | 'running' | 'succeeded' | 'failed'
  rowCount: number | null
  errorMsg: string | null
  createdAt: string
}
type ExportListResponse = { exports: ExportRecord[] }
type CreateExportResponse = { id: number, status: string }
type DownloadURLResponse = { url: string, expiresIn: number }

const FORMATS: ExportFormat[] = ['json', 'jsonl', 'csv', 'xlsx', 'md', 'coco']
const BASE_COLUMNS = ['submission_id', 'item_id', 'external_id', 'payload', 'answer']
const REVIEW_COLUMNS = ['ai_review.verdict', 'ai_review.overall_score', 'ai_review.reason', 'human_review.verdict', 'human_review.reason']
const POLL_INTERVAL_MS = 2000

interface ExportPanelProps {
  taskId: number
}

export default function ExportPanel({ taskId }: ExportPanelProps) {
  const [format, setFormat] = useState<ExportFormat>('csv')
  const [includeReviews, setIncludeReviews] = useState(false)
  const [selectedCols, setSelectedCols] = useState<Record<string, boolean>>({})
  const [renames, setRenames] = useState<Record<string, string>>({})
  const [records, setRecords] = useState<ExportRecord[]>([])
  const [creating, setCreating] = useState(false)
  // taskRef 做 stale guard:切任务后晚到的历史响应不污染当前任务。
  const taskRef = useRef(taskId)

  const availableColumns = includeReviews ? [...BASE_COLUMNS, ...REVIEW_COLUMNS] : BASE_COLUMNS

  const loadHistory = useCallback(async () => {
    try {
      const data = await apiGet<ExportListResponse>(`/tasks/${taskId}/exports`)
      if (taskRef.current !== taskId) return
      setRecords(data.exports)
    } catch (error) {
      // stale guard:切任务后晚到的失败不该弹到当前任务上下文。
      if (taskRef.current !== taskId) return
      Toast.error(error instanceof Error ? error.message : '加载导出历史失败')
    }
  }, [taskId])

  useEffect(() => {
    taskRef.current = taskId
    // eslint-disable-next-line react-hooks/set-state-in-effect -- 切任务时先清空旧历史
    setRecords([])
    void loadHistory()
  }, [taskId, loadHistory])

  // 有 queued/running 行时每 2s 轮询,全部 succeeded/failed 后停。
  useEffect(() => {
    if (!records.some((r) => r.status === 'queued' || r.status === 'running')) {
      return
    }
    const timer = window.setInterval(() => {
      void loadHistory()
    }, POLL_INTERVAL_MS)
    return () => window.clearInterval(timer)
  }, [records, loadHistory])

  function buildBody() {
    const columns = availableColumns
      .filter((src) => selectedCols[src])
      .map((src) => ({ source: src, export: (renames[src] || '').trim() || src }))
    return JSON.stringify({ format, include_reviews: includeReviews, field_map: { include_reviews: includeReviews, columns } })
  }

  async function createExport() {
    setCreating(true)
    try {
      const data = await apiPostRawJSON<CreateExportResponse>(`/tasks/${taskId}/exports`, buildBody())
      Toast.success(`已排队导出 #${data.id}`)
      await loadHistory()
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '导出入队失败')
    } finally {
      setCreating(false)
    }
  }

  async function download(id: number) {
    try {
      const data = await apiGet<DownloadURLResponse>(`/tasks/${taskId}/exports/${id}/download-url`)
      // 后端返回的下载地址在打开前必须校验协议(http/https),并加 noopener,noreferrer
      // 防止 javascript: 等恶意协议执行,以及新标签页反向操纵 opener。
      if (!isSafeURL(data.url)) {
        Toast.error('下载链接无效')
        return
      }
      window.open(data.url, '_blank', 'noopener,noreferrer')
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '获取下载链接失败')
    }
  }

  return (
    <section style={panelStyle} aria-label="数据导出">
      <h3 style={headingStyle}>数据导出</h3>

      <div id="ex-config">
      <div style={{ display: 'flex', gap: 'var(--space-sm)', flexWrap: 'wrap', marginBottom: 'var(--space-md)' }}>
        {FORMATS.map((f) => (
          <button
            key={f}
            type="button"
            aria-label={`格式 ${f}`}
            aria-pressed={format === f}
            onClick={() => setFormat(f)}
            style={format === f ? formatActiveStyle : formatStyle}
          >
            {f.toUpperCase()}
          </button>
        ))}
      </div>

      <label style={switchRowStyle}>
        <input type="checkbox" checked={includeReviews} onChange={(e) => setIncludeReviews(e.target.checked)} aria-label="含审核记录" />
        含审核记录
      </label>

      <div style={{ display: 'grid', gap: 4, margin: 'var(--space-md) 0' }}>
        <span style={hintStyle}>字段映射(不勾选则导出全部默认列)</span>
        <label style={{ display: 'flex', gap: 4, alignItems: 'center', fontWeight: 600 }}>
          <input
            type="checkbox"
            checked={availableColumns.length > 0 && availableColumns.every((src) => selectedCols[src])}
            onChange={(e) =>
              setSelectedCols(e.target.checked ? Object.fromEntries(availableColumns.map((src) => [src, true])) : {})
            }
            aria-label="全选字段"
          />
          全选
        </label>
        {availableColumns.map((src) => (
          <div key={src} style={{ display: 'flex', gap: 'var(--space-sm)', alignItems: 'center' }}>
            <label style={{ display: 'flex', gap: 4, minWidth: 200 }}>
              <input
                type="checkbox"
                checked={!!selectedCols[src]}
                onChange={(e) => setSelectedCols((cur) => ({ ...cur, [src]: e.target.checked }))}
                aria-label={`选择列 ${src}`}
              />
              {src}
            </label>
            <input
              type="text"
              placeholder="重命名(可选)"
              value={renames[src] || ''}
              aria-label={`重命名 ${src}`}
              onChange={(e) => setRenames((cur) => ({ ...cur, [src]: e.target.value }))}
              style={renameInputStyle}
            />
          </div>
        ))}
      </div>

      <button type="button" aria-label="开始导出" disabled={creating} onClick={() => void createExport()} style={primaryButtonStyle}>
        {creating ? '导出入队中…' : '开始导出'}
      </button>

      </div>
      <table id="ex-history" style={tableStyle}>
        <thead>
          <tr>
            <th style={thStyle}>ID</th>
            <th style={thStyle}>格式</th>
            <th style={thStyle}>状态</th>
            <th style={thStyle}>行数</th>
            <th style={thStyle}>操作</th>
          </tr>
        </thead>
        <tbody>
          {records.map((record) => (
            <tr key={record.id}>
              <td style={tdStyle}>#{record.id}</td>
              <td style={tdStyle}>{record.format}</td>
              <td style={tdStyle}>
                <StatusBadge status={record.status} />
                {record.status === 'failed' && record.errorMsg ? <span style={errorTextStyle}> {record.errorMsg}</span> : null}
              </td>
              <td style={tdStyle}>{record.rowCount ?? '—'}</td>
              <td style={tdStyle}>
                {record.status === 'succeeded' ? (
                  <button type="button" aria-label={`下载导出 #${record.id}`} onClick={() => void download(record.id)} style={linkButtonStyle}>
                    下载
                  </button>
                ) : (
                  '—'
                )}
              </td>
            </tr>
          ))}
          {records.length === 0 ? (
            <tr>
              <td style={tdStyle} colSpan={5}>
                <EmptyState title="暂无导出记录" body="创建导出后,历史记录会显示格式、状态、行数和下载入口。" variant="queue" />
              </td>
            </tr>
          ) : null}
        </tbody>
      </table>
    </section>
  )
}

const panelStyle: CSSProperties = {
  background: 'var(--lh-bg-card)',
  border: '1px solid var(--lh-border)',
  borderRadius: 'var(--radius-lg)',
  padding: 'var(--space-lg)',
  marginTop: 'var(--space-lg)',
}
const headingStyle: CSSProperties = { fontFamily: 'var(--lh-font-sans)', fontSize: 'var(--text-h2)', margin: 0, marginBottom: 'var(--space-md)' }
const hintStyle: CSSProperties = { fontSize: 'var(--text-sm)', color: 'var(--lh-text-3)' }
const switchRowStyle: CSSProperties = { display: 'flex', gap: 'var(--space-xs)', alignItems: 'center', fontSize: 'var(--text-sm)' }
const formatStyle: CSSProperties = { padding: '4px 14px', border: '1px solid var(--lh-border)', borderRadius: 'var(--radius-md)', background: 'var(--lh-bg)', cursor: 'pointer' }
const formatActiveStyle: CSSProperties = { ...formatStyle, background: 'var(--lh-primary)', color: '#fff', borderColor: 'var(--lh-primary)' }
const renameInputStyle: CSSProperties = { flex: 1, padding: '4px 8px', border: '1px solid var(--lh-border)', borderRadius: 'var(--radius-sm)' }
const primaryButtonStyle: CSSProperties = { padding: '8px 20px', border: 'none', borderRadius: 'var(--radius-md)', background: 'var(--lh-primary)', color: '#fff', cursor: 'pointer', fontWeight: 600 }
const tableStyle: CSSProperties = { width: '100%', marginTop: 'var(--space-lg)', borderCollapse: 'collapse', fontSize: 'var(--text-sm)' }
const thStyle: CSSProperties = { textAlign: 'left', padding: '6px 8px', borderBottom: '2px solid var(--lh-border)', color: 'var(--lh-text-2)' }
const tdStyle: CSSProperties = { padding: '6px 8px', borderBottom: '1px solid var(--lh-border)' }
const errorTextStyle: CSSProperties = { color: 'var(--lh-danger)', fontSize: 'var(--text-xs)' }
const linkButtonStyle: CSSProperties = { background: 'none', border: 'none', color: 'var(--lh-primary)', cursor: 'pointer', textDecoration: 'underline', padding: 0 }
