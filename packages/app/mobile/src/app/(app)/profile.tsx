import { useEffect, useRef, useState } from 'react';
import {
  Animated,
  KeyboardAvoidingView,
  Modal,
  Platform,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View,
} from 'react-native';
import { Stack, useRouter } from 'expo-router';
import { SymbolView } from 'expo-symbols';
import { SafeAreaView, useSafeAreaInsets } from 'react-native-safe-area-context';

import { ApiError } from '@/core/api';
import { useAuth } from '@/providers/AuthProvider';
import { usePalette, type Palette } from '@/theme/palette';

type SheetMode = 'profile' | 'password';
type ProfileErrors = Partial<Record<'nickname' | 'email', string>>;
type PasswordErrors = Partial<Record<'old_password' | 'new_password' | 'confirm_password', string>>;

export default function ProfileScreen() {
  const palette = usePalette();
  const router = useRouter();
  const { session, signOut, updateProfile, changePassword } = useAuth();
  const [sheet, setSheet] = useState<SheetMode | null>(null);
  const user = session?.user;
  const displayName = user?.nickname?.trim() || user?.username || 'Cove 用户';

  async function handleLogout() {
    await signOut();
    router.replace('/(auth)/login');
  }

  return (
    <SafeAreaView edges={['bottom']} style={[styles.safeArea, { backgroundColor: palette.page }]}>
      <Stack.Toolbar placement="left">
        <Stack.Toolbar.Button
          accessibilityLabel="返回"
          icon="chevron.left"
          hidesSharedBackground={false}
          separateBackground
          tintColor={palette.accent}
          onPress={() => router.back()}
        />
      </Stack.Toolbar>
      <Stack.Toolbar placement="right">
        <Stack.Toolbar.Button
          accessibilityLabel="编辑个人信息"
          hidesSharedBackground={false}
          separateBackground
          tintColor={palette.accent}
          onPress={() => setSheet('profile')}>
          编辑
        </Stack.Toolbar.Button>
      </Stack.Toolbar>
      <ScrollView
        contentInsetAdjustmentBehavior="automatic"
        contentContainerStyle={styles.content}>
        <View style={styles.hero}>
          <View style={styles.heroText}>
            <Text numberOfLines={1} style={[styles.name, { color: palette.text }]}>{displayName}</Text>
            <Text numberOfLines={1} style={[styles.username, { color: palette.textMuted }]}>@{user?.username}</Text>
          </View>
          <View style={[styles.avatar, { backgroundColor: palette.accent }]}>
            <Text style={[styles.avatarText, { color: palette.accentText }]}>
              {displayName.slice(0, 1).toUpperCase()}
            </Text>
          </View>
        </View>

        <ProfileSection title="基本信息" palette={palette}>
          <ProfileRow label="昵称" value={displayName} palette={palette} onPress={() => setSheet('profile')} />
          <ProfileRow label="用户名" value={`@${user?.username ?? ''}`} palette={palette} />
          <ProfileRow label="邮箱" value={user?.email?.trim() || '未设置'} palette={palette} onPress={() => setSheet('profile')} />
        </ProfileSection>

        <ProfileSection title="账户" palette={palette} account>
          <ProfileRow label="密码" value="修改密码" palette={palette} onPress={() => setSheet('password')} />
          <ProfileRow label="登录设备" value="暂未接入" palette={palette} />
        </ProfileSection>

        <Pressable
          accessibilityRole="button"
          onPress={() => void handleLogout()}
          style={({ pressed }) => [
            styles.logout,
            { borderColor: palette.border },
            pressed && styles.pressed,
          ]}>
          <Text style={[styles.logoutText, { color: palette.danger }]}>退出登录</Text>
        </Pressable>
        <Text style={[styles.footer, { color: palette.textMuted }]}>账号信息仅用于 Cove 服务。</Text>
      </ScrollView>

      {sheet ? (
        <ProfileSheet
          mode={sheet}
          initialNickname={user?.nickname ?? ''}
          initialEmail={user?.email ?? ''}
          palette={palette}
          onClose={() => setSheet(null)}
          onUpdateProfile={updateProfile}
          onChangePassword={changePassword}
        />
      ) : null}
    </SafeAreaView>
  );
}

