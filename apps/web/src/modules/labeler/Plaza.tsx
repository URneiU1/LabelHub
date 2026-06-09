import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Button, Toast } from '@douyinfe/semi-ui'
import { SchemaRenderer, parseAnswer, parseBundleSchema } from '../../renderer'
import type { AnswerValue, TemplateSchema, ValidationError } from '../../renderer/types'
import { validateAnswer } from '../../renderer/validator'
import { apiGet, apiPost, claimTask, listMyTasks, type AuditLog, type LabelerTaskItem, type LabelerTaskItems, type MyTask, type Submission, type Task, type TaskBundle } from '../../shared/api/client'
import EmptyState from '../../shared/components/EmptyState'
import { parsePayload } from '../../shared/components/payload'
import { setLabelerSection } from '../../shared/state/labelerSection'
import { normalizeStatus } from '../../shared/components/status'
import StatusBadge from '../../shared/components/StatusBadge'
import ItemNav from './ItemNav'
import MyData from './MyData'
import TaskPlaza from './TaskPlaza'
import { buildDraftKey, discardLocalDraft, isLocalDraftNewer, loadLocalDraft, markSynced, saveLocalDraft } from './offlineDraftStore'
import '../../styles/lh/workbench.css'
import '../../styles/lh/tasks.css'
import './plaza.css'

type View = 'plaza' | 'answer'
type PlazaTab = 'tasks' | 'mydata'

interface LabelerPlazaProps {
  initialView?: View
  initialPlazaTab?: PlazaTab
}

