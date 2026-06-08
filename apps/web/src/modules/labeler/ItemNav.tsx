import type { LabelerTaskItem, LabelerTaskItems, MyTask } from '../../shared/api/client'
import { normalizeStatus } from '../../shared/components/status'

// 题目导航里每种状态对应的中文标签 + 颜色 dot 类(复用 workbench.css 的 .wb-side__status--*)。
// available=待标(idle) / claimed=进行中(active) / draft=草稿 / submitted+审核中=已提交 / approved=通过 / rejected=打回 / revising=待修改 / taken=他人。
const STATUS_VIEW: Record<string, { label: string, dot: string }> = {
  available: { label: '待标', dot: 'idle' },
  claimed: { label: '进行中', dot: 'active' },
  draft: { label: '草稿', dot: 'draft' },
  submitted: { label: '已提交', dot: 'done' },
  ai_reviewing: { label: '已提交', dot: 'done' },
  human_reviewing: { label: '已提交', dot: 'done' },
  approved: { label: '通过', dot: 'done' },
  rejected: { label: '打回', dot: 'rejected' },
  revising: { label: '待修改', dot: 'rejected' },
  taken: { label: '他人', dot: 'idle' },
}

function statusView(status: string) {
  return STATUS_VIEW[normalizeStatus(status)] ?? { label: status || '待标', dot: 'idle' }
}

// 一题是否"已完成"(有过提交动作):提交/审核中/通过/打回/待修改 均计入进度。
const DONE_STATUSES = ['submitted', 'ai_reviewing', 'human_reviewing', 'approved', 'rejected', 'revising']

function isDoneItem(item: LabelerTaskItem) {
  return DONE_STATUSES.includes(normalizeStatus(item.status))
}

type ItemNavProps = {
  nav: LabelerTaskItems | null
  loading: boolean
  activeItemId: number | undefined
  onSelect: (item: LabelerTaskItem) => void
  onBack: () => void
  myTasks?: MyTask[]
  activeTaskId?: number
  onSwitchTask?: (taskId: number) => void
}

export default function ItemNav({ nav, loading, activeItemId, onSelect, onBack, myTasks = [], activeTaskId, onSwitchTask }: ItemNavProps) {
  const items = nav?.items ?? []
  const total = nav?.total ?? items.length
  const done = items.filter(isDoneItem).length
  const percent = total > 0 ? Math.round((done / total) * 100) : 0
  // 仅当我领取了多个大任务、且当前任务在其中时,才显示大任务切换器(避免受控 select 值不在选项内告警)。
  const canSwitchTask = !!onSwitchTask && myTasks.length > 1 && myTasks.some((myTask) => myTask.task.id === activeTaskId)

  return (
    <aside className="wb-side">
      <button type="button" className="lh-btn lh-btn--ghost lh-btn--sm wb-side__back" onClick={onBack}>
        ← 返回任务广场
      </button>
      {canSwitchTask ? (
        <label className="wb-side__task">
          <span className="wb-side__task-label">当前任务</span>
          <select
            className="wb-side__task-select"
            aria-label="切换标注任务"
            value={activeTaskId}
            onChange={(event) => onSwitchTask?.(Number(event.target.value))}
          >
            {myTasks.map((myTask) => (
              <option key={myTask.task.id} value={myTask.task.id}>
                {(myTask.task.title || `任务 #${myTask.task.id}`) + (myTask.myInProgress > 0 ? `（进行中 ${myTask.myInProgress}）` : '')}
              </option>
            ))}
          </select>
        </label>
      ) : null}
      <div className="wb-side__title">题目导航</div>
      <div className="wb-side__meta">{done} / {total} · 进度 {percent}%</div>
      <div className="wb-side__progress" role="progressbar" aria-valuenow={percent} aria-valuemin={0} aria-valuemax={100} aria-label="标注进度">
        <div className="wb-side__progress-fill" style={{ width: `${percent}%` }} />
      </div>

      {loading && items.length === 0 ? (
        <div className="wb-side__more">加载题目中...</div>
      ) : null}

      {items.map((item, index) => {
        const view = statusView(item.status)
        const active = item.itemId === activeItemId
        const label = item.externalId || `题目 ${item.itemId}`
        // 只能打开自己的题(已领/有提交);待标/他人的题不可点 —— 题目按顺序领取,无法从导航定向领某一题,
        // 领新题走页脚「下一题 / 跳过」。这样点导航只在自己的题间跳转,不会误触发领取。
        const openable = item.mine || item.submissionId != null
        return (
          <button
            key={item.itemId}
            type="button"
            disabled={!openable}
            title={openable ? undefined : '该题尚未领取,无法直接打开;用下方「下一题 / 跳过」领取下一题'}
            aria-label={`第 ${index + 1} 题 ${label} ${view.label}`}
            className={'wb-side__item' + (active ? ' wb-side__item--active' : '')}
            onClick={() => onSelect(item)}
          >
            <span className="wb-side__no">#{index + 1}</span>
            <span className="wb-side__name">{label}</span>
            <span className={`wb-side__status wb-side__status--${view.dot}`}>{view.label}</span>
          </button>
        )
      })}

      {!loading && items.length === 0 ? (
        <div className="wb-side__more">暂无题目</div>
      ) : null}
    </aside>
  )
}
