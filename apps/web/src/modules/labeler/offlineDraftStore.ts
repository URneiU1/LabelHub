// 离线草稿存储(P3.1):把标注答案草稿落到本地,网络断开/自动保存失败时不丢工作。
//
// 选型:localStorage(同步、小巧、零依赖),不上 IndexedDB。
// 单条 localStorage 值上限受浏览器整体配额约束(各家约 5MB / origin,且与其它键共享)。
// 标注答案是结构化键值(短文本/标签/打分),单题量级在 KB 级,远低于该上限;
// 即便偶发超额,saveLocalDraft 已用 try/catch 兜底——写失败只是丢掉离线兜底,
// 绝不影响在线作答主流程。若未来出现大文件型答案再评估迁 IndexedDB。
//
// 所有访问都包在 try/catch:隐私模式 / 配额满 / storage 被禁用都不能让作答页崩。

import type { AnswerValue } from '../../renderer/types'

// 本地草稿键的命名空间前缀,避免与登录态等其它 localStorage 键冲突。
const KEY_PREFIX = 'labelhub_offline_draft:'

// 标识本地草稿的四元组坐标:任务 / 题目 / 提交 / 修订号。
// submissionId / revisionNo 可能尚未生成(新题、未保存),用 'new' 占位以保持键稳定。
export type DraftCoord = {
  taskId: number
  itemId: number
  submissionId?: number | null
  revisionNo?: number | null
}

// 一条本地草稿的载荷。synced=true 表示这份答案已被服务端确认(自动保存/提交成功)。
export type LocalDraft = {
  answer: AnswerValue
  updatedAt: number
  templateVersion: number | null
  synced: boolean
}

/**
 * 构造本地草稿键:taskId:itemId:submissionId:revisionNo(缺失部分用 'new')。
 */
export function buildDraftKey(coord: DraftCoord): string {
  const submission = coord.submissionId != null ? String(coord.submissionId) : 'new'
  const revision = coord.revisionNo != null ? String(coord.revisionNo) : 'new'
  return `${KEY_PREFIX}${coord.taskId}:${coord.itemId}:${submission}:${revision}`
}

/**
 * 写入/覆盖一条本地草稿,标记为「未同步」并刷新 updatedAt。
 * 不可变:每次写入构造全新的 LocalDraft 对象。
 */
export function saveLocalDraft(key: string, answer: AnswerValue, templateVersion: number | null): void {
  const draft: LocalDraft = {
    answer,
    updatedAt: Date.now(),
    templateVersion,
    synced: false,
  }
  writeRaw(key, draft)
}

/**
 * 把已存在的本地草稿标记为已同步(服务端确认后调用)。
 * 草稿不存在则忽略;不可变更新,不就地改对象。
 */
export function markSynced(key: string): void {
  const draft = loadLocalDraft(key)
  if (!draft || draft.synced) {
    return
  }
  writeRaw(key, { ...draft, synced: true })
}

/**
 * 读取一条本地草稿;不存在 / 解析失败 / storage 不可用都返回 null。
 */
export function loadLocalDraft(key: string): LocalDraft | null {
  try {
    const raw = window.localStorage.getItem(key)
    if (!raw) {
      return null
    }
    const parsed = JSON.parse(raw) as Partial<LocalDraft>
    if (!parsed || typeof parsed.updatedAt !== 'number' || typeof parsed.answer !== 'object' || parsed.answer === null) {
      return null
    }
    return {
      answer: parsed.answer as AnswerValue,
      updatedAt: parsed.updatedAt,
      templateVersion: typeof parsed.templateVersion === 'number' ? parsed.templateVersion : null,
      synced: parsed.synced === true,
    }
  } catch {
    return null
  }
}

/**
 * 删除一条本地草稿(用户点「丢弃本地草稿」或确认恢复后清理)。
 */
export function discardLocalDraft(key: string): void {
  try {
    window.localStorage.removeItem(key)
  } catch {
    // storage 不可用:无本地草稿可删,忽略。
  }
}

/**
 * 判定本地草稿是否「比服务端更新、且可恢复」。
 *
 * 满足全部条件才算可恢复:
 *  - 存在本地草稿;
 *  - 模板版本与当前服务端模板一致(避免把旧版本答案套到新表单上);
 *  - 本地草稿尚未同步(synced=false),即最后一次本地写入从未被服务端确认。
 *
 * 当后端能提供 submission 的服务端更新时间(毫秒)时,再叠加时间戳比较:
 * 仅当本地 updatedAt 严格晚于服务端时间才算更新。后端当前未向前端暴露该时间,
 * 故 serverUpdatedAt 省略时退化为「未同步即更新」,这与 synced 语义一致。
 */
export function isLocalDraftNewer(draft: LocalDraft | null, currentTemplateVersion: number | null, serverUpdatedAt?: number | null): boolean {
  if (!draft || draft.synced) {
    return false
  }
  if (draft.templateVersion !== currentTemplateVersion) {
    return false
  }
  if (serverUpdatedAt != null) {
    return draft.updatedAt > serverUpdatedAt
  }
  return true
}

/**
 * 实际写入 localStorage 的私有助手;配额满/被禁用时静默失败。
 */
function writeRaw(key: string, draft: LocalDraft): void {
  try {
    window.localStorage.setItem(key, JSON.stringify(draft))
  } catch {
    // 配额超限 / storage 被禁用:放弃离线兜底,但不影响在线作答。
  }
}
