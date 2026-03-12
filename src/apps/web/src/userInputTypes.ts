// --- 字段定义类型 ---

export interface EnumOption {
  value: string
  label?: string
}

export interface OneOfOption {
  const: string
  title: string
}

export interface AnyOfOption {
  const: string
  title: string
}

// select (单选): type=string + enum 或 oneOf
export interface EnumFieldSchema {
  type: 'string'
  title?: string
  description?: string
  enum: string[]
  enumNames?: string[]
  default?: string
}

export interface OneOfFieldSchema {
  type: 'string'
  title?: string
  description?: string
  oneOf: OneOfOption[]
  default?: string
}

// multiselect (多选): type=array + items.enum 或 items.anyOf
export interface ArrayEnumFieldSchema {
  type: 'array'
  title?: string
  description?: string
  items: { type: 'string'; enum: string[] }
  minItems?: number
  maxItems?: number
  default?: string[]
}

export interface ArrayAnyOfFieldSchema {
  type: 'array'
  title?: string
  description?: string
  items: { anyOf: AnyOfOption[] }
  minItems?: number
  maxItems?: number
  default?: string[]
}

// boolean
export interface BooleanFieldSchema {
  type: 'boolean'
  title?: string
  description?: string
  default?: boolean
}

// text (纯文本输入): type=string 且无 enum/oneOf
export interface TextFieldSchema {
  type: 'string'
  title?: string
  description?: string
  default?: string
  minLength?: number
  maxLength?: number
  format?: string
}

// number
export interface NumberFieldSchema {
  type: 'number' | 'integer'
  title?: string
  description?: string
  default?: number
  minimum?: number
  maximum?: number
}

export type FieldSchema =
  | EnumFieldSchema
  | OneOfFieldSchema
  | ArrayEnumFieldSchema
  | ArrayAnyOfFieldSchema
  | BooleanFieldSchema
  | TextFieldSchema
  | NumberFieldSchema

// --- 请求/响应 ---

export interface RequestedSchema {
  properties: Record<string, FieldSchema>
  required?: string[]
}

export interface UserInputRequest {
  request_id: string
  message: string
  requestedSchema: RequestedSchema
}

// 值类型: string | string[] | boolean | number
export type FieldValue = string | string[] | boolean | number

export interface UserInputResponse {
  type: 'user_input_response'
  request_id: string
  answers: Record<string, FieldValue>
}

// --- 类型判断 ---

export function isEnumField(f: FieldSchema): f is EnumFieldSchema {
  return f.type === 'string' && 'enum' in f
}

export function isOneOfField(f: FieldSchema): f is OneOfFieldSchema {
  return f.type === 'string' && 'oneOf' in f
}

export function isArrayEnumField(f: FieldSchema): f is ArrayEnumFieldSchema {
  return f.type === 'array' && 'items' in f && 'enum' in (f as ArrayEnumFieldSchema).items
}

export function isArrayAnyOfField(f: FieldSchema): f is ArrayAnyOfFieldSchema {
  return f.type === 'array' && 'items' in f && 'anyOf' in (f as ArrayAnyOfFieldSchema).items
}

export function isBooleanField(f: FieldSchema): f is BooleanFieldSchema {
  return f.type === 'boolean'
}

export function isTextField(f: FieldSchema): f is TextFieldSchema {
  return f.type === 'string' && !('enum' in f) && !('oneOf' in f)
}

export function isNumberField(f: FieldSchema): f is NumberFieldSchema {
  return f.type === 'number' || f.type === 'integer'
}
