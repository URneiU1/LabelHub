import { useCallback, useEffect, useState } from 'react'
import { Toast } from '@douyinfe/semi-ui'
import {
  addAssignees,
  listAssignees,
  removeAssignee,
  type TaskAssigneeView,
} from '../../shared/api/client'

interface AssigneePanelProps {
  taskId: number
}

export default function AssigneePanel({ taskId }: AssigneePanelProps) {
  const [assignees, setAssignees] = useState<TaskAssigneeView[]>([])
  const [loading, setLoading] = useState(false)
  const [userIdDraft, setUserIdDraft] = useState('')
  const [adding, setAdding] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const data = await listAssignees(taskId)
      setAssignees(data)
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '加载指派列表失败')
    } finally {
      setLoading(false)
    }
  }, [taskId])

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- 切任务时先清空旧指派
    setAssignees([])
    void load()
  }, [load])

  async function add() {
    const userId = Number(userIdDraft.trim())
    if (!Number.isInteger(userId) || userId <= 0) {
      Toast.error('请输入正整数用户 ID')
      return
    }
    setAdding(true)
    try {
      await addAssignees(taskId, [userId])
      setUserIdDraft('')
      await load()
      Toast.success(`已指派用户 #${userId}`)
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
      Toast.success(`已取消用户 #${userId} 的指派`)
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '取消指派失败')
    }
  }

  return (
    <div className="lh-vflex" style={{ gap: 12 }}>
      <div className="lh-hflex">
        <input
          aria-label="assignee_user_id"
          className="taskform__input"
          value={userIdDraft}
          onChange={(event) => setUserIdDraft(event.target.value)}
          onKeyDown={(event) => { if (event.key === 'Enter') { event.preventDefault(); void add() } }}
          placeholder="标注员用户 ID"
        />
        <button type="button" aria-label="添加指派" disabled={adding} className="lh-btn lh-btn--primary" onClick={() => void add()}>
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
              用户 #{assignee.userId}
              <button type="button" aria-label={`移除指派 ${assignee.userId}`} className="taskform__tag-remove" onClick={() => void remove(assignee.userId)}>×</button>
            </span>
          ))}
        </div>
      )}
    </div>
  )
}
