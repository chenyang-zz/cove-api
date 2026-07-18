import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import './App.css'
import { AuthScreen } from '../features/auth/AuthScreen'
import { clearSession, getStoredSession, restoreSession } from '../features/auth/api'
import type { StoredSession } from '../features/auth/types'
import { ChatScreen } from '../features/chat/ChatScreen'
import { ProfileScreen } from '../features/profile/ProfileScreen'
import { useAppNavigation, useEdgeBackGesture } from './navigation'
import { hasNativeNavigationBridge, postNativeNavigation } from './nativeNavigation'

const coveIcon = '/cove-mark.svg'

type AuthState =
  | { status: 'restoring' }
  | { status: 'anonymous' }
  | { status: 'authenticated'; session: StoredSession }

type AuthenticatedAppProps = {
  session: StoredSession
  onLogout: () => void
  onSessionChange: (session: StoredSession) => void
}

type AnonymousAuthScreenProps = {
  onAuthenticated: (session: StoredSession) => void
}

export function AnonymousAuthScreen({ onAuthenticated }: AnonymousAuthScreenProps) {
  const nativeNavigation = hasNativeNavigationBridge()

  useEffect(() => {
    if (nativeNavigation) {
      postNativeNavigation('prepareRegister')
    }
  }, [nativeNavigation])

  const handleAuthenticated = useCallback((session: StoredSession) => {
    if (!postNativeNavigation('authCompleted')) {
      onAuthenticated(session)
    }
  }, [onAuthenticated])

  return (
    <AuthScreen
      nativePage={nativeNavigation}
      onModeChange={(mode) => mode === 'register' && postNativeNavigation('pushRegister')}
      onSubmissionStart={() => postNativeNavigation('prepareChat')}
      onSubmissionFailure={() => postNativeNavigation('prepareRegister')}
      onAuthenticated={handleAuthenticated}
    />
  )
}

export function NativeAuthenticatedBootstrap() {
  useEffect(() => {
    postNativeNavigation('authCompleted')
  }, [])

  return <main className="launch-screen" aria-label="正在打开对话页面" />
}

export function AuthenticatedApp({ session, onLogout, onSessionChange }: AuthenticatedAppProps) {
  const stageRef = useRef<HTMLDivElement | null>(null)
  const previousRouteRef = useRef<'chat' | 'profile'>('chat')
  const [profileVisited, setProfileVisited] = useState(false)
  const [profilePreparing, setProfilePreparing] = useState(false)
  const [profileNavigationLocked, setProfileNavigationLocked] = useState(false)
  const [chatFocusRequest, setChatFocusRequest] = useState(0)
  const { route, direction, navigate, back, reset } = useAppNavigation()

  useEffect(() => {
    if (route === 'profile') {
      setProfileVisited(true)
    }
    if (previousRouteRef.current === 'profile' && route === 'chat') {
      setChatFocusRequest((request) => request + 1)
    }
    previousRouteRef.current = route
  }, [route])

  useEffect(() => () => reset(), [reset])

  useLayoutEffect(() => {
    if (!profilePreparing) {
      return
    }

    // WKWebView may coalesce a newly mounted page and its final transform into
    // one paint. Force the off-screen style to be resolved, then wait through a
    // full frame before applying the route that starts the CSS transition.
    const profilePage = stageRef.current?.querySelector<HTMLElement>('.app-navigation-page--profile')
    void profilePage?.offsetWidth

    let navigationFrame = 0
    const initialFrame = window.requestAnimationFrame(() => {
      navigationFrame = window.requestAnimationFrame(() => {
        navigate('profile')
        setProfilePreparing(false)
      })
    })

    return () => {
      window.cancelAnimationFrame(initialFrame)
      window.cancelAnimationFrame(navigationFrame)
    }
  }, [navigate, profilePreparing])

  const openProfile = useCallback(() => {
    if (postNativeNavigation('pushProfile')) {
      return
    }
    setProfileVisited(true)
    setProfilePreparing(true)
  }, [])

  const returnToChat = useCallback(() => {
    back()
  }, [back])

  const edgeBackGesture = useEdgeBackGesture({
    enabled: route === 'profile' && !profileNavigationLocked,
    onBack: returnToChat,
    stageRef,
  })

  return (
    <div className="authenticated-app">
      <div
        className="app-navigation-stage"
        data-route={route}
        data-direction={direction}
        ref={stageRef}
        onPointerDown={edgeBackGesture.onPointerDown}
        onPointerMove={edgeBackGesture.onPointerMove}
        onPointerUp={edgeBackGesture.onPointerUp}
        onPointerCancel={edgeBackGesture.onPointerCancel}
      >
        <section className="app-navigation-page app-navigation-page--chat" aria-hidden={route !== 'chat'} inert={route !== 'chat' ? true : undefined}>
          <ChatScreen session={session} onLogout={onLogout} onOpenProfile={openProfile} focusRequest={chatFocusRequest} />
        </section>
        {profileVisited && (
          <section className="app-navigation-page app-navigation-page--profile" aria-hidden={route !== 'profile'} inert={route !== 'profile' ? true : undefined}>
            <ProfileScreen
              active={route === 'profile'}
              session={session}
              onBack={returnToChat}
              onLogout={onLogout}
              onSessionChange={onSessionChange}
              onNavigationLockChange={setProfileNavigationLocked}
            />
          </section>
        )}
      </div>
    </div>
  )
}

function App() {
  const [authState, setAuthState] = useState<AuthState>({ status: 'restoring' })

  useEffect(() => {
    let active = true
    restoreSession().then((session) => {
      if (!active) {
        return
      }
      setAuthState(session ? { status: 'authenticated', session } : { status: 'anonymous' })
    })
    return () => {
      active = false
    }
  }, [])

  useEffect(() => {
    function syncNativeProfileSession() {
      const session = getStoredSession()
      setAuthState(session ? { status: 'authenticated', session } : { status: 'anonymous' })
    }

    function handleNativeProfileLogout() {
      clearSession()
      setAuthState({ status: 'anonymous' })
    }

    window.addEventListener('cove:native-profile-session-changed', syncNativeProfileSession)
    window.addEventListener('cove:native-profile-logout', handleNativeProfileLogout)
    return () => {
      window.removeEventListener('cove:native-profile-session-changed', syncNativeProfileSession)
      window.removeEventListener('cove:native-profile-logout', handleNativeProfileLogout)
    }
  }, [])

  const handleLogout = useCallback(() => {
    clearSession()
    setAuthState({ status: 'anonymous' })
  }, [])

  const handleSessionChange = useCallback((session: StoredSession) => {
    setAuthState({ status: 'authenticated', session })
  }, [])

  if (authState.status === 'restoring') {
    return (
      <main className="launch-screen" aria-label="正在恢复登录状态">
        <img src={coveIcon} alt="" />
        <strong>Cove</strong>
        <span className="launch-screen__progress" />
      </main>
    )
  }

  if (authState.status === 'anonymous') {
    return (
      <AnonymousAuthScreen
        onAuthenticated={(session) => setAuthState({ status: 'authenticated', session })}
      />
    )
  }

  if (hasNativeNavigationBridge()) {
    return <NativeAuthenticatedBootstrap />
  }

  return (
    <AuthenticatedApp
      session={authState.session}
      onLogout={handleLogout}
      onSessionChange={handleSessionChange}
    />
  )
}

export default App
