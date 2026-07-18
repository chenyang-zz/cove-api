// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { StoredSession } from '../auth/types'

const mocks = vi.hoisted(() => ({
  changePassword: vi.fn(),
  refreshProfileSession: vi.fn(),
  updateProfile: vi.fn(),
}))

vi.mock('./api', () => ({
  changePassword: mocks.changePassword,
  refreshProfileSession: mocks.refreshProfileSession,
  updateProfile: mocks.updateProfile,
}))

import { ProfileScreen } from './ProfileScreen'

const session: StoredSession = {
  accessToken: 'access',
  refreshToken: 'refresh',
  user: {
    id: 'user-1',
    username: 'linhai',
    nickname: '林海',
    email: 'linhai@example.com',
    avatar: null,
  },
}

beforeEach(() => {
  vi.restoreAllMocks()
  mocks.refreshProfileSession.mockReset().mockResolvedValue(session)
  mocks.updateProfile.mockReset().mockResolvedValue(session)
  mocks.changePassword.mockReset().mockResolvedValue(undefined)
  vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
    callback(0)
    return 0
  })
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

describe('ProfileScreen', () => {
  it('renders cached identity while refreshing and keeps unsupported rows read-only', async () => {
    mocks.refreshProfileSession.mockReturnValue(new Promise(() => {}))
    render(
      <ProfileScreen session={session} onBack={vi.fn()} onLogout={vi.fn()} onSessionChange={vi.fn()} />,
    )

    expect(screen.getByRole('heading', { name: '个人信息' })).toBeTruthy()
    expect(screen.getAllByText('林海')).toHaveLength(2)
    expect(screen.getAllByText('@linhai')).toHaveLength(2)
    expect(screen.getByText('暂未接入')).toBeTruthy()
    expect(screen.queryByRole('button', { name: /登录设备/ })).toBeNull()
    expect(screen.queryByText('正在刷新个人信息…')).toBeNull()
    await waitFor(() => expect(mocks.refreshProfileSession).toHaveBeenCalledTimes(1))
  })

  it('focuses the page root instead of leaving focus on the back button when reactivated', () => {
    const props = {
      session,
      onBack: vi.fn(),
      onLogout: vi.fn(),
      onSessionChange: vi.fn(),
    }
    const { rerender } = render(<ProfileScreen {...props} active={false} />)
    const backButton = screen.getByRole('button', { name: '返回聊天' })
    backButton.focus()
    expect(document.activeElement).toBe(backButton)

    rerender(<ProfileScreen {...props} active />)

    expect(document.activeElement).toBe(screen.getByLabelText('个人信息'))
    expect(document.activeElement).not.toBe(backButton)
  })

  it('paints the first sheet below the viewport before sliding it open', () => {
    const frames: FrameRequestCallback[] = []
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
      frames.push(callback)
      return frames.length
    })
    vi.stubGlobal('cancelAnimationFrame', vi.fn())
    render(
      <ProfileScreen active={false} session={session} onBack={vi.fn()} onLogout={vi.fn()} onSessionChange={vi.fn()} />,
    )

    fireEvent.click(screen.getByRole('button', { name: '编辑' }))
    const layer = document.querySelector('.profile-sheet-layer') as HTMLElement
    expect(layer.getAttribute('data-state')).toBe('preparing')

    act(() => frames.shift()?.(0))
    expect(layer.getAttribute('data-state')).toBe('preparing')
    act(() => frames.shift()?.(16))
    expect(layer.getAttribute('data-state')).toBe('open')
  })

  it('locks parent navigation while an edit sheet is open', async () => {
    const user = userEvent.setup()
    const onNavigationLockChange = vi.fn()
    render(
      <ProfileScreen
        active
        session={session}
        onBack={vi.fn()}
        onLogout={vi.fn()}
        onSessionChange={vi.fn()}
        onNavigationLockChange={onNavigationLockChange}
      />,
    )

    await user.click(screen.getByRole('button', { name: '编辑' }))
    expect(onNavigationLockChange).toHaveBeenLastCalledWith(true)
    fireEvent.keyDown(document, { key: 'Escape' })
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(onNavigationLockChange).toHaveBeenLastCalledWith(false)
  })

  it('edits nickname and email in the bottom sheet', async () => {
    const user = userEvent.setup()
    const updated = {
      ...session,
      user: { ...session.user, nickname: '海风', email: 'sea@example.com' },
    }
    mocks.updateProfile.mockResolvedValue(updated)
    const onSessionChange = vi.fn()
    render(
      <ProfileScreen session={session} onBack={vi.fn()} onLogout={vi.fn()} onSessionChange={onSessionChange} />,
    )

    await user.click(screen.getByRole('button', { name: '编辑' }))
    const nickname = screen.getByLabelText('昵称')
    const email = screen.getByLabelText('邮箱')
    await user.clear(nickname)
    await user.type(nickname, '海风')
    await user.clear(email)
    await user.type(email, 'sea@example.com')
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => expect(mocks.updateProfile).toHaveBeenCalledWith({
      nickname: '海风',
      email: 'sea@example.com',
    }))
    expect(onSessionChange).toHaveBeenCalledWith(updated)
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(screen.getByText('个人信息已更新。')).toBeTruthy()
  })

  it('blocks duplicate profile submissions while saving', async () => {
    const user = userEvent.setup()
    let resolveUpdate: ((value: StoredSession) => void) | undefined
    mocks.updateProfile.mockReturnValue(new Promise<StoredSession>((resolve) => {
      resolveUpdate = resolve
    }))
    render(
      <ProfileScreen session={session} onBack={vi.fn()} onLogout={vi.fn()} onSessionChange={vi.fn()} />,
    )

    await user.click(screen.getByRole('button', { name: '编辑' }))
    const form = document.getElementById('profile-sheet-form') as HTMLFormElement
    fireEvent.submit(form)
    fireEvent.submit(form)

    expect(mocks.updateProfile).toHaveBeenCalledTimes(1)
    expect((screen.getByRole('button', { name: '保存中' }) as HTMLButtonElement).disabled).toBe(true)
    await act(async () => resolveUpdate?.(session))
  })

  it('validates password fields and preserves them after a failed request', async () => {
    const user = userEvent.setup()
    mocks.changePassword.mockRejectedValue(new Error('原密码错误'))
    render(
      <ProfileScreen session={session} onBack={vi.fn()} onLogout={vi.fn()} onSessionChange={vi.fn()} />,
    )

    await user.click(screen.getByRole('button', { name: /密码.*修改密码/ }))
    await user.click(screen.getByRole('button', { name: '保存' }))
    expect(screen.getByText('请输入原密码。')).toBeTruthy()
    expect(screen.getByText('新密码至少需要 6 个字符。')).toBeTruthy()

    const oldPassword = screen.getByLabelText(/^原密码/)
    const newPassword = screen.getByLabelText(/^新密码/)
    const confirmation = screen.getByLabelText(/^确认新密码/)
    await user.type(oldPassword, 'secret-old')
    await user.type(newPassword, 'secret-new')
    await user.type(confirmation, 'secret-new')
    await user.click(screen.getByRole('button', { name: '保存' }))

    expect(await screen.findByText('原密码错误')).toBeTruthy()
    expect((oldPassword as HTMLInputElement).value).toBe('secret-old')
    expect((newPassword as HTMLInputElement).value).toBe('secret-new')
  })

  it('closes a sheet with Escape or a blank-area click', async () => {
    const user = userEvent.setup()
    render(
      <ProfileScreen session={session} onBack={vi.fn()} onLogout={vi.fn()} onSessionChange={vi.fn()} />,
    )

    await user.click(screen.getByRole('button', { name: '编辑' }))
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(document.querySelector('.profile-sheet-layer')?.getAttribute('data-state')).toBe('closing')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())

    await user.click(screen.getByRole('button', { name: '编辑' }))
    const layer = document.querySelector('.profile-sheet-layer') as HTMLElement
    fireEvent.mouseDown(layer)
    expect(layer.getAttribute('data-state')).toBe('closing')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  })

  it('keeps a stable viewport and exposes keyboard height for an animated sheet transition', () => {
    const listeners = new Map<string, EventListener>()
    const viewport = {
      height: 844,
      width: 390,
      offsetTop: 280,
      addEventListener: vi.fn((type: string, listener: EventListener) => listeners.set(type, listener)),
      removeEventListener: vi.fn((type: string) => listeners.delete(type)),
    }
    vi.stubGlobal('visualViewport', viewport)
    const { container, unmount } = render(
      <ProfileScreen session={session} onBack={vi.fn()} onLogout={vi.fn()} onSessionChange={vi.fn()} />,
    )
    const root = container.querySelector<HTMLElement>('.profile-screen')
    expect(root?.style.getPropertyValue('--profile-viewport-height')).toBe('844px')

    viewport.height = 516
    act(() => listeners.get('resize')?.(new Event('resize')))
    expect(root?.style.getPropertyValue('--profile-viewport-height')).toBe('844px')
    expect(root?.style.getPropertyValue('--profile-keyboard-height')).toBe('328px')
    expect(root?.dataset.keyboardOpen).toBe('true')

    viewport.height = 844
    act(() => listeners.get('resize')?.(new Event('resize')))
    expect(root?.style.getPropertyValue('--profile-keyboard-height')).toBe('0px')
    expect(root?.dataset.keyboardOpen).toBe('false')

    unmount()
    expect(viewport.removeEventListener).toHaveBeenCalledWith('resize', expect.any(Function))
  })
})
