import { useCallback, useEffect, useMemo, useState } from 'react'
import { Toast } from '@douyinfe/semi-ui'
import {
  addAssignees,
  listAssigneeCandidates,
  listAssignees,
  removeAssignee,
  type AssigneeCandidate,
  type TaskAssigneeView,
} from '../../shared/api/client'

interface AssigneePanelProps {
  taskId: number
}

export default function AssigneePanel({ taskId }: AssigneePanelProps) {
  const [assignees, setAssignees] = useState<TaskAssigneeView[]>([])
  const [candidates, setCandidates] = useState<AssigneeCandidate[]>([])
  const [loading, setLoading] = useState(false)
  const [selectedId, setSelectedId] = useState('')
  const [adding, setAdding] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [assigneeList, candidateList] = await Promise.all([
        listAssignees(taskId),
        listAssigneeCandidates(taskId),
      ])
      setAssignees(assigneeList)
      setCandidates(candidateList)
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '加载指派列表失败')
    } finally {
      setLoading(false)
    }
  }, [taskId])

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- 切任务时先清空旧数据
    setAssignees([])
    setSelectedId('')
    void load()
  }, [load])

  const nameFor = useCallback((userId: number) => {
    const candidate = candidates.find((item) => item.userId === userId)
    return candidate ? (candidate.displayName || candidate.username) : `用户 #${userId}`
  }, [candidates])

  const assignedIds = useMemo(() => new Set(assignees.map((item) => item.userId)), [assignees])
  const unassigned = useMemo(
    () => candidates.filter((candidate) => !assignedIds.has(candidate.userId)),
    [candidates, assignedIds],
  )

  async function add() {
    const userId = Number(selectedId)
    if (!Number.isInteger(userId) || userId <= 0) {
      Toast.error('请先选择一个标注员')
      return
    }
    setAdding(true)
    try {
      await addAssignees(taskId, [userId])
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
      await removeAssignee(taskId, userId)
      setAssignees((current) => current.filter((item) => item.userId !== userId))
      Toast.success(`已取消 ${nameFor(userId)} 的指派`)
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '取消指派失败')
    }
  }

  return (
    <div className="lh-vflex" style={{ gap: 12 }}>
      <div className="lh-hflex">
        <select
          aria-label="assignee_candidate"
          className="taskform__input"
          value={selectedId}
          onChange={(event) => setSelectedId(event.target.value)}
          disabled={loading || unassigned.length === 0}
        >
          <option value="">{unassigned.length === 0 ? '无可指派的标注员' : '选择标注员…'}</option>
          {unassigned.map((candidate) => (
            <option key={candidate.userId} value={candidate.userId}>
              {(candidate.displayName || candidate.username)} (#{candidate.userId})
            </option>
          ))}
        </select>
        <button type="button" aria-label="添加指派" disabled={adding || !selectedId} className="lh-btn lh-btn--primary" onClick={() => void add()}>
          {adding ? '指派中…' : '指派'}
        </button>
      </div>

      {loading ? (
        <div className="lh-muted lh-text-13">加载指派列表…</div>
      ) : assignees.length === 0 ? (
        <div className="lh-muted lh-text-13">尚未指派标注员。</div>
      ) : (
        <div className="taskform__tags">
          {assignees.map((assignee) => (
            <span key={assignee.userId} className="lh-tag taskform__tag">
              {nameFor(assignee.userId)}
              <button type="button" aria-label={`移除指派 ${assignee.userId}`} className="taskform__tag-remove" onClick={() => void remove(assignee.userId)}>×</button>
            </span>
          ))}
        </div>
      )}
    </div>
  )
}