function ProfileSection({
  title,
  palette,
  account = false,
  children,
}: {
  title: string;
  palette: Palette;
  account?: boolean;
  children: React.ReactNode;
}) {
  return (
    <View style={[styles.section, account && styles.accountSection]}>
      <Text style={[styles.sectionTitle, { color: palette.textMuted }]}>{title}</Text>
      <View style={[styles.card, { backgroundColor: palette.surface, borderColor: palette.border }]}>{children}</View>
    </View>
  );
}

function ProfileRow({
  label,
  value,
  palette,
  onPress,
}: {
  label: string;
  value: string;
  palette: Palette;
  onPress?: () => void;
}) {
  const content = (
    <>
      <Text style={[styles.rowLabel, { color: palette.text }]}>{label}</Text>
      <View style={styles.rowValueGroup}>
        <Text numberOfLines={1} style={[styles.rowValue, { color: palette.textMuted }]}>{value}</Text>
        {onPress ? <SymbolView name="chevron.right" size={15} tintColor={palette.textMuted} weight="semibold" /> : null}
      </View>
    </>
  );

  if (onPress) {
    return (
      <Pressable accessibilityRole="button" onPress={onPress} style={({ pressed }) => [styles.row, pressed && styles.rowPressed]}>
        {content}
      </Pressable>
    );
  }
  return <View style={styles.row}>{content}</View>;
}

