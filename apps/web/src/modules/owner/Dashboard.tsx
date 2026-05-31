import { lazy, Suspense, useCallback, useEffect, useRef, useState, type MutableRefObject } from 'react'
import { Button, Modal, Toast } from '@douyinfe/semi-ui'
import { apiDelete, apiGet, apiPost, apiPostRawJSON, type Task } from '../../shared/api/client'
import EmptyState from '../../shared/components/EmptyState'
import LoadingBlock from '../../shared/components/LoadingBlock'
import StatusBadge from '../../shared/components/StatusBadge'
import ExportPanel from './ExportPanel'
import ImportPanel from './ImportPanel'
import ReviewResultsPanel from './ReviewResultsPanel'
import TaskManagePanel from './TaskManagePanel'
import { useOwnerSection, useOwnerSubView } from '../../shared/state/ownerSection'
// StatsBoard 依赖 VChart(体积大),懒加载切出独立 chunk,选中任务时才拉。
const StatsBoard = lazy(() => import('./StatsBoard'))

type TaskListResponse = Task[]
type ExportResponse = {
  task: Task
  rows: Array<Record<string, unknown>>
}
type AIPromptConfig = {
  id: number
  version: number
  promptTemplate: string
  dimensions: string
  passThreshold: number
  uncertainMin: number
  model: string
}
type AIPromptsResponse = {
  prompts: AIPromptConfig[]
  activePromptId: number | null
  aiReviewEnabled: boolean
}
type AIReviewSettingsResponse = {
  aiReviewEnabled: boolean
  activePromptId: number | null
}
type AIDryRunResult = {
  provider: string
  dryRunId?: number
  matchedExpected?: boolean
  result: {
    verdict: string
    overall_score: number
    dimensions: Array<{ name: string, score: number, reason: string }>
    reason: string
    model?: string
  }
}
type AIDryRunHistoryResult = AIDryRunResult['result'] & { provider?: string }
type QueuedDryRunResponse = {
  dryRunId: number
  status: 'queued' | 'running'
}
type GoldenSample = {
  id: number
  taskId: number
  aiPromptId: number | null
  payload: unknown
  expectedAnswer: unknown
  expectedVerdict: string
  notes: string | null
  createdAt: string
}
type GoldenSamplesResponse = {
  samples: GoldenSample[]
}
type GoldenSampleCreateResponse = {
  sample: GoldenSample
}
type GoldenSampleBatchDryRunResult = {
  goldenSampleId: number
  status: 'queued' | 'failed'
  dryRunId?: number
  error?: string
}
type GoldenSampleBatchDryRunResponse = {
  results: GoldenSampleBatchDryRunResult[]
  summary: {
    total: number
    queued: number
    failed: number
  }
}
type AIDryRunHistoryItem = {
  id: number
  taskId: number
  aiPromptId: number
  goldenSampleId: number | null
  promptVersion: number
  expectedVerdict: string | null
  actualVerdict: string | null
  matchedExpected: boolean | null
  status: string
  result: AIDryRunHistoryResult | null
  errorMsg: string | null
  createdAt: string
  finishedAt: string | null
}
type AIDryRunGuardStatus = {
  windowMinutes: number
  quotaMaxRuns: number
  circuitMaxFailures: number
  recentRuns: number
  recentFailures: number
  quotaRemaining: number | null
  circuitOpen: boolean
  state: string
}
type AIDryRunHistoryResponse = {
  dryRuns: AIDryRunHistoryItem[]
  guard?: AIDryRunGuardStatus
}
type GoldenSampleRunRow = {
  sampleId: number
  expectedVerdict: string
  status: 'running' | 'succeeded' | 'failed'
  actualVerdict?: string
  matchedExpected?: boolean
  score?: number
  provider?: string
  model?: string
  reason?: string
  dryRunId?: number
  error?: string
}
type DimensionRow = {
  id: string
  name: string
  description: string
  weight: string
}

