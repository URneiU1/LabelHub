import { useMemo, useState } from 'react'
import { Input, Select } from '@douyinfe/semi-ui'
import type { MyTask, Task } from '../../shared/api/client'
import EmptyState from '../../shared/components/EmptyState'
import { Markdown } from '../../shared/markdown'
import StatusBadge from '../../shared/components/StatusBadge'

type TaskPlazaProps = {
  tasks: Task[]
  myTasks: MyTask[]
  loading: boolean
  onEnter: (task: Task) => void
  onContinue: (myTask: MyTask) => void
}

const STATUS_OPTIONS = [
  { label: '全部状态', value: 'all' },
  { label: '已发布', value: 'published' },
  { label: '已结束', value: 'finished' },
  { label: '已暂停', value: 'paused' },
]

const STATUS_TEXT: Record<string, string> = {
  published: '已发布',
  finished: '已结束',
  ended: '已结束',
  paused: '已暂停',
  draft: '草稿',
}

// 任务广场:标注员浏览可领取任务。搜索按标题客户端过滤,状态下拉过滤,任务以卡片展示。
// 点击卡片先看任务详情(标题/描述/进度/状态/基线说明),确认「符合要求」后再调 onEnter 领取,
// 而不是点卡片立即领取(对应流程图「是否符合?→是→领取」)。
export default function TaskPlaza({ tasks, myTasks, loading, onEnter, onContinue }: TaskPlazaProps) {
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')
  // detailTask: 当前在详情弹层里查看的任务;null 表示弹层关闭。本地 state,不改 Plaza。
  const [detailTask, setDetailTask] = useState<Task | null>(null)

  const visible = useMemo(() => {
    const keyword = search.trim().toLowerCase()
    return tasks.filter((task) => {
      if (statusFilter !== 'all' && task.status !== statusFilter) {
        return false
      }
      if (keyword && !task.title.toLowerCase().includes(keyword)) {
        return false
      }
      return true
    })
  }, [tasks, search, statusFilter])

  return (
    <div className="lz-plaza">
      {myTasks.length > 0 ? (
        <section className="lz-claimed" aria-label="已领取的任务">
          <div className="lz-claimed__title">已领取的任务</div>
          <div className="lz-cards">
            {myTasks.map((myTask) => {
              const claimedTask = myTask.task
              const title = claimedTask.title || `任务 #${claimedTask.id}`
              const inProgress = myTask.myInProgress > 0
              return (
                <button
                  key={claimedTask.id}
                  type="button"
                  aria-label={`继续标注 ${title}`}
                  className="lz-card lz-card--claimed"
                  onClick={() => onContinue(myTask)}
                >
                  <div className="lz-card__head">
                    <span className="lz-card__title">{title}</span>
                    {inProgress ? <span className="lz-chip lz-chip--active">进行中 {myTask.myInProgress}</span> : null}
                  </div>
                  <div className="lz-card__meta">任务 #{claimedTask.id} · 我的提交 {myTask.myTotal} 条</div>
                  <div className="lz-card__cta">继续标注 →</div>
                </button>
              )
            })}
          </div>
        </section>
      ) : null}

      <div className="tasks-filters">
        <Input
          aria-label="搜索任务"
          placeholder="搜索任务名 / 标题"
          showClear
          value={search}
          onChange={(value) => setSearch(value)}
        />
        <Select
          aria-label="按状态筛选"
          value={statusFilter}
          onChange={(value) => setStatusFilter(String(value))}
          optionList={STATUS_OPTIONS}
          style={{ width: 160 }}
        />
      </div>

      {visible.length === 0 ? (
        <EmptyState
          title={loading ? '加载任务中' : tasks.length === 0 ? '暂无可领取任务' : '没有匹配的任务'}
          body={loading ? '正在拉取任务广场。' : tasks.length === 0 ? '当前没有可领取的题目,稍后刷新任务广场。' : '试试调整搜索词或状态筛选。'}
          variant="queue"
        />
      ) : (
        <div className="lz-cards">
          {visible.map((task) => {
            const percent = task.totalItems > 0 ? Math.round((task.finishedItems / task.totalItems) * 100) : 0
            return (
              <button
                key={task.id}
                type="button"
                aria-label={`查看任务详情 ${task.title}`}
                className="lz-card"
                disabled={loading}
                onClick={() => setDetailTask(task)}
              >
                <div className="lz-card__head">
                  <span className="lz-card__title">{task.title}</span>
                  <StatusBadge status={task.status} />
                </div>
                {task.description ? <div className="lz-card__desc">{task.description}</div> : null}
                <div className="lz-card__meta">任务 #{task.id} · {task.finishedItems} / {task.totalItems} 题</div>
                <div className="tasks-progress">
                  <div className="tasks-progress__bar">
                    <div className="tasks-progress__fill" style={{ width: `${percent}%` }} />
                  </div>
                </div>
              </button>
            )
          })}
        </div>
      )}

      {detailTask ? (
        <TaskDetailModal
          task={detailTask}
          onClose={() => setDetailTask(null)}
          onConfirm={() => {
            const task = detailTask
            setDetailTask(null)
            onEnter(task)
          }}
        />
      ) : null}
    </div>
  )
}

type TaskDetailModalProps = {
  task: Task
  onClose: () => void
  onConfirm: () => void
}

// 任务详情弹层:点卡片→看详情→确认领取。展示标题/描述/进度/状态/审核与基线说明,
// 提供「符合要求 · 领取任务」(确认领取)与「返回列表」(关闭)两个显式出口。
function TaskDetailModal({ task, onClose, onConfirm }: TaskDetailModalProps) {
  const percent = task.totalItems > 0 ? Math.round((task.finishedItems / task.totalItems) * 100) : 0
  const claimable = task.status === 'published'
  return (
    <>
      <div className="lz-detail__scrim" onClick={onClose} aria-hidden="true" />
      <div className="lz-detail" role="dialog" aria-modal="true" aria-label={`任务详情 ${task.title}`}>
        <div className="lz-detail__head">
          <div className="lz-detail__title">{task.title}</div>
          <StatusBadge status={task.status} label={STATUS_TEXT[task.status]} />
        </div>
        <div className="lz-detail__meta">任务 #{task.id} · {STATUS_TEXT[task.status] ?? task.status}</div>

        <div className="lz-detail__section">
          <div className="lz-detail__label">任务说明</div>
          <div className="lz-detail__body">{task.description || '该任务暂无文字说明。'}</div>
        </div>

        <div className="lz-detail__section">
          <div className="lz-detail__label">进度</div>
          <div className="lz-detail__body">{task.finishedItems} / {task.totalItems} 题（{percent}%）</div>
          <div className="tasks-progress" style={{ marginTop: 6 }}>
            <div className="tasks-progress__bar">
              <div className="tasks-progress__fill" style={{ width: `${percent}%` }} />
            </div>
          </div>
        </div>

        {task.baselineDescription ? (
          <div className="lz-detail__section">
            <div className="lz-detail__label">验收基线 / 审核规则</div>
            <div className="lz-detail__body"><Markdown text={task.baselineDescription} /></div>
          </div>
        ) : null}

        <div className="lz-detail__actions">
          <button type="button" className="lh-btn" aria-label="返回列表" onClick={onClose}>返回列表</button>
          <button
            type="button"
            className="lh-btn lh-btn--primary"
            aria-label={`符合要求 领取任务 ${task.title}`}
            disabled={!claimable}
            onClick={onConfirm}
          >
            {claimable ? '符合要求 · 领取任务' : '当前不可领取'}
          </button>
        </div>
      </div>
    </>
  )
}
