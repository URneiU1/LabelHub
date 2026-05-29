import { useMemo } from 'react'
import type { Submission } from '../../shared/api/client'
import { normalizeStatus } from '../../shared/components/status'
import EmptyState from '../../shared/components/EmptyState'
import StatusBadge from '../../shared/components/StatusBadge'

type MyDataProps = {
  submissions: Submission[]
  onOpen: (submission: Submission) => void
}

type Bucket = '已提交' | '通过' | '打回' | '待修改'

// 提交状态 → 我的数据聚合桶。submitted/审核中→已提交,approved→通过,rejected→打回,revising→待修改。
function bucketOf(status: string): Bucket | null {
  switch (normalizeStatus(status)) {
    case 'submitted':
    case 'ai_reviewing':
    case 'human_reviewing':
      return '已提交'
    case 'approved':
      return '通过'
    case 'rejected':
      return '打回'
    case 'revising':
      return '待修改'
    default:
      return null
  }
}

// 我的数据:跨任务汇总当前 labeler 的提交统计 + 可点击列表(点击打开对应题目)。
export default function MyData({ submissions, onOpen }: MyDataProps) {
  const counts = useMemo(() => {
    const acc: Record<Bucket, number> = { 已提交: 0, 通过: 0, 打回: 0, 待修改: 0 }
    for (const submission of submissions) {
      const bucket = bucketOf(submission.status)
      if (bucket) {
        acc[bucket] += 1
      }
    }
    return acc
  }, [submissions])

  const stats: Array<{ label: Bucket, tone?: string }> = [
    { label: '已提交', tone: 'primary' },
    { label: '通过', tone: 'success' },
    { label: '打回', tone: 'warning' },
    { label: '待修改', tone: 'warning' },
  ]

  return (
    <div className="lz-mydata">
      <div className="lh-stats">
        {stats.map((stat) => (
          <div key={stat.label} className="lh-stat">
            <div className="lh-stat__label">{stat.label}</div>
            <div className={'lh-stat__value' + (stat.tone ? ` lh-stat__value--${stat.tone}` : '')}>
              {counts[stat.label]}
            </div>
          </div>
        ))}
      </div>

      {submissions.length === 0 ? (
        <EmptyState title="还没有提交记录" body="领取并提交题目后,这里会显示你的全部标注记录。" variant="empty" />
      ) : (
        <div className="tasks-table">
          <table>
            <thead>
              <tr>
                <th>提交</th>
                <th style={{ width: 120 }}>状态</th>
                <th style={{ width: 120 }}>操作</th>
              </tr>
            </thead>
            <tbody>
              {submissions.map((submission) => (
                <tr key={submission.id} style={{ cursor: 'pointer' }} onClick={() => onOpen(submission)}>
                  <td>
                    <div className="tasks-table__title">提交 #{submission.id}</div>
                    <div className="tasks-table__meta">任务 #{submission.taskId} · 题目 #{submission.itemId}</div>
                  </td>
                  <td><StatusBadge status={submission.status} /></td>
                  <td>
                    <button
                      type="button"
                      className="lh-btn lh-btn--sm"
                      aria-label={`打开提交 #${submission.id}`}
                      onClick={(event) => {
                        event.stopPropagation()
                        onOpen(submission)
                      }}
                    >
                      打开
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
