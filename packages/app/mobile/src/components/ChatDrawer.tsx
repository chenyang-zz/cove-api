import { useEffect, useRef, useState } from 'react';
import {
  ActivityIndicator,
  Animated,
  KeyboardAvoidingView,
  Modal,
  Platform,
  Pressable,
  StyleSheet,
  Text,
  TextInput,
  useWindowDimensions,
  View,
} from 'react-native';
import { MenuView, type NativeActionEvent } from '@expo/ui/community/menu';
import { SymbolView } from 'expo-symbols';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { BrandMark } from '@/components/BrandMark';
import type { ConversationResponse, SessionUser } from '@/core/types';
import { usePalette } from '@/theme/palette';

type DrawerState = 'loading' | 'ready' | 'error';

const conversationFadeBandOpacities = Array.from({ length: 24 }, (_, index) => {
  const remaining = 1 - index / 23;
  return remaining * remaining;
});

type ChatDrawerProps = {
  visible: boolean;
  conversations: ConversationResponse[];
  activeConversationId?: string;
  state: DrawerState;
  error: string;
  user: SessionUser | null;
  onClose: () => void;
  onRetry: () => void;
  onNewChat: () => void;
  onSelectConversation: (conversationId: string) => void;
  onRenameConversation: (conversationId: string, title: string) => Promise<void>;
  onDeleteConversation: (conversationId: string) => Promise<void>;
  onOpenKnowledge: () => void;
  onOpenProfile: () => void;
  onLogout: () => void;
  busyConversationId?: string;
};

function formatDate(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return '';
  }
  return `${date.getMonth() + 1}/${date.getDate()}`;
}

