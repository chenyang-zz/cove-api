import { describe, expect, it } from 'vitest';

import { normalizeServerField, validateLogin, validateRegister } from './authValidation';

describe('auth validation', () => {
  it('validates login credentials', () => {
    expect(validateLogin({ login: '', password: '123' })).toEqual({
      login: '请输入用户名或邮箱。',
      password: '密码至少需要 6 个字符。',
    });
    expect(validateLogin({ login: 'cove', password: '123456' })).toEqual({});
  });

  it('validates registration fields', () => {
    expect(
      validateRegister({
        username: '',
        email: 'invalid',
        password: '123456',
        confirmPassword: '654321',
      }),
    ).toEqual({
      username: '请输入用户名。',
      email: '请输入有效的邮箱地址。',
      confirmPassword: '两次输入的密码不一致。',
    });
  });

  it('normalizes server field names', () => {
    expect(normalizeServerField('confirm_password')).toBe('confirmPassword');
    expect(normalizeServerField('unknown')).toBeNull();
  });
});