// 初始视图由路由经 props 注入(侧栏三入口用不同 key 重挂载):任务广场 / 标注工作台 / 我的贡献。
// 用 props 而非读 window.location,既不破坏「不包 Router 的测试」,也不打断 React Compiler 的记忆化。
export default function LabelerPlaza({ initialView = 'plaza', initialPlazaTab = 'tasks' }: LabelerPlazaProps = {}) {
  const [tasks, setTasks] = useState<Task[]>([])
  const [mySubmissions, setMySubmissions] = useState<Submission[]>([])
  // 已领取的任务(大任务粒度 + 我的进度):任务广场「已领取的任务」区块 + 工作台大任务切换器共用。
  const [myTasks, setMyTasks] = useState<MyTask[]>([])
  const [bundle, setBundle] = useState<TaskBundle | null>(null)
  const [answer, setAnswer] = useState<AnswerValue>({})
  const [errors, setErrors] = useState<ValidationError[]>([])
  const [loading, setLoading] = useState(false)
  const [autoSaveState, setAutoSaveState] = useState<'idle' | 'saving' | 'saved' | 'failed'>('idle')
  const lastSavedDraftKey = useRef('')
  const autoSaveSeq = useRef(0)
  // 标注工作台直达时只尝试一次「自动恢复进行中任务」,避免随依赖变化或重复加载反复触发。
  const didAutoResumeRef = useRef(false)

  // P3 离线草稿:localDraftSaved 表示自动保存网络失败但本地草稿已留底(非阻塞提示);
  // recoverableDraft 是「本地有比服务端更新的未同步草稿」时供用户恢复/丢弃的待恢复答案。
  const [localDraftSaved, setLocalDraftSaved] = useState(false)
  const [recoverableDraft, setRecoverableDraft] = useState<AnswerValue | null>(null)

  // 常驻「AI 求助」:不依赖模板是否配置 LLMTrigger 字段,作答页底部固定入口。
  const [assistOpen, setAssistOpen] = useState(false)
  const [assistLoading, setAssistLoading] = useState(false)
  const [assistText, setAssistText] = useState('')
  const [assistError, setAssistError] = useState('')

  // 4.3 视图编排:plaza(任务广场 + 我的数据)与 answer(三列作答页)切换。初始值由路由经 props 注入。
  const [view, setView] = useState<View>(initialView)
  const [plazaTab] = useState<PlazaTab>(initialPlazaTab)
  const [activeTask, setActiveTask] = useState<Task | null>(null)
  const [itemNav, setItemNav] = useState<LabelerTaskItems | null>(null)
  const [itemNavLoading, setItemNavLoading] = useState(false)

  const loadTasks = useCallback(async () => {
    try {
      const data = await apiGet<Task[]>('/labeler/tasks')
      setTasks(data)
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '加载任务失败')
    }
  }, [])

  const loadMySubmissions = useCallback(async (): Promise<Submission[]> => {
    try {
      const data = await apiGet<Submission[]>('/me/submissions')
      setMySubmissions(data)
      return data
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '加载我的提交失败')
      return []
    }
  }, [])

  const loadMyTasks = useCallback(async () => {
    try {
      setMyTasks(await listMyTasks())
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '加载已领取的任务失败')
    }
  }, [])

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void loadTasks()
    // 「已领取的任务」两个视图都要用(广场区块 + 工作台切换器),无条件加载。
    void loadMyTasks()
    // 标注工作台(answer)由下方「自动恢复进行中任务」effect 自行拉取我的提交,避免重复请求。
    if (initialView !== 'answer') {
      void loadMySubmissions()
    }
  }, [initialView, loadMyTasks, loadMySubmissions, loadTasks])

  // 把当前「实际显示」的分节同步给左栏高亮:作答页(有激活任务/题目)→ 标注工作台;
  // 否则按 plazaTab → 任务广场 / 我的贡献。这样「继续标注 / 返回任务广场」只切内部 view 时,
  // 侧栏也跟着对(不依赖 URL 变化)。setLabelerSection 是外部 store,非 React setState。
  useEffect(() => {
    const showingWorkbench = view === 'answer' && (activeTask !== null || bundle !== null || loading)
    setLabelerSection(showingWorkbench ? 'workbench' : (plazaTab === 'mydata' ? 'mine' : 'tasks'))
  }, [view, activeTask, bundle, loading, plazaTab])

  const schema = useMemo(() => parseBundleSchema(bundle, '当前任务未配置标注模板'), [bundle])
  const payload = useMemo(() => parsePayload(bundle?.item?.payload), [bundle?.item?.payload])

  // 题目导航数据(新端点)。失败时降级:返回 null,作答页仍可用领取兜底导航。
  const loadItemNav = useCallback(async (taskId: number): Promise<LabelerTaskItems | null> => {
    setItemNavLoading(true)
    try {
      const data = await apiGet<LabelerTaskItems>(`/tasks/${taskId}/labeler/items`)
      setItemNav(data)
      return data
    } catch (error) {
      setItemNav(null)
      Toast.error(error instanceof Error ? error.message : '加载题目导航失败,已切换为仅领取模式')
      return null
    } finally {
      setItemNavLoading(false)
    }
  }, [])

  // P3:载入某题后,若本地存在比服务端更新的未同步草稿(且模板版本一致),
  // 暂存为待恢复草稿(非阻塞,由作答页横幅让用户「恢复 / 丢弃」)。每次载题先清旧的离线状态。
  const detectRecoverableDraft = useCallback((data: TaskBundle, serverAnswer: AnswerValue) => {
    setLocalDraftSaved(false)
    setRecoverableDraft(computeRecoverableDraft(data, serverAnswer))
  }, [])

  const claim = useCallback(async (taskId: number) => {
    setLoading(true)
    try {
      const data = await apiPost<TaskBundle>(`/tasks/${taskId}/claim`, {})
      setBundle(data)
      const nextAnswer = parseAnswer(data.revision?.answer)
      autoSaveSeq.current += 1
      lastSavedDraftKey.current = answerDraftKey(nextAnswer)
      setAnswer(nextAnswer)
      setErrors([])
      setAutoSaveState('idle')
      detectRecoverableDraft(data, nextAnswer)
      Toast.success('已领取题目')
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '领取失败')
    } finally {
      setLoading(false)
    }
  }, [detectRecoverableDraft])

  const openSubmission = useCallback(async (submission: Submission) => {
    setLoading(true)
    try {
      const data = await apiGet<TaskBundle>(`/tasks/${submission.taskId}/items/${submission.itemId}`)
      setBundle(data)
      const nextAnswer = parseAnswer(data.revision?.answer)
      autoSaveSeq.current += 1
      lastSavedDraftKey.current = answerDraftKey(nextAnswer)
      setAnswer(nextAnswer)
      setErrors([])
      setAutoSaveState('idle')
      detectRecoverableDraft(data, nextAnswer)
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '加载待修改任务失败')
    } finally {
      setLoading(false)
    }
  }, [detectRecoverableDraft])

  const answerKey = useMemo(() => answerDraftKey(answer), [answer])
  const bundleTaskId = bundle?.task?.id ?? null
  const bundleItemId = bundle?.item?.id ?? null
  const bundleSubmissionId = bundle?.submission?.id ?? null
  const bundleRevisionNo = bundle?.submission?.currentRevisionId ?? bundle?.revision?.id ?? null

  // 当前题目对应的本地草稿键 + 模板版本(autosave / 提交 / 恢复共用)。bundle 缺失时为 null。
  const localDraftKey = useMemo(() => {
    if (bundleTaskId == null || bundleItemId == null) {
      return null
    }
    return buildDraftKey({
      taskId: bundleTaskId,
      itemId: bundleItemId,
      submissionId: bundleSubmissionId,
      revisionNo: bundleRevisionNo,
    })
  }, [bundleItemId, bundleRevisionNo, bundleSubmissionId, bundleTaskId])
  const templateVersion = bundle?.template?.version ?? null

  const saveDraft = useCallback(async () => {
    if (!bundle?.task || !bundle.item) {
      return
    }
    try {
      autoSaveSeq.current += 1
      const data = await apiPost<Submission>(`/tasks/${bundle.task.id}/items/${bundle.item.id}/draft`, { answer })
      setBundle({ ...bundle, submission: data })
      lastSavedDraftKey.current = answerDraftKey(answer)
      setAutoSaveState('saved')
      Toast.success('草稿已保存')
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '保存失败')
    }
  }, [answer, bundle])

  const submit = useCallback(async () => {
    if (!bundle?.task || !bundle.item) {
      return
    }
    if (!schema.ok) {
      Toast.error(schema.message)
      return
    }
    // P3:本阶段最终提交仅限在线。离线时阻断提交并友好提示,保留本地草稿(不清理)。
    if (!navigator.onLine) {
      if (localDraftKey) {
        saveLocalDraft(localDraftKey, answer, templateVersion)
      }
      setLocalDraftSaved(true)
      Toast.error('当前网络已断开,无法提交审核;你的答案已保存在本地草稿,联网后可再提交。')
      return
    }
    const validationErrors = validateAnswer(schema.schema, answer)
    setErrors(validationErrors)
    if (validationErrors.length > 0) {
      Toast.error(validationErrors[0].message)
      return
    }

    try {
      const data = await apiPost<Submission>(`/tasks/${bundle.task.id}/items/${bundle.item.id}/submit`, { answer })
      setBundle({ ...bundle, submission: data })
      autoSaveSeq.current += 1
      lastSavedDraftKey.current = answerDraftKey(answer)
      setAutoSaveState('idle')
      // 提交成功:服务端已收下本次答案 → 本地草稿标记已同步,清掉离线/恢复提示。
      const submittedKey = bundleDraftKey(bundle)
      if (submittedKey) {
        markSynced(submittedKey)
      }
      setLocalDraftSaved(false)
      setRecoverableDraft(null)
      void loadMySubmissions()
      Toast.success(`已提交，状态 ${data.status}`)
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '提交失败')
    }
  }, [answer, bundle, loadMySubmissions, localDraftKey, schema, templateVersion])

  // Ctrl/Cmd+Enter 提交:用 ref 持有最新 submit,只注册一次监听,避免随 answer 频繁重挂。
  // submit 内部已对无 bundle / schema 错误 / 校验失败兜底,这里无需重复判断。
  const submitRef = useRef(submit)
  useEffect(() => {
    submitRef.current = submit
  }, [submit])
  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if ((event.metaKey || event.ctrlKey) && event.key === 'Enter') {
        event.preventDefault()
        void submitRef.current()
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [])

  // 调 /llm/inline 求助:把当前 schema 摘要 + 题目数据 + 当前答案作为不可信数据发给后端(后端再转豆包)。
  // 不阻塞作答:用独立 loading,失败给中文友好提示,不打断表单填写。
  const askAssist = useCallback(async () => {
    if (!bundle?.item) {
      return
    }
    setAssistOpen(true)
    setAssistLoading(true)
    setAssistError('')
    try {
      const schemaSummary = schema.ok ? summarizeSchema(schema.schema) : '当前任务未配置可解析的标注模板。'
      const data = await apiPost<{ text: string, provider: string }>('/llm/inline', {
        prompt: '我在这道标注题上遇到困难,请结合题目要求给我答题思路和需要重点核对的点。',
        input: { schema: schemaSummary, payload, answer },
      })
      setAssistText(data.text)
    } catch (error) {
      setAssistError(error instanceof Error ? error.message : 'AI 求助暂时不可用,请稍后再试。')
    } finally {
      setAssistLoading(false)
    }
  }, [answer, bundle?.item, payload, schema])

  useEffect(() => {
    if (bundleTaskId == null || bundleItemId == null || !schema.ok || answerKey === lastSavedDraftKey.current) {
      return
    }
    const taskId = bundleTaskId
    const itemId = bundleItemId
    const timer = window.setTimeout(() => {
      // 在触发时捕获当前活动序号(已由最近一次 onChange / 手动保存推进),且不在定时器里改写它;
      // 任何更晚的改动或保存都会推进序号,使本次 autosave 的回调因序号不等而作废,
      // 杜绝旧闭包里的 answer 覆盖更新的 lastSavedDraftKey。
      const requestSeq = autoSaveSeq.current
      setAutoSaveState('saving')
      // P3:先把本地草稿落盘(标记未同步),网络调用失败也不丢这次改动。
      const draftKey = localDraftKey
      if (draftKey) {
        saveLocalDraft(draftKey, answer, templateVersion)
      }
      void apiPost<Submission>(`/tasks/${taskId}/items/${itemId}/draft`, { answer })
        .then((submission) => {
          if (autoSaveSeq.current !== requestSeq) {
            return
          }
          lastSavedDraftKey.current = answerKey
          setBundle((current) => current && current.task.id === taskId && current.item?.id === itemId ? { ...current, submission } : current)
          setAutoSaveState('saved')
          setLocalDraftSaved(false)
          // 服务端确认本次答案 → 本地草稿标记已同步。
          if (draftKey) {
            markSynced(draftKey)
          }
        })
        .catch(() => {
          if (autoSaveSeq.current === requestSeq) {
            setAutoSaveState('failed')
            // 网络失败:保留本地草稿,亮非阻塞「本地草稿已保存」状态。
            setLocalDraftSaved(true)
          }
        })
    }, 3000)
    return () => window.clearTimeout(timer)
  }, [answer, answerKey, bundleItemId, bundleTaskId, schema, localDraftKey, templateVersion])

  // ---- 4.3 新增导航编排(不触碰上方既有 claim/saveDraft/submit/autosave/快捷键逻辑) ----

  // 打开导航里"我的题"(已领/有提交),镜像 openSubmission,但用 itemId 直接定位。
  const openByItem = useCallback(async (taskId: number, itemId: number) => {
    setLoading(true)
    try {
      const data = await apiGet<TaskBundle>(`/tasks/${taskId}/items/${itemId}`)
      setBundle(data)
      const nextAnswer = parseAnswer(data.revision?.answer)
      autoSaveSeq.current += 1
      lastSavedDraftKey.current = answerDraftKey(nextAnswer)
      setAnswer(nextAnswer)
      setErrors([])
      setAutoSaveState('idle')
      // P3:与 claim/openSubmission 一致地检测可恢复的本地草稿(用稳定 setter + 纯函数,无需进依赖)。
      setLocalDraftSaved(false)
      setRecoverableDraft(computeRecoverableDraft(data, nextAnswer))
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '加载题目失败')
    } finally {
      setLoading(false)
    }
  }, [])

  // 进入某任务作答页。领取粒度取决于分发策略:
  //  - quota(配额抢单):按题领取,领下一道可用题并打开。
  //  - first_come / assigned(独占):整体领取大任务 → 所有题一次性解锁,再打开第一道可做的题。
  const enterTask = useCallback((task: Task) => {
    setActiveTask(task)
    setView('answer')
    if (task.distribution === 'quota') {
      void loadItemNav(task.id)
      void claim(task.id)
      return
    }
    void (async () => {
      try {
        await claimTask(task.id)
      } catch (error) {
        // 领取失败(如已被他人独占)→ 退回任务广场并提示。
        Toast.error(error instanceof Error ? error.message : '领取任务失败')
        setView('plaza')
        setActiveTask(null)
        void loadMyTasks()
        return
      }
      const nav = await loadItemNav(task.id)
      const workable = nav?.items?.find((item) => item.status === 'claimed' || item.status === 'draft' || item.status === 'revising') ?? nav?.items?.[0]
      if (workable) {
        void openByItem(task.id, workable.itemId)
      }
      void loadMyTasks()
    })()
  }, [claim, loadItemNav, loadMyTasks, openByItem])

  // 选中导航里的一题:我的题(mine 或有 submissionId)→ 直接打开;available/他人 → 领取下一题。
  const selectNavItem = useCallback((item: LabelerTaskItem) => {
    if (!activeTask) {
      return
    }
    if (item.mine || item.submissionId != null) {
      void openByItem(activeTask.id, item.itemId)
    } else {
      void claim(activeTask.id)
    }
  }, [activeTask, claim, openByItem])

  // 上一题/下一题:在题目导航列表里相对当前题移动;我的题直接打开,否则领取下一题。
  const stepItem = useCallback((delta: number) => {
    const items = itemNav?.items ?? []
    if (!activeTask || items.length === 0) {
      void claim(activeTask?.id ?? 0)
      return
    }
    const currentIndex = items.findIndex((item) => item.itemId === bundle?.item?.id)
    const nextIndex = Math.min(items.length - 1, Math.max(0, (currentIndex < 0 ? 0 : currentIndex) + delta))
    if (nextIndex === currentIndex) {
      return
    }
    selectNavItem(items[nextIndex])
  }, [activeTask, bundle?.item?.id, claim, itemNav, selectNavItem])

  // 跳过:跳到当前题之后第一道「我的、还没提交」的题(claimed/draft/revising),纯导航不重新领取
  // (整体领取后所有题已是我的,没有"下一道待领题",再 claim 只会跳回第一题并误弹「已领取题目」)。
  // 仅当配额任务、且确有 available 题时,跳过才真去领下一道。
  const skipItem = useCallback(() => {
    if (!activeTask) {
      return
    }
    const items = itemNav?.items ?? []
    const isWorkableMine = (item: LabelerTaskItem) =>
      (item.mine || item.submissionId != null) && (item.status === 'claimed' || item.status === 'draft' || item.status === 'revising')
    const currentIndex = items.findIndex((item) => item.itemId === bundle?.item?.id)
    const next = items.slice(currentIndex + 1).find(isWorkableMine) ?? items.find(isWorkableMine)
    if (next && next.itemId !== bundle?.item?.id) {
      void openByItem(activeTask.id, next.itemId)
      return
    }
    if (activeTask.distribution === 'quota' && items.some((item) => item.status === 'available')) {
      void claim(activeTask.id)
      return
    }
    Toast.info('没有更多可做的题了')
  }, [activeTask, bundle?.item?.id, claim, itemNav, openByItem])

  // 从"我的数据"打开一条提交:进入作答页,记录活动任务并加载题目导航,再复用既有 openSubmission。
  const openFromMyData = useCallback((submission: Submission) => {
    const matched = tasks.find((task) => task.id === submission.taskId)
    setActiveTask(matched ?? { id: submission.taskId, title: `任务 #${submission.taskId}`, description: null, baselineDescription: null, status: 'published', totalItems: 0, finishedItems: 0 })
    setView('answer')
    void loadItemNav(submission.taskId)
    void openSubmission(submission)
  }, [loadItemNav, openSubmission, tasks])

  // 进入某个「已领取的任务」继续标注:记录活动任务、加载题目导航;有进行中题(resumeItemId)直接恢复,
  // 否则领下一题。任务广场「继续标注」与工作台大任务切换器共用。
  // first_come / assigned 任务进入时幂等地整体领取一次,确保该任务所有题都解锁(不止恢复的那道)。
  const openTask = useCallback((myTask: MyTask) => {
    const task = myTask.task
    setActiveTask(task)
    setView('answer')
    void (async () => {
      if (task.distribution !== 'quota') {
        try {
          await claimTask(task.id)
        } catch {
          // 已拥有 / 被他人独占 → 忽略,按 resume 继续。
        }
      }
      void loadItemNav(task.id)
      if (myTask.resumeItemId != null) {
        void openByItem(task.id, myTask.resumeItemId)
      } else {
        void claim(task.id)
      }
      void loadMyTasks()
    })()
  }, [claim, loadItemNav, loadMyTasks, openByItem])

  // 工作台大任务切换器:切到我领取的另一个大任务(走 openTask 恢复/领题)。
  const switchToTask = useCallback((taskId: number) => {
    const target = myTasks.find((myTask) => myTask.task.id === taskId)
    if (target) {
      openTask(target)
    }
  }, [myTasks, openTask])

  // 标注工作台直达(侧栏「标注工作台」,initialView='answer')时,自动恢复该 labeler 最近一条
  // 「进行中」(draft 已领未提交 / revising 被打回待重做)的提交。没有这一步,领题后从侧栏进入工作台
  // 会因重挂载丢失内存里的活动任务而看不到题目,导致反复重领同一题。无可恢复项时不做事,
  // 由 isEmptyWorkbench 兜底回落到任务广场。仅在挂载后跑一次(didAutoResumeRef 守卫)。
  useEffect(() => {
    if (initialView !== 'answer' || didAutoResumeRef.current) {
      return
    }
    didAutoResumeRef.current = true
    void loadMySubmissions().then((subs) => {
      const resumable = subs.find((submission) => isResumable(submission.status))
      if (resumable) {
        openFromMyData(resumable)
      }
    })
  }, [initialView, loadMySubmissions, openFromMyData])

  const backToPlaza = useCallback(() => {
    setView('plaza')
    setBundle(null)
    setActiveTask(null)
    setItemNav(null)
    setAnswer({})
    setErrors([])
    setAutoSaveState('idle')
    autoSaveSeq.current += 1
    lastSavedDraftKey.current = ''
    setLocalDraftSaved(false)
    setRecoverableDraft(null)
    setAssistOpen(false)
    setAssistText('')
    setAssistError('')
    void loadTasks()
    void loadMySubmissions()
    void loadMyTasks()
  }, [loadMyTasks, loadMySubmissions, loadTasks])

  // P3:把本地草稿恢复为当前答案。沿用 onChange 的同步推进:推进 autoSaveSeq、置 idle 触发后续自动保存。
  function restoreLocalDraft() {
    if (!recoverableDraft) {
      return
    }
    autoSaveSeq.current += 1
    setAnswer(recoverableDraft)
    setAutoSaveState('idle')
    setRecoverableDraft(null)
    if (schema.ok && errors.length > 0) {
      setErrors(validateAnswer(schema.schema, recoverableDraft))
    }
    Toast.success('已恢复本地草稿')
  }

  // P3:丢弃当前题目的本地草稿(让离线恢复可逆)。清掉待恢复 + 离线提示。
  function discardCurrentLocalDraft() {
    if (localDraftKey) {
      discardLocalDraft(localDraftKey)
    }
    setRecoverableDraft(null)
    setLocalDraftSaved(false)
    Toast.info('已丢弃本地草稿')
  }

  // 切题时清空上一题的 AI 求助结果,避免建议串题。与既有 autosave 效果同样的同步置态模式。
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setAssistOpen(false)
    setAssistText('')
    setAssistError('')
  }, [bundle?.item?.id])

  // 提交/保存后刷新导航状态 + 我的任务进度,保持左侧进度、状态点与大任务切换器同步。
  // 仅在 bundle.submission?.status 变化时拉取(领题/提交后状态变,切换器也能纳入刚领取的任务)。
  useEffect(() => {
    if (view === 'answer' && activeTask) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      void loadItemNav(activeTask.id)
      void loadMyTasks()
    }
  }, [activeTask, bundle?.submission?.status, loadItemNav, loadMyTasks, view])

  // 标注工作台(initialView='answer')在没有激活任务时,不停在"准备开始标注"的空作答页,
  // 直接回落到任务广场,让用户先领题(避免侧栏直达工作台时进入死胡同)。
  const isEmptyWorkbench = view === 'answer' && !activeTask && !bundle && !loading
  if (view === 'plaza' || isEmptyWorkbench) {
    return (
      <div className="lz-shell">
        <div className="lh-page-head lh-hflex" style={{ alignItems: 'flex-start' }}>
          <div>
            <h1 className="lh-page-head__title">{plazaTab === 'mydata' ? '我的贡献' : '任务广场'}</h1>
            <div className="lh-page-head__crumb">{plazaTab === 'mydata' ? '查看我的标注记录与状态' : '领取题目开始标注'}</div>
          </div>
        </div>

        {plazaTab === 'tasks' ? (
          <TaskPlaza tasks={tasks} myTasks={myTasks} loading={loading} onEnter={enterTask} onContinue={openTask} />
        ) : (
          <MyData submissions={mySubmissions} onOpen={openFromMyData} />
        )}
      </div>
    )
  }

  return (
    <div className="wb-shell">
      <ItemNav
        nav={itemNav}
        loading={itemNavLoading}
        activeItemId={bundle?.item?.id}
        myTasks={myTasks}
        activeTaskId={activeTask?.id}
        onSwitchTask={switchToTask}
        onSelect={selectNavItem}
        onBack={backToPlaza}
      />

      <main className="wb-main">
        {bundle?.item ? (
          <>
            <div className="wb-head">
              <div>
                <h1 className="wb-head__title">{schema.ok ? schema.schema.title : '标注表单'}</h1>
                <div className="wb-head__meta">
                  {[`任务 ID ${bundle.task.id}`, `题目 ID ${bundle.item.id}`, bundle.template?.version != null ? `模板 v${bundle.template.version}` : null]
                    .filter(Boolean)
                    .join(' · ')}
                </div>
              </div>
              <div className="wb-head__actions">
                <StatusBadge status={bundle.submission?.status || 'draft'} label={bundle.submission ? undefined : '新题'} />
              </div>
            </div>

            {bundle.latestHumanReview?.reason ? (
              <div className="wb-banner" role="alert">
                <strong>上一轮被打回：</strong>
                {bundle.latestHumanReview.reason}
              </div>
            ) : null}

            {recoverableDraft ? (
              <div className="wb-banner wb-draft-recover" role="status">
                <span className="wb-draft-recover__text">
                  <strong>发现未同步的本地草稿。</strong>
                  上次有改动未成功保存到服务器,要恢复本地草稿吗?
                </span>
                <span className="wb-draft-recover__actions">
                  <Button size="small" theme="solid" onClick={restoreLocalDraft}>恢复本地草稿</Button>
                  <Button size="small" theme="borderless" type="tertiary" onClick={discardCurrentLocalDraft}>丢弃本地草稿</Button>
                </span>
              </div>
            ) : null}

            {schema.ok ? (
              <SchemaRenderer
                schema={schema.schema}
                payload={payload}
                value={answer}
                errors={errors}
                runtime={{ taskId: bundle.task.id, itemId: bundle.item.id, submissionId: bundle.submission?.id }}
                onChange={(next) => {
                  autoSaveSeq.current += 1
                  setAnswer(next)
                  if (answerDraftKey(next) !== lastSavedDraftKey.current) {
                    setAutoSaveState('idle')
                  }
                  if (errors.length > 0) {
                    setErrors(validateAnswer(schema.schema, next))
                  }
                }}
              />
            ) : (
              <div role="alert" className="wb-banner">{schema.message}</div>
            )}

            {assistOpen ? (
              <section className="wb-llm wb-assist" aria-label="AI 求助建议" aria-busy={assistLoading}>
                <div className="wb-llm__head">
                  <span className="wb-llm__title">✦ AI 求助 · 答题建议</span>
                  <button
                    type="button"
                    className="wb-llm__regen"
                    disabled={assistLoading}
                    onClick={() => void askAssist()}
                  >
                    {assistLoading ? '生成中…' : '重新生成'}
                  </button>
                </div>
                {assistLoading ? (
                  <div className="wb-llm__body wb-assist__loading">正在为你分析这道题,请稍候…</div>
                ) : assistError ? (
                  <div className="wb-llm__body wb-assist__error" role="alert">{assistError}</div>
                ) : (
                  <div className="wb-llm__body">{assistText}</div>
                )}
                <div className="wb-assist__note">AI 仅提供思路提示,最终判断仍以你的标注为准。</div>
              </section>
            ) : null}

            <div className="wb-footer">
              <Button theme="light" onClick={() => stepItem(-1)}>← 上一题</Button>
              <Button theme="light" onClick={() => stepItem(1)}>下一题 →</Button>
              <Button theme="light" onClick={skipItem}>跳过</Button>
              <Button theme="light" onClick={() => Toast.info('已记录上报(演示)')}>报告题目</Button>
              <Button theme="borderless" type="tertiary" loading={assistLoading} onClick={() => void askAssist()} title="遇到难题,让 AI 给点思路">✦ 遇到难题 · AI 求助</Button>
              <span className="wb-footer__shortcuts wb-draft-state" role="status">
                {localDraftSaved ? (
                  <>
                    <span className="wb-draft-state__pill" title="网络保存失败,答案已留存在本地草稿">本地草稿已保存 / local draft saved</span>
                    <button type="button" className="wb-draft-state__discard" onClick={discardCurrentLocalDraft}>丢弃本地草稿</button>
                  </>
                ) : (
                  autoSaveText(autoSaveState)
                )}
              </span>
              <Button disabled={!schema.ok} onClick={() => void saveDraft()} theme="light">保存草稿</Button>
              <Button disabled={!schema.ok} theme="solid" onClick={() => void submit()} title="提交审核 (Ctrl/Cmd + Enter)">提交审核</Button>
            </div>
          </>
        ) : (
          <EmptyState title="准备开始标注" body="正在领取题目,或在左侧题目导航选择一题开始工作。" variant="empty" />
        )}
      </main>

      <aside className="wb-right">
        <div className="wb-right__title">我的贡献（本任务）</div>

        {itemNav?.counts ? (
          <div className="wb-stats-card">
            {taskStatCells(itemNav.counts).map((cell) => (
              <div key={cell.label}>
                <div className="wb-stats-cell__label">{cell.label}</div>
                <div className={'wb-stats-cell__value' + (cell.toneClass ? ` ${cell.toneClass}` : '')}>{cell.value}</div>
              </div>
            ))}
          </div>
        ) : null}

        <div className="wb-history">
          <div className="wb-history__head">本题历史</div>
          {bundle?.auditLogs && bundle.auditLogs.length > 0 ? (
            bundle.auditLogs.map((log) => (
              <div key={log.id} className="wb-history__item">
                <div>
                  <div className={historyActionClass(log)}>{auditLogLabel(log)}</div>
                </div>
                <span className="wb-history__date">{formatAuditDate(log.createdAt)}</span>
              </div>
            ))
          ) : (
            <div className="wb-history__item">
              <div className="wb-history__action">{bundle?.item ? '暂无历史记录' : '领取题目后显示历史'}</div>
            </div>
          )}
        </div>

        <div className="wb-shortcuts">
          <div className="wb-shortcuts__title">快捷键</div>
          <div className="wb-shortcuts__row">
            <span className="wb-kbd">⌘/Ctrl + Enter</span> 提交审核
          </div>
        </div>
      </aside>
    </div>
  )
}

