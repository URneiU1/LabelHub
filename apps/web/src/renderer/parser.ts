import { showItemModes, widgetTypes, type AnswerValue, type FieldSchema, type ParseResult, type TemplateSchema, type WidgetType } from './types'

const widgetSet = new Set<string>(widgetTypes)
const showItemModeSet = new Set<string>(showItemModes)

export function parseTemplateSchema(raw: string | unknown): ParseResult<TemplateSchema> {
  const parsedResult = parseUnknown(raw)
  if (!parsedResult.ok) {
    return parseError('$', `invalid JSON: ${parsedResult.message}`)
  }
  const parsed = parsedResult.value
  if (!isRecord(parsed)) {
    return parseError('$', 'schema must be an object')
  }
  if (!Array.isArray(parsed.fields)) {
    return parseError('fields', 'fields must be an array')
  }
  if (parsed.fields.length === 0) {
    return parseError('fields', 'fields must be non-empty')
  }
  if (parsed.layout !== undefined && parsed.layout !== 'single_page') {
    return parseError('layout', 'layout must be single_page')
  }

  const fields: FieldSchema[] = []
  const names = new Set<string>()
  for (let index = 0; index < parsed.fields.length; index += 1) {
    const rawField = parsed.fields[index]
    const path = `fields[${index}]`
    if (!isRecord(rawField)) {
      return parseError(path, 'field must be an object')
    }
    const name = stringProp(rawField.name)
    if (!name) {
      return parseError(`${path}.name`, 'name is required')
    }
    if (names.has(name)) {
      return parseError(`${path}.name`, `duplicate name ${name}`)
    }
    names.add(name)

    const widget = stringProp(rawField.widget)
    if (!widgetSet.has(widget)) {
      return parseError(`${path}.widget`, `widget not in enum: ${widget}`)
    }

    const field: FieldSchema = {
      name,
      widget: widget as WidgetType,
      label: stringProp(rawField.label) || name,
    }
    if (typeof rawField.required === 'boolean') {
      field.required = rawField.required
    } else if ('required' in rawField) {
      return parseError(`${path}.required`, 'required must be boolean')
    }
    if ('options' in rawField) {
      const optionsResult = parseOptions(rawField.options, path)
      if (!optionsResult.ok) {
        return optionsResult
      }
      field.options = optionsResult.value
    } else if (widget === 'Radio' || widget === 'Tags') {
      return parseError(`${path}.options`, 'options must be non-empty')
    }
    const minLength = numberProp(rawField.minLength)
    const maxLength = numberProp(rawField.maxLength)
    if (minLength !== undefined) {
      field.minLength = minLength
    } else if ('minLength' in rawField) {
      return parseError(`${path}.minLength`, 'minLength must be number')
    }
    if (maxLength !== undefined) {
      field.maxLength = maxLength
    } else if ('maxLength' in rawField) {
      return parseError(`${path}.maxLength`, 'maxLength must be number')
    }
    if (field.minLength !== undefined && field.maxLength !== undefined && field.minLength > field.maxLength) {
      return parseError(`${path}.maxLength`, 'minLength cannot exceed maxLength')
    }
    if (typeof rawField.path === 'string') {
      field.path = rawField.path
    }
    if (typeof rawField.mode === 'string') {
      if (!showItemModeSet.has(rawField.mode)) {
        return parseError(`${path}.mode`, `mode not in enum: ${rawField.mode}`)
      }
      field.mode = rawField.mode as FieldSchema['mode']
    }
    const maxFiles = positiveIntegerProp(rawField.maxFiles)
    if (maxFiles !== undefined) {
      field.maxFiles = maxFiles
    } else if ('maxFiles' in rawField) {
      return parseError(`${path}.maxFiles`, 'maxFiles must be > 0')
    }
    if (typeof rawField.prompt === 'string') {
      field.prompt = rawField.prompt
    }
    if (typeof rawField.target_field === 'string') {
      field.target_field = rawField.target_field.trim()
    } else if ('target_field' in rawField) {
      return parseError(`${path}.target_field`, 'target_field must be string')
    }
    for (const [key, value] of Object.entries(rawField)) {
      if (key.startsWith('x-')) {
        field[key as `x-${string}`] = value
      }
    }
    fields.push(field)
  }

  for (let index = 0; index < fields.length; index += 1) {
    const field = fields[index]
    if (field.widget !== 'LLMTrigger') {
      continue
    }
    const allowExternal = field['x-allow-external-target'] === true
    if (!field.target_field) {
      if (!allowExternal) {
        return parseError(`fields[${index}].target_field`, 'target_field is required')
      }
      continue
    }
    if (!allowExternal && !names.has(field.target_field)) {
      return parseError(`fields[${index}].target_field`, 'target_field must reference an existing field')
    }
  }

  const schema: TemplateSchema = {
    version: integerProp(parsed.version),
    title: stringProp(parsed.title) || 'untitled_template',
    layout: 'single_page',
    fields,
  }
  if (Array.isArray(parsed.export_fields)) {
    schema.export_fields = parsed.export_fields.filter((item): item is string => typeof item === 'string' && item.trim() !== '')
  }
  for (const [key, value] of Object.entries(parsed)) {
    if (key.startsWith('x-')) {
      schema[key as `x-${string}`] = value
    }
  }

  return {
    ok: true,
    value: schema,
  }
}

export function parseAnswer(raw?: string | null): AnswerValue {
  if (!raw) {
    return {}
  }
  try {
    const parsed: unknown = JSON.parse(raw)
    return isRecord(parsed) ? parsed : { value: parsed }
  } catch {
    return { raw }
  }
}

function parseUnknown(raw: string | unknown): { ok: true, value: unknown } | { ok: false, message: string } {
  if (typeof raw !== 'string') {
    return { ok: true, value: raw }
  }
  try {
    return { ok: true, value: JSON.parse(raw) as unknown }
  } catch (error) {
    return { ok: false, message: error instanceof Error ? error.message : 'invalid JSON' }
  }
}

function parseError<T>(field: string, message: string): ParseResult<T> {
  return { ok: false, error: { field, message } }
}

function stringProp(value: unknown) {
  return typeof value === 'string' ? value.trim() : ''
}

function numberProp(value: unknown) {
  if (typeof value !== 'number' || !Number.isInteger(value) || value < 0) {
    return undefined
  }
  return value
}

function integerProp(value: unknown) {
  return typeof value === 'number' && Number.isInteger(value) ? value : undefined
}

function positiveIntegerProp(value: unknown) {
  return typeof value === 'number' && Number.isInteger(value) && value > 0 ? value : undefined
}

function parseOptions(value: unknown, path: string): ParseResult<Array<string | number>> {
  if (!Array.isArray(value)) {
    return parseError(`${path}.options`, 'options must be an array')
  }
  if (value.length === 0) {
    return parseError(`${path}.options`, 'options must be non-empty')
  }
  const options: Array<string | number> = []
  for (let index = 0; index < value.length; index += 1) {
    const item = value[index]
    if (typeof item === 'string') {
      const trimmed = item.trim()
      if (!trimmed) {
        return parseError(`${path}.options[${index}]`, 'option must be non-empty string or number')
      }
      options.push(trimmed)
      continue
    }
    if (typeof item === 'number') {
      options.push(item)
      continue
    }
    return parseError(`${path}.options[${index}]`, 'option must be string or number')
  }
  return { ok: true, value: options }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}
