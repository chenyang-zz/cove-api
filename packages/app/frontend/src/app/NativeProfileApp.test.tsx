// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { StoredSession } from '../features/auth/types'

vi.mock('../features/profile/ProfileScreen', () => ({
  ProfileScreen: ({
    active,
    onBack,
    onLogout,
    onSessionChange,
    onNavigationLockChange,
  }: {
    active: boolean
    onBack: () => void
    onLogout: () => void
    onSessionChange: (session: StoredSession) => void
    onNavigationLockChange: (locked: boolean) => void
  }) => (
    <section aria-label="原生个人信息" data-active={String(active)}>
      <button type="button" onClick={onBack}>返回</button>
      <button type="button" onClick={onLogout}>退出</button>
      <button type="button" onClick={() => onSessionChange(updatedSession)}>更新</button>
      <button type="button" onClick={() => onNavigationLockChange(true)}>锁定</button>
    </section>
  ),
}))

import { NativeProfileApp } from './NativeProfileApp'

const session: StoredSession = {
  accessToken: 'access',
  refreshToken: 'refresh',
  user: { id: 'user-1', username: 'linhai', nickname: '林海', email: null, avatar: null },
}

const updatedSession: StoredSession = {
  ...session,
  user: { ...session.user, nickname: '海风' },
}

afterEach(() => {
  cleanup()
  localStorage.clear()
  delete window.webkit
})

describe('NativeProfileApp', () => {
  it('prewarms inactive, activates from shared storage, and emits native actions', async () => {
    localStorage.setItem('cove.auth.session.v1', JSON.stringify(session))
    const postMessage = vi.fn()
    window.webkit = { messageHandlers: { coveNavigation: { postMessage } } }
    const user = userEvent.setup()
    render(<NativeProfileApp />)

    expect(screen.getByLabelText('原生个人信息').getAttribute('data-active')).toBe('false')
    expect(postMessage).toHaveBeenCalledWith({ action: 'profileReady' })

    fireEvent(window, new CustomEvent('cove:native-profile-activate'))
    await waitFor(() => expect(screen.getByLabelText('原生个人信息').getAttribute('data-active')).toBe('true'))

    await user.click(screen.getByRole('button', { name: '锁定' }))
    await user.click(screen.getByRole('button', { name: '更新' }))
    await user.click(screen.getByRole('button', { name: '返回' }))

    expect(postMessage).toHaveBeenCalledWith({ action: 'profileNavigationLock', locked: true })
    expect(postMessage).toHaveBeenCalledWith({ action: 'profileSessionChanged' })
    expect(postMessage).toHaveBeenCalledWith({ action: 'popProfile' })
  })

  it('clears the shared session before notifying native logout', async () => {
    localStorage.setItem('cove.auth.session.v1', JSON.stringify(session))
    const postMessage = vi.fn()
    window.webkit = { messageHandlers: { coveNavigation: { postMessage } } }
    const user = userEvent.setup()
    render(<NativeProfileApp />)

    await user.click(screen.getByRole('button', { name: '退出' }))

    expect(localStorage.getItem('cove.auth.session.v1')).toBeNull()
    expect(postMessage).toHaveBeenCalledWith({ action: 'profileLogout' })
  })
})
