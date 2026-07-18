import {
  CaretLeft,
  CaretRight,
  CheckCircle,
  WarningCircle,
} from '@phosphor-icons/react'
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type FormEvent,
  type MouseEvent,
} from 'react'
import { ApiError } from '../auth/api'
import type { StoredSession } from '../auth/types'
import { changePassword, refreshProfileSession, updateProfile } from './api'
import type { ProfileFieldErrors, ProfileSheetState } from './types'
import './ProfileScreen.css'

type ProfileScreenProps = {
  active?: boolean
  session: StoredSession
  onBack: () => void
  onLogout: () => void
  onSessionChange: (session: StoredSession) => void
  onNavigationLockChange?: (locked: boolean) => void
}

const emailPattern = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
const sheetEnterDuration = 320
const sheetExitDuration = 280

function apiFieldErrors(error: unknown): ProfileFieldErrors {
  if (!(error instanceof ApiError)) {
    return {}
  }
  return error.fieldErrors.reduce<ProfileFieldErrors>((errors, item) => {
    const field = item.field.toLowerCase()
    if (field === 'nickname' || field === 'email' || field === 'old_password' || field === 'new_password') {
      errors[field] = item.message
    }
    return errors
  }, {})
}

export function ProfileScreen({ active = true, session, onBack, onLogout, onSessionChange, onNavigationLockChange }: ProfileScreenProps) {
  const rootRef = useRef<HTMLElement | null>(null)
  const nicknameRef = useRef<HTMLInputElement | null>(null)
  const emailRef = useRef<HTMLInputElement | null>(null)
  const oldPasswordRef = useRef<HTMLInputElement | null>(null)
  const sheetTriggerRef = useRef<HTMLElement | null>(null)
  const sheetCloseTimerRef = useRef<number | null>(null)
  const sheetFocusTimerRef = useRef<number | null>(null)
  const keyboardHeightRef = useRef(0)
  const keyboardPreparationTimerRef = useRef<number | null>(null)
  const [sheet, setSheet] = useState<ProfileSheetState>(null)
  const [sheetEntered, setSheetEntered] = useState(false)
  const [sheetClosing, setSheetClosing] = useState(false)
  const [nickname, setNickname] = useState(session.user.nickname ?? '')
  const [email, setEmail] = useState(session.user.email ?? '')
  const [oldPassword, setOldPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [fieldErrors, setFieldErrors] = useState<ProfileFieldErrors>({})
  const [formError, setFormError] = useState('')
  const [refreshError, setRefreshError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [notice, setNotice] = useState('')

  const displayName = session.user.nickname?.trim() || '用户'
  const avatarLetter = displayName.slice(0, 1).toUpperCase()

  const loadProfile = useCallback(async () => {
    setRefreshError('')
    try {
      onSessionChange(await refreshProfileSession())
    } catch (error: unknown) {
      if (error instanceof ApiError && error.status === 401) {
        onLogout()
        return
      }
      setRefreshError(error instanceof Error ? error.message : '个人信息刷新失败。')
    }
  }, [onLogout, onSessionChange])

  useEffect(() => {
    if (active) {
      void loadProfile()
    }
  }, [active, loadProfile])

  useEffect(() => {
    onNavigationLockChange?.(Boolean(sheet) || submitting)
    return () => onNavigationLockChange?.(false)
  }, [onNavigationLockChange, sheet, submitting])

  useEffect(() => {
    if (!active || sheet) {
      return
    }
    const focusFrame = window.requestAnimationFrame(() => rootRef.current?.focus({ preventScroll: true }))
    return () => window.cancelAnimationFrame(focusFrame)
  }, [active, sheet])

  useEffect(() => {
    if (sheet?.kind !== 'profile') {
      setNickname(session.user.nickname ?? '')
      setEmail(session.user.email ?? '')
    }
  }, [session.user.email, session.user.nickname, sheet?.kind])

  useLayoutEffect(() => {
    const root = rootRef.current
    const viewport = window.visualViewport
    if (!root || !viewport) {
      return
    }
    const activeRoot = root
    const activeViewport = viewport
    let layoutHeight = Math.max(window.innerHeight, activeViewport.height)
    let layoutWidth = activeViewport.width

    function syncVisualViewport() {
      const widthChanged = Math.abs(activeViewport.width - layoutWidth) > 1
      if (widthChanged) {
        layoutHeight = activeViewport.height
        layoutWidth = activeViewport.width
      }

      layoutHeight = Math.max(layoutHeight, window.innerHeight, activeViewport.height)
      const keyboardHeight = Math.max(0, layoutHeight - activeViewport.height)
      const keyboardOpen = keyboardHeight > 20
      if (!keyboardOpen && activeViewport.height > layoutHeight) {
        layoutHeight = activeViewport.height
      }

      activeRoot.style.setProperty('--profile-viewport-height', `${layoutHeight}px`)
      activeRoot.style.setProperty('--profile-keyboard-height', `${keyboardHeight}px`)
      activeRoot.dataset.keyboardOpen = String(keyboardOpen)
      if (keyboardOpen) {
        keyboardHeightRef.current = keyboardHeight
        if (keyboardPreparationTimerRef.current !== null) {
          window.clearTimeout(keyboardPreparationTimerRef.current)
          keyboardPreparationTimerRef.current = null
        }
      }
    }

    syncVisualViewport()
    activeViewport.addEventListener('resize', syncVisualViewport)
    return () => activeViewport.removeEventListener('resize', syncVisualViewport)
  }, [])

  useLayoutEffect(() => {
    if (!sheet || sheetClosing || sheetEntered) {
      return
    }
    const reduceMotion = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false
    if (reduceMotion) {
      setSheetEntered(true)
      return
    }

    let secondFrame: number | null = null
    const firstFrame = window.requestAnimationFrame(() => {
      secondFrame = window.requestAnimationFrame(() => setSheetEntered(true))
    })
    return () => {
      window.cancelAnimationFrame(firstFrame)
      if (secondFrame !== null) {
        window.cancelAnimationFrame(secondFrame)
      }
    }
  }, [sheet, sheetClosing, sheetEntered])

  useEffect(() => {
    if (!sheet || sheetClosing || !sheetEntered) {
      return
    }
    const reduceMotion = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false
    sheetFocusTimerRef.current = window.setTimeout(() => {
      const input = sheet.kind === 'password'
        ? oldPasswordRef.current
        : sheet.focus === 'email'
          ? emailRef.current
          : nicknameRef.current
      const root = rootRef.current
      const viewport = window.visualViewport
      if (input && root && viewport && viewport.width < 900) {
        const anticipatedHeight =
          keyboardHeightRef.current || Math.min(360, Math.max(260, window.innerHeight * 0.38))
        const heightBeforeFocus = viewport.height
        root.style.setProperty('--profile-keyboard-height', `${anticipatedHeight}px`)
        root.dataset.keyboardOpen = 'true'
        void root.offsetHeight

        if (keyboardPreparationTimerRef.current !== null) {
          window.clearTimeout(keyboardPreparationTimerRef.current)
        }
        keyboardPreparationTimerRef.current = window.setTimeout(() => {
          if (viewport.height >= heightBeforeFocus - 20) {
            root.style.setProperty('--profile-keyboard-height', '0px')
            root.dataset.keyboardOpen = 'false'
          }
          keyboardPreparationTimerRef.current = null
        }, 650)
      }
      input?.focus({ preventScroll: true })
      sheetFocusTimerRef.current = null
    }, reduceMotion ? 0 : sheetEnterDuration)
    return () => {
      if (sheetFocusTimerRef.current !== null) {
        window.clearTimeout(sheetFocusTimerRef.current)
        sheetFocusTimerRef.current = null
      }
    }
  }, [sheet, sheetClosing, sheetEntered])

  const finishSheetClose = useCallback(() => {
    if (sheetCloseTimerRef.current !== null) {
      window.clearTimeout(sheetCloseTimerRef.current)
      sheetCloseTimerRef.current = null
    }
    setSheet(null)
    setSheetEntered(false)
    setSheetClosing(false)
    setFieldErrors({})
    setFormError('')
    setOldPassword('')
    setNewPassword('')
    setConfirmPassword('')
    const trigger = sheetTriggerRef.current
    window.requestAnimationFrame(() => trigger?.focus({ preventScroll: true }))
  }, [])

  const beginSheetClose = useCallback((allowWhileSubmitting = false) => {
    if (!sheet || sheetClosing || (submitting && !allowWhileSubmitting)) {
      return
    }
    if (document.activeElement instanceof HTMLElement) {
      document.activeElement.blur()
    }
    setSheetClosing(true)
    const reduceMotion = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false
    sheetCloseTimerRef.current = window.setTimeout(
      finishSheetClose,
      reduceMotion ? 0 : sheetExitDuration,
    )
  }, [finishSheetClose, sheet, sheetClosing, submitting])

  const closeSheet = useCallback(() => {
    beginSheetClose()
  }, [beginSheetClose])

  useEffect(() => () => {
    if (sheetCloseTimerRef.current !== null) {
      window.clearTimeout(sheetCloseTimerRef.current)
    }
    if (sheetFocusTimerRef.current !== null) {
      window.clearTimeout(sheetFocusTimerRef.current)
    }
    if (keyboardPreparationTimerRef.current !== null) {
      window.clearTimeout(keyboardPreparationTimerRef.current)
    }
  }, [])

  useEffect(() => {
    if (!sheet) {
      return
    }
    function closeWithEscape(event: KeyboardEvent) {
      if (event.key === 'Escape') {
        closeSheet()
      }
    }
    document.addEventListener('keydown', closeWithEscape)
    return () => document.removeEventListener('keydown', closeWithEscape)
  }, [closeSheet, sheet])

  function openProfileSheet(focus: 'nickname' | 'email' | null, trigger: HTMLElement) {
    sheetTriggerRef.current = trigger
    setSheetEntered(false)
    setSheetClosing(false)
    setNickname(session.user.nickname ?? '')
    setEmail(session.user.email ?? '')
    setFieldErrors({})
    setFormError('')
    setNotice('')
    setSheet({ kind: 'profile', focus })
  }

  function openPasswordSheet(trigger: HTMLElement) {
    sheetTriggerRef.current = trigger
    setSheetEntered(false)
    setSheetClosing(false)
    setOldPassword('')
    setNewPassword('')
    setConfirmPassword('')
    setFieldErrors({})
    setFormError('')
    setNotice('')
    setSheet({ kind: 'password' })
  }

  async function submitProfile(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (submitting) {
      return
    }
    const nextNickname = nickname.trim()
    const nextEmail = email.trim()
    const errors: ProfileFieldErrors = {}
    if (nextNickname.length > 64) {
      errors.nickname = '昵称不能超过 64 个字符。'
    }
    if (nextEmail.length > 255) {
      errors.email = '邮箱不能超过 255 个字符。'
    } else if (nextEmail && !emailPattern.test(nextEmail)) {
      errors.email = '请输入有效的邮箱地址。'
    }
    if (Object.keys(errors).length > 0) {
      setFieldErrors(errors)
      const target = errors.nickname ? nicknameRef.current : emailRef.current
      target?.focus({ preventScroll: true })
      return
    }

    setSubmitting(true)
    setFieldErrors({})
    setFormError('')
    try {
      onSessionChange(await updateProfile({ nickname: nextNickname, email: nextEmail }))
      setNotice('个人信息已更新。')
      beginSheetClose(true)
    } catch (error: unknown) {
      if (error instanceof ApiError && error.status === 401) {
        onLogout()
        return
      }
      setFieldErrors(apiFieldErrors(error))
      setFormError(error instanceof Error ? error.message : '个人信息保存失败。')
    } finally {
      setSubmitting(false)
    }
  }

  async function submitPassword(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (submitting) {
      return
    }
    const errors: ProfileFieldErrors = {}
    if (!oldPassword) {
      errors.old_password = '请输入原密码。'
    }
    if (newPassword.length < 6) {
      errors.new_password = '新密码至少需要 6 个字符。'
    } else if (newPassword.length > 255) {
      errors.new_password = '新密码不能超过 255 个字符。'
    } else if (newPassword === oldPassword) {
      errors.new_password = '新密码不能与原密码相同。'
    }
    if (confirmPassword !== newPassword) {
      errors.confirm_password = '两次输入的新密码不一致。'
    }
    if (Object.keys(errors).length > 0) {
      setFieldErrors(errors)
      setFormError('')
      return
    }

    setSubmitting(true)
    setFieldErrors({})
    setFormError('')
    try {
      await changePassword({ old_password: oldPassword, new_password: newPassword })
      setNotice('密码已更新。')
      beginSheetClose(true)
    } catch (error: unknown) {
      if (error instanceof ApiError && error.status === 401) {
        onLogout()
        return
      }
      setFieldErrors(apiFieldErrors(error))
      setFormError(error instanceof Error ? error.message : '密码修改失败。')
    } finally {
      setSubmitting(false)
    }
  }

  function closeFromScrim(event: MouseEvent<HTMLDivElement>) {
    if (event.target === event.currentTarget) {
      closeSheet()
    }
  }

  return (
    <main
      className="profile-screen"
      ref={rootRef}
      data-sheet-open={String(Boolean(sheet))}
      aria-label="个人信息"
      tabIndex={-1}
    >
      <div className="profile-page" inert={sheet ? true : undefined}>
        <header className="profile-header">
          <button className="profile-header__back" type="button" aria-label="返回聊天" onClick={onBack}>
            <CaretLeft size={28} weight="bold" />
          </button>
          <h1>个人信息</h1>
          <button
            className="profile-header__edit"
            type="button"
            onClick={(event) => openProfileSheet(null, event.currentTarget)}
          >
            编辑
          </button>
        </header>

        <div className="profile-content">
          <section className="profile-hero" aria-label="账户概览">
            <div>
              <strong>{displayName}</strong>
              <span>@{session.user.username}</span>
            </div>
            <span className="profile-avatar" aria-label={`头像：${avatarLetter}`}>{avatarLetter}</span>
          </section>

          {refreshError && (
            <div className="profile-banner profile-banner--error" role="alert">
              <WarningCircle size={17} />
              <span>{refreshError}</span>
              <button type="button" onClick={() => void loadProfile()}>重试</button>
            </div>
          )}
          {notice && (
            <p className="profile-banner profile-banner--success" role="status">
              <CheckCircle size={17} /> {notice}
            </p>
          )}

          <section className="profile-section" aria-labelledby="basic-information-title">
            <h2 id="basic-information-title">基本信息</h2>
            <div className="profile-card">
              <button type="button" onClick={(event) => openProfileSheet('nickname', event.currentTarget)}>
                <strong>昵称</strong>
                <span>{displayName}</span>
                <CaretRight size={18} weight="bold" />
              </button>
              <div className="profile-row profile-row--readonly">
                <strong>用户名</strong>
                <span>@{session.user.username}</span>
              </div>
              <button type="button" onClick={(event) => openProfileSheet('email', event.currentTarget)}>
                <strong>邮箱</strong>
                <span>{session.user.email?.trim() || '未设置'}</span>
                <CaretRight size={18} weight="bold" />
              </button>
            </div>
          </section>

          <section className="profile-section" aria-labelledby="account-information-title">
            <h2 id="account-information-title">账户</h2>
            <div className="profile-card">
              <button type="button" onClick={(event) => openPasswordSheet(event.currentTarget)}>
                <strong>密码</strong>
                <span>修改密码</span>
                <CaretRight size={18} weight="bold" />
              </button>
              <div className="profile-row profile-row--readonly">
                <strong>登录设备</strong>
                <span>暂未接入</span>
              </div>
            </div>
          </section>

          <button className="profile-logout" type="button" onClick={onLogout}>退出登录</button>
          <p className="profile-footer">账号信息仅用于 Cove 服务。</p>
        </div>
      </div>

      {sheet && (
        <div
          className="profile-sheet-layer"
          data-state={sheetClosing ? 'closing' : sheetEntered ? 'open' : 'preparing'}
          onMouseDown={closeFromScrim}
        >
          <div className="profile-sheet-keyboard-frame">
            <section className="profile-sheet" role="dialog" aria-modal="true" aria-labelledby="profile-sheet-title">
            <span className="profile-sheet__grabber" aria-hidden="true" />
            <header>
              <button type="button" onClick={closeSheet} disabled={submitting}>取消</button>
              <h2 id="profile-sheet-title">{sheet.kind === 'profile' ? '编辑个人信息' : '修改密码'}</h2>
              <button type="submit" form="profile-sheet-form" disabled={submitting}>
                {submitting ? '保存中' : '保存'}
              </button>
            </header>

            {sheet.kind === 'profile' ? (
              <form id="profile-sheet-form" className="profile-sheet__form" onSubmit={submitProfile}>
                <label>
                  <span>昵称</span>
                  <input
                    ref={nicknameRef}
                    type="text"
                    autoComplete="nickname"
                    maxLength={64}
                    value={nickname}
                    aria-invalid={Boolean(fieldErrors.nickname)}
                    onChange={(event) => setNickname(event.target.value)}
                  />
                  {fieldErrors.nickname && <small role="alert">{fieldErrors.nickname}</small>}
                </label>
                <label>
                  <span>邮箱</span>
                  <input
                    ref={emailRef}
                    type="email"
                    inputMode="email"
                    autoComplete="email"
                    autoCapitalize="none"
                    spellCheck={false}
                    maxLength={255}
                    placeholder="未设置"
                    value={email}
                    aria-invalid={Boolean(fieldErrors.email)}
                    onChange={(event) => setEmail(event.target.value)}
                  />
                  {fieldErrors.email && <small role="alert">{fieldErrors.email}</small>}
                </label>
                <p>邮箱留空后保存即可清除。</p>
                {formError && <p className="profile-sheet__error" role="alert">{formError}</p>}
              </form>
            ) : (
              <form id="profile-sheet-form" className="profile-sheet__form" onSubmit={submitPassword}>
                <label>
                  <span>原密码</span>
                  <input
                    ref={oldPasswordRef}
                    type="password"
                    autoComplete="current-password"
                    value={oldPassword}
                    aria-invalid={Boolean(fieldErrors.old_password)}
                    onChange={(event) => setOldPassword(event.target.value)}
                  />
                  {fieldErrors.old_password && <small role="alert">{fieldErrors.old_password}</small>}
                </label>
                <label>
                  <span>新密码</span>
                  <input
                    type="password"
                    autoComplete="new-password"
                    minLength={6}
                    maxLength={255}
                    value={newPassword}
                    aria-invalid={Boolean(fieldErrors.new_password)}
                    onChange={(event) => setNewPassword(event.target.value)}
                  />
                  {fieldErrors.new_password && <small role="alert">{fieldErrors.new_password}</small>}
                </label>
                <label>
                  <span>确认新密码</span>
                  <input
                    type="password"
                    autoComplete="new-password"
                    minLength={6}
                    maxLength={255}
                    value={confirmPassword}
                    aria-invalid={Boolean(fieldErrors.confirm_password)}
                    onChange={(event) => setConfirmPassword(event.target.value)}
                  />
                  {fieldErrors.confirm_password && <small role="alert">{fieldErrors.confirm_password}</small>}
                </label>
                {formError && <p className="profile-sheet__error" role="alert">{formError}</p>}
              </form>
            )}
            </section>
          </div>
        </div>
      )}
    </main>
  )
}
