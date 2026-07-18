import type {
  ApiEnvelope,
  ApiFieldError,
  AuthResponse,
  LoginInput,
  RegisterInput,
  StoredSession,
  UserResponse,
} from './types'

const SESSION_STORAGE_KEY = 'cove.auth.session.v1'
const API_BASE_URL = (import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8000').replace(
  /\/+$/,
  '',
)

let refreshInFlight: Promise<StoredSession | null> | null = null
let restoreInFlight: Promise<StoredSession | null> | null = null

export class ApiError extends Error {
  readonly status: number
  readonly fieldErrors: ApiFieldError[]

  constructor(status: number, message: string, fieldErrors: ApiFieldError[] = []) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.fieldErrors = fieldErrors
  }
}

function loadSession(): StoredSession | null {
  if (typeof window === 'undefined') {
    return null
  }

  const raw = window.localStorage.getItem(SESSION_STORAGE_KEY)
  if (!raw) {
    return null
  }

  try {
    const session = JSON.parse(raw) as Partial<StoredSession>
    if (
      typeof session.accessToken !== 'string' ||
      typeof session.refreshToken !== 'string' ||
      !session.user ||
      typeof session.user.id !== 'string' ||
      typeof session.user.username !== 'string'
    ) {
      clearSession()
      return null
    }
    return session as StoredSession
  } catch {
    clearSession()
    return null
  }
}

export function getStoredSession(): StoredSession | null {
  return loadSession()
}

function saveSession(session: StoredSession): StoredSession {
  if (typeof window !== 'undefined') {
    window.localStorage.setItem(SESSION_STORAGE_KEY, JSON.stringify(session))
  }
  return session
}

export function clearSession(): void {
  if (typeof window !== 'undefined') {
    window.localStorage.removeItem(SESSION_STORAGE_KEY)
  }
}

function sessionFromAuth(response: AuthResponse, currentUser?: StoredSession['user']): StoredSession {
  return {
    accessToken: response.access_token,
    refreshToken: response.refresh_token,
    user: currentUser ?? {
      id: response.user_id,
      username: response.username,
      nickname: null,
      email: response.email ?? null,
      avatar: null,
    },
  }
}

