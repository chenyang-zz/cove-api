import { useCallback, useEffect, useState } from 'react'
import { clearSession, getStoredSession } from '../features/auth/api'
import type { StoredSession } from '../features/auth/types'
import { ProfileScreen } from '../features/profile/ProfileScreen'
import { postNativeNavigation } from './nativeNavigation'

export function NativeProfileApp() {
  const [session, setSession] = useState<StoredSession | null>(() => getStoredSession())
  const [active, setActive] = useState(false)

  useEffect(() => {
    function activateProfile() {
      const current = getStoredSession()
      setSession(current)
      setActive(Boolean(current))
      if (!current) {
        postNativeNavigation('profileLogout')
      }
    }

    function hideProfile() {
      setActive(false)
    }

    window.addEventListener('cove:native-profile-activate', activateProfile)
    window.addEventListener('cove:native-profile-hidden', hideProfile)
    postNativeNavigation('profileReady')
    return () => {
      window.removeEventListener('cove:native-profile-activate', activateProfile)
      window.removeEventListener('cove:native-profile-hidden', hideProfile)
    }
  }, [])

  const handleSessionChange = useCallback((nextSession: StoredSession) => {
    setSession(nextSession)
    postNativeNavigation('profileSessionChanged')
  }, [])

  const handleLogout = useCallback(() => {
    clearSession()
    setActive(false)
    setSession(null)
    postNativeNavigation('profileLogout')
  }, [])

  if (!session) {
    return <main className="launch-screen" aria-label="正在准备个人信息" />
  }

  return (
    <ProfileScreen
      active={active}
      session={session}
      onBack={() => postNativeNavigation('popProfile')}
      onLogout={handleLogout}
      onSessionChange={handleSessionChange}
      onNavigationLockChange={(locked) => postNativeNavigation('profileNavigationLock', { locked })}
    />
  )
}
