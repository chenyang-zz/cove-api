import { fetch } from 'expo/fetch';

import { API_BASE_URL } from './config';
import { clearStoredSession, loadStoredSession, saveStoredSession } from './session';
import type {
  ApiEnvelope,
  ApiFieldError,
  AuthResponse,
  LoginInput,
  PasswordChangeInput,
  ProfileUpdateInput,
  RegisterInput,
  StoredSession,
  UserResponse,
} from './types';

let refreshInFlight: Promise<StoredSession | null> | null = null;

export class ApiError extends Error {
  readonly status: number;
  readonly fieldErrors: ApiFieldError[];

  constructor(status: number, message: string, fieldErrors: ApiFieldError[] = []) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.fieldErrors = fieldErrors;
  }
}

async function performEnvelope<T>(path: string, init: RequestInit = {}): Promise<ApiEnvelope<T>> {
  const headers = new Headers(init.headers);
  if (init.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }

  let response: Response;
  try {
    response = await fetch(`${API_BASE_URL}${path}`, { ...init, headers });
  } catch {
    throw new ApiError(0, '无法连接到服务器，请检查网络后重试。');
  }

  let envelope: ApiEnvelope<T>;
  try {
    envelope = (await response.json()) as ApiEnvelope<T>;
  } catch {
    throw new ApiError(response.status, '服务器返回了无法识别的响应。');
  }

  if (!response.ok || envelope.code !== 0) {
    throw new ApiError(
      response.status,
      envelope.message || '请求失败，请稍后重试。',
      envelope.errors ?? [],
    );
  }
  return envelope;
}

async function performRequest<T>(path: string, init: RequestInit = {}): Promise<T> {
  const envelope = await performEnvelope<T>(path, init);
  if (envelope.data === undefined) {
    throw new ApiError(200, '服务器响应缺少必要数据。');
  }
  return envelope.data;
}

function sessionFromAuth(response: AuthResponse, user?: StoredSession['user']): StoredSession {
  return {
    accessToken: response.access_token,
    refreshToken: response.refresh_token,
    user: user ?? {
      id: response.user_id,
      username: response.username,
      nickname: null,
      email: response.email ?? null,
      avatar: null,
    },
  };
}

async function hydrateSession(response: AuthResponse): Promise<StoredSession> {
  const initial = await saveStoredSession(sessionFromAuth(response));
  try {
    const user = await getCurrentUser();
    const current = (await loadStoredSession()) ?? initial;
    return saveStoredSession({ ...current, user });
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) {
      await clearStoredSession();
      throw error;
    }
    return (await loadStoredSession()) ?? initial;
  }
}

export async function login(input: LoginInput): Promise<StoredSession> {
  return hydrateSession(
    await performRequest<AuthResponse>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  );
}

