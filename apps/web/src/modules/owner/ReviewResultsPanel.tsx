import { useCallback, useEffect, useState } from 'react'
import { Toast } from '@douyinfe/semi-ui'
import { listOwnerReviewResults, type OwnerReviewResult } from '../../shared/api/client'
import StatusBadge from '../../shared/components/StatusBadge'
import EmptyState from '../../shared/components/EmptyState'
import LoadingBlock from '../../shared/components/LoadingBlock'

interface ReviewResultsPanelProps {
  taskId: number
}

const verdictLabel: Record<string, string> = {
  pass: '通过',
  reject: '打回',
  uncertain: '不确定',
  approve: '通过',
}

function aiBadgeStatus(verdict: string | null): string {
  if (verdict === 'pass') return 'approved'
  if (verdict === 'reject') return 'rejected'
  return 'draft'
}

export default function ReviewResultsPanel({ taskId }: ReviewResultsPanelProps) {
  const [results, setResults] = useState<OwnerReviewResult[]>([])
  const [nextCursor, setNextCursor] = useState('')
  const [hasMore, setHasMore] = useState(false)
  const [loading, setLoading] = useState(false)
  const [onlyDisagreed, setOnlyDisagreed] = useState(false)

  const load = useCallback(async (cursor?: string) => {
    setLoading(true)
    try {
      const page = await listOwnerReviewResults(taskId, cursor ? { cursor } : undefined)
      setResults((prev) => (cursor ? [...prev, ...page.results] : page.results))
      setNextCursor(page.nextCursor)
      setHasMore(page.hasMore)
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '加载审核结果失败')
    } finally {
      setLoading(false)
    }
  }, [taskId])

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- 切任务清空旧结果
    setResults([])
    setNextCursor('')
    setHasMore(false)
    setOnlyDisagreed(false)
    void load()
  }, [load])

  const shown = onlyDisagreed ? results.filter((result) => result.agreed === false) : results

  return (
    <div id="rv-results" style={{ marginTop: 'var(--space-lg)' }} aria-label="逐条质检结果">
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 'var(--space-md)' }}>
        <h4 style={{ margin: 0, fontSize: 'var(--text-base)', color: 'var(--lh-text-1)' }}>逐条质检结果</h4>
        <label style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 'var(--text-sm)', color: 'var(--lh-text-2)' }}>
          <input
            type="checkbox"
            aria-label="只看 AI 与人工不一致"
            checked={onlyDisagreed}
            onChange={(event) => setOnlyDisagreed(event.target.checked)}
          />
          只看 AI 与人工不一致
        </label>
      </div>

      {loading && results.length === 0 ? (
        <LoadingBlock title="审核结果加载中" rows={3} />
      ) : shown.length === 0 ? (
        <EmptyState
          title={onlyDisagreed ? '没有不一致的结果' : '暂无审核结果'}
          body={onlyDisagreed ? 'AI 预审与人工判定目前一致。' : '任务出现已定稿(通过/打回)的提交后会显示在这里,用于回看 AI 预审标准。'}
          variant="empty"
        />
      ) : (
        <div style={{ display: 'grid', gap: 'var(--space-sm)' }}>
          {shown.map((result) => (
            <div key={result.id} style={rowStyle}>
              <span style={{ color: 'var(--lh-text-3)', fontSize: 'var(--text-sm)' }}>题目 #{result.itemId}</span>
              <span style={cellStyle}>
                <span style={tagLabelStyle}>AI</span>
                <StatusBadge status={aiBadgeStatus(result.aiVerdict)} label={result.aiVerdict ? (verdictLabel[result.aiVerdict] ?? result.aiVerdict) : '—'} />
              </span>
              <span style={cellStyle}>
                <span style={tagLabelStyle}>人工</span>
                <StatusBadge status={result.status} label={result.humanVerdict ? (verdictLabel[result.humanVerdict] ?? result.humanVerdict) : '—'} />
              </span>
              <span style={{ fontSize: 'var(--text-sm)', fontWeight: 600, color: result.agreed === false ? 'var(--lh-danger)' : result.agreed === true ? 'var(--lh-success)' : 'var(--lh-text-3)' }}>
                {result.agreed === false ? '不一致' : result.agreed === true ? '一致' : 'N/A'}
              </span>
            </div>
          ))}
        </div>
      )}

      {hasMore && !onlyDisagreed ? (
        <button type="button" className="lh-btn" disabled={loading} onClick={() => void load(nextCursor)} style={{ marginTop: 'var(--space-md)' }}>
          {loading ? '加载中…' : '加载更多'}
        </button>
      ) : null}
    </div>
  )
}

const rowStyle: React.CSSProperties = {
  display: 'grid',
  gridTemplateColumns: '120px 1fr 1fr 80px',
  alignItems: 'center',
  gap: 'var(--space-sm)',
  padding: 'var(--space-sm) var(--space-md)',
  border: '1px solid var(--lh-border)',
  borderRadius: 'var(--radius-md)',
  background: 'var(--lh-bg-card)',
}

const cellStyle: React.CSSProperties = {
  display: 'flex',
  gap: 6,
  alignItems: 'center',
}

const tagLabelStyle: React.CSSProperties = {
  fontSize: 'var(--text-sm)',
  color: 'var(--lh-text-2)',
}