export default function OwnerDashboard() {
  const [tasks, setTasks] = useState<Task[]>([])
  const [selected, setSelected] = useState<Task | null>(null)
  // 任务详情区的当前视图。把原来一长条拆成左栏可切换的几节,主区一次只显示一节。
  // 默认 'ai'(AI 预审是本项目的核心能力,也保证选中任务后直接看到预审配置)。
  // 分节由全局「工作区」侧栏(AppLayout)驱动,经共享 store 桥接。
  const detailSection = useOwnerSection()
  const subView = useOwnerSubView()
  const [exportRows, setExportRows] = useState<Array<Record<string, unknown>>>([])
  const [prompts, setPrompts] = useState<AIPromptConfig[]>([])
  const [activePromptId, setActivePromptId] = useState<number | null>(null)
  const [aiReviewEnabled, setAIReviewEnabled] = useState(false)
  const [baselineDraft, setBaselineDraft] = useState('')
  const [savingBaseline, setSavingBaseline] = useState(false)
  const [promptTemplate, setPromptTemplate] = useState(defaultPromptTemplate)
  const [dimensionRows, setDimensionRows] = useState<DimensionRow[]>(defaultDimensionRows())
  const [passThreshold, setPassThreshold] = useState(defaultPassThreshold)
  const [uncertainMin, setUncertainMin] = useState(defaultUncertainMin)
  const [model, setModel] = useState(defaultModel)
  const [samplePayload, setSamplePayload] = useState('{"prompt":"示例题目"}')
  const [sampleAnswer, setSampleAnswer] = useState('{"summary":"示例答案"}')
  const [dryRun, setDryRun] = useState<AIDryRunResult | null>(null)
  const [goldenSamples, setGoldenSamples] = useState<GoldenSample[]>([])
  const [goldenPayload, setGoldenPayload] = useState(defaultGoldenPayload)
  const [goldenExpectedAnswer, setGoldenExpectedAnswer] = useState(defaultGoldenExpectedAnswer)
  const [goldenExpectedVerdict, setGoldenExpectedVerdict] = useState(defaultGoldenExpectedVerdict)
  const [goldenNotes, setGoldenNotes] = useState('')
  const [goldenPromptChoice, setGoldenPromptChoice] = useState('active')
  const [goldenSampleError, setGoldenSampleError] = useState('')
  const [goldenSampleLoading, setGoldenSampleLoading] = useState(false)
  const [goldenSampleLoadFailed, setGoldenSampleLoadFailed] = useState(false)
  const [creatingGoldenSample, setCreatingGoldenSample] = useState(false)
  const [deletingGoldenSampleId, setDeletingGoldenSampleId] = useState<number | null>(null)
  const [goldenRunRows, setGoldenRunRows] = useState<Record<number, GoldenSampleRunRow>>({})
  const [dryRunHistory, setDryRunHistory] = useState<AIDryRunHistoryItem[]>([])
  const [dryRunHistorySampleFilter, setDryRunHistorySampleFilter] = useState('all')
  const [dryRunHistoryLoading, setDryRunHistoryLoading] = useState(false)
  const [dryRunHistoryError, setDryRunHistoryError] = useState('')
  const [dryRunGuard, setDryRunGuard] = useState<AIDryRunGuardStatus | null>(null)
  const [promptError, setPromptError] = useState('')
  const [promptLoading, setPromptLoading] = useState(false)
  const [promptLoadFailed, setPromptLoadFailed] = useState(false)
  const [savingPrompt, setSavingPrompt] = useState(false)
  const [runningDryRun, setRunningDryRun] = useState(false)
  const [savingAIReviewSettings, setSavingAIReviewSettings] = useState(false)
  const goldenSamplePollTimers = useRef<Set<number>>(new Set())
  const promptLoadSeq = useRef(0)
  const goldenSampleLoadSeq = useRef(0)
  const selectedTaskIdRef = useRef<number | null>(null)
  const taskActionGeneration = useRef(0)
  const savePromptSeq = useRef(0)
  const aiReviewSettingsSeq = useRef(0)
  const dryRunSeq = useRef(0)
  const createGoldenSampleSeq = useRef(0)
  const deleteGoldenSampleSeq = useRef(0)
  const goldenSampleRunSeq = useRef(0)
  const dryRunHistorySeq = useRef(0)

  const resetGoldenSampleFormToDefaults = useCallback(() => {
    setGoldenPayload(defaultGoldenPayload)
    setGoldenExpectedAnswer(defaultGoldenExpectedAnswer)
    setGoldenExpectedVerdict(defaultGoldenExpectedVerdict)
    setGoldenNotes('')
    setGoldenPromptChoice('active')
  }, [])

  const loadTasks = useCallback(async () => {
    try {
      const data = await apiGet<TaskListResponse>('/tasks')
      const requestedTaskId = requestedNumberParam('taskId')
      const initialTask = data.find((item) => item.id === requestedTaskId) ?? data[0] ?? null
      setTasks(data)
      setSelected(initialTask)
      setBaselineDraft(initialTask?.baselineDescription ?? '')
      selectedTaskIdRef.current = initialTask?.id ?? null
      resetGoldenSampleFormToDefaults()
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '加载任务失败')
    }
  }, [resetGoldenSampleFormToDefaults])

  const fillPromptForm = useCallback((prompt: AIPromptConfig) => {
    setPromptTemplate(prompt.promptTemplate)
    setDimensionRows(dimensionRowsFromRaw(prompt.dimensions))
    setPassThreshold(String(prompt.passThreshold))
    setUncertainMin(String(prompt.uncertainMin))
    setModel(prompt.model)
  }, [])

  const resetPromptFormToDefaults = useCallback(() => {
    setPromptTemplate(defaultPromptTemplate)
    setDimensionRows(defaultDimensionRows())
    setPassThreshold(defaultPassThreshold)
    setUncertainMin(defaultUncertainMin)
    setModel(defaultModel)
  }, [])

  const loadPrompts = useCallback(async (taskId: number) => {
    const requestSeq = promptLoadSeq.current + 1
    promptLoadSeq.current = requestSeq
    taskActionGeneration.current += 1
    setPrompts([])
    setActivePromptId(null)
    setAIReviewEnabled(false)
    setDryRun(null)
    setPromptError('')
    setPromptLoading(true)
    setPromptLoadFailed(false)
    setSavingPrompt(false)
    setRunningDryRun(false)
    setSavingAIReviewSettings(false)
    try {
      const data = await apiGet<AIPromptsResponse>(`/tasks/${taskId}/ai-prompts`)
      if (promptLoadSeq.current !== requestSeq) return
      setPrompts(data.prompts)
      setActivePromptId(data.activePromptId)
      setAIReviewEnabled(data.aiReviewEnabled)
      setDryRun(null)
      setPromptError('')
      setPromptLoading(false)
      setPromptLoadFailed(false)
      const requestedPromptId = requestedNumberParam('taskId') === taskId ? requestedNumberParam('aiPromptId') : null
      const promptForForm = data.prompts.find((prompt) => prompt.id === requestedPromptId) ?? data.prompts[0]
      if (promptForForm) {
        fillPromptForm(promptForForm)
      } else {
        resetPromptFormToDefaults()
      }
    } catch (error) {
      if (promptLoadSeq.current !== requestSeq) return
      setPrompts([])
      setActivePromptId(null)
      setAIReviewEnabled(false)
      setDryRun(null)
      resetPromptFormToDefaults()
      setPromptLoading(false)
      setPromptLoadFailed(true)
      setPromptError(error instanceof Error ? error.message : '加载 AI Prompt 失败')
    }
  }, [fillPromptForm, resetPromptFormToDefaults])

  const loadGoldenSamples = useCallback(async (taskId: number) => {
    const requestSeq = goldenSampleLoadSeq.current + 1
    goldenSampleLoadSeq.current = requestSeq
    setGoldenSamples([])
    setGoldenRunRows({})
    setGoldenSampleError('')
    setGoldenSampleLoading(true)
    setGoldenSampleLoadFailed(false)
    setCreatingGoldenSample(false)
    setDeletingGoldenSampleId(null)
    try {
      const data = await apiGet<GoldenSamplesResponse>(`/tasks/${taskId}/golden-samples`)
      if (goldenSampleLoadSeq.current !== requestSeq || selectedTaskIdRef.current !== taskId) return
      setGoldenSamples(data.samples)
      setGoldenSampleError('')
      setGoldenSampleLoading(false)
      setGoldenSampleLoadFailed(false)
    } catch (error) {
      if (goldenSampleLoadSeq.current !== requestSeq || selectedTaskIdRef.current !== taskId) return
      setGoldenSamples([])
      setGoldenRunRows({})
      setGoldenSampleLoading(false)
      setGoldenSampleLoadFailed(true)
      setGoldenSampleError(error instanceof Error ? error.message : '加载 golden samples 失败')
    }
  }, [])

  const loadDryRunHistory = useCallback(async (taskId: number, sampleID: number | null) => {
    const requestSeq = dryRunHistorySeq.current + 1
    dryRunHistorySeq.current = requestSeq
    setDryRunHistoryLoading(true)
    setDryRunHistoryError('')
    const sampleQuery = sampleID ? `golden_sample_id=${sampleID}&` : ''
    try {
      const data = await apiGet<AIDryRunHistoryResponse>(`/tasks/${taskId}/ai-dry-runs?${sampleQuery}limit=10`)
      if (dryRunHistorySeq.current !== requestSeq || selectedTaskIdRef.current !== taskId) return
      setDryRunHistory(data.dryRuns)
      setDryRunGuard(data.guard ?? null)
      setDryRunHistoryLoading(false)
      setDryRunHistoryError('')
    } catch (error) {
      if (dryRunHistorySeq.current !== requestSeq || selectedTaskIdRef.current !== taskId) return
      setDryRunHistory([])
      setDryRunGuard(null)
      setDryRunHistoryLoading(false)
      setDryRunHistoryError(error instanceof Error ? error.message : '加载 dry-run history 失败')
    }
  }, [])

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void loadTasks()
  }, [loadTasks])

  useEffect(() => () => {
    for (const timer of goldenSamplePollTimers.current) {
      window.clearTimeout(timer)
    }
    goldenSamplePollTimers.current.clear()
  }, [selected?.id])

  useEffect(() => {
    if (selected) {
      selectedTaskIdRef.current = selected.id
      // eslint-disable-next-line react-hooks/set-state-in-effect
      void loadPrompts(selected.id)
      void loadGoldenSamples(selected.id)
    }
  }, [loadGoldenSamples, loadPrompts, selected])

  useEffect(() => {
    if (selected) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      void loadDryRunHistory(selected.id, dryRunHistorySampleID(dryRunHistorySampleFilter))
    }
  }, [dryRunHistorySampleFilter, loadDryRunHistory, selected])

  function beginTaskAction(taskId: number, seqRef: MutableRefObject<number>) {
    seqRef.current += 1
    return { taskId, generation: taskActionGeneration.current, seq: seqRef.current }
  }

  function isCurrentTaskAction(guard: { taskId: number, generation: number, seq: number }, seqRef: MutableRefObject<number>) {
    return selectedTaskIdRef.current === guard.taskId && taskActionGeneration.current === guard.generation && seqRef.current === guard.seq
  }

  // 任务创建/编辑/状态迁移后:把 saved task 合并进列表并保持/切换选中。
  // 不调 selectTask 的全量重置(那会清掉 AI/golden 面板状态);只在确实换了任务时才走 selectTask。
  function applyTaskSaved(saved: Task, created: boolean) {
    setTasks((current) => {
      if (current.some((task) => task.id === saved.id)) {
        return current.map((task) => task.id === saved.id ? saved : task)
      }
      return [saved, ...current]
    })
    if (created) {
      selectTask(saved)
      return
    }
    if (saved.id === selectedTaskIdRef.current) {
      setSelected(saved)
      setBaselineDraft(saved.baselineDescription ?? '')
    }
  }

  // 数据集导入/批量编辑后任务的 total_items 会变,重新拉列表但保持当前选中。
  async function reloadTasksKeepSelection() {
    try {
      const data = await apiGet<TaskListResponse>('/tasks')
      setTasks(data)
      const current = data.find((task) => task.id === selectedTaskIdRef.current)
      if (current) {
        setSelected(current)
      }
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '刷新任务失败')
    }
  }

  function selectTask(task: Task) {
    if (task.id === selectedTaskIdRef.current) return
    selectedTaskIdRef.current = task.id
    taskActionGeneration.current += 1
    setExportRows([])
    setPrompts([])
    setActivePromptId(null)
    setAIReviewEnabled(false)
    setDryRun(null)
    setPromptError('')
    setPromptLoading(true)
    setPromptLoadFailed(false)
    setSavingPrompt(false)
    setRunningDryRun(false)
    setSavingAIReviewSettings(false)
    setGoldenSamples([])
    setGoldenRunRows({})
    setGoldenSampleError('')
    setGoldenSampleLoading(true)
    setGoldenSampleLoadFailed(false)
    setCreatingGoldenSample(false)
    setDeletingGoldenSampleId(null)
    setDryRunHistory([])
    setDryRunGuard(null)
    setDryRunHistorySampleFilter('all')
    setDryRunHistoryError('')
    setDryRunHistoryLoading(true)
    resetGoldenSampleFormToDefaults()
    setBaselineDraft(task.baselineDescription ?? '')
    setSelected(task)
  }

  async function exportJSON(taskId: number) {
    try {
      const data = await apiGet<ExportResponse>(`/tasks/${taskId}/export/json`)
      setExportRows(data.rows)
      Toast.success(`导出 ${data.rows.length} 条 approved 数据`)
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '导出失败')
    }
  }

  async function savePrompt() {
    if (!selected || promptLoading || promptLoadFailed) return
    const guard = beginTaskAction(selected.id, savePromptSeq)
    setSavingPrompt(true)
    setPromptError('')
    try {
      const dimensions = parseDimensionsRows(dimensionRows)
      const data = await apiPost<{ prompt: AIPromptConfig, activePromptId: number }>(`/tasks/${selected.id}/ai-prompts`, {
        prompt_template: promptTemplate,
        dimensions,
        pass_threshold: Number(passThreshold),
        uncertain_min: Number(uncertainMin),
        model,
      })
      if (!isCurrentTaskAction(guard, savePromptSeq)) return
      setPrompts((current) => [data.prompt, ...current])
      setActivePromptId(data.activePromptId)
      fillPromptForm(data.prompt)
      Toast.success('AI Prompt 已保存')
    } catch (error) {
      if (!isCurrentTaskAction(guard, savePromptSeq)) return
      const message = error instanceof Error ? error.message : '保存 AI Prompt 失败'
      setPromptError(message)
      Toast.error(message)
    } finally {
      if (isCurrentTaskAction(guard, savePromptSeq)) {
        setSavingPrompt(false)
      }
    }
  }

  async function saveBaseline() {
    if (!selected) return
    const taskId = selected.id
    setSavingBaseline(true)
    try {
      const data = await apiPost<{ task: Task }>(`/tasks/${taskId}/baseline`, { baselineDescription: baselineDraft })
      setSelected(data.task)
      setBaselineDraft(data.task.baselineDescription ?? '')
      setTasks((current) => current.map((task) => task.id === taskId ? data.task : task))
      Toast.success('Baseline 已保存')
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '保存 baseline 失败')
    } finally {
      setSavingBaseline(false)
    }
  }

  async function updateAIReviewSettings(enabled: boolean) {
    if (!selected || promptLoading || promptLoadFailed) return
    const taskId = selected.id
    const guard = beginTaskAction(taskId, aiReviewSettingsSeq)
    setSavingAIReviewSettings(true)
    setPromptError('')
    try {
      const data = await apiPost<AIReviewSettingsResponse>(`/tasks/${taskId}/ai-review-settings`, { enabled })
      if (!isCurrentTaskAction(guard, aiReviewSettingsSeq)) return
      setAIReviewEnabled(data.aiReviewEnabled)
      setActivePromptId(data.activePromptId)
      setTasks((current) => current.map((task) => task.id === taskId ? { ...task, aiReviewEnabled: data.aiReviewEnabled, aiPromptId: data.activePromptId } : task))
      Toast.success(enabled ? 'AI review 已启用' : 'AI review 已关闭')
    } catch (error) {
      if (!isCurrentTaskAction(guard, aiReviewSettingsSeq)) return
      const message = error instanceof Error ? error.message : '更新 AI review 设置失败'
      setPromptError(message)
      Toast.error(message)
    } finally {
      if (isCurrentTaskAction(guard, aiReviewSettingsSeq)) {
        setSavingAIReviewSettings(false)
      }
    }
  }

  async function runDryRun() {
    if (!selected || !activePromptId || promptLoading || promptLoadFailed) return
    const taskId = selected.id
    const guard = beginTaskAction(taskId, dryRunSeq)
    setRunningDryRun(true)
    setPromptError('')
    setDryRun(null)
    try {
      const body = buildAIDryRunBody(samplePayload, sampleAnswer)
      const data = await apiPostRawJSON<AIDryRunResult>(`/tasks/${taskId}/ai-prompts/${activePromptId}/dry-run`, body)
      if (!isCurrentTaskAction(guard, dryRunSeq)) return
      setDryRun(data)
      Toast.success('dry-run 完成')
    } catch (error) {
      if (!isCurrentTaskAction(guard, dryRunSeq)) return
      const message = error instanceof Error ? error.message : 'dry-run 失败'
      setDryRun(null)
      setPromptError(message)
      Toast.error(message)
    } finally {
      if (isCurrentTaskAction(guard, dryRunSeq)) {
        setRunningDryRun(false)
      }
    }
  }

  async function createGoldenSample() {
    if (!selected || goldenSampleLoading || goldenSampleLoadFailed) return
    const guard = beginTaskAction(selected.id, createGoldenSampleSeq)
    setCreatingGoldenSample(true)
    setGoldenSampleError('')
    try {
      const body = buildGoldenSampleCreateBody({
        payload: goldenPayload,
        expectedAnswer: goldenExpectedAnswer,
        expectedVerdict: goldenExpectedVerdict,
        notes: goldenNotes,
        aiPromptId: resolveGoldenPromptId(),
      })
      const data = await apiPostRawJSON<GoldenSampleCreateResponse>(`/tasks/${selected.id}/golden-samples`, body)
      if (!isCurrentTaskAction(guard, createGoldenSampleSeq)) return
      setGoldenSamples((current) => [data.sample, ...current.filter((sample) => sample.id !== data.sample.id)])
      setGoldenNotes('')
      setGoldenSampleError('')
      Toast.success('Golden sample 已创建')
    } catch (error) {
      if (!isCurrentTaskAction(guard, createGoldenSampleSeq)) return
      const message = error instanceof Error ? error.message : '创建 golden sample 失败'
      setGoldenSampleError(message)
      Toast.error(message)
    } finally {
      if (isCurrentTaskAction(guard, createGoldenSampleSeq)) {
        setCreatingGoldenSample(false)
      }
    }
  }

  async function deleteGoldenSample(sample: GoldenSample) {
    if (!selected || goldenSampleLoading || goldenSampleLoadFailed) return
    Modal.confirm({
      title: '删除 golden sample',
      content: `确认删除 golden sample #${sample.id}?`,
      okText: '删除',
      cancelText: '取消',
      onOk: () => confirmDeleteGoldenSample(sample),
    })
  }

  async function confirmDeleteGoldenSample(sample: GoldenSample) {
    if (!selected) return
    const guard = beginTaskAction(selected.id, deleteGoldenSampleSeq)
    setDeletingGoldenSampleId(sample.id)
    setGoldenSampleError('')
    try {
      await apiDelete<{ deleted: boolean }>(`/tasks/${selected.id}/golden-samples/${sample.id}`)
      if (!isCurrentTaskAction(guard, deleteGoldenSampleSeq)) return
      setGoldenSamples((current) => current.filter((currentSample) => currentSample.id !== sample.id))
      setGoldenRunRows((current) => {
        const next = { ...current }
        delete next[sample.id]
        return next
      })
      Toast.success('Golden sample 已删除')
    } catch (error) {
      if (!isCurrentTaskAction(guard, deleteGoldenSampleSeq)) return
      const message = error instanceof Error ? error.message : '删除 golden sample 失败'
      setGoldenSampleError(message)
      Toast.error(message)
    } finally {
      if (isCurrentTaskAction(guard, deleteGoldenSampleSeq)) {
        setDeletingGoldenSampleId(null)
      }
    }
  }

  async function runGoldenSample(sample: GoldenSample) {
    if (!selected || goldenSampleLoading || goldenSampleLoadFailed) return
    const guard = beginTaskAction(selected.id, goldenSampleRunSeq)
    setGoldenSampleError('')
    await runGoldenSampleWithGuard(selected.id, sample, guard)
  }

  async function runAllGoldenSamples() {
    if (!selected || goldenSamples.length === 0 || goldenSampleLoading || goldenSampleLoadFailed) return
    const samples = goldenSamples
    const guard = beginTaskAction(selected.id, goldenSampleRunSeq)
    setGoldenSampleError('')
    setGoldenRunRows((current) => {
      const next = { ...current }
      for (const sample of samples) {
        next[sample.id] = {
          sampleId: sample.id,
          expectedVerdict: sample.expectedVerdict,
          status: 'running',
        }
      }
      return next
    })
    try {
      const data = await apiPost<GoldenSampleBatchDryRunResponse>(`/tasks/${selected.id}/golden-samples/dry-runs`, {
        sample_ids: samples.map((sample) => sample.id),
      })
      if (!isCurrentTaskAction(guard, goldenSampleRunSeq)) return
      const sampleById = new Map(samples.map((sample) => [sample.id, sample]))
      const resultBySampleId = new Map(data.results.map((result) => [result.goldenSampleId, result]))
      // 批量已改为异步入队:成功入队的标记 running 并复用单样本 poller 拉取结果,解析失败的直接标 failed。
      setGoldenRunRows((current) => {
        const next = { ...current }
        for (const sample of samples) {
          const result = resultBySampleId.get(sample.id)
          if (result?.status === 'queued') {
            next[sample.id] = {
              sampleId: sample.id,
              expectedVerdict: sample.expectedVerdict,
              status: 'running',
              reason: result.status,
              dryRunId: result.dryRunId,
            }
          } else {
            next[sample.id] = {
              sampleId: sample.id,
              expectedVerdict: sample.expectedVerdict,
              status: 'failed',
              error: result?.error || 'golden sample dry-run 入队失败',
            }
          }
        }
        return next
      })
      for (const result of data.results) {
        if (result.status === 'queued' && result.dryRunId !== undefined) {
          const sample = sampleById.get(result.goldenSampleId)
          if (sample) {
            void pollGoldenSampleRun(selected.id, sample, result.dryRunId, guard, 0)
          }
        }
      }
      if (data.summary.failed > 0) {
        setGoldenSampleError(data.results.find((result) => result.status === 'failed')?.error || '部分 golden sample 入队失败')
      } else {
        setGoldenSampleError('')
      }
      void loadDryRunHistory(selected.id, dryRunHistorySampleID(dryRunHistorySampleFilter))
    } catch (error) {
      if (!isCurrentTaskAction(guard, goldenSampleRunSeq)) return
      const message = error instanceof Error ? error.message : 'golden sample batch dry-run 失败'
      setGoldenRunRows((current) => {
        const next = { ...current }
        for (const sample of samples) {
          next[sample.id] = {
            sampleId: sample.id,
            expectedVerdict: sample.expectedVerdict,
            status: 'failed',
            error: message,
          }
        }
        return next
      })
      setGoldenSampleError(message)
    }
  }

  async function runGoldenSampleWithGuard(taskId: number, sample: GoldenSample, guard: { taskId: number, generation: number, seq: number }) {
    setGoldenRunRows((current) => ({
      ...current,
      [sample.id]: {
        sampleId: sample.id,
        expectedVerdict: sample.expectedVerdict,
        status: 'running',
      },
    }))
    try {
      const data = await apiPost<QueuedDryRunResponse & Partial<AIDryRunResult>>(`/tasks/${taskId}/golden-samples/${sample.id}/dry-run`, {})
      if (!isCurrentTaskAction(guard, goldenSampleRunSeq)) return
      if (data.result) {
        setDryRun(data as AIDryRunResult)
        setGoldenRunRows((current) => ({
          ...current,
          [sample.id]: goldenSampleResultToRunRow(sample, data as AIDryRunResult),
        }))
        void loadDryRunHistory(taskId, dryRunHistorySampleID(dryRunHistorySampleFilter))
        return
      }
      if (typeof data.dryRunId !== 'number') {
        // 入队响应缺少 dryRunId 则无可轮询的目标,直接置为失败而不是空转。
        const message = 'dry-run 入队异常:响应缺少 dryRunId'
        setGoldenRunRows((current) => ({
          ...current,
          [sample.id]: { sampleId: sample.id, expectedVerdict: sample.expectedVerdict, status: 'failed', error: message },
        }))
        setGoldenSampleError(message)
        return
      }
      setGoldenRunRows((current) => ({
        ...current,
        [sample.id]: {
          sampleId: sample.id,
          expectedVerdict: sample.expectedVerdict,
          status: 'running',
          reason: data.status,
          dryRunId: data.dryRunId,
        },
      }))
      void pollGoldenSampleRun(taskId, sample, data.dryRunId, guard, 0)
    } catch (error) {
      if (!isCurrentTaskAction(guard, goldenSampleRunSeq)) return
      const message = error instanceof Error ? error.message : 'golden sample dry-run 失败'
      setGoldenRunRows((current) => ({
        ...current,
        [sample.id]: {
          sampleId: sample.id,
          expectedVerdict: sample.expectedVerdict,
          status: 'failed',
          error: message,
        },
      }))
      setGoldenSampleError(message)
      void loadDryRunHistory(taskId, dryRunHistorySampleID(dryRunHistorySampleFilter))
    }
  }

  async function pollGoldenSampleRun(taskId: number, sample: GoldenSample, dryRunId: number, guard: { taskId: number, generation: number, seq: number }, attempt: number) {
    if (attempt > 20 || !isCurrentTaskAction(guard, goldenSampleRunSeq)) return
    try {
      const data = await apiGet<AIDryRunHistoryResponse>(`/tasks/${taskId}/ai-dry-runs?golden_sample_id=${sample.id}&limit=10`)
      if (!isCurrentTaskAction(guard, goldenSampleRunSeq)) return
      const run = data.dryRuns.find((item) => item.id === dryRunId)
      if (run?.status === 'succeeded') {
        if (run.result) {
          setDryRun({
            provider: run.result.provider || 'ai-worker',
            dryRunId,
            matchedExpected: run.matchedExpected ?? undefined,
            result: run.result,
          })
        }
        setGoldenRunRows((current) => ({
          ...current,
          [sample.id]: {
            sampleId: sample.id,
            expectedVerdict: sample.expectedVerdict,
            status: 'succeeded',
            actualVerdict: run.actualVerdict ?? undefined,
            matchedExpected: run.matchedExpected ?? undefined,
            score: run.result?.overall_score,
            provider: run.result?.provider || (run.result ? 'ai-worker' : undefined),
            model: run.result?.model,
            dryRunId,
            reason: run.result?.reason ?? run.errorMsg ?? undefined,
          },
        }))
        setDryRunHistory(data.dryRuns)
        setDryRunGuard(data.guard ?? null)
        return
      }
      if (run?.status === 'failed') {
        setGoldenRunRows((current) => ({
          ...current,
          [sample.id]: {
            sampleId: sample.id,
            expectedVerdict: sample.expectedVerdict,
            status: 'failed',
            dryRunId,
            error: run.errorMsg ?? 'golden sample dry-run 失败',
          },
        }))
        setDryRunHistory(data.dryRuns)
        setDryRunGuard(data.guard ?? null)
        return
      }
    } catch {
      // 下一轮继续轮询,最终由 history 刷新兜底。
    }
    const timer = window.setTimeout(() => {
      goldenSamplePollTimers.current.delete(timer)
      void pollGoldenSampleRun(taskId, sample, dryRunId, guard, attempt + 1)
    }, 1000)
    goldenSamplePollTimers.current.add(timer)
  }

  function resolveGoldenPromptId() {
    if (goldenPromptChoice === 'none') return null
    if (goldenPromptChoice === 'active') return activePromptId
    const promptId = Number(goldenPromptChoice)
    return Number.isFinite(promptId) && promptId > 0 ? promptId : null
  }

  function promptVersionLabel(promptId: number | null) {
    if (!promptId) return '未绑定 prompt'
    const prompt = prompts.find((candidate) => candidate.id === promptId)
    return prompt ? `Prompt v${prompt.version} (#${prompt.id})` : `Prompt #${promptId}`
  }

  const promptActionDisabled = promptLoading || promptLoadFailed
  const goldenActionDisabled = goldenSampleLoading || goldenSampleLoadFailed
  const anyGoldenRunRunning = Object.values(goldenRunRows).some((row) => row.status === 'running')
  const goldenRunResults = goldenSamples.map((sample) => goldenRunRows[sample.id]).filter((row): row is GoldenSampleRunRow => Boolean(row))
  const selectedHistorySampleID = dryRunHistorySampleID(dryRunHistorySampleFilter)
  const dryRunHistorySummary = summarizeDryRunHistory(dryRunHistory)

  return (
    <div>
      <div style={{ marginBottom: 'var(--space-xl)' }}>
        <h1 style={{ fontFamily: 'var(--lh-font-sans)', fontSize: 'var(--text-h1)', margin: 0, fontWeight: 700 }}>Owner 任务负责人</h1>
        <p style={{ fontFamily: 'var(--lh-font-sans)', color: 'var(--lh-text-2)', marginTop: 'var(--space-xs)' }}>任务发布 · 模板搭建 · 审核配置 · 数据导出</p>
      </div>

      <div>
        <section style={{ ...panelStyle, minHeight: 600 }}>
          {detailSection === 'tasks' && (
            <TaskManagePanel
              tasks={tasks}
              selected={selected}
              onSelect={selectTask}
              onTaskSaved={applyTaskSaved}
              onTasksChanged={() => void reloadTasksKeepSelection()}
            />
          )}
          {detailSection !== 'tasks' && tasks.length > 0 && (
            <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-sm)', flexWrap: 'wrap', marginBottom: 'var(--space-lg)' }}>
              <span style={{ fontSize: 'var(--text-sm)', color: 'var(--lh-text-3)' }}>当前任务</span>
              {tasks.map((task) => (
                <button
                  key={task.id}
                  type="button"
                  onClick={() => selectTask(task)}
                  style={task.id === selected?.id ? activeTaskChipStyle : taskChipStyle}
                >
                  {task.title}
                </button>
              ))}
            </div>
          )}
          {detailSection !== 'tasks' && selected ? (
            <>
              <div style={{ borderBottom: '1px solid var(--lh-border)', paddingBottom: 'var(--space-md)', marginBottom: 'var(--space-lg)' }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <h2 style={{ ...headingStyle, fontSize: 'var(--text-h1)' }}>{selected.title}</h2>
                  <div style={{ display: 'flex', gap: 'var(--space-sm)' }}>
                    <Button onClick={() => void exportJSON(selected.id)} theme="light">导出数据</Button>
                    <a href={`/owner/tasks/${selected.id}/templates`} style={templateDesignerLinkStyle}>模板 Designer</a>
                  </div>
                </div>
                <p style={{ color: 'var(--lh-text-2)', marginTop: 'var(--space-sm)', fontSize: 'var(--text-base)' }}>
                  {selected.description ?? '配置标注模板与 AI 预审参数。'}
                </p>
              </div>

              <div style={controlStripStyle}>
                <MetricCell label="TASK" value={`#${selected.id}`} detail={`${selected.finishedItems}/${selected.totalItems} finished`} />
                <MetricCell label="AI REVIEW" value={aiReviewEnabled ? 'ON' : 'OFF'} detail={activePromptId ? promptVersionLabel(activePromptId) : 'no active prompt'} tone={aiReviewEnabled ? 'success' : 'muted'} />
                <MetricCell label="PROMPTS" value={String(prompts.length)} detail={promptLoading ? 'loading config' : promptLoadFailed ? 'load failed' : 'versions loaded'} />
                <MetricCell label="EVAL SET" value={String(goldenSamples.length)} detail={goldenSampleLoading ? 'loading samples' : `${Object.keys(goldenRunRows).length} recent runs`} tone="teal" />
                <MetricCell label="HISTORY" value={String(dryRunHistorySummary.total)} detail={`${formatPercent(dryRunHistorySummary.matchRate)} match / avg ${formatOptionalNumber(dryRunHistorySummary.averageScore)}`} />
              </div>

              {detailSection === 'template' && (
                <section style={aiPromptSectionStyle} aria-label="模板搭建">
                  <h3 style={subHeadingStyle}>模板搭建</h3>
                  <p style={mutedStyle}>用可视化 Designer 拖拽物料、配置 visibleWhen / customRule 的显隐与校验规则。</p>
                  <a href={`/owner/tasks/${selected.id}/templates`} style={templateDesignerLinkStyle}>打开 Designer</a>
                </section>
              )}
              {detailSection === 'dataset' && (
                <ImportPanel taskId={selected.id} onImported={() => void reloadTasksKeepSelection()} />
              )}

              {detailSection === 'review' && (
                <section style={aiPromptSectionStyle} aria-label="审核结果">
                  <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-sm)', marginBottom: 'var(--space-md)' }}>
                    <h3 style={subHeadingStyle}>审核结果(只读)</h3>
                    <StatusBadge status="draft" label="只读视图" />
                  </div>
                  <p style={mutedStyle}>
                    人工审核的初审 / 复审 / 终审「动作」在 Reviewer 工作台完成,Owner 这里只读审核汇总结果,不做审核操作。
                  </p>
                  <div style={controlStripStyle}>
                    <MetricCell label="PROGRESS" value={`${selected.finishedItems}/${selected.totalItems}`} detail="已完成 / 总题数" tone="teal" />
                    <MetricCell label="AI REVIEW" value={aiReviewEnabled ? 'ON' : 'OFF'} detail={aiReviewEnabled ? 'AI 预审已启用' : 'AI 预审未启用'} tone={aiReviewEnabled ? 'success' : 'muted'} />
                  </div>
                  <p style={mutedStyle}>
                    通过率、AI vs 人工差异、三级审核进度等汇总图表见左侧「数据看板」;下方是逐条质检结果,用于回看 AI 预审标准。
                  </p>
                  <ReviewResultsPanel taskId={selected.id} />
                </section>
              )}

              {detailSection === 'stats' && (
                <Suspense fallback={<LoadingBlock title="看板加载中" rows={3} />}>
                  <StatsBoard taskId={selected.id} />
                </Suspense>
              )}
              {detailSection === 'export' && (
                <div className="lh-sub-views" data-sub={subView ?? 'all'}>
                  <ExportPanel taskId={selected.id} />
                </div>
              )}

              {detailSection === 'ai' && (
                <div className="lh-sub-views" data-sub={subView ?? 'all'}>
              <div id="ai-baseline" style={{ background: 'var(--lh-bg)', padding: 'var(--space-md)', borderRadius: 'var(--radius-md)', marginBottom: 'var(--space-lg)', border: '1px solid var(--lh-border)' }}>
                <div style={aiSettingsRowStyle}>
                  <div style={{ fontWeight: 600, fontSize: 'var(--text-sm)', color: 'var(--lh-text-3)', textTransform: 'uppercase' }}>Baseline 说明</div>
                  <Button aria-label="保存 baseline" disabled={savingBaseline} loading={savingBaseline} onClick={() => void saveBaseline()} theme="light">保存 baseline</Button>
                </div>
                <textarea
                  aria-label="baseline_description"
                  value={baselineDraft}
                  onChange={(event) => setBaselineDraft(event.target.value)}
                  style={{ ...textareaStyle, minHeight: 96, marginTop: 'var(--space-sm)' }}
                  placeholder="写清楚 AI 判断质量的基线、必须保留的信息和常见打回标准。"
                />
              </div>

              <section id="ai-prompts" style={aiPromptSectionStyle}>
                <div id="ai-config">
                <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-sm)', marginBottom: 'var(--space-md)' }}>
                  <h3 style={subHeadingStyle}>AI 自动预审配置</h3>
                  <StatusBadge status={aiReviewEnabled ? 'running' : 'draft'} label={aiReviewEnabled ? 'AI review: 已启用' : 'AI review: 已关闭'} />
                </div>

                <div style={{ ...aiSettingsRowStyle, background: 'var(--lh-bg-card)', padding: 'var(--space-md)', borderRadius: 'var(--radius-md)', border: '1px solid var(--lh-border)', marginBottom: 'var(--space-lg)' }}>
                  <div>
                    <div style={{ color: 'var(--lh-text-1)', fontWeight: 600 }}>启用 AI review</div>
                    <div style={{ fontSize: 'var(--text-sm)' }}>开启后，系统将对标注结果进行实时质量评估</div>
                  </div>
                  {aiReviewEnabled ? (
                    <Button aria-label="关闭 AI review" disabled={promptActionDisabled || savingAIReviewSettings} loading={savingAIReviewSettings} onClick={() => void updateAIReviewSettings(false)} theme="light" type="danger">
                      关闭服务
                    </Button>
                  ) : (
                    <Button aria-label="启用 AI review" disabled={!activePromptId || promptActionDisabled || savingAIReviewSettings} loading={savingAIReviewSettings} onClick={() => void updateAIReviewSettings(true)} theme="solid">
                      开启服务
                    </Button>
                  )}
                </div>

                {!activePromptId ? <p style={{ ...mutedStyle, marginBottom: 'var(--space-md)' }}>保存 AI Prompt 后才能启用 AI review</p> : null}
                {promptError ? <div role="alert" style={{ ...alertStyle, marginBottom: 'var(--space-md)' }}>{promptError}</div> : null}

                <div style={{ display: 'grid', gap: 'var(--space-lg)' }}>
                  <div style={configEditorGridStyle}>
                    <label style={fieldStyle}>
                      <span style={{ fontWeight: 600 }}>Prompt 模板 (Handlebars)</span>
                      <textarea aria-label="prompt_template" value={promptTemplate} onChange={(event) => setPromptTemplate(event.target.value)} style={{ ...textareaStyle, height: 200 }} placeholder="输入审阅 Prompt..." />
                    </label>
                    <DimensionEditor rows={dimensionRows} onChange={setDimensionRows} />
                  </div>

                  <div style={formGridStyle}>
                    <label style={fieldStyle}>
                      <span style={{ fontWeight: 600 }}>通过阈值 (Pass)</span>
                      <input aria-label="pass_threshold" type="number" min={0} max={100} value={passThreshold} onChange={(event) => setPassThreshold(event.target.value)} style={inputStyle} />
                      <input aria-label="pass_threshold_slider" type="range" min={0} max={100} value={passThreshold} onChange={(event) => setPassThreshold(event.target.value)} />
                    </label>
                    <label style={fieldStyle}>
                      <span style={{ fontWeight: 600 }}>待定区间 (Min)</span>
                      <input aria-label="uncertain_min" type="number" min={0} max={100} value={uncertainMin} onChange={(event) => setUncertainMin(event.target.value)} style={inputStyle} />
                      <input aria-label="uncertain_min_slider" type="range" min={0} max={100} value={uncertainMin} onChange={(event) => setUncertainMin(event.target.value)} />
                    </label>
                    <label style={fieldStyle}>
                      <span style={{ fontWeight: 600 }}>模型选择</span>
                      <input aria-label="model" value={model} onChange={(event) => setModel(event.target.value)} style={inputStyle} placeholder="gpt-4o / doubao-pro" />
                    </label>
                  </div>
                </div>

                <Button aria-label="保存 AI Prompt" disabled={promptActionDisabled || savingPrompt} loading={savingPrompt} theme="solid" onClick={() => void savePrompt()} style={{ marginTop: 'var(--space-lg)', width: 140 }}>
                  保存配置
                </Button>

                </div>
                <div id="ai-dryrun" style={{ ...dryRunPanelStyle, marginTop: 'var(--space-2xl)', background: '#fafafa', padding: 'var(--space-lg)', borderRadius: 'var(--radius-lg)' }}>
                  <h4 style={{ ...subHeadingStyle, marginBottom: 'var(--space-md)' }}>AI Dry-run 测试</h4>
                  <div style={dryRunGridStyle}>
                    <label style={fieldStyle}>
                      <span>Sample Payload</span>
                      <textarea aria-label="sample_payload" value={samplePayload} onChange={(event) => setSamplePayload(event.target.value)} style={{ ...textareaStyle, height: 120 }} />
                    </label>
                    <label style={fieldStyle}>
                      <span>Sample Answer</span>
                      <textarea aria-label="sample_answer" value={sampleAnswer} onChange={(event) => setSampleAnswer(event.target.value)} style={{ ...textareaStyle, height: 120 }} />
                    </label>
                  </div>
                  <Button aria-label="运行 dry-run" disabled={!activePromptId || promptActionDisabled || runningDryRun} loading={runningDryRun} onClick={() => void runDryRun()} style={{ marginTop: 'var(--space-md)' }}>
                    执行测试
                  </Button>
                  {dryRun && (
                    <div style={{ ...dryRunResultStyle, background: 'var(--lh-bg-card)', borderRadius: 'var(--radius-md)', boxShadow: 'var(--shadow-sm)', border: '1px solid var(--lh-border)' }}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 'var(--space-sm)' }}>
                        <strong style={{ fontFamily: 'var(--lh-font-mono)', fontSize: 'var(--text-sm)' }}>{dryRun.provider}</strong>
                        <span style={{ ...verdictPillStyle, color: dryRun.result.verdict === 'pass' ? 'var(--lh-success)' : 'var(--lh-danger)' }}>
                          {dryRun.result.verdict} ({dryRun.result.overall_score})
                        </span>
                      </div>
                      <div style={{ fontSize: 'var(--text-base)', marginBottom: 'var(--space-md)' }}>{dryRun.result.reason}</div>
                      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: 'var(--space-sm)' }}>
                        {dryRun.result.dimensions.map((dimension) => (
                          <div key={dimension.name} style={{ padding: 'var(--space-sm)', background: 'var(--lh-bg)', borderRadius: 'var(--radius-sm)' }}>
                            <div style={{ fontSize: 11, color: 'var(--lh-text-3)' }}>{dimension.name}</div>
                            <div style={{ fontWeight: 600 }}>{dimension.score}</div>
                            <div style={{ fontSize: 12, marginTop: 4 }}>{dimension.reason}</div>
                          </div>
                        ))}
                      </div>
                    </div>
                  )}
                </div>

                <div id="ai-golden" style={{ ...goldenSampleSectionStyle, marginTop: 'var(--space-2xl)' }}>
                  <div style={aiSettingsRowStyle}>
                    <h3 style={subHeadingStyle}>Golden Samples (评测集)</h3>
                    <Button aria-label="Run all visible samples" disabled={goldenActionDisabled || goldenSamples.length === 0 || anyGoldenRunRunning} onClick={() => void runAllGoldenSamples()} theme="light">
                      批量运行评测
                    </Button>
                  </div>
                  {goldenSampleError ? <div role="alert" style={{ ...alertStyle, marginTop: 'var(--space-md)' }}>{goldenSampleError}</div> : null}

                  <div style={{ display: 'grid', gap: 'var(--space-md)', marginTop: 'var(--space-md)', padding: 'var(--space-md)', background: '#f8f9fa', borderRadius: 'var(--radius-md)' }}>
                    <div style={dryRunGridStyle}>
                      <label style={fieldStyle}>
                        <span>Payload</span>
                        <textarea aria-label="golden_sample_payload" value={goldenPayload} onChange={(event) => setGoldenPayload(event.target.value)} style={{ ...textareaStyle, height: 80 }} />
                      </label>
                      <label style={fieldStyle}>
                        <span>Expected Answer</span>
                        <textarea aria-label="golden_sample_expected_answer" value={goldenExpectedAnswer} onChange={(event) => setGoldenExpectedAnswer(event.target.value)} style={{ ...textareaStyle, height: 80 }} />
                      </label>
                    </div>
                    <div style={formGridStyle}>
                      <label style={fieldStyle}>
                        <span>预期结论</span>
                        <select aria-label="golden_sample_expected_verdict" value={goldenExpectedVerdict} onChange={(event) => setGoldenExpectedVerdict(event.target.value)} style={inputStyle}>
                          <option value="pass">pass</option>
                          <option value="reject">reject</option>
                          <option value="uncertain">uncertain</option>
                        </select>
                      </label>
                      <label style={fieldStyle}>
                        <span>绑定 Prompt</span>
                        <select aria-label="golden_sample_prompt" value={goldenPromptChoice} onChange={(event) => setGoldenPromptChoice(event.target.value)} style={inputStyle}>
                          <option value="active">当前 active prompt{activePromptId ? ` (#${activePromptId})` : ''}</option>
                          <option value="none">不绑定 prompt</option>
                          {prompts.map((prompt) => (
                            <option key={prompt.id} value={String(prompt.id)}>Prompt v{prompt.version} #{prompt.id}</option>
                          ))}
                        </select>
                      </label>
                      <label style={fieldStyle}>
                        <span>Notes</span>
                        <input aria-label="golden_sample_notes" value={goldenNotes} onChange={(event) => setGoldenNotes(event.target.value)} style={inputStyle} />
                      </label>
                    </div>
                    <Button aria-label="创建 golden sample" disabled={goldenActionDisabled || creatingGoldenSample} loading={creatingGoldenSample} onClick={() => void createGoldenSample()} theme="solid" style={{ width: 140 }}>
                      添加样本
                    </Button>
                  </div>

                  <div style={{ marginTop: 'var(--space-xl)' }}>
                    {goldenSampleLoading ? (
                      <LoadingBlock title="加载 golden samples" rows={2} />
                    ) : goldenSamples.length === 0 ? (
                      <EmptyState title="暂无 golden samples" body="添加已知答案样本后,可以用来回归评测 AI prompt。" variant="empty" />
                    ) : (
                      <div style={goldenSampleListStyle}>
                        {goldenSamples.map((sample) => {
                          const runRow = goldenRunRows[sample.id]
                          return (
                            <div key={sample.id} style={{ ...goldenSampleItemStyle, background: 'var(--lh-bg-card)', borderRadius: 'var(--radius-md)', boxShadow: 'var(--shadow-sm)', padding: 'var(--space-md)', border: '1px solid var(--lh-border)' }}>
                              <div style={goldenSampleHeaderStyle}>
                                <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-sm)' }}>
                                  <strong style={{ fontSize: 'var(--text-base)' }}>#{sample.id} · {sample.expectedVerdict}</strong>
                                  <StatusBadge status={verdictStatus(sample.expectedVerdict)} label={sample.expectedVerdict} />
                                </div>
                                <span style={{ fontSize: 'var(--text-sm)', color: 'var(--lh-text-3)' }}>{promptVersionLabel(sample.aiPromptId)}</span>
                                <span style={{ fontSize: 'var(--text-sm)', color: 'var(--lh-text-3)' }}>{formatDateTime(sample.createdAt)}</span>
                              </div>
                              {sample.notes ? <div style={{ ...mutedStyle, marginTop: 'var(--space-sm)' }}>{sample.notes}</div> : null}
                              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: 'var(--space-sm)', marginTop: 'var(--space-md)' }}>
                                <pre style={{ ...compactPreviewStyle, fontSize: 11 }}>{formatCompactJSON(sample.payload)}</pre>
                                <pre style={{ ...compactPreviewStyle, fontSize: 11 }}>{formatCompactJSON(sample.expectedAnswer)}</pre>
                              </div>
                              <div style={{ ...goldenSampleActionsStyle, justifyContent: 'flex-end', marginTop: 'var(--space-sm)' }}>
                                <Button aria-label={`Run golden sample ${sample.id}`} size="small" disabled={goldenActionDisabled || runRow?.status === 'running' || anyGoldenRunRunning} loading={runRow?.status === 'running'} onClick={() => void runGoldenSample(sample)} theme="solid">测试</Button>
                                <Button aria-label={`查看 golden sample ${sample.id} history`} size="small" disabled={dryRunHistoryLoading} onClick={() => setDryRunHistorySampleFilter(String(sample.id))} theme="light">历史</Button>
                                <Button aria-label={`删除 golden sample ${sample.id}`} size="small" disabled={goldenActionDisabled || deletingGoldenSampleId === sample.id || anyGoldenRunRunning} loading={deletingGoldenSampleId === sample.id} onClick={() => void deleteGoldenSample(sample)} type="danger" theme="light">删除</Button>
                              </div>
                            </div>
                          )
                        })}
                      </div>
                    )}
                  </div>

                  {goldenRunResults.length > 0 ? (
                    <div style={resultTableWrapStyle}>
                      <table style={resultTableStyle}>
                        <thead>
                          <tr style={{ background: '#f8f9fa' }}>
                            <th style={resultCellStyle}>sample</th>
                            <th style={resultCellStyle}>expected</th>
                            <th style={resultCellStyle}>actual</th>
                            <th style={resultCellStyle}>matched</th>
                            <th style={resultCellStyle}>score</th>
                            <th style={resultCellStyle}>provider</th>
                            <th style={resultCellStyle}>dryRunId</th>
                            <th style={resultCellStyle}>reason / error</th>
                          </tr>
                        </thead>
                        <tbody>
                          {goldenRunResults.map((row) => (
                            <tr key={row.sampleId}>
                              <td style={resultCellStyle}>#{row.sampleId}</td>
                              <td style={resultCellStyle}>{row.expectedVerdict}</td>
                              <td style={resultCellStyle}>{row.actualVerdict ? <StatusBadge status={verdictStatus(row.actualVerdict)} label={row.actualVerdict} /> : <StatusBadge status={row.status} />}</td>
                              <td style={resultCellStyle}>{row.matchedExpected === undefined ? '-' : row.matchedExpected ? 'matched' : 'mismatch'}</td>
                              <td style={resultCellStyle}>{row.score ?? '-'}</td>
                              <td style={resultCellStyle}>{[row.provider, row.model].filter(Boolean).join(' / ') || '-'}</td>
                              <td style={resultCellStyle}>{row.dryRunId ?? '-'}</td>
                              <td style={resultCellStyle}>{row.error || row.reason || '-'}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  ) : null}

                  <div id="ai-history" style={historyPanelStyle}>
                    <div style={aiSettingsRowStyle}>
                      <h3 style={subHeadingStyle}>Dry-run 历史记录</h3>
                      <Button disabled={!selected || dryRunHistoryLoading} loading={dryRunHistoryLoading} onClick={() => selected && void loadDryRunHistory(selected.id, selectedHistorySampleID)} theme="light">
                        刷新
                      </Button>
                    </div>
                    <label style={{ ...fieldStyle, marginTop: 'var(--space-md)' }}>
                      <span>按样本筛选</span>
                      <select aria-label="dry_run_history_sample_filter" value={dryRunHistorySampleFilter} onChange={(event) => setDryRunHistorySampleFilter(event.target.value)} style={{ ...inputStyle, width: 260 }}>
                        <option value="all">全部记录</option>
                        {goldenSamples.map((sample) => (
                          <option key={sample.id} value={String(sample.id)}>Sample #{sample.id}</option>
                        ))}
                      </select>
                    </label>
                    {dryRunHistoryError ? <p style={errorTextStyle}>{dryRunHistoryError}</p> : null}
                    {dryRunHistory.length > 0 && (
                      <div style={{ ...historySummaryStyle, background: 'var(--lh-bg)', padding: 'var(--space-sm) var(--space-md)', borderRadius: 'var(--radius-sm)', border: '1px solid var(--lh-border)' }}>
                        <span style={{ fontWeight: 600 }}>统计:</span>
                        <span>总计 {dryRunHistorySummary.total}</span>
                        <span style={{ color: 'var(--lh-success)' }}>匹配 {dryRunHistorySummary.matched}</span>
                        <span style={{ color: 'var(--lh-danger)' }}>不匹配 {dryRunHistorySummary.mismatch}</span>
                        <span style={{ color: 'var(--lh-text-3)' }}>失败 {dryRunHistorySummary.failed}</span>
                        <span>匹配率 {formatPercent(dryRunHistorySummary.matchRate)}</span>
                        <span>均分 {formatOptionalNumber(dryRunHistorySummary.averageScore)}</span>
                      </div>
                    )}
                    {dryRunGuard ? (
                      <div style={{ ...historySummaryStyle, background: 'var(--lh-bg-card)', padding: 'var(--space-sm) var(--space-md)', borderRadius: 'var(--radius-sm)', border: '1px solid var(--lh-border)' }}>
                        <span style={{ fontWeight: 600 }}>Guard:</span>
                        <span>{formatDryRunGuardState(dryRunGuard)}</span>
                        <span>{dryRunGuard.windowMinutes}m window</span>
                        <span>runs {dryRunGuard.recentRuns}{dryRunGuard.quotaMaxRuns > 0 ? `/${dryRunGuard.quotaMaxRuns}` : ''}</span>
                        <span>failures {dryRunGuard.recentFailures}{dryRunGuard.circuitMaxFailures > 0 ? `/${dryRunGuard.circuitMaxFailures}` : ''}</span>
                      </div>
                    ) : null}
                    {dryRunHistoryLoading ? (
                      <LoadingBlock title="加载 dry-run history" rows={2} />
                    ) : dryRunHistory.length === 0 ? (
                      <EmptyState title="暂无 dry-run history" body="运行 golden sample 后会显示最近结果和匹配率。" variant="queue" />
                    ) : (
                      <div style={resultTableWrapStyle}>
                        <table style={resultTableStyle}>
                          <thead>
                            <tr style={{ background: '#f8f9fa' }}>
                              <th style={resultCellStyle}>ID</th>
                              <th style={resultCellStyle}>样本</th>
                              <th style={resultCellStyle}>预期</th>
                              <th style={resultCellStyle}>实际</th>
                              <th style={resultCellStyle}>匹配</th>
                              <th style={resultCellStyle}>状态</th>
                              <th style={resultCellStyle}>Prompt</th>
                              <th style={resultCellStyle}>完成时间</th>
                            </tr>
                          </thead>
                          <tbody>
                            {dryRunHistory.map((run) => (
                              <tr key={run.id}>
                                <td style={resultCellStyle}>#{run.id}</td>
                                <td style={resultCellStyle}>{run.goldenSampleId ? `#${run.goldenSampleId}` : '-'}</td>
                                <td style={resultCellStyle}>{run.expectedVerdict || '-'}</td>
                                <td style={resultCellStyle}>{run.actualVerdict || '-'}</td>
                                <td style={resultCellStyle}>
                                  <span style={{ color: run.matchedExpected ? 'var(--lh-success)' : run.matchedExpected === false ? 'var(--lh-danger)' : 'inherit', fontWeight: 600 }}>
                                    {formatMatched(run.matchedExpected)}
                                  </span>
                                </td>
                                <td style={resultCellStyle}>
                                  <StatusBadge status={run.status} />
                                  {run.errorMsg ? <span style={errorTextStyle}> {run.errorMsg}</span> : null}
                                </td>
                                <td style={resultCellStyle}>v{run.promptVersion} #{run.aiPromptId}</td>
                                <td style={resultCellStyle}>{formatDateTime(run.finishedAt || run.createdAt)}</td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    )}
                  </div>
                </div>
              </section>
                </div>
              )}
              {exportRows.length > 0 ? (
                <pre style={{ marginTop: 'var(--space-lg)', padding: 'var(--space-md)', background: 'var(--lh-bg)', overflow: 'auto', maxHeight: 260, borderRadius: 'var(--radius-md)', border: '1px solid var(--lh-border)' }}>
                  {JSON.stringify(exportRows.slice(0, 3), null, 2)}
                </pre>
              ) : null}
            </>
          ) : null}
          {detailSection !== 'tasks' && !selected && (
            <div style={{ display: 'flex', height: '100%', alignItems: 'center', justifyContent: 'center', color: 'var(--lh-text-3)' }}>
              请在「任务管理」中选择一个任务进行配置
            </div>
          )}
        </section>
      </div>
    </div>
  )
}

