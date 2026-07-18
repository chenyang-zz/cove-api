import { useMemo, useRef, useState } from 'react';
import {
  KeyboardAvoidingView,
  Platform,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View,
} from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useRouter } from 'expo-router';

import { ApiError } from '@/core/api';
import {
  normalizeServerField,
  validateLogin,
  validateRegister,
  type LoginField,
  type RegisterField,
} from '@/core/authValidation';
import { useAuth } from '@/providers/AuthProvider';
import { usePalette } from '@/theme/palette';

import { BrandMark } from './BrandMark';
import { Field } from './Field';

type Mode = 'login' | 'register';
type FieldName = LoginField | RegisterField;
type FieldErrors = Partial<Record<FieldName, string>>;

export function AuthScreen({ mode }: { mode: Mode }) {
  const palette = usePalette();
  const router = useRouter();
  const { signIn, signUp } = useAuth();
  const isLogin = mode === 'login';
  const [loginValue, setLoginValue] = useState('');
  const [username, setUsername] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [errors, setErrors] = useState<FieldErrors>({});
  const [formError, setFormError] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const refs = useRef<Partial<Record<FieldName, TextInput | null>>>({});

  const fieldOrder = useMemo<FieldName[]>(
    () => (isLogin ? ['login', 'password'] : ['username', 'email', 'password', 'confirmPassword']),
    [isLogin],
  );

  function focusFirstError(next: FieldErrors) {
    const first = fieldOrder.find((field) => next[field]);
    if (first) {
      requestAnimationFrame(() => refs.current[first]?.focus());
    }
  }

  async function submit() {
    if (submitting) {
      return;
    }
    const nextErrors: FieldErrors = isLogin
      ? validateLogin({ login: loginValue, password })
      : validateRegister({ username, email, password, confirmPassword });
    setErrors(nextErrors);
    setFormError('');
    if (Object.keys(nextErrors).length > 0) {
      focusFirstError(nextErrors);
      return;
    }

    setSubmitting(true);
    try {
      if (isLogin) {
        await signIn({ login: loginValue.trim(), password });
      } else {
        await signUp({
          username: username.trim(),
          ...(email.trim() ? { email: email.trim() } : {}),
          password,
        });
      }
      router.replace('/(app)/chat');
    } catch (error) {
      if (error instanceof ApiError) {
        const serverErrors: FieldErrors = {};
        for (const fieldError of error.fieldErrors) {
          const field = normalizeServerField(fieldError.field);
          if (field && !serverErrors[field]) {
            serverErrors[field] = fieldError.message;
          }
        }
        setErrors(serverErrors);
        setFormError(error.message);
        focusFirstError(serverErrors);
      } else {
        setFormError('发生了意外错误，请稍后重试。');
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <SafeAreaView
      edges={['top', 'bottom']}
      style={[styles.safeArea, { backgroundColor: palette.page }]}>
      <KeyboardAvoidingView
        behavior={Platform.OS === 'ios' ? 'padding' : undefined}
        style={styles.flex}>
        <ScrollView
          automaticallyAdjustKeyboardInsets
          keyboardShouldPersistTaps="handled"
          contentContainerStyle={styles.scrollContent}>
          <View style={styles.panel}>
            <View style={styles.brandRow}>
              <BrandMark />
              <Text style={[styles.brand, { color: palette.text }]}>Cove</Text>
            </View>
            <View style={styles.heading}>
              <Text style={[styles.title, { color: palette.text }]}>
                {isLogin ? '欢迎回来' : '创建你的账号'}
              </Text>
              <Text style={[styles.subtitle, { color: palette.textMuted }]}>
                {isLogin ? '登录后继续使用你的 Cove。' : '只需一分钟，马上开始使用 Cove。'}
              </Text>
            </View>

            {formError ? (
              <View style={[styles.alert, { backgroundColor: palette.dangerSurface }]}>
                <Text style={[styles.alertText, { color: palette.danger }]}>{formError}</Text>
              </View>
            ) : null}

            <View style={styles.form}>
              {isLogin ? (
                <Field
                  ref={(node) => {
                    refs.current.login = node;
                  }}
                  label="用户名或邮箱"
                  value={loginValue}
                  error={errors.login}
                  onChangeText={setLoginValue}
                  autoCapitalize="none"
                  autoCorrect={false}
                  autoComplete="username"
                  returnKeyType="next"
                  onSubmitEditing={() => refs.current.password?.focus()}
                />
              ) : (
                <>
                  <Field
                    ref={(node) => {
                      refs.current.username = node;
                    }}
                    label="用户名"
                    value={username}
                    error={errors.username}
                    onChangeText={setUsername}
                    autoCapitalize="none"
                    autoCorrect={false}
                    autoComplete="username-new"
                    returnKeyType="next"
                    onSubmitEditing={() => refs.current.email?.focus()}
                  />
                  <Field
                    ref={(node) => {
                      refs.current.email = node;
                    }}
                    label="邮箱"
                    optional="可选"
                    value={email}
                    error={errors.email}
                    onChangeText={setEmail}
                    autoCapitalize="none"
                    autoCorrect={false}
                    keyboardType="email-address"
                    autoComplete="email"
                    returnKeyType="next"
                    onSubmitEditing={() => refs.current.password?.focus()}
                  />
                </>
              )}
              <Field
                ref={(node) => {
                  refs.current.password = node;
                }}
                label="密码"
                value={password}
                error={errors.password}
                onChangeText={setPassword}
                password
                autoCapitalize="none"
                autoComplete={isLogin ? 'current-password' : 'new-password'}
                returnKeyType={isLogin ? 'done' : 'next'}
                onSubmitEditing={isLogin ? () => void submit() : () => refs.current.confirmPassword?.focus()}
              />
              {!isLogin ? (
                <Field
                  ref={(node) => {
                    refs.current.confirmPassword = node;
                  }}
                  label="确认密码"
                  value={confirmPassword}
                  error={errors.confirmPassword}
                  onChangeText={setConfirmPassword}
                  password
                  autoCapitalize="none"
                  autoComplete="new-password"
                  returnKeyType="done"
                  onSubmitEditing={() => void submit()}
                />
              ) : null}
            </View>

            <Pressable
              accessibilityRole="button"
              disabled={submitting}
              onPress={() => void submit()}
              style={({ pressed }) => [
                styles.primaryButton,
                { backgroundColor: pressed ? palette.accentPressed : palette.accent },
                submitting && styles.disabled,
              ]}>
              <Text style={[styles.primaryText, { color: palette.accentText }]}>
                {submitting ? '请稍候…' : isLogin ? '登录' : '创建账号'}
              </Text>
            </Pressable>

            <Pressable
              accessibilityRole="button"
              hitSlop={8}
              onPress={
                isLogin ? () => router.push('/(auth)/register') : () => router.back()
              }
              style={({ pressed }) => [styles.switchRow, pressed && styles.switchPressed]}>
              <Text style={[styles.switchAction, { color: palette.accent }]}>
                {isLogin ? '还没有账号？创建账号' : '已有账号？返回登录'}
              </Text>
            </Pressable>
          </View>
        </ScrollView>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safeArea: { flex: 1 },
  flex: { flex: 1 },
  scrollContent: {
    flexGrow: 1,
    alignItems: 'center',
    paddingHorizontal: 20,
    paddingTop: 32,
    paddingBottom: 20,
  },
  panel: { width: '100%', maxWidth: 390 },
  brandRow: { height: 36, flexDirection: 'row', alignItems: 'center', gap: 8 },
  brand: { fontSize: 19, fontWeight: '600' },
  heading: { marginTop: 38, marginBottom: 30, gap: 12 },
  title: { fontSize: 34, lineHeight: 38, fontWeight: '600', letterSpacing: -0.55 },
  subtitle: { fontSize: 15, lineHeight: 24 },
  alert: { marginBottom: 18, borderRadius: 12, paddingHorizontal: 14, paddingVertical: 12 },
  alertText: { fontSize: 14, lineHeight: 20 },
  form: { gap: 20 },
  primaryButton: {
    height: 52,
    marginTop: 24,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: 15,
  },
  primaryText: { fontSize: 15, fontWeight: '600' },
  switchRow: { minHeight: 44, marginTop: 16, alignItems: 'center', justifyContent: 'center' },
  switchAction: { fontSize: 14, lineHeight: 18, fontWeight: '600' },
  switchPressed: { opacity: 0.55 },
  disabled: { opacity: 0.58 },
});
