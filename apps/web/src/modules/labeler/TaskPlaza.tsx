import { useMemo, useState } from 'react'
import { Input, Select } from '@douyinfe/semi-ui'
import type { Task } from '../../shared/api/client'
import EmptyState from '../../shared/components/EmptyState'
import StatusBadge from '../../shared/components/StatusBadge'

type TaskPlazaProps = {
  tasks: Task[]
  loading: boolean
  onEnter: (task: Task) => void
}

const STATUS_OPTIONS = [
  { label: '全部状态', value: 'all' },
  { label: '已发布', value: 'published' },
  { label: '已结束', value: 'finished' },
  { label: '已暂停', value: 'paused' },
]

// 任务广场:标注员浏览可领取任务。搜索按标题客户端过滤,状态下拉过滤,任务以卡片展示。
// 点击卡片进入该任务作答页(由父组件 onEnter 处理领取/打开)。
export default function TaskPlaza({ tasks, loading, onEnter }: TaskPlazaProps) {
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')

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
                aria-label={`领取题目 ${task.title}`}
                className="lz-card"
                disabled={loading}
                onClick={() => onEnter(task)}
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
    </div>
  )
}
