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
import AssigneePanel from './AssigneePanel'

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
  const [overlapCount, setOverlapCount] = useState('1')
  const [overlapCoveragePct, setOverlapCoveragePct] = useState('0')
  const [leaseTimeoutMinutes, setLeaseTimeoutMinutes] = useState('30')
  const [reviewSamplingPct, setReviewSamplingPct] = useState('100')
  const [dailySubmissionLimit, setDailySubmissionLimit] = useState('0')
  const [humanReviewEnabled, setHumanReviewEnabled] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  // 选中的任务变化时,把表单回填成该任务的真实值;新建模式恢复默认。
  // 受控表单需在 task prop 变化时整体回填,这里的同步 setState 是预期行为。
  useEffect(() => {
    syncFormFromTask(task, {
      setTitle, setDescription, setRichHtml, setTags, setRewardAmount,
      setRewardUnit, setDeadline, setQuotaPerUser, setDistribution, setTagDraft, setError,
      setOverlapCount, setOverlapCoveragePct, setLeaseTimeoutMinutes, setReviewSamplingPct, setDailySubmissionLimit,
      setHumanReviewEnabled,
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
    const overlap = Number(overlapCount)
    const overlapCoverage = Number(overlapCoveragePct)
    const leaseTimeout = Number(leaseTimeoutMinutes)
    const reviewSampling = Number(reviewSamplingPct)
    const dailyLimit = Number(dailySubmissionLimit)
    const amount = Number(rewardAmount)
    const input = {
      title: title.trim(),
      description: description.trim() || null,
      richDescription: richDescriptionPayload(richHtml),
      tags,
      rewardConfig: rewardConfigPayload(Number.isFinite(amount) ? amount : 0, rewardUnit),
      distribution,
      quotaPerUser: Number.isFinite(quota) && quota > 0 ? quota : 0,
      overlapCount: Number.isFinite(overlap) && overlap > 0 ? overlap : 1,
      overlapCoveragePct: Number.isFinite(overlapCoverage) && overlapCoverage >= 0 ? overlapCoverage : 0,
      leaseTimeoutMinutes: Number.isFinite(leaseTimeout) && leaseTimeout > 0 ? leaseTimeout : 30,
      reviewSamplingPct: Number.isFinite(reviewSampling) && reviewSampling >= 0 ? reviewSampling : 100,
      dailySubmissionLimitPerLabeler: Number.isFinite(dailyLimit) && dailyLimit >= 0 ? dailyLimit : 0,
      humanReviewEnabled,
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

      <section className="taskform__section">
        <h3 className="taskform__section-title">分发设置</h3>
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
        {isEdit && task && task.distribution === 'assigned' ? (
          <div className="taskform__field">
            <label className="taskform__label">指派标注员</label>
            <AssigneePanel taskId={task.id} />
          </div>
        ) : distribution === 'assigned' ? (
          <div className="lh-muted lh-text-13" style={{ marginTop: 8, marginBottom: 8 }}>
            {isEdit ? '保存后即可在此指派标注员。' : '创建并保存任务后,可在此指派标注员。'}
          </div>
        ) : null}
        <div className="taskform__row">
          <NumberField id="task-quota" label="每人配额" ariaLabel="task_quota_per_user" value={quotaPerUser} onChange={setQuotaPerUser} placeholder="0 表示不限" min={0} />
          <NumberField id="task-daily-limit" label="每日提交上限" ariaLabel="task_daily_submission_limit" value={dailySubmissionLimit} onChange={setDailySubmissionLimit} placeholder="0 表示不限" min={0} />
        </div>
        <NumberField id="task-lease-timeout" label="领题租约 (分钟)" ariaLabel="task_lease_timeout_minutes" value={leaseTimeoutMinutes} onChange={setLeaseTimeoutMinutes} min={1} max={10080} />
      </section>

      <section className="taskform__section">
        <h3 className="taskform__section-title">质量设置</h3>
        <div className="taskform__row">
          <NumberField id="task-overlap-count" label="重复标注人数" ariaLabel="task_overlap_count" value={overlapCount} onChange={setOverlapCount} min={1} max={10} />
          <NumberField id="task-overlap-coverage" label="重复标注覆盖率 (%)" ariaLabel="task_overlap_coverage_pct" value={overlapCoveragePct} onChange={setOverlapCoveragePct} min={0} max={100} />
        </div>
        <NumberField id="task-review-sampling" label="人工审核抽检率 (%)" ariaLabel="task_review_sampling_pct" value={reviewSamplingPct} onChange={setReviewSamplingPct} min={0} max={100} />
        <label className="taskform__check">
          <input aria-label="task_human_review_enabled" type="checkbox" checked={humanReviewEnabled} onChange={(event) => setHumanReviewEnabled(event.target.checked)} />
          启用人工审核
        </label>
      </section>

      <section className="taskform__section">
        <h3 className="taskform__section-title">发布设置</h3>
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
        <div className="taskform__field">
          <label className="taskform__label" htmlFor="task-deadline">截止时间</label>
          <input id="task-deadline" aria-label="task_deadline" type="datetime-local" className="taskform__input" value={deadline} onChange={(event) => setDeadline(event.target.value)} />
        </div>
      </section>

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

type NumberFieldProps = {
  id: string
  label: string
  ariaLabel: string
  value: string
  onChange: Dispatch<SetStateAction<string>>
  placeholder?: string
  min: number
  max?: number
}

function NumberField({ id, label, ariaLabel, value, onChange, placeholder, min, max }: NumberFieldProps) {
  return (
    <div className="taskform__field">
      <label className="taskform__label" htmlFor={id}>{label}</label>
      <input id={id} aria-label={ariaLabel} type="number" min={min} max={max} className="taskform__input" value={value} onChange={(event) => onChange(event.target.value)} placeholder={placeholder} />
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
  setOverlapCount: Dispatch<SetStateAction<string>>
  setOverlapCoveragePct: Dispatch<SetStateAction<string>>
  setLeaseTimeoutMinutes: Dispatch<SetStateAction<string>>
  setReviewSamplingPct: Dispatch<SetStateAction<string>>
  setDailySubmissionLimit: Dispatch<SetStateAction<string>>
  setHumanReviewEnabled: Dispatch<SetStateAction<boolean>>
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
    setters.setOverlapCount(String(task.overlapCount ?? 1))
    setters.setOverlapCoveragePct(String(task.overlapCoveragePct ?? 0))
    setters.setLeaseTimeoutMinutes(String(task.leaseTimeoutMinutes ?? 30))
    setters.setReviewSamplingPct(String(task.reviewSamplingPct ?? 100))
    setters.setDailySubmissionLimit(String(task.dailySubmissionLimitPerLabeler ?? 0))
    setters.setHumanReviewEnabled(task.humanReviewEnabled ?? true)
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
    setters.setOverlapCount('1')
    setters.setOverlapCoveragePct('0')
    setters.setLeaseTimeoutMinutes('30')
    setters.setReviewSamplingPct('100')
    setters.setDailySubmissionLimit('0')
    setters.setHumanReviewEnabled(true)
  }
  setters.setTagDraft('')
  setters.setError('')
}