export function ChatDrawer({
  visible,
  conversations,
  activeConversationId,
  state,
  error,
  user,
  onClose,
  onRetry,
  onNewChat,
  onSelectConversation,
  onRenameConversation,
  onDeleteConversation,
  onOpenKnowledge,
  onOpenProfile,
  onLogout,
  busyConversationId,
}: ChatDrawerProps) {
  const palette = usePalette();
  const { width } = useWindowDimensions();
  const insets = useSafeAreaInsets();
  const drawerWidth = Math.min(310, width - 48);
  const progress = useRef(new Animated.Value(0)).current;
  const conversationScrollY = useRef(new Animated.Value(0)).current;
  const conversationTopFadeOpacity = conversationScrollY.interpolate({
    inputRange: [0, 12],
    outputRange: [0, 1],
    extrapolate: 'clamp',
  });
  const displayName = user?.nickname?.trim() || user?.username || 'Cove 用户';
  const [renameTarget, setRenameTarget] = useState<ConversationResponse | null>(null);
  const [renameTitle, setRenameTitle] = useState('');
  const [deleteTarget, setDeleteTarget] = useState<ConversationResponse | null>(null);
  const [actionPendingId, setActionPendingId] = useState<string>();
  const [actionError, setActionError] = useState('');

  useEffect(() => {
    if (!visible) {
      setRenameTarget(null);
      setDeleteTarget(null);
      setActionPendingId(undefined);
      setActionError('');
      return;
    }
    progress.setValue(0);
    Animated.spring(progress, {
      toValue: 1,
      damping: 24,
      stiffness: 280,
      mass: 0.86,
      useNativeDriver: true,
    }).start();
  }, [progress, visible]);

  function close(afterClose?: () => void) {
    Animated.timing(progress, {
      toValue: 0,
      duration: 190,
      useNativeDriver: true,
    }).start(({ finished }) => {
      if (finished) {
        onClose();
        afterClose?.();
      }
    });
  }

  function beginRename(conversation: ConversationResponse) {
    setActionError('');
    setDeleteTarget(null);
    setRenameTarget(conversation);
    setRenameTitle(conversation.title || '新对话');
  }

  function beginDelete(conversation: ConversationResponse) {
    setActionError('');
    setRenameTarget(null);
    setDeleteTarget(conversation);
  }

  function handleConversationAction(
    conversation: ConversationResponse,
    event: NativeActionEvent,
  ) {
    if (actionPendingId || conversation.id === busyConversationId) {
      return;
    }
    if (event.nativeEvent.event === 'rename') {
      beginRename(conversation);
    } else if (event.nativeEvent.event === 'delete') {
      beginDelete(conversation);
    }
  }

  function dismissDialog() {
    if (actionPendingId) {
      return;
    }
    setRenameTarget(null);
    setDeleteTarget(null);
    setActionError('');
  }

  async function submitRename() {
    const target = renameTarget;
    const title = renameTitle.trim();
    if (!target || actionPendingId) {
      return;
    }
    if (!title) {
      setActionError('请输入会话名称。');
      return;
    }
    setActionPendingId(target.id);
    setActionError('');
    try {
      await onRenameConversation(target.id, title);
      setRenameTarget(null);
    } catch (caught) {
      setActionError(caught instanceof Error ? caught.message : '重命名失败，请稍后重试。');
    } finally {
      setActionPendingId(undefined);
    }
  }

  async function confirmDelete() {
    const target = deleteTarget;
    if (!target || actionPendingId) {
      return;
    }
    const deletingActiveConversation = target.id === activeConversationId;
    setActionPendingId(target.id);
    setActionError('');
    try {
      await onDeleteConversation(target.id);
      setDeleteTarget(null);
      if (deletingActiveConversation) {
        close();
      }
    } catch (caught) {
      setActionError(caught instanceof Error ? caught.message : '删除失败，请稍后重试。');
    } finally {
      setActionPendingId(undefined);
    }
  }

  const translateX = progress.interpolate({ inputRange: [0, 1], outputRange: [-drawerWidth, 0] });
  const scrimOpacity = progress.interpolate({ inputRange: [0, 1], outputRange: [0, 1] });

  return (
    <Modal
      transparent
      visible={visible}
      animationType="none"
      presentationStyle="overFullScreen"
      statusBarTranslucent
      onRequestClose={() => close()}>
      <View style={styles.modalRoot}>
        <Animated.View
          style={[
            styles.drawer,
            {
              width: drawerWidth,
              backgroundColor: palette.surface,
              shadowColor: palette.shadow,
              transform: [{ translateX }],
            },
          ]}>
          <View
            style={[
              styles.safeArea,
              { paddingTop: insets.top, paddingBottom: insets.bottom },
            ]}>
            <View style={styles.header}>
              <View style={styles.brand}>
                <BrandMark size={32} />
                <Text style={[styles.brandName, { color: palette.text }]}>Cove</Text>
              </View>
              <Pressable
                accessibilityRole="button"
                accessibilityLabel="关闭会话列表"
                hitSlop={6}
                onPress={() => close()}
                style={({ pressed }) => [styles.closeButton, pressed && styles.pressed]}>
                <SymbolView name="xmark" size={18} tintColor={palette.textSecondary} weight="semibold" />
              </Pressable>
            </View>

            <Pressable
              accessibilityRole="button"
              onPress={() => close(onNewChat)}
              style={({ pressed }) => [
                styles.shortcut,
                styles.newChat,
                { backgroundColor: palette.input, borderColor: palette.border },
                pressed && styles.rowPressed,
              ]}>
              <SymbolView name="plus" size={18} tintColor={palette.textSecondary} weight="medium" />
              <Text style={[styles.shortcutText, { color: palette.text }]}>新对话</Text>
            </Pressable>

            <Pressable
              accessibilityRole="button"
              accessibilityLabel="打开知识库"
              onPress={() => close(onOpenKnowledge)}
              style={({ pressed }) => [
                styles.shortcut,
                styles.knowledge,
                { backgroundColor: palette.input, borderColor: palette.border },
                pressed && styles.rowPressed,
              ]}>
              <SymbolView name="books.vertical" size={18} tintColor={palette.textSecondary} weight="medium" />
              <Text style={[styles.shortcutText, { color: palette.text }]}>知识库</Text>
              <SymbolView name="chevron.right" size={13} tintColor={palette.textMuted} weight="semibold" />
            </Pressable>

            <Text style={[styles.recentLabel, { color: palette.textMuted }]}>最近对话</Text>
            {state === 'loading' ? (
              <View accessibilityLabel="正在加载最近对话" style={styles.skeletonList}>
                {[0, 1, 2, 3].map((index) => (
                  <View key={index} style={styles.skeletonRow}>
                    <View
                      style={[
                        styles.skeletonTitle,
                        { width: index % 2 === 0 ? '58%' : '42%', backgroundColor: palette.surfaceMuted },
                      ]}
                    />
                    <View style={[styles.skeletonDate, { backgroundColor: palette.surfaceMuted }]} />
                    <View style={[styles.skeletonMore, { backgroundColor: palette.surfaceMuted }]} />
                  </View>
                ))}
              </View>
            ) : state === 'error' ? (
              <View style={styles.centerState}>
                <Text style={[styles.stateText, { color: palette.textMuted }]}>{error}</Text>
                <Pressable accessibilityRole="button" onPress={onRetry} style={styles.retryButton}>
                  <Text style={[styles.retryText, { color: palette.accent }]}>重试</Text>
                </Pressable>
              </View>
            ) : (
              <View style={styles.conversationListFrame}>
                <Animated.FlatList
                  style={styles.conversationList}
                  data={conversations}
                  keyExtractor={(item) => item.id}
                  contentContainerStyle={conversations.length ? styles.listContent : styles.emptyList}
                  onScroll={Animated.event(
                    [{ nativeEvent: { contentOffset: { y: conversationScrollY } } }],
                    { useNativeDriver: true },
                  )}
                  scrollEventThrottle={16}
                  ListEmptyComponent={
                    <Text style={[styles.stateText, { color: palette.textMuted }]}>发送第一条消息后，会话会保存在这里。</Text>
                  }
                  renderItem={({ item }) => {
                    const active = item.id === activeConversationId;
                    const actionsDisabled = Boolean(actionPendingId) || item.id === busyConversationId;
                    const actionTrigger = (
                      <View
                        accessible
                        accessibilityRole="button"
                        accessibilityLabel={`管理会话：${item.title || '新对话'}`}
                        accessibilityState={{ disabled: actionsDisabled }}
                        style={[
                          styles.conversationMore,
                          actionsDisabled && styles.actionDisabled,
                        ]}>
                        {actionPendingId === item.id ? (
                          <ActivityIndicator color={palette.textMuted} size="small" />
                        ) : (
                          <SymbolView name="ellipsis" size={16} tintColor={palette.textMuted} weight="semibold" />
                        )}
                      </View>
                    );
                    return (
                      <View
                        style={[
                          styles.conversationRow,
                          active && { backgroundColor: palette.surfaceMuted },
                        ]}>
                        <Pressable
                          accessibilityRole="button"
                          accessibilityLabel={`打开会话：${item.title || '新对话'}`}
                          accessibilityState={{ selected: active }}
                          onPress={() => close(() => onSelectConversation(item.id))}
                          style={({ pressed }) => [styles.conversationMain, pressed && styles.rowPressed]}>
                          <Text
                            numberOfLines={1}
                            style={[
                              styles.conversationTitle,
                              active ? styles.activeConversationTitle : undefined,
                              { color: active ? palette.text : palette.textSecondary },
                            ]}>
                            {item.title || '新对话'}
                          </Text>
                          <Text style={[styles.conversationDate, { color: palette.textMuted }]}>
                            {formatDate(item.updated_at)}
                          </Text>
                        </Pressable>
                        {actionsDisabled ? actionTrigger : (
                          <MenuView
                            actions={[
                              { id: 'rename', title: '重命名', image: 'pencil' },
                              {
                                id: 'delete',
                                title: '删除',
                                image: 'trash',
                                attributes: { destructive: true },
                              },
                            ]}
                            onPressAction={(event) => handleConversationAction(item, event)}
                            style={styles.conversationMenu}>
                            {actionTrigger}
                          </MenuView>
                        )}
                      </View>
                    );
                  }}
                />
                <Animated.View
                  pointerEvents="none"
                  style={[styles.conversationTopFade, { opacity: conversationTopFadeOpacity }]}>
                  {conversationFadeBandOpacities.map((opacity, index) => (
                    <View
                      key={index}
                      style={[
                        styles.conversationTopFadeBand,
                        { backgroundColor: palette.surface, opacity },
                      ]}
                    />
                  ))}
                </Animated.View>
              </View>
            )}

            <View style={[styles.accountDivider, { backgroundColor: palette.border }]} />
            <View style={styles.accountRow}>
              <Pressable
                accessibilityRole="button"
                accessibilityLabel="打开个人信息"
                onPress={() => close(onOpenProfile)}
                style={({ pressed }) => [
                  styles.profileEntry,
                  { backgroundColor: palette.input },
                  pressed && styles.rowPressed,
                ]}>
                <View style={[styles.avatar, { backgroundColor: palette.accent }]}>
                  <Text style={[styles.avatarText, { color: palette.accentText }]}>{displayName.slice(0, 1).toUpperCase()}</Text>
                </View>
                <View style={styles.identity}>
                  <Text numberOfLines={1} style={[styles.displayName, { color: palette.text }]}>{displayName}</Text>
                  <Text numberOfLines={1} style={[styles.username, { color: palette.textMuted }]}>@{user?.username ?? ''}</Text>
                </View>
                <SymbolView name="chevron.right" size={13} tintColor={palette.textMuted} weight="semibold" />
              </Pressable>
              <Pressable
                accessibilityRole="button"
                accessibilityLabel="退出登录"
                hitSlop={5}
                onPress={() => close(onLogout)}
                style={({ pressed }) => [styles.logoutButton, pressed && styles.pressed]}>
                <SymbolView name="rectangle.portrait.and.arrow.right" size={20} tintColor={palette.danger} weight="medium" />
              </Pressable>
            </View>
          </View>
        </Animated.View>
        <Animated.View style={[styles.scrim, { opacity: scrimOpacity, backgroundColor: palette.scrim }]}>
          <Pressable
            accessibilityRole="button"
            accessibilityLabel="关闭会话列表"
            onPress={() => close()}
            style={StyleSheet.absoluteFill}
          />
        </Animated.View>

        {renameTarget || deleteTarget ? (
          <KeyboardAvoidingView
            accessibilityViewIsModal
            behavior={Platform.OS === 'ios' ? 'padding' : undefined}
            style={styles.dialogLayer}>
            <Pressable
              accessibilityLabel="关闭对话框"
              onPress={dismissDialog}
              style={[StyleSheet.absoluteFill, { backgroundColor: palette.scrim }]}
            />
            {renameTarget ? (
              <View
                accessibilityRole="alert"
                accessibilityLabel="重命名会话"
                style={[
                  styles.dialog,
                  {
                    backgroundColor: palette.surface,
                    borderColor: palette.border,
                    shadowColor: palette.shadow,
                  },
                ]}>
                <Text style={[styles.dialogTitle, { color: palette.text }]}>重命名会话</Text>
                <TextInput
                  autoFocus
                  value={renameTitle}
                  maxLength={256}
                  editable={!actionPendingId}
                  returnKeyType="done"
                  selectionColor={palette.accent}
                  selectTextOnFocus
                  onChangeText={(value) => {
                    setRenameTitle(value);
                    if (actionError) {
                      setActionError('');
                    }
                  }}
                  onSubmitEditing={() => void submitRename()}
                  style={[
                    styles.renameInput,
                    {
                      backgroundColor: palette.input,
                      borderColor: actionError ? palette.danger : palette.accent,
                      color: palette.text,
                    },
                  ]}
                />
                {actionError ? (
                  <Text accessibilityRole="alert" style={[styles.dialogError, { color: palette.danger }]}>
                    {actionError}
                  </Text>
                ) : null}
                <View style={styles.dialogActions}>
                  <Pressable
                    accessibilityRole="button"
                    disabled={Boolean(actionPendingId)}
                    onPress={dismissDialog}
                    style={({ pressed }) => [
                      styles.dialogButton,
                      { borderColor: palette.border },
                      pressed && styles.buttonPressed,
                    ]}>
                    <Text style={[styles.dialogButtonLabel, { color: palette.textSecondary }]}>取消</Text>
                  </Pressable>
                  <Pressable
                    accessibilityRole="button"
                    disabled={Boolean(actionPendingId) || !renameTitle.trim()}
                    onPress={() => void submitRename()}
                    style={({ pressed }) => [
                      styles.dialogButton,
                      { backgroundColor: palette.accent, borderColor: palette.accent },
                      pressed && styles.buttonPressed,
                      (!renameTitle.trim() || actionPendingId) && styles.actionDisabled,
                    ]}>
                    {actionPendingId ? (
                      <ActivityIndicator color={palette.accentText} size="small" />
                    ) : (
                      <Text style={[styles.dialogButtonLabel, { color: palette.accentText }]}>保存</Text>
                    )}
                  </Pressable>
                </View>
              </View>
            ) : deleteTarget ? (
              <View
                accessibilityRole="alert"
                accessibilityLabel="删除会话"
                style={[
                  styles.dialog,
                  {
                    backgroundColor: palette.surface,
                    borderColor: palette.border,
                    shadowColor: palette.shadow,
                  },
                ]}>
                <Text style={[styles.dialogTitle, { color: palette.text }]}>删除会话？</Text>
                <Text style={[styles.deleteDescription, { color: palette.textSecondary }]}>
                  “{deleteTarget.title || '新对话'}”及其消息将被永久删除。
                </Text>
                {actionError ? (
                  <Text accessibilityRole="alert" style={[styles.dialogError, { color: palette.danger }]}>
                    {actionError}
                  </Text>
                ) : null}
                <View style={styles.dialogActions}>
                  <Pressable
                    accessibilityRole="button"
                    disabled={Boolean(actionPendingId)}
                    onPress={dismissDialog}
                    style={({ pressed }) => [
                      styles.dialogButton,
                      { borderColor: palette.border },
                      pressed && styles.buttonPressed,
                    ]}>
                    <Text style={[styles.dialogButtonLabel, { color: palette.textSecondary }]}>取消</Text>
                  </Pressable>
                  <Pressable
                    accessibilityRole="button"
                    disabled={Boolean(actionPendingId)}
                    onPress={() => void confirmDelete()}
                    style={({ pressed }) => [
                      styles.dialogButton,
                      { backgroundColor: palette.danger, borderColor: palette.danger },
                      pressed && styles.buttonPressed,
                      actionPendingId && styles.actionDisabled,
                    ]}>
                    {actionPendingId ? (
                      <ActivityIndicator color={palette.page} size="small" />
                    ) : (
                      <Text style={[styles.dialogButtonLabel, { color: palette.page }]}>删除</Text>
                    )}
                  </Pressable>
                </View>
              </View>
            ) : null}
          </KeyboardAvoidingView>
        ) : null}
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  modalRoot: { flex: 1, flexDirection: 'row' },
  drawer: {
    height: '100%',
    zIndex: 2,
    shadowOffset: { width: 18, height: 0 },
    shadowOpacity: 0.16,
    shadowRadius: 25,
    elevation: 16,
  },
  safeArea: { flex: 1 },
  scrim: { flex: 1 },
  header: { height: 58, paddingHorizontal: 13, flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between' },
  brand: { flexDirection: 'row', alignItems: 'center', gap: 11 },
  brandName: { fontSize: 19, lineHeight: 24, fontWeight: '600' },
  closeButton: { width: 34, height: 40, alignItems: 'center', justifyContent: 'center' },
  shortcut: {
    height: 46,
    marginHorizontal: 13,
    paddingHorizontal: 12,
    borderRadius: 14,
    borderWidth: 1,
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
  },
  newChat: { marginTop: 19 },
  knowledge: { marginTop: 8 },
  shortcutText: { flex: 1, fontSize: 14, lineHeight: 18, fontWeight: '600' },
  recentLabel: { marginTop: 16, marginLeft: 23, marginBottom: 6, fontSize: 11, lineHeight: 15, fontWeight: '600' },
  conversationListFrame: { position: 'relative', flex: 1 },
  conversationList: { flex: 1 },
  conversationTopFade: { position: 'absolute', top: 0, right: 0, left: 0, zIndex: 2, height: 24 },
  conversationTopFadeBand: { flex: 1 },
  listContent: { paddingHorizontal: 13, paddingBottom: 12 },
  emptyList: { flexGrow: 1, paddingHorizontal: 23, paddingTop: 16 },
  conversationRow: { height: 48, borderRadius: 12, flexDirection: 'row', alignItems: 'stretch', overflow: 'hidden' },
  conversationMain: { minWidth: 0, flex: 1, paddingLeft: 10, flexDirection: 'row', alignItems: 'center' },
  conversationTitle: { flex: 1, marginRight: 8, fontSize: 13, lineHeight: 18 },
  activeConversationTitle: { fontWeight: '600' },
  conversationDate: { width: 36, textAlign: 'right', fontSize: 10, lineHeight: 14 },
  conversationMenu: { width: 44, height: 48 },
  conversationMore: { width: 44, height: 48, alignItems: 'center', justifyContent: 'center' },
  skeletonList: { flex: 1, paddingHorizontal: 13 },
  skeletonRow: { height: 48, paddingHorizontal: 10, flexDirection: 'row', alignItems: 'center' },
  skeletonTitle: { height: 11, borderRadius: 5.5 },
  skeletonDate: { width: 28, height: 8, marginLeft: 'auto', borderRadius: 4 },
  skeletonMore: { width: 17, height: 5, marginLeft: 11, marginRight: 1, borderRadius: 2.5 },
  centerState: { flex: 1, paddingHorizontal: 23, paddingTop: 30, alignItems: 'center' },
  stateText: { fontSize: 12, lineHeight: 18, textAlign: 'center' },
  retryButton: { minWidth: 64, minHeight: 36, marginTop: 8, alignItems: 'center', justifyContent: 'center' },
  retryText: { fontSize: 13, fontWeight: '600' },
  accountDivider: { height: StyleSheet.hairlineWidth, marginHorizontal: 13 },
  accountRow: { height: 59, paddingHorizontal: 8, flexDirection: 'row', alignItems: 'center', gap: 5 },
  profileEntry: { height: 49, minWidth: 0, flex: 1, paddingHorizontal: 5, borderRadius: 12, flexDirection: 'row', alignItems: 'center' },
  avatar: { width: 32, height: 32, borderRadius: 10, alignItems: 'center', justifyContent: 'center' },
  avatarText: { fontSize: 13, lineHeight: 17, fontWeight: '600' },
  identity: { minWidth: 0, flex: 1, marginLeft: 10 },
  displayName: { fontSize: 13, lineHeight: 17, fontWeight: '600' },
  username: { fontSize: 11, lineHeight: 15 },
  logoutButton: { width: 39, height: 44, alignItems: 'center', justifyContent: 'center' },
  pressed: { opacity: 0.58 },
  rowPressed: { opacity: 0.7, transform: [{ scale: 0.99 }] },
  buttonPressed: { opacity: 0.78, transform: [{ scale: 0.98 }] },
  actionDisabled: { opacity: 0.42 },
  dialogLayer: {
    position: 'absolute',
    inset: 0,
    zIndex: 8,
    alignItems: 'center',
    justifyContent: 'center',
    paddingHorizontal: 19,
  },
  dialog: {
    width: '100%',
    maxWidth: 352,
    borderRadius: 18,
    borderWidth: 1,
    padding: 19,
    shadowOffset: { width: 0, height: 24 },
    shadowOpacity: 0.24,
    shadowRadius: 30,
    elevation: 24,
  },
  dialogTitle: { fontSize: 18, lineHeight: 24, fontWeight: '700' },
  renameInput: {
    height: 46,
    marginTop: 15,
    borderRadius: 12,
    borderWidth: 1,
    paddingHorizontal: 11,
    paddingVertical: 0,
    fontSize: 16,
    lineHeight: 22,
    fontWeight: '600',
  },
  deleteDescription: { marginTop: 6, minHeight: 40, fontSize: 13, lineHeight: 19.5 },
  dialogError: { marginTop: 8, fontSize: 12, lineHeight: 17 },
  dialogActions: { marginTop: 23, flexDirection: 'row', justifyContent: 'flex-end', gap: 8 },
  dialogButton: {
    width: 82,
    height: 42,
    borderRadius: 15,
    borderWidth: 1,
    alignItems: 'center',
    justifyContent: 'center',
  },
  dialogButtonLabel: { fontSize: 14, lineHeight: 20, fontWeight: '700' },
});