function MetricCell({ label, value, detail, tone = 'muted' }: { label: string, value: string, detail: string, tone?: 'success' | 'teal' | 'muted' }) {
  const accent = tone === 'success' ? 'var(--lh-success)' : tone === 'teal' ? 'var(--lh-cyan)' : 'var(--lh-text-1)'
  return (
    <div style={metricCellStyle}>
      <div style={metricLabelStyle}>{label}</div>
      <div style={{ ...metricValueStyle, color: accent }}>{value}</div>
      <div style={metricDetailStyle}>{detail}</div>
    </div>
  )
}

function DimensionEditor({ rows, onChange }: { rows: DimensionRow[], onChange: (rows: DimensionRow[]) => void }) {
  function updateRow(index: number, patch: Partial<DimensionRow>) {
    onChange(rows.map((row, rowIndex) => rowIndex === index ? { ...row, ...patch } : row))
  }
  function removeRow(index: number) {
    if (rows.length <= 1) return
    onChange(rows.filter((_, rowIndex) => rowIndex !== index))
  }
  function addRow() {
    onChange([...rows, { id: `new-${Date.now()}`, name: '', description: '', weight: '1' }])
  }
  return (
    <section style={dimensionPanelStyle}>
      <div style={aiSettingsRowStyle}>
        <span style={{ fontWeight: 600 }}>评分维度</span>
        <Button aria-label="新增维度" onClick={addRow} theme="light">新增维度</Button>
      </div>
      <div style={{ display: 'grid', gap: 'var(--space-sm)' }}>
        {rows.map((row, index) => (
          <div key={row.id} style={dimensionRowStyle}>
            <label style={compactFieldStyle}>
              <span>名称</span>
              <input aria-label={`dimension_name_${index}`} value={row.name} onChange={(event) => updateRow(index, { name: event.target.value })} style={inputStyle} />
            </label>
            <label style={compactFieldStyle}>
              <span>说明</span>
              <input aria-label={`dimension_description_${index}`} value={row.description} onChange={(event) => updateRow(index, { description: event.target.value })} style={inputStyle} />
            </label>
            <label style={compactFieldStyle}>
              <span>权重</span>
              <input aria-label={`dimension_weight_${index}`} type="number" min={0} max={1} step="0.05" value={row.weight} onChange={(event) => updateRow(index, { weight: event.target.value })} style={inputStyle} />
            </label>
            <Button aria-label={`删除维度 ${index + 1}`} disabled={rows.length <= 1} onClick={() => removeRow(index)} theme="light" type="danger">删除</Button>
          </div>
        ))}
      </div>
    </section>
  )
}