// 把导航 counts 映射为右侧"我的贡献(本任务)"卡片,仅展示有真实数值的桶。
function taskStatCells(counts: Record<string, number>) {
  const submitted = (counts.submitted ?? 0) + (counts.ai_reviewing ?? 0) + (counts.human_reviewing ?? 0)
  const cells: Array<{ label: string, value: number, toneClass?: string }> = []
  if (submitted > 0) {
    cells.push({ label: '已提交', value: submitted })
  }
  if ((counts.approved ?? 0) > 0) {
    cells.push({ label: '通过', value: counts.approved, toneClass: 'wb-stats-cell__value--success' })
  }
  if ((counts.rejected ?? 0) > 0) {
    cells.push({ label: '打回', value: counts.rejected, toneClass: 'wb-stats-cell__value--danger' })
  }
  return cells
}

function answerDraftKey(answer: AnswerValue) {
  return JSON.stringify(answer)
}

// 「进行中」的提交 = labeler 仍需亲自动手的状态:draft(已领未提交)与 revising(被打回待重做)。
// 工作台直达时优先恢复这些,审核中 / 终态的提交不算「进行中」。
function isResumable(status: string): boolean {
  const normalized = normalizeStatus(status)
  return normalized === 'draft' || normalized === 'revising'
}

// 由 bundle 推导本地草稿坐标键(taskId:itemId:submissionId:revisionNo)。
// 无 task/item 时返回 null(尚不能定位草稿)。revisionNo 优先用 submission.currentRevisionId,
// 退化用 revision.id(后端 SubmissionRevision 仅暴露 id,这里把它当作修订标识)。
function bundleDraftKey(bundle: TaskBundle): string | null {
  if (!bundle.task || !bundle.item) {
    return null
  }
  return buildDraftKey({
    taskId: bundle.task.id,
    itemId: bundle.item.id,
    submissionId: bundle.submission?.id ?? null,
    revisionNo: bundle.submission?.currentRevisionId ?? bundle.revision?.id ?? null,
  })
}

