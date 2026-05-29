import { useState } from 'react'
import { Toast } from '@douyinfe/semi-ui'
import { ApiError, transitionTask, type Task } from '../../shared/api/client'
import StatusBadge from '../../shared/components/StatusBadge'
import EmptyState from '../../shared/components/EmptyState'
import TaskForm from './TaskForm'
import ImportPanel from './ImportPanel'
import AssigneePanel from './AssigneePanel'
import {
  availableTransitions,
  computeTaskStats,
  distributionLabel,
  taskProgressPercent,
  taskStatusLabel,
  TRANSITION_LABELS,
  type TaskTransition,
} from './taskManageHelpers'
import '../../styles/lh/tasks.css'

interface TaskManagePanelProps {
  tasks: Task[]
  selected: Task | null
  onSelect: (task: Task) => void
  // 任务被创建/编辑/迁移后,Dashboard 负责刷新列表并保持选中。
  onTaskSaved: (task: Task, created: boolean) => void
  onTasksChanged: () => void
}

type DrawerTab = 'info' | 'dataset' | 'distribution'

export default function TaskManagePanel({ tasks, selected, onSelect, onTaskSaved, onTasksChanged }: TaskManagePanelProps) {
  // drawerMode: 'create' 新建草稿 / 'edit' 编辑选中任务 / null 关闭。
  const [drawerMode, setDrawerMode] = useState<'create' | 'edit' | null>(null)
  const [drawerTab, setDrawerTab] = useState<DrawerTab>('info')
  const [transitioning, setTransitioning] = useState<TaskTransition | null>(null)

  const stats = computeTaskStats(tasks)
  const drawerTask = drawerMode === 'edit' ? selected : null

  function openCreate() {
    setDrawerMode('create')
    setDrawerTab('info')
  }

  function openEdit() {
    if (!selected) return
    setDrawerMode('edit')
    setDrawerTab('info')
  }

  function handleSaved(task: Task, created: boolean) {
    onTaskSaved(task, created)
    if (created) {
      // 新建后切到编辑态,方便接着配模板/导入数据。
      setDrawerMode('edit')
    }
  }

  async function runTransition(action: TaskTransition) {
    if (!selected || transitioning) return
    setTransitioning(action)
    try {
      const updated = await transitionTask(selected.id, action)
      onTaskSaved(updated, false)
      Toast.success(`已${TRANSITION_LABELS[action]}任务`)
    } catch (error) {
      // 422 INVALID_STATE(如未绑定模板就发布)走友好 ApiError 文案。
      const message = error instanceof ApiError ? error.message : error instanceof Error ? error.message : '操作失败'
      Toast.error(message)
    } finally {
      setTransitioning(null)
    }
  }

  return (
    <section className="lh-task-manage">
      <div className="lh-page-head lh-hflex" style={{ alignItems: 'flex-start' }}>
        <div>
          <h2 className="lh-page-head__title">任务管理</h2>
          <div className="lh-page-head__crumb">维护任务全生命周期:草稿 → 发布中 → 已暂停 → 已结束</div>
        </div>
        <div className="lh-spacer" />
        <button type="button" aria-label="新建任务" className="lh-btn lh-btn--primary" onClick={openCreate}>+ 新建任务</button>
      </div>

      <div className="lh-stats">
        {stats.map((stat) => (
          <div key={stat.key} className="lh-stat">
            <div className="lh-stat__label">{stat.label}</div>
            <div className={'lh-stat__value' + (stat.tone ? ` lh-stat__value--${stat.tone}` : '')}>{stat.value}</div>
          </div>
        ))}
      </div>

      <div className="tasks-table">
        <table>
          <thead>
            <tr>
              <th>任务</th>
              <th style={{ width: 110 }}>状态</th>
              <th style={{ width: 120 }}>分发策略</th>
              <th style={{ width: 200 }}>配额 / 进度</th>
            </tr>
          </thead>
          <tbody>
            {tasks.map((task) => {
              const percent = taskProgressPercent(task)
              return (
                <tr
                  key={task.id}
                  className={selected?.id === task.id ? 'tasks-table__row--active' : ''}
                  onClick={() => onSelect(task)}
                  style={{ cursor: 'pointer' }}
                >
                  <td>
                    <div className="tasks-table__title">{task.title}</div>
                    <div className="tasks-table__meta">{task.id} · {taskStatusLabel(task.status)}</div>
                  </td>
                  <td><StatusBadge status={task.status} label={taskStatusLabel(task.status)} /></td>
                  <td>{distributionLabel(task.distribution ?? 'first_come')}</td>
                  <td>
                    {percent === null ? (
                      <span className="lh-muted">—</span>
                    ) : (
                      <>
                        <div className="lh-text-13" style={{ color: 'var(--lh-text-1)' }}>{task.finishedItems} / {task.totalItems}</div>
                        <div className="tasks-progress">
                          <div className="tasks-progress__bar">
                            <div className={'tasks-progress__fill' + (task.status === 'ended' ? ' tasks-progress__fill--success' : task.status === 'paused' ? ' tasks-progress__fill--warning' : '')} style={{ width: `${percent}%` }} />
                          </div>
                        </div>
                      </>
                    )}
                  </td>
                </tr>
              )
            })}
            {tasks.length === 0 ? (
              <tr>
                <td colSpan={4}>
                  <EmptyState title="暂无任务" body="点击「新建任务」创建第一个标注任务。" variant="empty" />
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>

      {selected ? (
        <div className="lh-hflex lh-task-manage__actions">
          <button type="button" aria-label="编辑任务" className="lh-btn" onClick={openEdit}>编辑信息</button>
          <span className="lh-task-manage__status">
            <StatusBadge status={selected.status} label={taskStatusLabel(selected.status)} />
          </span>
          {availableTransitions(selected.status).map((action) => (
            <button
              key={action}
              type="button"
              aria-label={`${TRANSITION_LABELS[action]}任务`}
              disabled={transitioning !== null}
              className={'lh-btn' + (action === 'publish' || action === 'resume' ? ' lh-btn--primary' : '')}
              onClick={() => void runTransition(action)}
            >
              {transitioning === action ? '处理中…' : TRANSITION_LABELS[action]}
            </button>
          ))}
        </div>
      ) : null}

      {drawerMode ? (
        <>
          <div className="tasks-drawer__scrim" onClick={() => setDrawerMode(null)} aria-hidden="true" />
          <aside className="tasks-drawer" aria-label="任务编辑抽屉">
            <div className="tasks-drawer__head">
              <h3>{drawerMode === 'create' ? '新建任务' : `编辑任务 · ${drawerTask?.title ?? ''}`}</h3>
              <button type="button" className="tasks-drawer__close" aria-label="关闭抽屉" onClick={() => setDrawerMode(null)}>✕</button>
            </div>

            {drawerMode === 'edit' && drawerTask ? (
              <div className="tasks-drawer__tabs">
                {([
                  ['info', '基础信息'],
                  ['dataset', '数据集'],
                  ['distribution', '分发策略'],
                ] as Array<[DrawerTab, string]>).map(([key, label]) => (
                  <button
                    key={key}
                    type="button"
                    aria-label={`抽屉标签 ${label}`}
                    aria-pressed={drawerTab === key}
                    className={'tasks-drawer__tab' + (drawerTab === key ? ' tasks-drawer__tab--active' : '')}
                    onClick={() => setDrawerTab(key)}
                  >
                    {label}
                  </button>
                ))}
              </div>
            ) : null}

            <div className="tasks-drawer__body">
              {drawerMode === 'create' ? (
                <TaskForm task={null} onSaved={handleSaved} />
              ) : drawerTask ? (
                <>
                  {drawerTab === 'info' ? <TaskForm task={drawerTask} onSaved={handleSaved} /> : null}
                  {drawerTab === 'dataset' ? <ImportPanel taskId={drawerTask.id} onImported={onTasksChanged} /> : null}
                  {drawerTab === 'distribution' ? (
                    drawerTask.distribution === 'assigned' ? (
                      <AssigneePanel taskId={drawerTask.id} />
                    ) : (
                      <div className="lh-muted lh-text-13">
                        当前分发策略为「{distributionLabel(drawerTask.distribution ?? 'first_come')}」。
                        {drawerTask.distribution === 'quota'
                          ? `每人配额:${drawerTask.quotaPerUser || 0} 题(在「基础信息」里修改)。`
                          : '改为「指派」后可在此管理指派的标注员。'}
                      </div>
                    )
                  ) : null}
                </>
              ) : null}
            </div>
          </aside>
        </>
      ) : null}
    </section>
  )
}
