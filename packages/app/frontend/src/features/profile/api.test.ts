// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from 'vitest'
import { clearSession } from '../auth/api'
import type { ApiEnvelope, AuthResponse, StoredSession, UserResponse } from '../auth/types'
import { changePassword, refreshProfileSession, updateProfile } from './api'

const storageKey = 'cove.auth.session.v1'

function jsonResponse<T>(envelope: ApiEnvelope<T>, status = 200): Response {
  return new Response(JSON.stringify(envelope), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

function session(): StoredSession {
  return {
    accessToken: 'access-old',
    refreshToken: 'refresh-old',
    user: {
      id: 'user-1',
      username: 'linhai',
      nickname: '林海',
      email: 'old@example.com',
      avatar: null,
    },
  }
}

function userResponse(overrides: Partial<UserResponse> = {}): UserResponse {
  return {
    id: 'user-1',
    username: 'linhai',
    nickname: '海风',
    email: 'sea@example.com',
    avatar: null,
    created_at: '2026-07-13T08:00:00Z',
    ...overrides,
  }
}

beforeEach(() => {
  clearSession()
  localStorage.setItem(storageKey, JSON.stringify(session()))
  vi.restoreAllMocks()
})

describe('profile API', () => {
  it('updates profile data and persists the returned user in the session', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse<UserResponse>({ code: 0, message: 'ok', data: userResponse() }),
    )
    vi.stubGlobal('fetch', fetchMock)

    const updated = await updateProfile({ nickname: '海风', email: 'sea@example.com' })

    expect(updated.user.nickname).toBe('海风')
    expect(JSON.parse(localStorage.getItem(storageKey) ?? '{}').user.email).toBe('sea@example.com')
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining('/api/auth/profile'),
      expect.objectContaining({ method: 'PUT' }),
    )
    expect(JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body))).toEqual({
      nickname: '海风',
      email: 'sea@example.com',
    })
  })

  it('sends an empty email so the server can clear it', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse<UserResponse>({
        code: 0,
        message: 'ok',
        data: userResponse({ email: null }),
      }),
    )
    vi.stubGlobal('fetch', fetchMock)

    await updateProfile({ nickname: '海风', email: '' })

    expect(JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body)).email).toBe('')
  })

  it('handles password success responses without data', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ code: 0, message: 'ok' }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(changePassword({ old_password: 'secret-old', new_password: 'secret-new' })).resolves.toBeUndefined()
    expect(JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body))).toEqual({
      old_password: 'secret-old',
      new_password: 'secret-new',
    })
  })

  it('refreshes a 401 token once before retrying the profile update', async () => {
    const refreshed: AuthResponse = {
      user_id: 'user-1',
      username: 'linhai',
      email: 'old@example.com',
      access_token: 'access-new',
      refresh_token: 'refresh-new',
    }
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse({ code: 40101, message: 'expired' }, 401))
      .mockResolvedValueOnce(jsonResponse<AuthResponse>({ code: 0, message: 'ok', data: refreshed }))
      .mockResolvedValueOnce(jsonResponse<UserResponse>({ code: 0, message: 'ok', data: userResponse() }))
    vi.stubGlobal('fetch', fetchMock)

    await updateProfile({ nickname: '海风', email: 'sea@example.com' })

    expect(fetchMock).toHaveBeenCalledTimes(3)
    const retryHeaders = fetchMock.mock.calls[2]?.[1]?.headers as Headers
    expect(retryHeaders.get('Authorization')).toBe('Bearer access-new')
  })

  it('refreshes the cached profile from auth me', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
      jsonResponse<UserResponse>({ code: 0, message: 'ok', data: userResponse({ nickname: '新昵称' }) }),
    ))

    const updated = await refreshProfileSession()

    expect(updated.user.nickname).toBe('新昵称')
  })
})
