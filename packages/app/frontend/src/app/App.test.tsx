// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { StoredSession } from '../features/auth/types'

let chatMounts = 0
let chatUnmounts = 0

vi.mock('../features/chat/ChatScreen', async () => {
  const React = await import('react')
  return {
    ChatScreen: ({ onOpenProfile, focusRequest = 0 }: { onOpenProfile: () => void; focusRequest?: number }) => {
      React.useEffect(() => {
        chatMounts += 1
        return () => {
          chatUnmounts += 1
        }
      }, [])
      return <><button type="button" onClick={onOpenProfile}>打开个人信息</button><span data-testid="chat-focus-request">{focusRequest}</span></>
    },
  }
})

vi.mock('../features/profile/ProfileScreen', async () => {
  const React = await import('react')
  return {
    ProfileScreen: ({ onBack, onNavigationLockChange }: { onBack: () => void; onNavigationLockChange?: (locked: boolean) => void }) => {
      const [locked, setLocked] = React.useState(false)
      React.useEffect(() => {
        onNavigationLockChange?.(locked)
      }, [locked, onNavigationLockChange])
      return (
        <section aria-label="个人信息页面">
          <button type="button" onClick={onBack}>返回</button>
          <button type="button" onClick={() => setLocked((value) => !value)}>锁定导航</button>
        </section>
      )
    },
  }
})

vi.mock('../features/auth/AuthScreen', () => ({
  AuthScreen: ({
    onAuthenticated,
    onModeChange,
    onSubmissionStart,
    onSubmissionFailure,
  }: {
    onAuthenticated: (session: StoredSession) => void
    onModeChange?: (mode: 'login' | 'register') => boolean
    onSubmissionStart?: (mode: 'login' | 'register') => void
    onSubmissionFailure?: (mode: 'login' | 'register') => void
  }) => (
    <>
      <button type="button" onClick={() => onModeChange?.('register')}>还没有账号？创建账号</button>
      <button type="button" onClick={() => onSubmissionStart?.('login')}>开始认证</button>
      <button type="button" onClick={() => onSubmissionFailure?.('login')}>认证失败</button>
      <button
        type="button"
        onClick={() => onAuthenticated({
          accessToken: 'access',
          refreshToken: 'refresh',
          user: { id: 'user-1', username: 'linhai', nickname: '林海', email: null, avatar: null },
        })}
      >
        完成登录
      </button>
    </>
  ),
}))

import { AnonymousAuthScreen, AuthenticatedApp, NativeAuthenticatedBootstrap } from './App'

const session: StoredSession = {
  accessToken: 'access',
  refreshToken: 'refresh',
  user: { id: 'user-1', username: 'linhai', nickname: '林海', email: null, avatar: null },
}

afterEach(() => {
  cleanup()
  delete window.webkit
  chatMounts = 0
  chatUnmounts = 0
})

