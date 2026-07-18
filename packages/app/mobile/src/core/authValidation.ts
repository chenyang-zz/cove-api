export type LoginField = 'login' | 'password';
export type RegisterField = 'username' | 'email' | 'password' | 'confirmPassword';

const emailPattern = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

function passwordError(password: string): string | undefined {
  if (password.length < 6) {
    return '密码至少需要 6 个字符。';
  }
  if (password.length > 255) {
    return '密码不能超过 255 个字符。';
  }
  return undefined;
}

export function validateLogin(input: {
  login: string;
  password: string;
}): Partial<Record<LoginField, string>> {
  const errors: Partial<Record<LoginField, string>> = {};
  if (!input.login.trim()) {
    errors.login = '请输入用户名或邮箱。';
  }
  const password = passwordError(input.password);
  if (password) {
    errors.password = password;
  }
  return errors;
}

export function validateRegister(input: {
  username: string;
  email: string;
  password: string;
  confirmPassword: string;
}): Partial<Record<RegisterField, string>> {
  const errors: Partial<Record<RegisterField, string>> = {};
  if (!input.username.trim()) {
    errors.username = '请输入用户名。';
  } else if (input.username.trim().length > 64) {
    errors.username = '用户名不能超过 64 个字符。';
  }
  if (input.email.trim() && !emailPattern.test(input.email.trim())) {
    errors.email = '请输入有效的邮箱地址。';
  }
  const password = passwordError(input.password);
  if (password) {
    errors.password = password;
  }
  if (input.confirmPassword !== input.password) {
    errors.confirmPassword = '两次输入的密码不一致。';
  }
  return errors;
}

export function normalizeServerField(field: string): LoginField | RegisterField | null {
  const normalized = field.replace(/_/g, '').toLowerCase();
  const fields: Record<string, LoginField | RegisterField> = {
    login: 'login',
    username: 'username',
    email: 'email',
    password: 'password',
    confirmpassword: 'confirmPassword',
  };
  return fields[normalized] ?? null;
}