const defaultPromptTemplate = '请根据 payload 和 answer 完成结构化预审。'
const defaultPassThreshold = '80'
const defaultUncertainMin = '60'
const defaultModel = ''
const defaultGoldenPayload = '{"prompt":"示例题目"}'
const defaultGoldenExpectedAnswer = '{"summary":"示例答案"}'
const defaultGoldenExpectedVerdict = 'pass'

const defaultDimensionsJSON = JSON.stringify([
  { name: '相关性', description: '是否相关', weight: 1 },
  { name: '准确性', description: '是否准确', weight: 1 },
  { name: '格式合规', description: '是否符合格式', weight: 1 },
], null, 2)

function defaultDimensionRows() {
  return dimensionRowsFromRaw(defaultDimensionsJSON)
}

function dimensionRowsFromRaw(raw: string): DimensionRow[] {
  try {
    const parsed = JSON.parse(raw) as unknown
    if (!Array.isArray(parsed)) {
      return []
    }
    const rows = parsed.flatMap((dimension, index) => {
      if (typeof dimension !== 'object' || dimension === null) return []
      const record = dimension as Record<string, unknown>
      return [{
        id: `dimension-${index}`,
        name: typeof record.name === 'string' ? record.name : '',
        description: typeof record.description === 'string' ? record.description : '',
        weight: record.weight === undefined || record.weight === null ? '1' : String(record.weight),
      }]
    })
    return rows.length > 0 ? rows : [{ id: 'dimension-0', name: '', description: '', weight: '1' }]
  } catch {
    return [{ id: 'dimension-0', name: '', description: '', weight: '1' }]
  }
}

