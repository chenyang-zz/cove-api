export type ProfileUpdateInput = {
  nickname: string
  email: string
}

export type PasswordChangeInput = {
  old_password: string
  new_password: string
}

export type ProfileSheetState =
  | { kind: 'profile'; focus: 'nickname' | 'email' | null }
  | { kind: 'password' }
  | null

export type ProfileFieldErrors = Partial<
  Record<'nickname' | 'email' | 'old_password' | 'new_password' | 'confirm_password', string>
>
