import { useCallback, useEffect, useMemo, useState } from 'react'
import { Toast } from '@douyinfe/semi-ui'
import {
  addReviewers,
  listReviewerCandidates,
  listReviewers,
  removeReviewer,
  type ReviewerCandidate,
  type TaskReviewerView,
} from '../../shared/api/client'

interface ReviewerPanelProps {
  taskId: number
}

// 审核员指派面板:owner 在此查看任务已指派的审核员、从 reviewer 候选里添加、移除。
// 结构与 AssigneePanel 一致(候选下拉 + tag 列表),但操作的是 task_reviewers。
export default function ReviewerPanel({ taskId }: ReviewerPanelProps) {
  const [reviewers, setReviewers] = useState<TaskReviewerView[]>([])
  const [candidates, setCandidates] = useState<ReviewerCandidate[]>([])
  const [loading, setLoading] = useState(false)
  const [selectedId, setSelectedId] = useState('')
  const [adding, setAdding] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [reviewerList, candidateList] = await Promise.all([
        listReviewers(taskId),
        listReviewerCandidates(taskId),
      ])
      setReviewers(reviewerList)
      setCandidates(candidateList)
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '加载审核员列表失败')
    } finally {
      setLoading(false)
    }
  }, [taskId])

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- 切任务时先清空旧数据
    setReviewers([])
    setSelectedId('')
    void load()
  }, [load])

  const nameFor = useCallback((userId: number) => {
    const candidate = candidates.find((item) => item.userId === userId)
    return candidate ? (candidate.displayName || candidate.username) : `用户 #${userId}`
  }, [candidates])

  const assignedIds = useMemo(() => new Set(reviewers.map((item) => item.userId)), [reviewers])
  const unassigned = useMemo(
    () => candidates.filter((candidate) => !assignedIds.has(candidate.userId)),
    [candidates, assignedIds],
  )

  async function add() {
    const userId = Number(selectedId)
    if (!Number.isInteger(userId) || userId <= 0) {
      Toast.error('请先选择一个审核员')
      return
    }
    setAdding(true)
    try {
      await addReviewers(taskId, [userId])
      setSelectedId('')
      await load()
      Toast.success(`已指派 ${nameFor(userId)}`)
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '指派失败')
    } finally {
      setAdding(false)
    }
  }

  async function remove(userId: number) {
    try {
      await removeReviewer(taskId, userId)
      setReviewers((current) => current.filter((item) => item.userId !== userId))
      Toast.success(`已取消 ${nameFor(userId)} 的指派`)
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '取消指派失败')
    }
  }

  return (
    <div className="lh-vflex" style={{ gap: 12 }}>
      <div className="lh-muted lh-text-13">
        指派审核员后,只有被指派的审核员能在审核工作台看到并复核本任务的提交。
      </div>
      <div className="lh-hflex">
        <select
          aria-label="reviewer_candidate"
          className="taskform__input"
          value={selectedId}
          onChange={(event) => setSelectedId(event.target.value)}
          disabled={loading || unassigned.length === 0}
        >
          <option value="">{unassigned.length === 0 ? '无可指派的审核员' : '选择审核员…'}</option>
          {unassigned.map((candidate) => (
            <option key={candidate.userId} value={candidate.userId}>
              {(candidate.displayName || candidate.username)} (#{candidate.userId})
            </option>
          ))}
        </select>
        <button type="button" aria-label="添加审核员" disabled={adding || !selectedId} className="lh-btn lh-btn--primary" onClick={() => void add()}>
          {adding ? '指派中…' : '指派'}
        </button>
      </div>

      {loading ? (
        <div className="lh-muted lh-text-13">加载审核员列表…</div>
      ) : reviewers.length === 0 ? (
        <div className="lh-muted lh-text-13">尚未指派审核员。</div>
      ) : (
        <div className="taskform__tags">
          {reviewers.map((reviewer) => (
            <span key={reviewer.userId} className="lh-tag taskform__tag">
              {nameFor(reviewer.userId)}
              <button type="button" aria-label={`移除审核员 ${reviewer.userId}`} className="taskform__tag-remove" onClick={() => void remove(reviewer.userId)}>×</button>
            </span>
          ))}
        </div>
      )}
    </div>
  )
}
