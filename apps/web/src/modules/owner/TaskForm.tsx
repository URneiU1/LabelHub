import { useEffect, useState, type Dispatch, type SetStateAction } from 'react'
import { Toast } from '@douyinfe/semi-ui'
import {
  createTask,
  updateTask,
  type Task,
  type TaskDistribution,
} from '../../shared/api/client'
import {
  DISTRIBUTION_LABELS,
  datetimeLocalToISO,
  isoToDatetimeLocal,
  parseRewardConfig,
  parseRichDescriptionHtml,
  parseTags,
  richDescriptionPayload,
  rewardConfigPayload,
} from './taskManageHelpers'

interface TaskFormProps {
  // 编辑模式传入既有任务;新建模式传 null。
  task: Task | null
  onSaved: (task: Task, created: boolean) => void
}

const DISTRIBUTIONS: TaskDistribution[] = ['first_come', 'assigned', 'quota']

export default function TaskForm({ task, onSaved }: TaskFormProps) {
  const isEdit = Boolean(task)
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [richHtml, setRichHtml] = useState('')
  const [tags, setTags] = useState<string[]>([])
  const [tagDraft, setTagDraft] = useState('')
  const [rewardAmount, setRewardAmount] = useState('')
  const [rewardUnit, setRewardUnit] = useState('元/条')
  const [deadline, setDeadline] = useState('')
  const [quotaPerUser, setQuotaPerUser] = useState('')
  const [distribution, setDistribution] = useState<TaskDistribution>('first_come')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  // 选中的任务变化时,把表单回填成该任务的真实值;新建模式恢复默认。
  // 受控表单需在 task prop 变化时整体回填,这里的同步 setState 是预期行为。
  useEffect(() => {
    syncFormFromTask(task, {
      setTitle, setDescription, setRichHtml, setTags, setRewardAmount,
      setRewardUnit, setDeadline, setQuotaPerUser, setDistribution, setTagDraft, setError,
    })
  }, [task])

  function addTag() {
    const value = tagDraft.trim()
    if (!value || tags.includes(value)) {
      setTagDraft('')
      return
    }
    setTags((current) => [...current, value])
    setTagDraft('')
  }

  function removeTag(tag: string) {
    setTags((current) => current.filter((item) => item !== tag))
  }

  async function submit() {
    if (saving) return
    if (!title.trim()) {
      setError('任务标题不能为空')
      return
    }
    setSaving(true)
    setError('')
    const quota = Number(quotaPerUser)
    const amount = Number(rewardAmount)
    const input = {
      title: title.trim(),
      description: description.trim() || null,
      richDescription: richDescriptionPayload(richHtml),
      tags,
      rewardConfig: rewardConfigPayload(Number.isFinite(amount) ? amount : 0, rewardUnit),
      distribution,
      quotaPerUser: Number.isFinite(quota) && quota > 0 ? quota : 0,
      deadline: datetimeLocalToISO(deadline),
    }
    try {
      if (task) {
        const saved = await updateTask(task.id, input)
        onSaved(saved, false)
        Toast.success('任务已更新')
      } else {
        const saved = await createTask(input)
        onSaved(saved, true)
        Toast.success('任务已创建为草稿')
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : '保存任务失败'
      setError(message)
      Toast.error(message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="lh-vflex" style={{ gap: 16 }}>
      {error ? <div role="alert" className="taskform__error">{error}</div> : null}

      <div className="taskform__field">
        <label className="taskform__label" htmlFor="task-title">任务标题<span className="req">*</span></label>
        <input id="task-title" aria-label="task_title" className="taskform__input" value={title} onChange={(event) => setTitle(event.target.value)} placeholder="例如:商品标题清洗 v3" />
      </div>

      <div className="taskform__field">
        <label className="taskform__label" htmlFor="task-description">任务简介</label>
        <textarea id="task-description" aria-label="task_description" className="taskform__textarea" value={description} onChange={(event) => setDescription(event.target.value)} placeholder="一句话说明这个任务在标什么。" />
      </div>

      <div className="taskform__field">
        <label className="taskform__label" htmlFor="task-rich">富文本说明</label>
        <textarea id="task-rich" aria-label="task_rich_description" className="taskform__textarea" value={richHtml} onChange={(event) => setRichHtml(event.target.value)} placeholder="支持 HTML 片段,会保存到 richDescription。" />
      </div>

      <div className="taskform__field">
        <label className="taskform__label">标签</label>
        <div className="taskform__tags">
          {tags.map((tag) => (
            <span key={tag} className="lh-tag lh-tag--primary taskform__tag">
              {tag}
              <button type="button" aria-label={`移除标签 ${tag}`} className="taskform__tag-remove" onClick={() => removeTag(tag)}>×</button>
            </span>
          ))}
        </div>
        <div className="lh-hflex" style={{ marginTop: 6 }}>
          <input
            aria-label="task_tag_input"
            className="taskform__input"
            value={tagDraft}
            onChange={(event) => setTagDraft(event.target.value)}
            onKeyDown={(event) => { if (event.key === 'Enter') { event.preventDefault(); addTag() } }}
            placeholder="输入标签后回车"
          />
          <button type="button" aria-label="添加标签" className="lh-btn" onClick={addTag}>添加</button>
        </div>
      </div>

      <div className="taskform__row">
        <div className="taskform__field">
          <label className="taskform__label" htmlFor="task-reward-amount">奖励金额</label>
          <input id="task-reward-amount" aria-label="task_reward_amount" type="number" min={0} step="0.01" className="taskform__input" value={rewardAmount} onChange={(event) => setRewardAmount(event.target.value)} placeholder="0.30" />
        </div>
        <div className="taskform__field">
          <label className="taskform__label" htmlFor="task-reward-unit">奖励单位</label>
          <input id="task-reward-unit" aria-label="task_reward_unit" className="taskform__input" value={rewardUnit} onChange={(event) => setRewardUnit(event.target.value)} placeholder="元/条" />
        </div>
      </div>

      <div className="taskform__row">
        <div className="taskform__field">
          <label className="taskform__label" htmlFor="task-deadline">截止时间</label>
          <input id="task-deadline" aria-label="task_deadline" type="datetime-local" className="taskform__input" value={deadline} onChange={(event) => setDeadline(event.target.value)} />
        </div>
        <div className="taskform__field">
          <label className="taskform__label" htmlFor="task-quota">每人配额</label>
          <input id="task-quota" aria-label="task_quota_per_user" type="number" min={0} className="taskform__input" value={quotaPerUser} onChange={(event) => setQuotaPerUser(event.target.value)} placeholder="0 表示不限" />
        </div>
      </div>

      <div className="taskform__field">
        <label className="taskform__label">分发策略</label>
        <div className="taskform__strategy">
          {DISTRIBUTIONS.map((value) => (
            <button
              key={value}
              type="button"
              aria-label={`分发策略 ${DISTRIBUTION_LABELS[value]}`}
              aria-pressed={distribution === value}
              className={'lh-btn' + (distribution === value ? ' lh-btn--primary' : '')}
              onClick={() => setDistribution(value)}
            >
              {DISTRIBUTION_LABELS[value]}
            </button>
          ))}
        </div>
      </div>

      <div className="lh-hflex" style={{ justifyContent: 'flex-end', gap: 10 }}>
        <button
          type="button"
          aria-label={isEdit ? '保存任务' : '创建任务'}
          disabled={saving}
          className="lh-btn lh-btn--primary lh-btn--lg"
          onClick={() => void submit()}
        >
          {saving ? '保存中…' : isEdit ? '保存修改' : '创建草稿'}
        </button>
      </div>
    </div>
  )
}

type FormSetters = {
  setTitle: Dispatch<SetStateAction<string>>
  setDescription: Dispatch<SetStateAction<string>>
  setRichHtml: Dispatch<SetStateAction<string>>
  setTags: Dispatch<SetStateAction<string[]>>
  setRewardAmount: Dispatch<SetStateAction<string>>
  setRewardUnit: Dispatch<SetStateAction<string>>
  setDeadline: Dispatch<SetStateAction<string>>
  setQuotaPerUser: Dispatch<SetStateAction<string>>
  setDistribution: Dispatch<SetStateAction<TaskDistribution>>
  setTagDraft: Dispatch<SetStateAction<string>>
  setError: Dispatch<SetStateAction<string>>
}

// 把回填逻辑抽成普通函数:既复用,又让 effect 内不出现直接的 setState 调用。
function syncFormFromTask(task: Task | null, setters: FormSetters) {
  if (task) {
    const reward = parseRewardConfig(task.rewardConfig)
    setters.setTitle(task.title)
    setters.setDescription(task.description ?? '')
    setters.setRichHtml(parseRichDescriptionHtml(task.richDescription))
    setters.setTags(parseTags(task.tags))
    setters.setRewardAmount(reward.amount > 0 ? String(reward.amount) : '')
    setters.setRewardUnit(reward.unit)
    setters.setDeadline(isoToDatetimeLocal(task.deadline))
    setters.setQuotaPerUser(task.quotaPerUser ? String(task.quotaPerUser) : '')
    setters.setDistribution((task.distribution as TaskDistribution) || 'first_come')
  } else {
    setters.setTitle('')
    setters.setDescription('')
    setters.setRichHtml('')
    setters.setTags([])
    setters.setRewardAmount('')
    setters.setRewardUnit('元/条')
    setters.setDeadline('')
    setters.setQuotaPerUser('')
    setters.setDistribution('first_come')
  }
  setters.setTagDraft('')
  setters.setError('')
}
