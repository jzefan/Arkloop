export interface UserInputOption {
  value: string
  label: string
  description?: string
  recommended?: boolean
}

export interface UserInputQuestion {
  id: string
  header?: string
  question: string
  options: UserInputOption[]
  allow_other?: boolean
}

export interface UserInputRequest {
  request_id: string
  questions: UserInputQuestion[]
}

export interface UserInputAnswer {
  type: 'option' | 'other'
  value: string
}

export interface UserInputResponse {
  type: 'user_input_response'
  request_id: string
  answers: Record<string, UserInputAnswer>
}
