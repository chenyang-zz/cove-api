import * as SecureStore from 'expo-secure-store';

import type { StoredSession } from './types';

const SESSION_KEY = 'cove.auth.session.v1';

function isStoredSession(value: unknown): value is StoredSession {
  if (!value || typeof value !== 'object') {
    return false;
  }
  const session = value as Partial<StoredSession>;
  return Boolean(
    typeof session.accessToken === 'string' &&
      typeof session.refreshToken === 'string' &&
      session.user &&
      typeof session.user.id === 'string' &&
      typeof session.user.username === 'string',
  );
}

export async function loadStoredSession(): Promise<StoredSession | null> {
  const raw = await SecureStore.getItemAsync(SESSION_KEY);
  if (!raw) {
    return null;
  }
  try {
    const parsed: unknown = JSON.parse(raw);
    if (isStoredSession(parsed)) {
      return parsed;
    }
  } catch {
    // Invalid persisted data is cleared below.
  }
  await clearStoredSession();
  return null;
}

export async function saveStoredSession(session: StoredSession): Promise<StoredSession> {
  await SecureStore.setItemAsync(SESSION_KEY, JSON.stringify(session));
  return session;
}

export async function clearStoredSession(): Promise<void> {
  await SecureStore.deleteItemAsync(SESSION_KEY);
}