describe('AuthenticatedApp', () => {
  it('delegates profile navigation to the native iOS bridge when available', async () => {
    const postMessage = vi.fn()
    window.webkit = { messageHandlers: { coveNavigation: { postMessage } } }
    const user = userEvent.setup()
    render(
      <AuthenticatedApp session={session} onLogout={vi.fn()} onSessionChange={vi.fn()} />,
    )

    await user.click(screen.getByRole('button', { name: '打开个人信息' }))

    expect(postMessage).toHaveBeenCalledWith({ action: 'pushProfile' })
    expect(screen.queryByLabelText('个人信息页面')).toBeNull()
    expect(history.state.route).toBe('chat')
  })

  it('keeps chat mounted while the independent profile page opens and closes', async () => {
    const user = userEvent.setup()
    render(
      <AuthenticatedApp session={session} onLogout={vi.fn()} onSessionChange={vi.fn()} />,
    )
    expect(chatMounts).toBe(1)

    await user.click(screen.getByRole('button', { name: '打开个人信息' }))
    await waitFor(() => expect(screen.getByLabelText('个人信息页面')).toBeTruthy())
    await waitFor(() => expect(history.state.route).toBe('profile'))
    expect(chatMounts).toBe(1)
    expect(chatUnmounts).toBe(0)

    await user.click(screen.getByRole('button', { name: '返回' }))
    expect(screen.getByLabelText('个人信息页面').closest('.app-navigation-page')?.getAttribute('aria-hidden')).toBe('true')
    expect(screen.getByTestId('chat-focus-request').textContent).toBe('1')
    expect(chatMounts).toBe(1)
    expect(chatUnmounts).toBe(0)
  })

  it('uses popstate to restore the chat page', async () => {
    const user = userEvent.setup()
    render(
      <AuthenticatedApp session={session} onLogout={vi.fn()} onSessionChange={vi.fn()} />,
    )
    await user.click(screen.getByRole('button', { name: '打开个人信息' }))
    await waitFor(() => expect(history.state.route).toBe('profile'))
    const profileState = history.state
    act(() => {
      window.dispatchEvent(new PopStateEvent('popstate', { state: { ...profileState, route: 'chat' } }))
    })

    expect(screen.getByLabelText('个人信息页面').closest('.app-navigation-page')?.getAttribute('aria-hidden')).toBe('true')
    expect(screen.getByTestId('chat-focus-request').textContent).toBe('1')
  })

  it('returns to chat after a completed left-edge swipe', async () => {
    const user = userEvent.setup()
    render(
      <AuthenticatedApp session={session} onLogout={vi.fn()} onSessionChange={vi.fn()} />,
    )
    await user.click(screen.getByRole('button', { name: '打开个人信息' }))
    await waitFor(() => expect(screen.getByLabelText('个人信息页面').closest('.app-navigation-page')?.getAttribute('aria-hidden')).toBe('false'))
    const stage = document.querySelector<HTMLElement>('.app-navigation-stage')
    expect(stage).toBeTruthy()

    fireEvent.pointerDown(stage!, { pointerId: 1, button: 0, clientX: 10, clientY: 140, timeStamp: 0 })
    fireEvent.pointerMove(stage!, { pointerId: 1, clientX: 520, clientY: 140, timeStamp: 16 })
    fireEvent.pointerUp(stage!, { pointerId: 1, clientX: 540, clientY: 140, timeStamp: 20 })

    expect(screen.getByLabelText('个人信息页面').closest('.app-navigation-page')?.getAttribute('aria-hidden')).toBe('true')
  })

  it('cancels a short left-edge swipe and keeps the profile page active', async () => {
    const user = userEvent.setup()
    render(
      <AuthenticatedApp session={session} onLogout={vi.fn()} onSessionChange={vi.fn()} />,
    )
    await user.click(screen.getByRole('button', { name: '打开个人信息' }))
    await waitFor(() => expect(screen.getByLabelText('个人信息页面').closest('.app-navigation-page')?.getAttribute('aria-hidden')).toBe('false'))
    const stage = document.querySelector<HTMLElement>('.app-navigation-stage')
    expect(stage).toBeTruthy()

    fireEvent.pointerDown(stage!, { pointerId: 1, button: 0, clientX: 10, clientY: 140, timeStamp: 0 })
    fireEvent.pointerMove(stage!, { pointerId: 1, clientX: 120, clientY: 140, timeStamp: 16 })
    fireEvent.pointerUp(stage!, { pointerId: 1, clientX: 120, clientY: 140, timeStamp: 300 })

    expect(screen.getByLabelText('个人信息页面').closest('.app-navigation-page')?.getAttribute('aria-hidden')).toBe('false')
    expect(stage?.dataset.swipeState).toBeUndefined()
  })

  it('does not start the edge-back gesture while profile interaction is locked', async () => {
    const user = userEvent.setup()
    render(
      <AuthenticatedApp session={session} onLogout={vi.fn()} onSessionChange={vi.fn()} />,
    )
    await user.click(screen.getByRole('button', { name: '打开个人信息' }))
    await waitFor(() => expect(screen.getByLabelText('个人信息页面').closest('.app-navigation-page')?.getAttribute('aria-hidden')).toBe('false'))
    await user.click(screen.getByRole('button', { name: '锁定导航' }))
    const stage = document.querySelector<HTMLElement>('.app-navigation-stage')
    expect(stage).toBeTruthy()

    fireEvent.pointerDown(stage!, { pointerId: 1, button: 0, clientX: 10, clientY: 140, timeStamp: 0 })
    fireEvent.pointerMove(stage!, { pointerId: 1, clientX: 520, clientY: 140, timeStamp: 16 })
    fireEvent.pointerUp(stage!, { pointerId: 1, clientX: 540, clientY: 140, timeStamp: 20 })

    expect(screen.getByLabelText('个人信息页面').closest('.app-navigation-page')?.getAttribute('aria-hidden')).toBe('false')
  })

  it('resets the browser history to chat when the authenticated app unmounts', async () => {
    const user = userEvent.setup()
    const view = render(
      <AuthenticatedApp session={session} onLogout={vi.fn()} onSessionChange={vi.fn()} />,
    )
    await user.click(screen.getByRole('button', { name: '打开个人信息' }))
    await waitFor(() => expect(history.state.route).toBe('profile'))
    expect(history.state.route).toBe('profile')

    view.unmount()

    expect(history.state.route).toBe('chat')
  })
})

describe('AnonymousAuthScreen', () => {
  it('keeps one route-aware native preload while authentication state changes', async () => {
    const postMessage = vi.fn()
    window.webkit = { messageHandlers: { coveNavigation: { postMessage } } }
    const user = userEvent.setup()

    render(<AnonymousAuthScreen onAuthenticated={vi.fn()} />)

    await waitFor(() => {
      expect(postMessage).toHaveBeenCalledWith({ action: 'prepareRegister' })
    })
    expect(postMessage).not.toHaveBeenCalledWith({ action: 'prepareChat' })
    await user.click(screen.getByRole('button', { name: '还没有账号？创建账号' }))

    expect(postMessage).toHaveBeenCalledWith({ action: 'pushRegister' })

    await user.click(screen.getByRole('button', { name: '开始认证' }))
    expect(postMessage).toHaveBeenCalledWith({ action: 'prepareChat' })

    await user.click(screen.getByRole('button', { name: '认证失败' }))
    expect(postMessage).toHaveBeenLastCalledWith({ action: 'prepareRegister' })
  })

  it('hands successful authentication to the native chat transition', async () => {
    const postMessage = vi.fn()
    const onAuthenticated = vi.fn()
    window.webkit = { messageHandlers: { coveNavigation: { postMessage } } }

    const user = userEvent.setup()
    render(<AnonymousAuthScreen onAuthenticated={onAuthenticated} />)
    await user.click(screen.getByRole('button', { name: '完成登录' }))

    expect(postMessage).toHaveBeenCalledWith({ action: 'authCompleted' })
    expect(onAuthenticated).not.toHaveBeenCalled()
  })
})

describe('NativeAuthenticatedBootstrap', () => {
  it('opens a restored session in the independent native chat controller', async () => {
    const postMessage = vi.fn()
    window.webkit = { messageHandlers: { coveNavigation: { postMessage } } }

    render(<NativeAuthenticatedBootstrap />)

    await waitFor(() => {
      expect(postMessage).toHaveBeenCalledWith({ action: 'authCompleted' })
    })
  })
})
