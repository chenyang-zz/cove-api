// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  hasNativeNavigationBridge,
  isNativeChatEntry,
  isNativeProfileEntry,
  isNativeRegisterEntry,
  postNativeNavigation,
} from './nativeNavigation'

afterEach(() => {
  delete window.webkit
})

describe('native navigation bridge', () => {
  it('detects profile entry URLs', () => {
    expect(isNativeProfileEntry({ search: '?coveRoute=profile' } as Location)).toBe(true)
    expect(isNativeProfileEntry({ search: '?coveRoute=chat' } as Location)).toBe(false)
  })

  it('detects register entry URLs', () => {
    expect(isNativeRegisterEntry({ search: '?coveRoute=register' } as Location)).toBe(true)
    expect(isNativeRegisterEntry({ search: '?coveRoute=profile' } as Location)).toBe(false)
  })

  it('detects native chat entry URLs', () => {
    expect(isNativeChatEntry({ search: '?coveRoute=chat' } as Location)).toBe(true)
    expect(isNativeChatEntry({ search: '?coveRoute=register' } as Location)).toBe(false)
  })

  it('posts structured actions when the iOS bridge exists', () => {
    const postMessage = vi.fn()
    window.webkit = { messageHandlers: { coveNavigation: { postMessage } } }

    expect(hasNativeNavigationBridge()).toBe(true)
    expect(postNativeNavigation('profileNavigationLock', { locked: true })).toBe(true)
    expect(postMessage).toHaveBeenCalledWith({ action: 'profileNavigationLock', locked: true })
  })

  it('falls back without changing state outside the native shell', () => {
    expect(hasNativeNavigationBridge()).toBe(false)
    expect(postNativeNavigation('pushProfile')).toBe(false)
  })
})