export async function register(input: RegisterInput): Promise<StoredSession> {
  return hydrateSession(
    await performRequest<AuthResponse>('/api/auth/register', {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  );
}

export function refreshSession(): Promise<StoredSession | null> {
  if (refreshInFlight) {
    return refreshInFlight;
  }

  refreshInFlight = loadStoredSession()
    .then(async (current) => {
      if (!current) {
        return null;
      }
      const response = await performRequest<AuthResponse>('/api/auth/refresh', {
        method: 'POST',
        body: JSON.stringify({ refresh_token: current.refreshToken }),
      });
      return saveStoredSession(sessionFromAuth(response, current.user));
    })
    .catch(async (error: unknown) => {
      if (error instanceof ApiError && error.status === 401) {
        await clearStoredSession();
      }
      throw error;
    })
    .finally(() => {
      refreshInFlight = null;
    });

  return refreshInFlight;
}

export async function authenticatedRequest<T>(
  path: string,
  init: RequestInit = {},
  retryAfterRefresh = true,
): Promise<T> {
  const session = await loadStoredSession();
  if (!session) {
    throw new ApiError(401, '请先登录。');
  }

  const headers = new Headers(init.headers);
  headers.set('Authorization', `Bearer ${session.accessToken}`);
  try {
    return await performRequest<T>(path, { ...init, headers });
  } catch (error) {
    if (!(error instanceof ApiError) || error.status !== 401 || !retryAfterRefresh) {
      throw error;
    }
    const refreshed = await refreshSession();
    if (!refreshed) {
      throw new ApiError(401, '登录状态已失效，请重新登录。');
    }
    const retryHeaders = new Headers(init.headers);
    retryHeaders.set('Authorization', `Bearer ${refreshed.accessToken}`);
    return performRequest<T>(path, { ...init, headers: retryHeaders });
  }
}

export async function authenticatedCommand(
  path: string,
  init: RequestInit = {},
  retryAfterRefresh = true,
): Promise<void> {
  const session = await loadStoredSession();
  if (!session) {
    throw new ApiError(401, '请先登录。');
  }

  const headers = new Headers(init.headers);
  headers.set('Authorization', `Bearer ${session.accessToken}`);
  try {
    await performEnvelope<never>(path, { ...init, headers });
  } catch (error) {
    if (!(error instanceof ApiError) || error.status !== 401 || !retryAfterRefresh) {
      throw error;
    }
    const refreshed = await refreshSession();
    if (!refreshed) {
      throw new ApiError(401, '登录状态已失效，请重新登录。');
    }
    const retryHeaders = new Headers(init.headers);
    retryHeaders.set('Authorization', `Bearer ${refreshed.accessToken}`);
    await performEnvelope<never>(path, { ...init, headers: retryHeaders });
  }
}

async function performMultipartRequest<T>(
  path: string,
  accessToken: string,
  createBody: () => FormData,
): Promise<T> {
  let response: Response;
  try {
    response = await fetch(`${API_BASE_URL}${path}`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${accessToken}` },
      body: createBody(),
    });
  } catch {
    throw new ApiError(0, '无法连接到服务器，请检查网络后重试。');
  }

  let envelope: ApiEnvelope<T>;
  try {
    envelope = (await response.json()) as ApiEnvelope<T>;
  } catch {
    throw new ApiError(response.status, '服务器返回了无法识别的响应。');
  }
  if (!response.ok || envelope.code !== 0) {
    throw new ApiError(
      response.status,
      envelope.message || '请求失败，请稍后重试。',
      envelope.errors ?? [],
    );
  }
  if (envelope.data === undefined) {
    throw new ApiError(200, '服务器响应缺少必要数据。');
  }
  return envelope.data;
}

export async function authenticatedMultipartRequest<T>(
  path: string,
  createBody: () => FormData,
  retryAfterRefresh = true,
): Promise<T> {
  const session = await loadStoredSession();
  if (!session) {
    throw new ApiError(401, '请先登录。');
  }
  try {
    return await performMultipartRequest<T>(path, session.accessToken, createBody);
  } catch (error) {
    if (!(error instanceof ApiError) || error.status !== 401 || !retryAfterRefresh) {
      throw error;
    }
    const refreshed = await refreshSession();
    if (!refreshed) {
      throw new ApiError(401, '登录状态已失效，请重新登录。');
    }
    return performMultipartRequest<T>(path, refreshed.accessToken, createBody);
  }
}

export function getCurrentUser(): Promise<UserResponse> {
  return authenticatedRequest<UserResponse>('/api/auth/me');
}

export async function updateProfile(input: ProfileUpdateInput): Promise<StoredSession> {
  const user = await authenticatedRequest<UserResponse>('/api/auth/profile', {
    method: 'PUT',
    body: JSON.stringify(input),
  });
  const current = await loadStoredSession();
  if (!current) {
    throw new ApiError(401, '登录状态已失效，请重新登录。');
  }
  return saveStoredSession({ ...current, user });
}

export async function changePassword(input: PasswordChangeInput): Promise<void> {
  const session = await loadStoredSession();
  if (!session) {
    throw new ApiError(401, '请先登录。');
  }
  const headers = new Headers({ Authorization: `Bearer ${session.accessToken}` });
  try {
    await performEnvelope<never>('/api/auth/password', {
      method: 'POST',
      headers,
      body: JSON.stringify(input),
    });
  } catch (error) {
    if (!(error instanceof ApiError) || error.status !== 401) {
      throw error;
    }
    const refreshed = await refreshSession();
    if (!refreshed) {
      throw new ApiError(401, '登录状态已失效，请重新登录。');
    }
    await performEnvelope<never>('/api/auth/password', {
      method: 'POST',
      headers: { Authorization: `Bearer ${refreshed.accessToken}` },
      body: JSON.stringify(input),
    });
  }
}

export async function restoreSession(): Promise<StoredSession | null> {
  const current = await loadStoredSession();
  if (!current) {
    return null;
  }
  try {
    const user = await getCurrentUser();
    return saveStoredSession({ ...((await loadStoredSession()) ?? current), user });
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) {
      await clearStoredSession();
      return null;
    }
    return current;
  }
}

export async function logout(): Promise<void> {
  await clearStoredSession();
}
