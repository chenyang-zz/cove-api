// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { StoredSession } from '../features/auth/types'

const session: StoredSession = {
  accessToken: 'access',
  refreshToken: 'refresh',
  user: { id: 'user-1', username: 'linhai', nickname: null, email: null, avatar: null },
}

vi.mock('../features/auth/AuthScreen', () => ({
  AuthScreen: ({
    initialMode,
    nativePage,
    onAuthenticated,
    onModeChange,
    onSubmissionStart,
    onSubmissionFailure,
  }: {
    initialMode: string
    nativePage: boolean
    onAuthenticated: (session: StoredSession) => void
    onModeChange: (mode: 'login' | 'register') => boolean
    onSubmissionStart: (mode: 'login' | 'register') => void
    onSubmissionFailure: (mode: 'login' | 'register') => void
  }) => (
    <section aria-label="原生注册" data-mode={initialMode} data-native={String(nativePage)}>
      <button type="button" onClick={() => onModeChange('login')}>返回登录</button>
      <button type="button" onClick={() => onSubmissionStart('register')}>开始注册</button>
      <button type="button" onClick={() => onSubmissionFailure('register')}>注册失败</button>
      <button type="button" onClick={() => onAuthenticated(session)}>注册成功</button>
    </section>
  ),
}))

import { NativeRegisterApp } from './NativeRegisterApp'

afterEach(() => {
  cleanup()
  delete window.webkit
})

describe('NativeRegisterApp', () => {
  it('announces readiness, returns natively, and completes authentication', async () => {
    const postMessage = vi.fn()
    window.webkit = { messageHandlers: { coveNavigation: { postMessage } } }
    const user = userEvent.setup()
    render(<NativeRegisterApp />)

    const page = screen.getByLabelText('原生注册')
    expect(page.getAttribute('data-mode')).toBe('register')
    expect(page.getAttribute('data-native')).toBe('true')
    expect(postMessage).toHaveBeenCalledWith({ action: 'registerReady' })

    await user.click(screen.getByRole('button', { name: '返回登录' }))
    await user.click(screen.getByRole('button', { name: '开始注册' }))
    await user.click(screen.getByRole('button', { name: '注册失败' }))
    await user.click(screen.getByRole('button', { name: '注册成功' }))

    expect(postMessage).toHaveBeenCalledWith({ action: 'popRegister' })
    expect(postMessage).toHaveBeenCalledWith({ action: 'prepareChat' })
    expect(postMessage).toHaveBeenCalledWith({ action: 'prepareRegister' })
    expect(postMessage).toHaveBeenCalledWith({ action: 'authCompleted' })
  })
})