function parseDimensionsRows(rows: DimensionRow[]): Array<Record<string, unknown>> {
  return rows.map((row) => {
    const name = row.name.trim()
    if (!name) {
      throw new Error('dimension name is required')
    }
    const weight = Number(row.weight)
    if (!Number.isFinite(weight)) {
      throw new Error('dimension weight is required')
    }
    return {
      name,
      description: row.description.trim(),
      weight,
    }
  })
}

function buildGoldenSampleCreateBody(input: { payload: string, expectedAnswer: string, expectedVerdict: string, notes: string, aiPromptId: number | null }) {
  const payload = normalizeJSONInput(input.payload, 'payload')
  const expectedAnswer = normalizeJSONInput(input.expectedAnswer, 'expected_answer')
  const fields = [
    `"payload":${payload}`,
    `"expected_answer":${expectedAnswer}`,
    `"expected_verdict":${JSON.stringify(input.expectedVerdict)}`,
  ]
  const notes = input.notes.trim()
  if (notes) {
    fields.push(`"notes":${JSON.stringify(notes)}`)
  }
  if (input.aiPromptId) {
    fields.push(`"ai_prompt_id":${input.aiPromptId}`)
  }
  return `{${fields.join(',')}}`
}

function buildAIDryRunBody(payloadInput: string, answerInput: string) {
  const payload = normalizeJSONInput(payloadInput, 'payload')
  const answer = normalizeJSONInput(answerInput, 'answer')
  return `{"payload":${payload},"answer":${answer}}`
}