async function performEnvelope<T>(path: string, init: RequestInit = {}): Promise<ApiEnvelope<T>> {
  const headers = new Headers(init.headers)
  if (init.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  let response: Response
  try {
    response = await fetch(`${API_BASE_URL}${path}`, { ...init, headers })
  } catch (error: unknown) {
    if (error instanceof ApiError) {
      throw error
    }
    throw new ApiError(0, '无法连接到服务器，请检查网络后重试。')
  }

  let envelope: ApiEnvelope<T>
  try {
    envelope = (await response.json()) as ApiEnvelope<T>
  } catch {
    throw new ApiError(response.status, '服务器返回了无法识别的响应。')
  }

  if (!response.ok || envelope.code !== 0) {
    throw new ApiError(
      response.status,
      envelope.message || '请求失败，请稍后重试。',
      envelope.errors ?? [],
    )
  }

  return envelope
}

async function performRequest<T>(path: string, init: RequestInit = {}): Promise<T> {
  const envelope = await performEnvelope<T>(path, init)
  if (envelope.data === undefined) {
    throw new ApiError(200, '服务器响应缺少必要数据。')
  }
  return envelope.data
}

async function hydrateAuthenticatedSession(response: AuthResponse): Promise<StoredSession> {
  const initial = saveSession(sessionFromAuth(response))
  try {
    const user = await getCurrentUser()
    const current = loadSession() ?? initial
    return saveSession({
      ...current,
      user: {
        id: user.id,
        username: user.username,
        nickname: user.nickname,
        email: user.email,
        avatar: user.avatar,
      },
    })
  } catch (error: unknown) {
    if (error instanceof ApiError && error.status === 401) {
      clearSession()
      throw error
    }
    return loadSession() ?? initial
  }
}

export async function login(input: LoginInput): Promise<StoredSession> {
  const response = await performRequest<AuthResponse>('/api/auth/login', {
    method: 'POST',
    body: JSON.stringify(input),
  })
  return hydrateAuthenticatedSession(response)
}

export async function register(input: RegisterInput): Promise<StoredSession> {
  const response = await performRequest<AuthResponse>('/api/auth/register', {
    method: 'POST',
    body: JSON.stringify(input),
  })
  return hydrateAuthenticatedSession(response)
}

export function refreshSession(): Promise<StoredSession | null> {
  if (refreshInFlight) {
    return refreshInFlight
  }

  const current = loadSession()
  if (!current) {
    return Promise.resolve(null)
  }

  refreshInFlight = performRequest<AuthResponse>('/api/auth/refresh', {
    method: 'POST',
    body: JSON.stringify({ refresh_token: current.refreshToken }),
  })
    .then((response) => saveSession(sessionFromAuth(response, current.user)))
    .catch((error: unknown) => {
      if (error instanceof ApiError && error.status === 401) {
        clearSession()
      }
      throw error
    })
    .finally(() => {
      refreshInFlight = null
    })

  return refreshInFlight
}

export async function authenticatedRequest<T>(
  path: string,
  init: RequestInit = {},
  retryAfterRefresh = true,
): Promise<T> {
  const session = loadSession()
  if (!session) {
    throw new ApiError(401, '请先登录。')
  }

  const headers = new Headers(init.headers)
  headers.set('Authorization', `Bearer ${session.accessToken}`)

  try {
    return await performRequest<T>(path, { ...init, headers })
  } catch (error: unknown) {
    if (!(error instanceof ApiError) || error.status !== 401 || !retryAfterRefresh) {
      throw error
    }

    const refreshed = await refreshSession()
    if (!refreshed) {
      throw new ApiError(401, '登录状态已失效，请重新登录。')
    }

    const retryHeaders = new Headers(init.headers)
    retryHeaders.set('Authorization', `Bearer ${refreshed.accessToken}`)
    return performRequest<T>(path, { ...init, headers: retryHeaders })
  }
}

export async function authenticatedCommand(
  path: string,
  init: RequestInit = {},
  retryAfterRefresh = true,
): Promise<void> {
  const session = loadSession()
  if (!session) {
    throw new ApiError(401, '请先登录。')
  }

  const headers = new Headers(init.headers)
  headers.set('Authorization', `Bearer ${session.accessToken}`)

  try {
    await performEnvelope<never>(path, { ...init, headers })
  } catch (error: unknown) {
    if (!(error instanceof ApiError) || error.status !== 401 || !retryAfterRefresh) {
      throw error
    }
    const refreshed = await refreshSession()
    if (!refreshed) {
      throw new ApiError(401, '登录状态已失效，请重新登录。')
    }
    const retryHeaders = new Headers(init.headers)
    retryHeaders.set('Authorization', `Bearer ${refreshed.accessToken}`)
    await performEnvelope<never>(path, { ...init, headers: retryHeaders })
  }
}

export function getCurrentUser(): Promise<UserResponse> {
  return authenticatedRequest<UserResponse>('/api/auth/me')
}

export function saveCurrentUser(user: UserResponse): StoredSession {
  const current = loadSession()
  if (!current) {
    throw new ApiError(401, '请先登录。')
  }
  return saveSession({
    ...current,
    user: {
      id: user.id,
      username: user.username,
      nickname: user.nickname,
      email: user.email,
      avatar: user.avatar,
    },
  })
}

export function restoreSession(): Promise<StoredSession | null> {
  if (restoreInFlight) {
    return restoreInFlight
  }

  if (!loadSession()) {
    return Promise.resolve(null)
  }

  restoreInFlight = getCurrentUser()
    .then((user) => {
      const current = loadSession()
      if (!current) {
        return null
      }
      return saveSession({
        ...current,
        user: {
          id: user.id,
          username: user.username,
          nickname: user.nickname,
          email: user.email,
          avatar: user.avatar,
        },
      })
    })
    .catch((error: unknown) => {
      if (error instanceof ApiError && error.status === 401) {
        clearSession()
        return null
      }
      return loadSession()
    })
    .finally(() => {
      restoreInFlight = null
    })

  return restoreInFlight
}
