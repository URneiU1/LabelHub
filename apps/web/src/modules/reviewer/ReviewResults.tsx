import { useCallback, useEffect, useState } from 'react'
import { Button, Toast } from '@douyinfe/semi-ui'
import { listReviewResults, type ReviewResult } from '../../shared/api/client'
import EmptyState from '../../shared/components/EmptyState'
import StatusBadge from '../../shared/components/StatusBadge'
import { formatScore, formatTime } from './format'

// 审核结果列表:消费 GET /reviewer/results(已定稿 approved/rejected),
// 支持游标分页「加载更多」,点击某行回调打开只读详情。
type ReviewResultsProps = {
  onOpenResult: (result: ReviewResult) => void
}

export default function ReviewResults({ onOpenResult }: ReviewResultsProps) {
  const [results, setResults] = useState<ReviewResult[]>([])
  const [nextCursor, setNextCursor] = useState('')
  const [hasMore, setHasMore] = useState(false)
  const [loading, setLoading] = useState(false)
  const [loaded, setLoaded] = useState(false)

  const loadResults = useCallback(async (cursor?: string) => {
    setLoading(true)
    try {
      const page = await listReviewResults(cursor ? { cursor } : undefined)
      setResults((current) => (cursor ? [...current, ...page.results] : page.results))
      setNextCursor(page.nextCursor)
      setHasMore(page.hasMore)
      setLoaded(true)
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '加载审核结果失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void loadResults()
  }, [loadResults])

  if (loaded && results.length === 0) {
    return (
      <section className="ai-section" aria-label="审核结果">
        <EmptyState title="暂无审核结果" body="完成定稿(通过 / 拒绝)的提交会出现在这里。" variant="done" />
      </section>
    )
  }

  return (
    <section className="ai-section" aria-label="审核结果">
      <div className="ai-section__head">
        <span className="ai-section__title">审核结果列表</span>
        <span className="ai-section__aside lh-muted">已定稿 · 通过 / 拒绝</span>
      </div>

      <div role="list">
        {results.map((result) => (
          <button
            key={result.id}
            type="button"
            role="listitem"
            className="ai-log-row"
            onClick={() => onOpenResult(result)}
            style={resultRowStyle}
            aria-label={`查看 Submission #${result.id} 审核结果`}
          >
            <span className="ai-log-row__time">SUB-{result.id}</span>
            <StatusBadge status={result.status} />
            <span style={resultMetaStyle}>
              <span>Task #{result.taskId} · Item #{result.itemId}</span>
              <span className="lh-muted">
                决定 {formatVerdict(result.finalVerdict)} · AI {formatScore(result.aiScore)} · {formatTime(result.updatedAt, 'date-time')}
              </span>
            </span>
          </button>
        ))}
      </div>

      {hasMore ? (
        <div style={{ marginTop: 12 }}>
          <Button loading={loading} onClick={() => void loadResults(nextCursor)} theme="light">加载更多</Button>
        </div>
      ) : null}
    </section>
  )
}

const resultRowStyle = {
  width: '100%',
  border: 0,
  borderBottom: '1px solid var(--lh-divider)',
  background: 'transparent',
  textAlign: 'left' as const,
  cursor: 'pointer',
}

const resultMetaStyle = {
  display: 'grid',
  gap: 2,
  fontSize: 13,
  color: 'var(--lh-text-1)',
}

function formatVerdict(verdict: string | null | undefined) {
  if (verdict === 'approve') return '通过'
  if (verdict === 'reject') return '拒绝'
  return verdict ?? '-'
}

