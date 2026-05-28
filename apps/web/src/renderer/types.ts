export const widgetTypes = [
  'ShowItem',
  'Group',
  'Tabs',
  'Input',
  'TextArea',
  'Radio',
  'Tags',
  'RichText',
  'JSONEditor',
  'FileUpload',
  'LLMTrigger',
] as const

export type WidgetType = typeof widgetTypes[number]

export const showItemModes = ['auto', 'text', 'video', 'image', 'markdown', 'json'] as const

export type ShowItemMode = typeof showItemModes[number]

export type FieldOption = string | number

export type TabSchema = {
  _draftId?: string
  label: string
  fields: FieldSchema[]
}

export type RequiredWhen = {
  field: string
  equals?: unknown
  notEmpty?: boolean
}

export type FieldSchema = {
  _draftId?: string
  name: string
  widget: WidgetType
  label: string
  required?: boolean
  requiredWhen?: RequiredWhen
  regex?: string
  fields?: FieldSchema[]
  tabs?: TabSchema[]
  options?: FieldOption[]
  minLength?: number
  maxLength?: number
  path?: string
  mode?: ShowItemMode
  maxFiles?: number
  prompt?: string
  target_field?: string
  [key: `x-${string}`]: unknown
}

export type TemplateSchema = {
  version?: number
  title: string
  layout: 'single_page'
  fields: FieldSchema[]
  export_fields?: string[]
  [key: `x-${string}`]: unknown
}

export type AnswerValue = Record<string, unknown>

export type RenderPayload = Record<string, unknown>

export type SchemaParseError = {
  field: string
  message: string
}

export type ParseResult<T> =
  | { ok: true, value: T }
  | { ok: false, error: SchemaParseError }

export type ValidationError = {
  field: string
  message: string
}

export type RenderRuntime = {
  taskId?: number
  itemId?: number
  submissionId?: number
}

export type WidgetProps = {
  field: FieldSchema
  value: unknown
  answer: AnswerValue
  payload: RenderPayload
  runtime?: RenderRuntime
  readOnly?: boolean
  onChange: (name: string, value: unknown) => void
}
