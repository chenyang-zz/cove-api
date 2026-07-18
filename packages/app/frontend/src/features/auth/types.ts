export type ApiFieldError = {
  field: string
  tag: string
  param: string
  message: string
}

export type ApiEnvelope<T> = {
  code: number
  message: string
  data?: T
  errors?: ApiFieldError[]
}

export type AuthResponse = {
  user_id: string
  username: string
  email?: string | null
  access_token: string
  refresh_token: string
}

export type UserResponse = {
  id: string
  username: string
  nickname: string | null
  email: string | null
  avatar: string | null
  created_at: string
  updated_at?: string
}

export type SessionUser = {
  id: string
  username: string
  nickname: string | null
  email: string | null
  avatar: string | null
}

export type StoredSession = {
  accessToken: string
  refreshToken: string
  user: SessionUser
}

export type LoginInput = {
  login: string
  password: string
}

export type RegisterInput = {
  username: string
  email?: string
  password: string
}
