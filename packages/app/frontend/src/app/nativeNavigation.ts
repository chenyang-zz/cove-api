export type NativeNavigationAction =
  | 'prepareRegister'
  | 'pushRegister'
  | 'popRegister'
  | 'registerReady'
  | 'authCompleted'
  | 'prepareChat'
  | 'chatReady'
  | 'chatSessionReady'
  | 'chatLogout'
  | 'pushProfile'
  | 'popProfile'
  | 'profileReady'
  | 'profileNavigationLock'
  | 'profileSessionChanged'
  | 'profileLogout'

type NativeNavigationPayload = Record<string, string | number | boolean | null>

type NativeNavigationHandler = {
  postMessage: (message: { action: NativeNavigationAction } & NativeNavigationPayload) => void
}

declare global {
  interface Window {
    webkit?: {
      messageHandlers?: {
        coveNavigation?: NativeNavigationHandler
      }
    }
  }
}

export function hasNativeNavigationBridge(): boolean {
  return typeof window !== 'undefined'
    && typeof window.webkit?.messageHandlers?.coveNavigation?.postMessage === 'function'
}

export function postNativeNavigation(
  action: NativeNavigationAction,
  payload: NativeNavigationPayload = {},
): boolean {
  const handler = window.webkit?.messageHandlers?.coveNavigation
  if (!handler) {
    return false
  }
  handler.postMessage({ action, ...payload })
  return true
}

export function isNativeProfileEntry(location: Pick<Location, 'search'> = window.location): boolean {
  return new URLSearchParams(location.search).get('coveRoute') === 'profile'
}

export function isNativeRegisterEntry(location: Pick<Location, 'search'> = window.location): boolean {
  return new URLSearchParams(location.search).get('coveRoute') === 'register'
}

export function isNativeChatEntry(location: Pick<Location, 'search'> = window.location): boolean {
  return new URLSearchParams(location.search).get('coveRoute') === 'chat'
}