// 纯函数:某 bundle 是否有「比服务端更新、且与服务端答案不同」的本地草稿。
// 有则返回该本地答案供恢复,否则返回 null。在三条载题路径间复用,不依赖组件闭包。
function computeRecoverableDraft(bundle: TaskBundle, serverAnswer: AnswerValue): AnswerValue | null {
  const draftKey = bundleDraftKey(bundle)
  if (!draftKey) {
    return null
  }
  const draft = loadLocalDraft(draftKey)
  if (!isLocalDraftNewer(draft, bundle.template?.version ?? null)) {
    return null
  }
  if (draft && answerDraftKey(draft.answer) !== answerDraftKey(serverAnswer)) {
    return draft.answer
  }
  return null
}

// summarizeSchema:给 AI 求助用的轻量模板摘要(标题 + 字段名/类型/是否必填),
// 不把完整 schema JSON 塞进 prompt,既省 token 又减少注入面。
function summarizeSchema(schema: TemplateSchema): string {
  const fields = (schema.fields ?? [])
    .map((field) => {
      const flags = [field.widget, field.required ? '必填' : '选填'].filter(Boolean).join('/')
      return `- ${field.label || field.name}(${flags})`
    })
    .join('\n')
  return [`表单标题:${schema.title || '未命名'}`, '字段:', fields || '(无字段)'].join('\n')
}