function ProfileSheet({
  mode,
  initialNickname,
  initialEmail,
  palette,
  onClose,
  onUpdateProfile,
  onChangePassword,
}: {
  mode: SheetMode;
  initialNickname: string;
  initialEmail: string;
  palette: Palette;
  onClose: () => void;
  onUpdateProfile: (input: { nickname: string; email: string }) => Promise<unknown>;
  onChangePassword: (input: { old_password: string; new_password: string }) => Promise<void>;
}) {
  const [nickname, setNickname] = useState(initialNickname);
  const [email, setEmail] = useState(initialEmail);
  const [oldPassword, setOldPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [profileErrors, setProfileErrors] = useState<ProfileErrors>({});
  const [passwordErrors, setPasswordErrors] = useState<PasswordErrors>({});
  const [formError, setFormError] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const progress = useRef(new Animated.Value(0)).current;
  const insets = useSafeAreaInsets();

  useEffect(() => {
    Animated.spring(progress, {
      toValue: 1,
      damping: 25,
      stiffness: 270,
      mass: 0.85,
      useNativeDriver: true,
    }).start();
  }, [progress]);

  function close() {
    if (submitting) {
      return;
    }
    Animated.timing(progress, {
      toValue: 0,
      duration: 190,
      useNativeDriver: true,
    }).start(({ finished }) => {
      if (finished) {
        onClose();
      }
    });
  }

  async function submit() {
    if (submitting) {
      return;
    }
    setFormError('');
    if (mode === 'profile') {
      const next: ProfileErrors = {};
      if (nickname.length > 64) {
        next.nickname = '昵称不能超过 64 个字符。';
      }
      if (email.trim() && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim())) {
        next.email = '请输入有效的邮箱地址。';
      }
      setProfileErrors(next);
      if (Object.keys(next).length > 0) {
        return;
      }
    } else {
      const next: PasswordErrors = {};
      if (!oldPassword) {
        next.old_password = '请输入原密码。';
      }
      if (newPassword.length < 6 || newPassword.length > 255) {
        next.new_password = '新密码长度应为 6–255 位。';
      }
      if (confirmPassword !== newPassword) {
        next.confirm_password = '两次输入的新密码不一致。';
      }
      setPasswordErrors(next);
      if (Object.keys(next).length > 0) {
        return;
      }
    }

    setSubmitting(true);
    try {
      if (mode === 'profile') {
        await onUpdateProfile({ nickname: nickname.trim(), email: email.trim() });
      } else {
        await onChangePassword({ old_password: oldPassword, new_password: newPassword });
      }
      onClose();
    } catch (error) {
      if (error instanceof ApiError) {
        if (mode === 'profile') {
          const next: ProfileErrors = {};
          for (const item of error.fieldErrors) {
            if (item.field === 'nickname' || item.field === 'email') {
              next[item.field] = item.message;
            }
          }
          setProfileErrors(next);
        } else {
          const next: PasswordErrors = {};
          for (const item of error.fieldErrors) {
            if (item.field === 'old_password' || item.field === 'new_password') {
              next[item.field] = item.message;
            }
          }
          setPasswordErrors(next);
        }
        setFormError(error.message);
      } else {
        setFormError('发生了意外错误，请稍后重试。');
      }
    } finally {
      setSubmitting(false);
    }
  }

  const translateY = progress.interpolate({ inputRange: [0, 1], outputRange: [360, 0] });
  const scrimOpacity = progress.interpolate({ inputRange: [0, 1], outputRange: [0, 1] });

  return (
    <Modal transparent presentationStyle="overFullScreen" statusBarTranslucent animationType="none" onRequestClose={close}>
      <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} style={styles.modalRoot}>
        <Animated.View style={[StyleSheet.absoluteFill, { opacity: scrimOpacity, backgroundColor: palette.scrim }]}>
          <Pressable accessibilityRole="button" accessibilityLabel="关闭编辑面板" onPress={close} style={StyleSheet.absoluteFill} />
        </Animated.View>
        <Animated.View
          style={[
            styles.sheet,
            {
              backgroundColor: palette.surface,
              borderColor: palette.border,
              shadowColor: palette.shadow,
              paddingBottom: Math.max(insets.bottom, 24),
              transform: [{ translateY }],
            },
          ]}>
          <View style={[styles.grabber, { backgroundColor: palette.borderStrong }]} />
          <View style={styles.sheetHeader}>
            <Pressable disabled={submitting} hitSlop={8} onPress={close} style={({ pressed }) => pressed && styles.pressed}>
              <Text style={[styles.sheetAction, { color: palette.accent }]}>取消</Text>
            </Pressable>
            <Text style={[styles.sheetTitle, { color: palette.text }]}>{mode === 'profile' ? '编辑个人信息' : '修改密码'}</Text>
            <Pressable disabled={submitting} hitSlop={8} onPress={() => void submit()} style={({ pressed }) => pressed && styles.pressed}>
              <Text style={[styles.sheetAction, styles.saveAction, { color: palette.accent }]}>{submitting ? '保存中' : '保存'}</Text>
            </Pressable>
          </View>
          <ScrollView keyboardShouldPersistTaps="handled" contentContainerStyle={styles.sheetForm}>
            {mode === 'profile' ? (
              <>
                <SheetField label="昵称" value={nickname} onChangeText={setNickname} error={profileErrors.nickname} palette={palette} autoComplete="nickname" maxLength={64} />
                <SheetField label="邮箱" value={email} onChangeText={setEmail} error={profileErrors.email} palette={palette} autoCapitalize="none" autoComplete="email" keyboardType="email-address" maxLength={255} placeholder="未设置" />
                <Text style={[styles.helper, { color: palette.textMuted }]}>邮箱留空后保存即可清除。</Text>
              </>
            ) : (
              <>
                <SheetField label="原密码" value={oldPassword} onChangeText={setOldPassword} error={passwordErrors.old_password} palette={palette} secureTextEntry autoComplete="current-password" />
                <SheetField label="新密码" value={newPassword} onChangeText={setNewPassword} error={passwordErrors.new_password} palette={palette} secureTextEntry autoComplete="new-password" />
                <SheetField label="确认新密码" value={confirmPassword} onChangeText={setConfirmPassword} error={passwordErrors.confirm_password} palette={palette} secureTextEntry autoComplete="new-password" />
              </>
            )}
            {formError ? <Text style={[styles.sheetError, { color: palette.danger }]}>{formError}</Text> : null}
          </ScrollView>
        </Animated.View>
      </KeyboardAvoidingView>
    </Modal>
  );
}

