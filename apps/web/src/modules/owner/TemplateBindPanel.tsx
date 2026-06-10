import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Toast } from '@douyinfe/semi-ui'
import { apiGet, updateTask, type Task, type TaskTemplate } from '../../shared/api/client'
import EmptyState from '../../shared/components/EmptyState'

interface TemplateBindPanelProps {
  task: Task
  // 切换绑定成功后,Dashboard 负责刷新列表并保持选中(与 TaskForm 同一回调)。
  onTaskSaved: (task: Task, created: boolean) => void
}

// TemplateBindPanel:任务编辑抽屉的「模板」标签页。
// 展示本任务的全部模板版本与当前绑定;draft 与 paused 任务可显式切换绑定版本。
// 发布中 / 已结束模板冻结(与后端 TaskTemplateFrozen 一致),只读展示。
export default function TemplateBindPanel({ task, onTaskSaved }: TemplateBindPanelProps) {
  const [templates, setTemplates] = useState<TaskTemplate[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [bindingId, setBindingId] = useState<number | null>(null)
  const frozen = task.status !== 'draft' && task.status !== 'paused'

  const loadTemplates = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const data = await apiGet<TaskTemplate[]>(`/tasks/${task.id}/templates`)
      setTemplates(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载模板版本失败')
    } finally {
      setLoading(false)
    }
  }, [task.id])

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- loadTemplates 首行同步置 loading=true 是预期加载态
    void loadTemplates()
  }, [loadTemplates])

  async function bind(template: TaskTemplate) {
    if (bindingId !== null || frozen || template.id === task.templateId) return
    setBindingId(template.id)
    try {
      const updated = await updateTask(task.id, { templateId: template.id })
      onTaskSaved(updated, false)
      Toast.success(`已绑定模板 v${template.version ?? template.id}`)
    } catch (err) {
      Toast.error(err instanceof Error ? err.message : '切换模板版本失败')
    } finally {
      setBindingId(null)
    }
  }

  return (
    <div className="lh-vflex" style={{ gap: 12 }}>
      {frozen ? (
        <div className="lh-muted lh-text-13">
          任务已{task.status === 'published' ? '发布' : '离开草稿态'},模板版本已冻结;如需修改请复制任务。
        </div>
      ) : (
        <div className="lh-muted lh-text-13">
          选择一个版本作为标注员作答使用的模板。新建/编辑版本请前往模板搭建页。
        </div>
      )}

      {error ? <div role="alert" className="taskform__error">{error}</div> : null}

      {loading ? (
        <div className="lh-muted lh-text-13">加载模板版本中…</div>
      ) : templates.length === 0 ? (
        <EmptyState title="暂无模板版本" body="先到模板搭建页创建第一个模板版本,任务发布前必须绑定模板。" variant="empty" />
      ) : (
        <ul className="lh-vflex" style={{ gap: 8, listStyle: 'none', margin: 0, padding: 0 }}>
          {templates.map((template) => {
            const isBound = template.id === task.templateId
            return (
              <li key={template.id} className="lh-hflex" style={{ gap: 10, alignItems: 'center' }}>
                <strong style={{ minWidth: 36 }}>v{template.version ?? '-'}</strong>
                <span className="lh-text-13" style={{ color: 'var(--lh-text-2)' }}>
                  #{template.id}
                  {template.createdAt ? ` · ${new Date(template.createdAt).toLocaleString()}` : ''}
                </span>
                <span className="lh-spacer" />
                {isBound ? (
                  <span className="lh-tag lh-tag--primary">当前绑定</span>
                ) : (
                  <button
                    type="button"
                    aria-label={`绑定模板版本 v${template.version ?? template.id}`}
                    className="lh-btn"
                    disabled={frozen || bindingId !== null}
                    onClick={() => void bind(template)}
                  >
                    {bindingId === template.id ? '绑定中…' : '绑定此版本'}
                  </button>
                )}
              </li>
            )
          })}
        </ul>
      )}

      <div>
        <Link className="lh-btn" to={`/owner/tasks/${task.id}/templates`} aria-label="前往模板搭建页">
          管理模板版本 / 新建版本 →
        </Link>
      </div>
    </div>
  )
}
