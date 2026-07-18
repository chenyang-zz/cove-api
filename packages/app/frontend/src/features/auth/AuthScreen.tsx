import { useRef, useState, type FormEvent } from 'react'
import './AuthScreen.css'
import { ApiError, login, register } from './api'
import type { StoredSession } from './types'

const coveIcon = '/cove-mark.svg'

type AuthMode = 'login' | 'register'
type FieldName = 'login' | 'username' | 'email' | 'password' | 'confirmPassword'
type FieldErrors = Partial<Record<FieldName, string>>

type AuthScreenProps = {
  onAuthenticated: (session: StoredSession) => void
  initialMode?: AuthMode
  nativePage?: boolean
  onModeChange?: (mode: AuthMode) => boolean
  onSubmissionStart?: (mode: AuthMode) => void
  onSubmissionFailure?: (mode: AuthMode) => void
}

const emailPattern = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

function normalizeServerField(field: string): FieldName | null {
  const normalized = field.replace(/_/g, '').toLowerCase()
  const fields: Record<string, FieldName> = {
    login: 'login',
    username: 'username',
    email: 'email',
    password: 'password',
    confirmpassword: 'confirmPassword',
  }
  return fields[normalized] ?? null
}

export function AuthScreen({
  onAuthenticated,
  initialMode = 'login',
  nativePage = false,
  onModeChange,
  onSubmissionStart,
  onSubmissionFailure,
}: AuthScreenProps) {
  const [mode, setMode] = useState<AuthMode>(initialMode)
  const [loginValue, setLoginValue] = useState('')
  const [username, setUsername] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [showConfirmPassword, setShowConfirmPassword] = useState(false)
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({})
  const [formError, setFormError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const fieldRefs = useRef<Partial<Record<FieldName, HTMLInputElement | null>>>({})

  const isLogin = mode === 'login'

  function focusFirstError(errors: FieldErrors) {
    const order: FieldName[] = isLogin
      ? ['login', 'password']
      : ['username', 'email', 'password', 'confirmPassword']
    const firstInvalid = order.find((field) => errors[field])
    if (firstInvalid) {
      window.requestAnimationFrame(() => fieldRefs.current[firstInvalid]?.focus())
    }
  }

  function validate(): FieldErrors {
    const errors: FieldErrors = {}
    if (isLogin) {
      if (!loginValue.trim()) {
        errors.login = '请输入用户名或邮箱。'
      }
    } else {
      if (!username.trim()) {
        errors.username = '请输入用户名。'
      } else if (username.trim().length > 64) {
        errors.username = '用户名不能超过 64 个字符。'
      }
      if (email.trim() && !emailPattern.test(email.trim())) {
        errors.email = '请输入有效的邮箱地址。'
      }
    }

    if (password.length < 6) {
      errors.password = '密码至少需要 6 个字符。'
    } else if (password.length > 255) {
      errors.password = '密码不能超过 255 个字符。'
    }

    if (!isLogin && confirmPassword !== password) {
      errors.confirmPassword = '两次输入的密码不一致。'
    }

    return errors
  }

  function switchMode(nextMode: AuthMode) {
    if (submitting || nextMode === mode) {
      return
    }
    if (onModeChange?.(nextMode)) {
      return
    }
    setMode(nextMode)
    setPassword('')
    setConfirmPassword('')
    setShowPassword(false)
    setShowConfirmPassword(false)
    setFieldErrors({})
    setFormError('')
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (submitting) {
      return
    }

    const clientErrors = validate()
    setFieldErrors(clientErrors)
    setFormError('')
    if (Object.keys(clientErrors).length > 0) {
      focusFirstError(clientErrors)
      return
    }

    setSubmitting(true)
    try {
      onSubmissionStart?.(mode)
      const session = isLogin
        ? await login({ login: loginValue.trim(), password })
        : await register({
            username: username.trim(),
            ...(email.trim() ? { email: email.trim() } : {}),
            password,
          })
      onAuthenticated(session)
    } catch (error: unknown) {
      onSubmissionFailure?.(mode)
      if (error instanceof ApiError) {
        const serverErrors: FieldErrors = {}
        for (const fieldError of error.fieldErrors) {
          const field = normalizeServerField(fieldError.field)
          if (field && !serverErrors[field]) {
            serverErrors[field] = fieldError.message
          }
        }
        setFieldErrors(serverErrors)
        setFormError(error.message)
        focusFirstError(serverErrors)
      } else {
        setFormError('发生了意外错误，请稍后重试。')
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <main className={nativePage ? `auth-shell auth-shell--native-page auth-shell--native-${mode}` : 'auth-shell'}>
      <section className="auth-panel" aria-labelledby="auth-title">
        <div className="brand-lockup">
          <img className="brand-lockup__icon" src={coveIcon} alt="" />
          <span>Cove</span>
        </div>

        <header className="auth-heading">
          <h1 id="auth-title">{isLogin ? '欢迎回来' : '创建你的账号'}</h1>
          <p>{isLogin ? '登录后继续使用你的 Cove。' : '只需一分钟，马上开始使用 Cove。'}</p>
        </header>

        {!nativePage && (
          <div
            className={isLogin ? 'auth-tabs' : 'auth-tabs auth-tabs--register'}
            role="tablist"
            aria-label="选择认证方式"
          >
            <span className="auth-tabs__indicator" aria-hidden="true" />
            <button
              type="button"
              role="tab"
              aria-selected={isLogin}
              className={isLogin ? 'auth-tabs__item auth-tabs__item--active' : 'auth-tabs__item'}
              onClick={() => switchMode('login')}
            >
              登录
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={!isLogin}
              className={!isLogin ? 'auth-tabs__item auth-tabs__item--active' : 'auth-tabs__item'}
              onClick={() => switchMode('register')}
            >
              注册
            </button>
          </div>
        )}

        <form className="auth-form" onSubmit={handleSubmit} noValidate>
          {formError && (
            <div className="form-alert" role="alert">
              {formError}
            </div>
          )}

          {isLogin ? (
            <div className="field-group">
              <label htmlFor="login">用户名或邮箱</label>
              <input
                ref={(node) => {
                  fieldRefs.current.login = node
                }}
                id="login"
                name="login"
                type="text"
                autoComplete="username"
                autoCapitalize="none"
                spellCheck={false}
                value={loginValue}
                aria-invalid={Boolean(fieldErrors.login)}
                aria-describedby={fieldErrors.login ? 'login-error' : undefined}
                onChange={(event) => setLoginValue(event.target.value)}
              />
              {fieldErrors.login && (
                <p className="field-error" id="login-error">
                  {fieldErrors.login}
                </p>
              )}
            </div>
          ) : (
            <>
              <div className="field-group">
                <label htmlFor="username">用户名</label>
                <input
                  ref={(node) => {
                    fieldRefs.current.username = node
                  }}
                  id="username"
                  name="username"
                  type="text"
                  autoComplete="username"
                  autoCapitalize="none"
                  spellCheck={false}
                  maxLength={64}
                  value={username}
                  aria-invalid={Boolean(fieldErrors.username)}
                  aria-describedby={fieldErrors.username ? 'username-error' : undefined}
                  onChange={(event) => setUsername(event.target.value)}
                />
                {fieldErrors.username && (
                  <p className="field-error" id="username-error">
                    {fieldErrors.username}
                  </p>
                )}
              </div>

              <div className="field-group">
                <label htmlFor="email">
                  邮箱 <span>可选</span>
                </label>
                <input
                  ref={(node) => {
                    fieldRefs.current.email = node
                  }}
                  id="email"
                  name="email"
                  type="email"
                  inputMode="email"
                  autoComplete="email"
                  autoCapitalize="none"
                  spellCheck={false}
                  maxLength={255}
                  value={email}
                  aria-invalid={Boolean(fieldErrors.email)}
                  aria-describedby={fieldErrors.email ? 'email-error' : undefined}
                  onChange={(event) => setEmail(event.target.value)}
                />
                {fieldErrors.email && (
                  <p className="field-error" id="email-error">
                    {fieldErrors.email}
                  </p>
                )}
              </div>
            </>
          )}

          <div className="field-group">
            <label htmlFor="password">密码</label>
            <div className="password-input">
              <input
                ref={(node) => {
                  fieldRefs.current.password = node
                }}
                id="password"
                name="password"
                type={showPassword ? 'text' : 'password'}
                autoComplete={isLogin ? 'current-password' : 'new-password'}
                minLength={6}
                maxLength={255}
                value={password}
                aria-invalid={Boolean(fieldErrors.password)}
                aria-describedby={fieldErrors.password ? 'password-error' : undefined}
                onChange={(event) => setPassword(event.target.value)}
              />
              <button
                type="button"
                className="password-input__toggle"
                aria-label={showPassword ? '隐藏密码' : '显示密码'}
                onClick={() => setShowPassword((visible) => !visible)}
              >
                {showPassword ? '隐藏' : '显示'}
              </button>
            </div>
            {fieldErrors.password && (
              <p className="field-error" id="password-error">
                {fieldErrors.password}
              </p>
            )}
          </div>

          {!isLogin && (
            <div className="field-group">
              <label htmlFor="confirm-password">确认密码</label>
              <div className="password-input">
                <input
                  ref={(node) => {
                    fieldRefs.current.confirmPassword = node
                  }}
                  id="confirm-password"
                  name="confirmPassword"
                  type={showConfirmPassword ? 'text' : 'password'}
                  autoComplete="new-password"
                  minLength={6}
                  maxLength={255}
                  value={confirmPassword}
                  aria-invalid={Boolean(fieldErrors.confirmPassword)}
                  aria-describedby={fieldErrors.confirmPassword ? 'confirm-password-error' : undefined}
                  onChange={(event) => setConfirmPassword(event.target.value)}
                />
                <button
                  type="button"
                  className="password-input__toggle"
                  aria-label={showConfirmPassword ? '隐藏确认密码' : '显示确认密码'}
                  onClick={() => setShowConfirmPassword((visible) => !visible)}
                >
                  {showConfirmPassword ? '隐藏' : '显示'}
                </button>
              </div>
              {fieldErrors.confirmPassword && (
                <p className="field-error" id="confirm-password-error">
                  {fieldErrors.confirmPassword}
                </p>
              )}
            </div>
          )}

          <button className="primary-button" type="submit" disabled={submitting}>
            {submitting ? (isLogin ? '正在登录...' : '正在创建...') : isLogin ? '登录' : '创建账号'}
          </button>
        </form>

        {nativePage && (
          <button
            className="auth-mode-link"
            type="button"
            disabled={submitting}
            onClick={() => switchMode(isLogin ? 'register' : 'login')}
          >
            {isLogin ? '还没有账号？创建账号' : '已有账号？返回登录'}
          </button>
        )}
      </section>
    </main>
  )
}
