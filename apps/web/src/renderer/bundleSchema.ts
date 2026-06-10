import type { TaskBundle } from '../shared/api/client'
import { parseTemplateSchema } from './parser'
import type { TemplateSchema } from './types'

export type ParsedSchema =
  | { ok: true, schema: TemplateSchema }
  | { ok: false, message: string }

// parseBundleSchema 从 task bundle 的模板快照解析出可渲染 schema。
// 缺少快照时返回 missingMessage —— 默认是审核侧文案,标注侧传入自己的文案。
export function parseBundleSchema(
  bundle: TaskBundle | null | undefined,
  missingMessage = '当前提交缺少模板快照',
): ParsedSchema {
  if (!bundle?.template?.schemaJson) {
    return { ok: false, message: missingMessage }
  }
  const result = parseTemplateSchema(bundle.template.schemaJson)
  if (!result.ok) {
    return { ok: false, message: `${result.error.field}: ${result.error.message}` }
  }
  return { ok: true, schema: result.value }
}