function autoSaveText(state: 'idle' | 'saving' | 'saved' | 'failed') {
  switch (state) {
    case 'saving':
      return '自动保存中...'
    case 'saved':
      return '已自动保存'
    case 'failed':
      return '自动保存失败'
    default:
      return '3s 自动保存'
  }
}

function auditLogLabel(log: AuditLog) {
  const actor = log.actorType === 'ai' ? 'AI' : log.actorType === 'system' ? '系统' : log.actorType === 'human' ? '审核员' : log.actorType || ''
  const eventLabel = AUDIT_EVENT_LABELS[log.event] ?? log.event
  return [actor, eventLabel].filter(Boolean).join(' · ')
}

const AUDIT_EVENT_LABELS: Record<string, string> = {
  claim: '领取',
  draft: '保存草稿',
  submit: '提交',
  ai_pass: 'AI 通过',
  ai_revise: 'AI 打回',
  ai_uncertain: 'AI 待定',
  human_approve: '复审通过',
  human_revise: '复审打回',
  resubmit: '重新提交',
  export: '导出',
}

function historyActionClass(log: AuditLog) {
  if (log.event.includes('revise') || log.toState === 'revising') {
    return 'wb-history__action wb-history__action--reject'
  }
  if (log.event === 'resubmit' || log.event === 'submit') {
    return 'wb-history__action wb-history__action--current'
  }
  return 'wb-history__who'
}

function formatAuditDate(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  }
  const pad = (n: number) => n.toString().padStart(2, '0')
  return `${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
}
