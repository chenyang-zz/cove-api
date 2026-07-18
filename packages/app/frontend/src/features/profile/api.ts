import {
  authenticatedCommand,
  authenticatedRequest,
  getCurrentUser,
  saveCurrentUser,
} from '../auth/api'
import type { StoredSession, UserResponse } from '../auth/types'
import type { PasswordChangeInput, ProfileUpdateInput } from './types'

export async function refreshProfileSession(): Promise<StoredSession> {
  return saveCurrentUser(await getCurrentUser())
}

export async function updateProfile(input: ProfileUpdateInput): Promise<StoredSession> {
  const user = await authenticatedRequest<UserResponse>('/api/auth/profile', {
    method: 'PUT',
    body: JSON.stringify(input),
  })
  return saveCurrentUser(user)
}

export function changePassword(input: PasswordChangeInput): Promise<void> {
  return authenticatedCommand('/api/auth/password', {
    method: 'POST',
    body: JSON.stringify(input),
  })
}