function goldenSampleResultToRunRow(sample: GoldenSample, result: AIDryRunResult): GoldenSampleRunRow {
  return {
    sampleId: sample.id,
    expectedVerdict: sample.expectedVerdict,
    status: 'succeeded',
    actualVerdict: result.result.verdict,
    matchedExpected: result.matchedExpected,
    score: result.result.overall_score,
    provider: result.provider,
    model: result.result.model,
    reason: result.result.reason,
    dryRunId: result.dryRunId,
  }
}

function dryRunHistorySampleID(value: string) {
  if (value === 'all') return null
  const sampleID = Number(value)
  return Number.isFinite(sampleID) && sampleID > 0 ? sampleID : null
}

function normalizeJSONInput(raw: string, label: string) {
  const trimmed = raw.trim()
  if (!trimmed || trimmed === 'null') {
    throw new Error(`${label} is required`)
  }
  JSON.parse(trimmed)
  return trimmed
}

function formatCompactJSON(value: unknown) {
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}

function formatDateTime(value: string) {
  if (!value) return '-'
  const time = new Date(value)
  if (Number.isNaN(time.getTime())) return value
  return time.toLocaleString()
}

function formatMatched(value: boolean | null | undefined) {
  if (value === true) return 'matched'
  if (value === false) return 'mismatch'
  return '-'
}

