import type { AnswerValue, FieldSchema, RequiredWhen, TemplateSchema, ValidationError } from './types'

export function validateAnswer(schema: TemplateSchema, answer: AnswerValue): ValidationError[] {
  const errors: ValidationError[] = []
  for (const field of leafFields(schema.fields)) {
    if (field.widget === 'ShowItem' || field.widget === 'Group' || field.widget === 'Tabs') {
      continue
    }
    const value = answer[field.name]
    if ((field.required || requiredWhenMatches(field.requiredWhen, answer)) && isEmpty(value)) {
      errors.push({ field: field.name, message: `${field.label} is required` })
      continue
    }
    if (typeof value === 'string') {
      if (field.minLength !== undefined && value.trim().length < field.minLength) {
        errors.push({ field: field.name, message: `${field.label} must be at least ${field.minLength} characters` })
      }
      if (field.maxLength !== undefined && value.length > field.maxLength) {
        errors.push({ field: field.name, message: `${field.label} must be at most ${field.maxLength} characters` })
      }
      if (field.regex && !isEmpty(value) && !new RegExp(field.regex).test(value)) {
        errors.push({ field: field.name, message: `${field.label} format is invalid` })
      }
    }
  }
  return errors
}

function leafFields(fields: FieldSchema[]): FieldSchema[] {
  const result: FieldSchema[] = []
  for (const field of fields) {
    if (field.widget === 'Group' && field.fields) {
      result.push(...leafFields(field.fields))
      continue
    }
    if (field.widget === 'Tabs' && field.tabs) {
      for (const tab of field.tabs) {
        result.push(...leafFields(tab.fields))
      }
      continue
    }
    result.push(field)
  }
  return result
}

function requiredWhenMatches(condition: RequiredWhen | undefined, answer: AnswerValue) {
  if (!condition) {
    return false
  }
  const value = answer[condition.field]
  if ('equals' in condition) {
    return value === condition.equals
  }
  return condition.notEmpty === true && !isEmpty(value)
}

function isEmpty(value: unknown) {
  if (value === undefined || value === null) {
    return true
  }
  if (typeof value === 'string') {
    return value.trim() === ''
  }
  if (Array.isArray(value)) {
    return value.length === 0
  }
  return false
}