function SheetField({ label, error, palette, ...props }: { label: string; error?: string; palette: Palette } & React.ComponentProps<typeof TextInput>) {
  return (
    <View style={styles.sheetField}>
      <Text style={[styles.sheetLabel, { color: palette.text }]}>{label}</Text>
      <TextInput
        {...props}
        placeholderTextColor={palette.textMuted}
        selectionColor={palette.accent}
        style={[styles.sheetInput, { color: palette.text, backgroundColor: palette.input, borderColor: error ? palette.danger : palette.borderStrong }]}
      />
      {error ? <Text style={[styles.fieldError, { color: palette.danger }]}>{error}</Text> : null}
    </View>
  );
}

const styles = StyleSheet.create({
  safeArea: { flex: 1 },
  content: { paddingHorizontal: 20, paddingTop: 26, paddingBottom: 30 },
  hero: { height: 72, flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between' },
  heroText: { minWidth: 0, flex: 1, paddingRight: 18 },
  name: { fontSize: 30, lineHeight: 36, fontWeight: '700', letterSpacing: -0.5 },
  username: { marginTop: 4, fontSize: 15, lineHeight: 20 },
  avatar: { width: 72, height: 72, borderRadius: 16, alignItems: 'center', justifyContent: 'center' },
  avatarText: { fontSize: 28, fontWeight: '700' },
  section: { marginTop: 29 },
  accountSection: { marginTop: 32 },
  sectionTitle: { marginBottom: 12, fontSize: 12, lineHeight: 15, fontWeight: '700' },
  card: { borderRadius: 12, borderWidth: StyleSheet.hairlineWidth, overflow: 'hidden' },
  row: {
    height: 60,
    paddingHorizontal: 15,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    borderBottomWidth: StyleSheet.hairlineWidth,
    borderBottomColor: 'rgba(128, 157, 161, 0.28)',
  },
  rowPressed: { opacity: 0.58 },
  rowLabel: { fontSize: 15, lineHeight: 20, fontWeight: '600' },
  rowValueGroup: { width: 174, minWidth: 0, flexDirection: 'row', alignItems: 'center', justifyContent: 'flex-end', gap: 8 },
  rowValue: { flexShrink: 1, fontSize: 14, lineHeight: 20, textAlign: 'right' },
  logout: { height: 50, marginTop: 38, borderRadius: 12, borderWidth: 1, alignItems: 'center', justifyContent: 'center' },
  logoutText: { fontSize: 15, lineHeight: 20, fontWeight: '600' },
  footer: { marginTop: 31, textAlign: 'center', fontSize: 12, lineHeight: 16 },
  modalRoot: { flex: 1, justifyContent: 'flex-end' },
  sheet: {
    minHeight: 330,
    maxHeight: '82%',
    borderTopLeftRadius: 22,
    borderTopRightRadius: 22,
    borderWidth: StyleSheet.hairlineWidth,
    paddingBottom: 24,
    shadowOffset: { width: 0, height: -8 },
    shadowOpacity: 0.14,
    shadowRadius: 24,
    elevation: 12,
  },
  grabber: { alignSelf: 'center', width: 36, height: 5, marginTop: 5, borderRadius: 3 },
  sheetHeader: { height: 57, paddingHorizontal: 19, flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between' },
  sheetTitle: { position: 'absolute', left: 90, right: 90, textAlign: 'center', fontSize: 16, lineHeight: 21, fontWeight: '600' },
  sheetAction: { minWidth: 44, fontSize: 15, lineHeight: 20 },
  saveAction: { textAlign: 'right', fontWeight: '600' },
  sheetForm: { paddingHorizontal: 19, paddingBottom: 10, gap: 15 },
  sheetField: { gap: 6 },
  sheetLabel: { fontSize: 13, lineHeight: 17, fontWeight: '600' },
  sheetInput: { height: 48, borderRadius: 12, borderWidth: 1, paddingHorizontal: 12, fontSize: 16 },
  helper: { marginTop: -6, fontSize: 11, lineHeight: 15 },
  fieldError: { fontSize: 11, lineHeight: 14 },
  sheetError: { fontSize: 12, lineHeight: 16 },
  pressed: { opacity: 0.55 },
});