function verdictStatus(verdict: string) {
  if (verdict === 'pass') return 'approved'
  if (verdict === 'reject') return 'rejected'
  return 'revising'
}

function summarizeDryRunHistory(runs: AIDryRunHistoryItem[]) {
  const summary = runs.reduce((current, run) => {
    current.total += 1
    if (run.matchedExpected === true) {
      current.matched += 1
    } else if (run.matchedExpected === false) {
      current.mismatch += 1
    }
    if (run.status === 'failed') {
      current.failed += 1
    }
    if (typeof run.result?.overall_score === 'number' && Number.isFinite(run.result.overall_score)) {
      current.scoreTotal += run.result.overall_score
      current.scored += 1
    }
    return current
  }, { total: 0, matched: 0, mismatch: 0, failed: 0, scoreTotal: 0, scored: 0 })
  return {
    ...summary,
    matchRate: summary.total > 0 ? Math.round((summary.matched / summary.total) * 100) : null,
    averageScore: summary.scored > 0 ? Math.round(summary.scoreTotal / summary.scored) : null,
  }
}

function formatPercent(value: number | null) {
  return value === null ? '-%' : `${value}%`
}

function formatOptionalNumber(value: number | null) {
  return value === null ? '-' : String(value)
}

function formatDryRunGuardState(guard: AIDryRunGuardStatus) {
  if (guard.state === 'disabled') return 'disabled'
  if (guard.circuitOpen || guard.state === 'circuit_open') return 'circuit open'
  if (guard.state === 'quota_exhausted') return 'quota exhausted'
  if (guard.quotaRemaining !== null) return `${guard.quotaRemaining} quota left`
  return 'ok'
}

