// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { StoredSession } from '../features/auth/types'

const { getStoredSession, clearSession } = vi.hoisted(() => ({
  getStoredSession: vi.fn<() => StoredSession | null>(),
  clearSession: vi.fn(),
}))

vi.mock('../features/auth/api', () => ({
  getStoredSession,
  clearSession,
}))

vi.mock('./App', () => ({
  AuthenticatedApp: ({ onLogout }: { onLogout: () => void }) => (
    <button type="button" onClick={onLogout}>退出测试对话</button>
  ),
}))

import { NativeChatApp } from './NativeChatApp'

const session: StoredSession = {
  accessToken: 'access',
  refreshToken: 'refresh',
  user: { id: 'user-1', username: 'linhai', nickname: '林海', email: null, avatar: null },
}

afterEach(() => {
  cleanup()
  delete window.webkit
  getStoredSession.mockReset()
  clearSession.mockReset()
})

describe('NativeChatApp', () => {
  it('preloads while anonymous and becomes ready after native authentication', async () => {
    const postMessage = vi.fn()
    window.webkit = { messageHandlers: { coveNavigation: { postMessage } } }
    getStoredSession.mockReturnValueOnce(null).mockReturnValue(session)

    render(<NativeChatApp />)
    expect(screen.getByLabelText('正在准备对话页面')).toBeTruthy()
    expect(postMessage).toHaveBeenCalledWith({ action: 'chatReady' })

    act(() => window.dispatchEvent(new CustomEvent('cove:native-chat-authenticated')))

    await waitFor(() => expect(screen.getByRole('button', { name: '退出测试对话' })).toBeTruthy())
    expect(postMessage).toHaveBeenCalledWith({ action: 'chatSessionReady' })
  })

  it('clears the session and returns to the native authentication root', () => {
    const postMessage = vi.fn()
    window.webkit = { messageHandlers: { coveNavigation: { postMessage } } }
    getStoredSession.mockReturnValue(session)

    render(<NativeChatApp />)
    fireEvent.click(screen.getByRole('button', { name: '退出测试对话' }))

    expect(clearSession).toHaveBeenCalledOnce()
    expect(postMessage).toHaveBeenCalledWith({ action: 'chatLogout' })
  })
})
