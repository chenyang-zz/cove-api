import { useCallback, useEffect, useState } from 'react'
import { clearSession, getStoredSession } from '../features/auth/api'
import type { StoredSession } from '../features/auth/types'
import { AuthenticatedApp } from './App'
import { postNativeNavigation } from './nativeNavigation'

export function NativeChatApp() {
  const [session, setSession] = useState<StoredSession | null>(() => getStoredSession())

  useEffect(() => {
    function handleAuthenticated() {
      setSession(getStoredSession())
    }

    function handleProfileSessionChanged() {
      setSession(getStoredSession())
    }

    window.addEventListener('cove:native-chat-authenticated', handleAuthenticated)
    window.addEventListener('cove:native-profile-session-changed', handleProfileSessionChanged)
    postNativeNavigation('chatReady')
    return () => {
      window.removeEventListener('cove:native-chat-authenticated', handleAuthenticated)
      window.removeEventListener('cove:native-profile-session-changed', handleProfileSessionChanged)
    }
  }, [])

  useEffect(() => {
    if (session) {
      postNativeNavigation('chatSessionReady')
    }
  }, [session])

  const handleLogout = useCallback(() => {
    clearSession()
    setSession(null)
    postNativeNavigation('chatLogout')
  }, [])

  if (!session) {
    return <main className="launch-screen" aria-label="正在准备对话页面" />
  }

  return (
    <AuthenticatedApp
      session={session}
      onLogout={handleLogout}
      onSessionChange={setSession}
    />
  )
}
