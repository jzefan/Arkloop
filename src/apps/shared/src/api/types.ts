export type LoginRequest = {
  login: string
  password: string
  cf_turnstile_token?: string
}

export type LoginResponse = {
  token_type: string
  access_token: string
}