function requestedNumberParam(key: string) {
  const raw = new URLSearchParams(window.location.search).get(key)
  if (!raw) {
    return null
  }
  const value = Number(raw)
  return Number.isInteger(value) && value > 0 ? value : null
}

const panelStyle: React.CSSProperties = {
  background: 'var(--lh-bg-card)',
  border: '1px solid var(--lh-border)',
  borderRadius: 'var(--radius-lg)',
  padding: 'var(--space-xl)',
  boxShadow: 'var(--shadow-md)',
}

const headingStyle: React.CSSProperties = {
  fontFamily: 'var(--lh-font-sans)',
  fontSize: 'var(--text-h2)',
  margin: 0,
  color: 'var(--lh-text-1)',
  fontWeight: 600,
}

const subHeadingStyle: React.CSSProperties = {
  fontFamily: 'var(--lh-font-sans)',
  fontSize: '1.1rem',
  margin: 0,
  color: 'var(--lh-text-1)',
  fontWeight: 600,
}

const aiPromptSectionStyle: React.CSSProperties = {
  marginTop: 'var(--space-2xl)',
}

const controlStripStyle: React.CSSProperties = {
  display: 'grid',
  gridTemplateColumns: 'repeat(auto-fit, minmax(150px, 1fr))',
  gap: 'var(--space-sm)',
  marginBottom: 'var(--space-lg)',
  padding: 'var(--space-sm)',
  border: '1px solid var(--lh-border)',
  borderRadius: 'var(--radius-lg)',
  background: 'var(--lh-bg-elev)',
}

const metricCellStyle: React.CSSProperties = {
  minWidth: 0,
  padding: 'var(--space-md)',
  border: '1px solid var(--lh-border)',
  borderRadius: 'var(--radius-md)',
  background: 'var(--lh-bg-card)',
}

const metricLabelStyle: React.CSSProperties = {
  fontFamily: 'var(--lh-font-mono)',
  fontSize: 10,
  color: 'var(--lh-text-3)',
}

const metricValueStyle: React.CSSProperties = {
  marginTop: 4,
  fontWeight: 700,
  fontSize: 'var(--text-h2)',
}

const metricDetailStyle: React.CSSProperties = {
  marginTop: 2,
  color: 'var(--lh-text-2)',
  fontSize: 'var(--text-sm)',
  whiteSpace: 'nowrap',
  overflow: 'hidden',
  textOverflow: 'ellipsis',
}

const aiSettingsRowStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'space-between',
  gap: 'var(--space-md)',
}

const formGridStyle: React.CSSProperties = {
  display: 'grid',
  gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))',
  gap: 'var(--space-lg)',
}

const configEditorGridStyle: React.CSSProperties = {
  display: 'grid',
  gridTemplateColumns: 'repeat(auto-fit, minmax(260px, 1fr))',
  gap: 'var(--space-lg)',
}

const dryRunGridStyle: React.CSSProperties = {
  display: 'grid',
  gridTemplateColumns: 'repeat(auto-fit, minmax(260px, 1fr))',
  gap: 'var(--space-lg)',
}

const fieldStyle: React.CSSProperties = {
  display: 'grid',
  gap: 8,
  color: 'var(--lh-text-2)',
  fontSize: 'var(--text-sm)',
}

const compactFieldStyle: React.CSSProperties = {
  ...fieldStyle,
  gap: 4,
}

const dimensionPanelStyle: React.CSSProperties = {
  display: 'grid',
  gap: 'var(--space-md)',
  padding: 'var(--space-md)',
  border: '1px solid var(--lh-border)',
  borderRadius: 'var(--radius-md)',
  background: 'var(--lh-bg)',
}

const dimensionRowStyle: React.CSSProperties = {
  display: 'grid',
  gridTemplateColumns: 'minmax(120px, 0.9fr) minmax(160px, 1.5fr) minmax(90px, 0.5fr) auto',
  gap: 'var(--space-sm)',
  alignItems: 'end',
}

const inputStyle: React.CSSProperties = {
  width: '100%',
  minHeight: 40,
  border: '1px solid var(--lh-border-strong)',
  borderRadius: 'var(--radius-sm)',
  padding: '0 var(--space-md)',
  boxSizing: 'border-box',
  fontSize: 'var(--text-base)',
  transition: 'border-color var(--duration-fast)',
}

const textareaStyle: React.CSSProperties = {
  ...inputStyle,
  minHeight: 100,
  padding: 'var(--space-sm) var(--space-md)',
  resize: 'vertical',
  fontFamily: 'var(--lh-font-sans)',
  lineHeight: 1.5,
}

const dryRunPanelStyle: React.CSSProperties = {
  marginTop: 'var(--space-xl)',
}

const dryRunResultStyle: React.CSSProperties = {
  marginTop: 'var(--space-lg)',
  padding: 'var(--space-lg)',
}

const verdictPillStyle: React.CSSProperties = {
  padding: '2px 8px',
  borderRadius: 'var(--radius-sm)',
  background: 'var(--lh-bg-elev)',
  border: '1px solid var(--lh-border)',
  fontFamily: 'var(--lh-font-mono)',
  fontSize: 'var(--text-sm)',
  fontWeight: 700,
}

const goldenSampleSectionStyle: React.CSSProperties = {
  marginTop: 'var(--space-xl)',
}

const goldenSampleListStyle: React.CSSProperties = {
  display: 'grid',
  gap: 'var(--space-md)',
  marginTop: 'var(--space-lg)',
}

const goldenSampleItemStyle: React.CSSProperties = {
  transition: 'transform var(--duration-fast)',
}

const goldenSampleHeaderStyle: React.CSSProperties = {
  display: 'flex',
  flexWrap: 'wrap',
  gap: 'var(--space-sm)',
  alignItems: 'center',
  justifyContent: 'space-between',
}

const goldenSampleActionsStyle: React.CSSProperties = {
  display: 'flex',
  gap: 'var(--space-sm)',
}

const compactPreviewStyle: React.CSSProperties = {
  margin: 0,
  padding: 'var(--space-sm)',
  background: 'var(--lh-bg)',
  borderRadius: 'var(--radius-sm)',
  border: '1px solid var(--lh-border)',
  maxHeight: 120,
  overflow: 'auto',
  whiteSpace: 'pre-wrap',
  color: 'var(--lh-text-2)',
}

const resultTableWrapStyle: React.CSSProperties = {
  marginTop: 'var(--space-lg)',
  overflowX: 'auto',
  borderRadius: 'var(--radius-md)',
  border: '1px solid var(--lh-border)',
}

const resultTableStyle: React.CSSProperties = {
  width: '100%',
  borderCollapse: 'collapse',
  fontSize: 'var(--text-sm)',
  background: 'white',
}

const resultCellStyle: React.CSSProperties = {
  borderBottom: '1px solid var(--lh-border)',
  padding: 'var(--space-md)',
  textAlign: 'left',
  verticalAlign: 'top',
}

const historyPanelStyle: React.CSSProperties = {
  marginTop: 'var(--space-2xl)',
}

const historySummaryStyle: React.CSSProperties = {
  display: 'flex',
  flexWrap: 'wrap',
  gap: 'var(--space-md)',
  marginTop: 'var(--space-md)',
  color: 'var(--lh-text-2)',
  fontSize: 'var(--text-sm)',
}

const alertStyle: React.CSSProperties = {
  padding: 'var(--space-md)',
  borderRadius: 'var(--radius-md)',
  border: '1px solid var(--lh-danger)',
  background: '#fff1f0',
  color: 'var(--lh-danger)',
}

const errorTextStyle: React.CSSProperties = {
  color: 'var(--lh-danger)',
  margin: 'var(--space-sm) 0 0',
}

const mutedStyle: React.CSSProperties = {
  color: 'var(--lh-text-3)',
  fontSize: 'var(--text-sm)',
}

const taskChipStyle: React.CSSProperties = {
  padding: 'var(--space-xs) var(--space-md)',
  borderRadius: 'var(--radius-md)',
  border: '1px solid var(--lh-border)',
  background: 'var(--lh-bg-card)',
  color: 'var(--lh-text-2)',
  fontSize: 'var(--text-sm)',
  cursor: 'pointer',
}

const activeTaskChipStyle: React.CSSProperties = {
  ...taskChipStyle,
  borderColor: 'var(--lh-primary)',
  background: 'var(--lh-primary-soft)',
  color: 'var(--lh-primary)',
  fontWeight: 600,
}

const templateDesignerLinkStyle: React.CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  justifyContent: 'center',
  minHeight: 32,
  padding: '0 var(--space-md)',
  borderRadius: 'var(--radius-sm)',
  background: 'var(--lh-primary)',
  color: '#fff',
  fontFamily: 'var(--lh-font-sans)',
  fontSize: 'var(--text-sm)',
  fontWeight: 600,
  textDecoration: 'none',
}
