import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type PropsWithChildren,
} from 'react';

import {
  changePassword as changePasswordRequest,
  login,
  logout,
  register,
  restoreSession,
  updateProfile as updateProfileRequest,
} from '@/core/api';
import type {
  LoginInput,
  PasswordChangeInput,
  ProfileUpdateInput,
  RegisterInput,
  StoredSession,
} from '@/core/types';

type AuthStatus = 'restoring' | 'anonymous' | 'authenticated';

type AuthContextValue = {
  status: AuthStatus;
  session: StoredSession | null;
  signIn: (input: LoginInput) => Promise<StoredSession>;
  signUp: (input: RegisterInput) => Promise<StoredSession>;
  signOut: () => Promise<void>;
  updateProfile: (input: ProfileUpdateInput) => Promise<StoredSession>;
  changePassword: (input: PasswordChangeInput) => Promise<void>;
};

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: PropsWithChildren) {
  const [status, setStatus] = useState<AuthStatus>('restoring');
  const [session, setSession] = useState<StoredSession | null>(null);

  useEffect(() => {
    let active = true;
    void restoreSession()
      .then((restored) => {
        if (!active) {
          return;
        }
        setSession(restored);
        setStatus(restored ? 'authenticated' : 'anonymous');
      })
      .catch(() => {
        if (!active) {
          return;
        }
        setSession(null);
        setStatus('anonymous');
      });
    return () => {
      active = false;
    };
  }, []);

  const signIn = useCallback(async (input: LoginInput) => {
    const next = await login(input);
    setSession(next);
    setStatus('authenticated');
    return next;
  }, []);

  const signUp = useCallback(async (input: RegisterInput) => {
    const next = await register(input);
    setSession(next);
    setStatus('authenticated');
    return next;
  }, []);

  const signOut = useCallback(async () => {
    await logout();
    setSession(null);
    setStatus('anonymous');
  }, []);

  const updateProfile = useCallback(async (input: ProfileUpdateInput) => {
    const next = await updateProfileRequest(input);
    setSession(next);
    return next;
  }, []);

  const changePassword = useCallback(async (input: PasswordChangeInput) => {
    await changePasswordRequest(input);
  }, []);

  const value = useMemo(
    () => ({ status, session, signIn, signUp, signOut, updateProfile, changePassword }),
    [changePassword, session, signIn, signOut, signUp, status, updateProfile],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const value = useContext(AuthContext);
  if (!value) {
    throw new Error('useAuth must be used within AuthProvider.');
  }
  return value;
}
