// @vitest-environment jsdom

import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { clearSession } from './api'
import { AuthScreen } from './AuthScreen'
import type { ApiEnvelope, AuthResponse } from './types'

function jsonResponse<T>(envelope: ApiEnvelope<T>, status = 200): Response {
  return new Response(JSON.stringify(envelope), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

beforeEach(() => {
  clearSession()
  vi.restoreAllMocks()
  vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
    callback(0)
    return 0
  })
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

describe('AuthScreen', () => {
  it('moves the tab indicator when switching authentication modes', async () => {
    const user = userEvent.setup()
    render(<AuthScreen onAuthenticated={vi.fn()} />)

    const tablist = screen.getByRole('tablist')
    expect(tablist.classList.contains('auth-tabs--register')).toBe(false)
    expect(tablist.querySelector('.auth-tabs__indicator')).toBeTruthy()

    await user.click(screen.getByRole('tab', { name: '注册' }))

    expect(tablist.classList.contains('auth-tabs--register')).toBe(true)
    expect(screen.getByRole('tab', { name: '注册' }).getAttribute('aria-selected')).toBe('true')
  })

  it('delegates native registration navigation without replacing the login page', async () => {
    const onModeChange = vi.fn().mockReturnValue(true)
    const user = userEvent.setup()
    render(
      <AuthScreen
        nativePage
        onAuthenticated={vi.fn()}
        onModeChange={onModeChange}
      />,
    )

    expect(screen.queryByRole('tablist')).toBeNull()
    await user.type(screen.getByLabelText('用户名或邮箱'), 'linhai')
    await user.click(screen.getByRole('button', { name: '还没有账号？创建账号' }))

    expect(onModeChange).toHaveBeenCalledWith('register')
    expect((screen.getByLabelText('用户名或邮箱') as HTMLInputElement).value).toBe('linhai')
    expect(screen.queryByLabelText('用户名')).toBeNull()
  })

  it('renders registration as an immersive native page with the footer back action', async () => {
    const onModeChange = vi.fn().mockReturnValue(true)
    const user = userEvent.setup()
    render(
      <AuthScreen
        initialMode="register"
        nativePage
        onAuthenticated={vi.fn()}
        onModeChange={onModeChange}
      />,
    )

    expect(screen.queryByRole('tablist')).toBeNull()
    expect(document.querySelector('.auth-page-header')).toBeNull()
    expect(screen.getByRole('heading', { name: '创建你的账号' })).toBeTruthy()
    await user.click(screen.getByRole('button', { name: '已有账号？返回登录' }))
    expect(onModeChange).toHaveBeenCalledWith('login')
  })

  it('shows inline validation and focuses the first invalid registration field', async () => {
    const onSubmissionStart = vi.fn()
    const user = userEvent.setup()
    render(<AuthScreen onAuthenticated={vi.fn()} onSubmissionStart={onSubmissionStart} />)

    await user.click(screen.getByRole('tab', { name: '注册' }))
    await user.click(screen.getByRole('button', { name: '创建账号' }))

    expect(screen.getByText('请输入用户名。')).toBeTruthy()
    expect(screen.getByText('密码至少需要 6 个字符。')).toBeTruthy()
    expect(document.activeElement).toBe(screen.getByLabelText('用户名'))
    expect(onSubmissionStart).not.toHaveBeenCalled()
  })

  it('creates a session from a mocked registration response', async () => {
    const response: AuthResponse = {
      user_id: 'user-2',
      username: 'muyu',
      email: 'muyu@example.com',
      access_token: 'access-new',
      refresh_token: 'refresh-new',
    }
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse<AuthResponse>({ code: 0, message: 'ok', data: response }),
    )
    vi.stubGlobal('fetch', fetchMock)
    const onAuthenticated = vi.fn()
    const onSubmissionStart = vi.fn()
    const onSubmissionFailure = vi.fn()
    const user = userEvent.setup()
    render(
      <AuthScreen
        onAuthenticated={onAuthenticated}
        onSubmissionStart={onSubmissionStart}
        onSubmissionFailure={onSubmissionFailure}
      />,
    )

    await user.click(screen.getByRole('tab', { name: '注册' }))
    await user.type(screen.getByLabelText('用户名'), 'muyu')
    await user.type(screen.getByLabelText('邮箱 可选'), 'muyu@example.com')
    await user.type(screen.getByLabelText('密码'), 'secret123')
    await user.type(screen.getByLabelText('确认密码'), 'secret123')
    await user.click(screen.getByRole('button', { name: '创建账号' }))

    await waitFor(() => expect(onAuthenticated).toHaveBeenCalledTimes(1))
    expect(onSubmissionStart).toHaveBeenCalledWith('register')
    expect(onSubmissionFailure).not.toHaveBeenCalled()
    const request = fetchMock.mock.calls[0]?.[1] as RequestInit
    expect(JSON.parse(String(request.body))).toEqual({
      username: 'muyu',
      email: 'muyu@example.com',
      password: 'secret123',
    })
  })

  it('renders server credential errors without losing form values', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(jsonResponse({ code: 40102, message: '邮箱或密码错误' }, 401)),
    )
    const onSubmissionStart = vi.fn()
    const onSubmissionFailure = vi.fn()
    const user = userEvent.setup()
    render(
      <AuthScreen
        onAuthenticated={vi.fn()}
        onSubmissionStart={onSubmissionStart}
        onSubmissionFailure={onSubmissionFailure}
      />,
    )

    await user.type(screen.getByLabelText('用户名或邮箱'), 'unknown-user')
    await user.type(screen.getByLabelText('密码'), 'secret123')
    await user.click(screen.getByRole('button', { name: '登录' }))

    expect((await screen.findByRole('alert')).textContent).toContain('邮箱或密码错误')
    expect((screen.getByLabelText('用户名或邮箱') as HTMLInputElement).value).toBe('unknown-user')
    expect(onSubmissionStart).toHaveBeenCalledWith('login')
    expect(onSubmissionFailure).toHaveBeenCalledWith('login')
  })
})
