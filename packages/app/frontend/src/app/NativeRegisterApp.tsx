import { useCallback, useEffect } from 'react'
import { AuthScreen } from '../features/auth/AuthScreen'
import type { StoredSession } from '../features/auth/types'
import { postNativeNavigation } from './nativeNavigation'

export function NativeRegisterApp() {
  useEffect(() => {
    postNativeNavigation('registerReady')
  }, [])

  const handleAuthenticated = useCallback((_session: StoredSession) => {
    postNativeNavigation('authCompleted')
  }, [])

  return (
    <AuthScreen
      initialMode="register"
      nativePage
      onAuthenticated={handleAuthenticated}
      onModeChange={(mode) => mode === 'login' && postNativeNavigation('popRegister')}
      onSubmissionStart={() => postNativeNavigation('prepareChat')}
      onSubmissionFailure={() => postNativeNavigation('prepareRegister')}
    />
  )
}
