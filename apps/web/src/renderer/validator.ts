import { Parser as ExprParser, type Values } from 'expr-eval'
import type { AnswerValue, CustomRule, FieldSchema, RequiredWhen, TemplateSchema, ValidationError, VisibleWhen } from './types'

const exprParser = new ExprParser()

export function validateAnswer(schema: TemplateSchema, answer: AnswerValue): ValidationError[] {
  const errors: ValidationError[] = []
  for (const field of leafFields(schema.fields)) {
    if (field.widget === 'ShowItem' || field.widget === 'Group' || field.widget === 'Tabs') {
      continue
    }
    if (!visibleWhenMatches(field.visibleWhen, answer)) {
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
    if (field.customRule && !isEmpty(value) && !customRuleMatches(field.customRule, value, answer)) {
      errors.push({ field: field.name, message: field.customRule.message })
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

function visibleWhenMatches(condition: VisibleWhen | undefined, answer: AnswerValue) {
  if (!condition) {
    return true
  }
  const value = answer[condition.field]
  if ('equals' in condition) {
    return value === condition.equals
  }
  return condition.notEmpty === true && !isEmpty(value)
}

function customRuleMatches(rule: CustomRule, value: unknown, answer: AnswerValue) {
  try {
    const scope = buildExprScope(value, answer)
    return Boolean(exprParser.parse(rule.expr).evaluate(scope))
  } catch {
    // A runtime failure (e.g. a missing sibling referenced by the expr) is treated as
    // an invalid value so the field's customRule message surfaces rather than silently passing.
    return false
  }
}

function buildExprScope(value: unknown, answer: AnswerValue): Values {
  // expr-eval's `Values` type only models number/string/function/nested-object scopes,
  // but at runtime it accepts arbitrary answer values (booleans, arrays, undefined) and
  // coerces them during comparison. The whole evaluate() call is wrapped in try/catch,
  // so casting the raw answer scope is safe and avoids deep-sanitizing every value.
  const scope: Record<string, unknown> = {
    ...answer,
    value,
    answer,
    len: exprLen,
  }
  return scope as Values
}

function exprLen(input: unknown) {
  if (typeof input === 'string' || Array.isArray(input)) {
    return input.length
  }
  return 0
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
