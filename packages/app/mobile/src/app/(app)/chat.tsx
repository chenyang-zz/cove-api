import { useCallback, useEffect, useRef, useState } from 'react';
import {
  ActivityIndicator,
  Animated,
  Easing,
  FlatList,
  Keyboard,
  KeyboardAvoidingView,
  Platform,
  Pressable,
  StyleSheet,
  Text,
  TextInput,
  View,
} from 'react-native';
import type { LayoutChangeEvent, NativeScrollEvent, NativeSyntheticEvent } from 'react-native';
import { useRouter } from 'expo-router';
import { SymbolView } from 'expo-symbols';
import { SafeAreaView } from 'react-native-safe-area-context';

import { AssistantMessage } from '@/components/AssistantMessage';
import { ChatDrawer } from '@/components/ChatDrawer';
import { ApiError } from '@/core/api';
import {
  deleteConversation,
  listConversations,
  listMessages,
  renameConversation,
  streamChat,
} from '@/core/chat';
import {
  appendTokenPart,
  appendToolPart,
  applyThinkEvent,
  toChatMessages,
} from '@/core/chatMessages';
import type {
  ChatMessage,
  ChatStreamEvent,
  ChatThinkEvent,
  ChatToolEvent,
  ConversationResponse,
} from '@/core/types';
import { useAuth } from '@/providers/AuthProvider';
import { usePalette } from '@/theme/palette';

let localSequence = 0;
const streamFollowThreshold = 72;

function localId(role: 'user' | 'assistant') {
  localSequence += 1;
  return `${role}-${Date.now()}-${localSequence}`;
}

function localMessage(role: 'user' | 'assistant', content: string): ChatMessage {
  const message: ChatMessage = {
    id: localId(role),
    role,
    content,
    meta_data: null,
    images: [],
    sender_persona_id: null,
    feedback: null,
    created_at: new Date().toISOString(),
    pending: true,
  };
  return role === 'assistant' ? { ...message, parts: [] } : message;
}

export default function ChatScreen() {
  const palette = usePalette();
  const router = useRouter();
  const { session, signOut } = useAuth();
  const [draft, setDraft] = useState('');
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [messageState, setMessageState] = useState<'loading' | 'ready' | 'error'>('loading');
  const [conversationId, setConversationId] = useState<string>();
  const [conversations, setConversations] = useState<ConversationResponse[]>([]);
  const [conversationState, setConversationState] = useState<'loading' | 'ready' | 'error'>('loading');
  const [conversationError, setConversationError] = useState('');
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [enableKnowledge, setEnableKnowledge] = useState(true);
  const [streaming, setStreaming] = useState(false);
  const [error, setError] = useState('');
  const abortRef = useRef<AbortController | null>(null);
  const listRef = useRef<FlatList<ChatMessage>>(null);
  const historyGenerationRef = useRef(0);
  const conversationIdRef = useRef<string | undefined>(undefined);
  const shouldFollowStreamRef = useRef(true);
  const hasPositionedMessagesRef = useRef(false);
  const initialPositioningTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const isMessageListInteractingRef = useRef(false);
  const isAutoFollowingRef = useRef(false);
  const streamScrollPosition = useRef(new Animated.Value(0)).current;
  const messageListMetricsRef = useRef({ contentHeight: 0, viewportHeight: 0, offsetY: 0 });
  const activeConversation = conversations.find((item) => item.id === conversationId);

  const loadConversationHistory = useCallback(async (nextConversationId: string) => {
    const generation = ++historyGenerationRef.current;
    setMessageState('loading');
    setError('');
    try {
      const response = await listMessages(nextConversationId, 30);
      if (generation !== historyGenerationRef.current || conversationIdRef.current !== nextConversationId) {
        return;
      }
      setMessages(toChatMessages(response.list));
      setMessageState('ready');
    } catch (caught) {
      if (generation !== historyGenerationRef.current || conversationIdRef.current !== nextConversationId) {
        return;
      }
      setMessageState('error');
      setError(caught instanceof Error ? caught.message : '消息加载失败，请稍后重试。');
      if (caught instanceof ApiError && caught.status === 401) {
        await signOut();
      }
    }
  }, [signOut]);

  const refreshConversations = useCallback(async (selectFirst = false) => {
    setConversationState('loading');
    setConversationError('');
    try {
      const response = await listConversations(1, 20);
      const sorted = [...response.list].sort(
        (left, right) => Date.parse(right.updated_at) - Date.parse(left.updated_at),
      );
      setConversations(sorted);
      setConversationState('ready');
      if (selectFirst && !conversationIdRef.current && sorted.length > 0) {
        const nextConversationId = sorted[0].id;
        conversationIdRef.current = nextConversationId;
        setConversationId(nextConversationId);
        await loadConversationHistory(nextConversationId);
      } else if (selectFirst && sorted.length === 0) {
        setMessageState('ready');
      }
    } catch (caught) {
      setConversationState('error');
      setConversationError(caught instanceof Error ? caught.message : '会话加载失败，请稍后重试。');
      if (!conversationIdRef.current) {
        setMessageState('ready');
      }
      if (caught instanceof ApiError && caught.status === 401) {
        await signOut();
      }
    }
  }, [loadConversationHistory, signOut]);

  useEffect(() => {
    void refreshConversations(true);
    return () => {
      historyGenerationRef.current += 1;
      abortRef.current?.abort();
      if (initialPositioningTimerRef.current) {
        clearTimeout(initialPositioningTimerRef.current);
      }
    };
  }, [refreshConversations]);

  useEffect(() => {
    const listenerId = streamScrollPosition.addListener(({ value }) => {
      listRef.current?.scrollToOffset({ offset: Math.max(0, value), animated: false });
    });
    return () => {
      streamScrollPosition.stopAnimation();
      streamScrollPosition.removeListener(listenerId);
    };
  }, [streamScrollPosition]);

  useEffect(() => {
    streamScrollPosition.stopAnimation();
    if (initialPositioningTimerRef.current) {
      clearTimeout(initialPositioningTimerRef.current);
      initialPositioningTimerRef.current = null;
    }
    shouldFollowStreamRef.current = true;
    hasPositionedMessagesRef.current = false;
    isMessageListInteractingRef.current = false;
    isAutoFollowingRef.current = false;
    messageListMetricsRef.current = {
      ...messageListMetricsRef.current,
      contentHeight: 0,
      offsetY: 0,
    };
  }, [conversationId, streamScrollPosition]);

  useEffect(() => {
    if (!streaming) {
      return;
    }
    shouldFollowStreamRef.current = true;
    requestAnimationFrame(() => scrollMessageListToBottom());
  }, [streaming]);

  function startNewConversation() {
    abortRef.current?.abort();
    abortRef.current = null;
    historyGenerationRef.current += 1;
    conversationIdRef.current = undefined;
    setConversationId(undefined);
    setMessages([]);
    setMessageState('ready');
    setStreaming(false);
    setError('');
  }

  function selectConversation(nextConversationId: string) {
    if (nextConversationId === conversationIdRef.current) {
      return;
    }
    abortRef.current?.abort();
    abortRef.current = null;
    conversationIdRef.current = nextConversationId;
    setConversationId(nextConversationId);
    setMessages([]);
    setStreaming(false);
    void loadConversationHistory(nextConversationId);
  }

  async function handleRenameConversation(nextConversationId: string, title: string) {
    try {
      const updated = await renameConversation(nextConversationId, title);
      setConversations((current) => current.map((conversation) => (
        conversation.id === updated.id ? updated : conversation
      )));
    } catch (caught) {
      if (caught instanceof ApiError && caught.status === 401) {
        await signOut();
      }
      throw caught;
    }
  }

  async function handleDeleteConversation(nextConversationId: string) {
    try {
      await deleteConversation(nextConversationId);
    } catch (caught) {
      if (caught instanceof ApiError && caught.status === 401) {
        await signOut();
      }
      throw caught;
    }
    setConversations((current) => current.filter((conversation) => conversation.id !== nextConversationId));
    if (conversationIdRef.current !== nextConversationId) {
      return;
    }
    abortRef.current?.abort();
    abortRef.current = null;
    historyGenerationRef.current += 1;
    conversationIdRef.current = undefined;
    setConversationId(undefined);
    setMessages([]);
    setMessageState('ready');
    setStreaming(false);
    setError('');
  }

  async function handleLogout() {
    await signOut();
    router.replace('/(auth)/login');
  }

  function updateMessage(id: string, updater: (message: ChatMessage) => ChatMessage) {
    setMessages((current) =>
      current.map((message) => (message.id === id ? updater(message) : message)),
    );
  }

  function updateMessageListMetrics(event: NativeSyntheticEvent<NativeScrollEvent>) {
    const { contentOffset, contentSize, layoutMeasurement } = event.nativeEvent;
    messageListMetricsRef.current = {
      contentHeight: contentSize.height,
      viewportHeight: layoutMeasurement.height,
      offsetY: contentOffset.y,
    };
  }

  function updateMessageFollowState(event: NativeSyntheticEvent<NativeScrollEvent>) {
    updateMessageListMetrics(event);
    const { contentOffset, contentSize, layoutMeasurement } = event.nativeEvent;
    const distanceFromBottom = Math.max(
      0,
      contentSize.height - layoutMeasurement.height - contentOffset.y,
    );
    shouldFollowStreamRef.current = distanceFromBottom <= streamFollowThreshold;
  }

  function handleMessageScroll(event: NativeSyntheticEvent<NativeScrollEvent>) {
    updateMessageListMetrics(event);
    if (!isMessageListInteractingRef.current && !isAutoFollowingRef.current) {
      updateMessageFollowState(event);
    }
  }

  function handleMessageScrollEnd(event: NativeSyntheticEvent<NativeScrollEvent>) {
    isMessageListInteractingRef.current = false;
    updateMessageFollowState(event);
  }

  function scrollMessageListToBottom(immediate = false) {
    const { contentHeight, viewportHeight, offsetY } = messageListMetricsRef.current;
    const targetOffset = Math.max(0, contentHeight - viewportHeight);
    streamScrollPosition.stopAnimation();
    if (immediate || Math.abs(targetOffset - offsetY) < 1) {
      isAutoFollowingRef.current = false;
      messageListMetricsRef.current.offsetY = targetOffset;
      listRef.current?.scrollToOffset({ offset: targetOffset, animated: false });
      return;
    }

    isAutoFollowingRef.current = true;
    streamScrollPosition.setValue(offsetY);
    const duration = Math.min(220, Math.max(110, Math.abs(targetOffset - offsetY) * 0.22));
    Animated.timing(streamScrollPosition, {
      toValue: targetOffset,
      duration,
      easing: Easing.out(Easing.cubic),
      useNativeDriver: false,
    }).start(({ finished }) => {
      if (finished) {
        isAutoFollowingRef.current = false;
        shouldFollowStreamRef.current = true;
        messageListMetricsRef.current.offsetY = targetOffset;
      }
    });
  }

  function scheduleInitialMessagePositioning() {
    const expectedConversationId = conversationIdRef.current;
    if (initialPositioningTimerRef.current) {
      clearTimeout(initialPositioningTimerRef.current);
    }
    requestAnimationFrame(() => scrollMessageListToBottom(true));
    initialPositioningTimerRef.current = setTimeout(() => {
      initialPositioningTimerRef.current = null;
      if (conversationIdRef.current !== expectedConversationId) {
        return;
      }
      requestAnimationFrame(() => {
        if (conversationIdRef.current !== expectedConversationId) {
          return;
        }
        scrollMessageListToBottom(true);
        hasPositionedMessagesRef.current = true;
      });
    }, 180);
  }

  function stopInitialMessagePositioning() {
    if (initialPositioningTimerRef.current) {
      clearTimeout(initialPositioningTimerRef.current);
      initialPositioningTimerRef.current = null;
    }
    hasPositionedMessagesRef.current = true;
  }

  function handleMessageContentSizeChange(_width: number, height: number) {
    messageListMetricsRef.current.contentHeight = height;
    if (messages.length === 0) {
      hasPositionedMessagesRef.current = false;
      return;
    }
    if (!hasPositionedMessagesRef.current) {
      scheduleInitialMessagePositioning();
      return;
    }
    if (streaming && shouldFollowStreamRef.current && !isMessageListInteractingRef.current) {
      scrollMessageListToBottom();
    }
  }

  function handleMessageListLayout(event: LayoutChangeEvent) {
    messageListMetricsRef.current.viewportHeight = event.nativeEvent.layout.height;
    if (messages.length > 0 && !hasPositionedMessagesRef.current) {
      scheduleInitialMessagePositioning();
      return;
    }
    if (streaming && shouldFollowStreamRef.current) {
      requestAnimationFrame(() => scrollMessageListToBottom());
    }
  }

  async function sendMessage() {
    const text = draft.trim();
    if (!text || streaming || messageState === 'loading') {
      return;
    }
    const user = localMessage('user', text);
    const assistant = localMessage('assistant', '');
    const controller = new AbortController();
    abortRef.current = controller;
    setDraft('');
    setError('');
    setMessageState('ready');
    setStreaming(true);
    setMessages((current) => [...current, user, assistant]);
    let terminal = false;

    try {
      await streamChat(
        {
          ...(conversationIdRef.current ? { conversation_id: conversationIdRef.current } : {}),
          message: text,
          enable_knowledge: enableKnowledge,
        },
        controller.signal,
        (event: ChatStreamEvent) => {
          if (event.type === 'meta' && 'conversation_id' in event) {
            const nextConversationId = String(event.conversation_id);
            const title = 'title' in event && String(event.title).trim() ? String(event.title) : '新对话';
            const now = new Date().toISOString();
            conversationIdRef.current = nextConversationId;
            setConversationId(nextConversationId);
            setConversations((current) => {
              const existing = current.find((item) => item.id === nextConversationId);
              const next: ConversationResponse = {
                id: nextConversationId,
                title,
                is_group: existing?.is_group ?? false,
                member_persona_ids: existing?.member_persona_ids ?? [],
                enable_tools: existing?.enable_tools ?? false,
                created_at: existing?.created_at ?? now,
                updated_at: now,
              };
              return [next, ...current.filter((item) => item.id !== nextConversationId)];
            });
          } else if (event.type === 'token' && 'text' in event) {
            updateMessage(assistant.id, (message) => ({
              ...message,
              content: message.content + String(event.text),
              parts: appendTokenPart(message.parts ?? [], String(event.text)),
              thinking: message.thinking ? { ...message.thinking, active: false } : undefined,
            }));
          } else if (event.type === 'think') {
            const think = event as ChatThinkEvent;
            if (
              (think.status === 'thinking' || think.status === 'done')
              && Number.isInteger(think.iteration)
              && think.iteration >= 0
            ) {
              updateMessage(assistant.id, (message) => applyThinkEvent(message, think));
            }
          } else if (event.type === 'tool_call' || event.type === 'tool_result') {
            updateMessage(assistant.id, (message) => ({
              ...message,
              parts: appendToolPart(message.parts ?? [], event as ChatToolEvent),
              thinking: message.thinking ? { ...message.thinking, active: false } : undefined,
            }));
          } else if (event.type === 'done') {
            terminal = true;
            updateMessage(assistant.id, (message) => ({
              ...message,
              id: 'text' in event && event.text ? String(event.text) : message.id,
              pending: false,
              thinking: undefined,
            }));
            setMessages((current) => current.map((message) => (
              message.id === user.id ? { ...message, pending: false } : message
            )));
            setStreaming(false);
          } else if (event.type === 'error' && 'content' in event) {
            terminal = true;
            setError(String(event.content));
            updateMessage(assistant.id, (message) => ({
              ...message,
              pending: false,
              thinking: undefined,
            }));
            setStreaming(false);
          }
        },
      );
      if (!terminal && !controller.signal.aborted) {
        setError('消息流提前结束，请重新发送。');
        updateMessage(assistant.id, (message) => ({ ...message, pending: false, thinking: undefined }));
      }
    } catch (caught) {
      if (!controller.signal.aborted) {
        const message = caught instanceof Error ? caught.message : '回复中断，请稍后重试。';
        setError(message);
        updateMessage(assistant.id, (item) => ({ ...item, pending: false, thinking: undefined }));
        if (caught instanceof ApiError && caught.status === 401) {
          await signOut();
        }
      }
    } finally {
      if (controller.signal.aborted) {
        updateMessage(assistant.id, (message) => ({
          ...message,
          pending: false,
          thinking: undefined,
          meta_data: { ...(message.meta_data ?? {}), interrupted: true },
        }));
      }
      if (abortRef.current === controller) {
        abortRef.current = null;
      }
      setStreaming(false);
    }
  }

  return (
    <>
      <SafeAreaView
        edges={['top', 'bottom']}
        style={[styles.safeArea, { backgroundColor: palette.page }]}>
        <KeyboardAvoidingView
          behavior={Platform.OS === 'ios' ? 'padding' : undefined}
          style={styles.flex}>
          <View style={[styles.header, { borderBottomColor: palette.border }]}>
            <Pressable
              accessibilityRole="button"
              accessibilityLabel="打开会话列表"
              hitSlop={6}
              onPress={() => {
                Keyboard.dismiss();
                setDrawerOpen(true);
              }}
              style={({ pressed }) => [styles.headerButton, pressed && styles.pressed]}>
              <View style={styles.headerIconFrame}>
                <View accessibilityElementsHidden importantForAccessibility="no-hide-descendants" style={styles.menuIcon}>
                  <View style={[styles.menuIconLine, { backgroundColor: palette.textSecondary }]} />
                  <View style={[styles.menuIconLine, { backgroundColor: palette.textSecondary }]} />
                  <View style={[styles.menuIconLine, { backgroundColor: palette.textSecondary }]} />
                </View>
              </View>
            </Pressable>
            <View pointerEvents="none" style={styles.headerTitle}>
              <Text numberOfLines={1} style={[styles.title, { color: palette.text }]}>
                {activeConversation?.title || '新对话'}
              </Text>
              <Text style={[styles.subtitle, { color: palette.textMuted }]}>
                {streaming ? 'Cove 正在回复' : 'Cove A'}
              </Text>
            </View>
            <Pressable
              accessibilityRole="button"
              accessibilityLabel="更多功能，暂不可用"
              disabled
              hitSlop={8}
              style={styles.headerButton}>
              <View style={styles.headerIconFrame}>
                <SymbolView
                  name="ellipsis"
                  size={20}
                  tintColor={palette.textSecondary}
                  weight="semibold"
                />
              </View>
            </Pressable>
          </View>

          <FlatList
            ref={listRef}
            data={messages}
            keyExtractor={(item) => item.id}
            keyboardDismissMode="interactive"
            keyboardShouldPersistTaps="handled"
            onContentSizeChange={handleMessageContentSizeChange}
            onLayout={handleMessageListLayout}
            onScroll={handleMessageScroll}
            onScrollBeginDrag={() => {
              streamScrollPosition.stopAnimation();
              stopInitialMessagePositioning();
              isAutoFollowingRef.current = false;
              isMessageListInteractingRef.current = true;
              shouldFollowStreamRef.current = false;
            }}
            onScrollEndDrag={handleMessageScrollEnd}
            onMomentumScrollBegin={() => {
              isMessageListInteractingRef.current = true;
              shouldFollowStreamRef.current = false;
            }}
            onMomentumScrollEnd={handleMessageScrollEnd}
            scrollEventThrottle={16}
            contentContainerStyle={messages.length
              ? [styles.messageList, messages[0]?.role === 'user' && styles.messageListStartsWithUser]
              : styles.emptyList}
            ListEmptyComponent={
              messageState === 'loading' ? (
                <View style={styles.loadingState}>
                  <ActivityIndicator color={palette.accent} />
                </View>
              ) : messageState === 'error' ? (
                <View style={styles.loadingState}>
                  <Text style={[styles.emptyError, { color: palette.danger }]}>{error}</Text>
                  {conversationId ? (
                    <Pressable
                      accessibilityRole="button"
                      onPress={() => void loadConversationHistory(conversationId)}
                      style={styles.retryHistory}>
                      <Text style={[styles.retryHistoryText, { color: palette.accent }]}>重新加载</Text>
                    </Pressable>
                  ) : null}
                </View>
              ) : (
                <View accessibilityLabel="新对话" style={styles.emptyReady} />
              )
            }
            renderItem={({ item }) => item.role === 'assistant' ? (
              <AssistantMessage message={item} streaming={streaming} />
            ) : (
              <View style={[styles.userMessage, { backgroundColor: palette.surfaceMuted }]}>
                <Text style={[styles.userMessageText, { color: palette.text }]}>{item.content}</Text>
              </View>
            )}
          />

          {error && messageState !== 'error' ? (
            <Text style={[styles.error, { color: palette.danger, backgroundColor: palette.dangerSurface }]}>
              {error}
            </Text>
          ) : null}

          <View style={styles.composerArea}>
            <View
              style={[
                styles.composer,
                {
                  backgroundColor: palette.surface,
                  borderColor: palette.borderStrong,
                  shadowColor: palette.shadow,
                },
              ]}>
              <TextInput
                value={draft}
                onChangeText={setDraft}
                editable={!streaming && messageState !== 'loading'}
                multiline
                maxLength={8000}
                placeholder="问问 Cove…"
                placeholderTextColor={palette.textMuted}
                selectionColor={palette.accent}
                style={[styles.composerInput, { color: palette.text }]}
              />
              <View style={styles.composerToolbar}>
                <View style={styles.toolGroup}>
                  <View style={[styles.attachTool, streaming && styles.toolDisabled]}>
                    <SymbolView
                      name="paperclip"
                      size={18}
                      tintColor={palette.textSecondary}
                      weight="regular"
                    />
                  </View>
                  <Pressable
                    accessibilityRole="button"
                    accessibilityLabel="知识库"
                    accessibilityState={{ selected: enableKnowledge, disabled: streaming }}
                    disabled={streaming}
                    onPress={() => setEnableKnowledge((current) => !current)}
                    style={({ pressed }) => [
                      styles.knowledgeTool,
                      {
                        backgroundColor: enableKnowledge ? palette.surfaceMuted : 'transparent',
                        borderColor: enableKnowledge ? palette.accent : 'transparent',
                      },
                      pressed && styles.pressed,
                      streaming && styles.toolDisabled,
                    ]}>
                    <SymbolView
                      name="tablecells"
                      size={17}
                      tintColor={enableKnowledge ? palette.text : palette.textSecondary}
                      weight="regular"
                    />
                  </Pressable>
                  <View style={[styles.webTool, streaming && styles.toolDisabled]}>
                    <Text style={[styles.webGlyph, { color: palette.textSecondary }]}>◉</Text>
                  </View>
                </View>
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={streaming ? '停止回复' : '发送消息'}
                  disabled={!streaming && (!draft.trim() || messageState === 'loading')}
                  onPress={streaming ? () => abortRef.current?.abort() : () => void sendMessage()}
                  style={({ pressed }) => [
                    styles.sendButton,
                    { backgroundColor: palette.accent },
                    pressed && styles.pressed,
                    (streaming || messageState === 'loading') && styles.disabled,
                  ]}>
                  <SymbolView
                    name="arrow.up"
                    size={18}
                    tintColor={palette.accentText}
                    weight="bold"
                  />
                </Pressable>
              </View>
            </View>
            <Text style={[styles.disclaimer, { color: palette.textMuted }]}>
              Cove 可能会出错，请核对重要信息。
            </Text>
          </View>
        </KeyboardAvoidingView>
      </SafeAreaView>
      <ChatDrawer
        visible={drawerOpen}
        conversations={conversations}
        activeConversationId={conversationId}
        state={conversationState}
        error={conversationError}
        user={session?.user ?? null}
        onClose={() => setDrawerOpen(false)}
        onRetry={() => void refreshConversations(false)}
        onNewChat={startNewConversation}
        onSelectConversation={selectConversation}
        onRenameConversation={handleRenameConversation}
        onDeleteConversation={handleDeleteConversation}
        onOpenKnowledge={() => router.push('/(app)/knowledge')}
        onOpenProfile={() => router.push('/(app)/profile')}
        onLogout={() => void handleLogout()}
        busyConversationId={streaming ? conversationId : undefined}
      />
    </>
  );
}

const styles = StyleSheet.create({
  safeArea: { flex: 1 },
  flex: { flex: 1 },
  header: {
    height: 58,
    paddingHorizontal: 8,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
  headerButton: { width: 44, height: 44, alignItems: 'center', justifyContent: 'center' },
  headerIconFrame: { width: 22, height: 22, alignItems: 'center', justifyContent: 'center' },
  menuIcon: {
    width: 20,
    height: 16,
    justifyContent: 'space-between',
  },
  menuIconLine: {
    width: 20,
    height: 2,
    borderRadius: 1,
  },
  headerTitle: { position: 'absolute', left: 60, right: 60, alignItems: 'center' },
  title: { fontSize: 15, lineHeight: 19, fontWeight: '600' },
  subtitle: { marginTop: 1, fontSize: 11, lineHeight: 14 },
  messageList: {
    paddingLeft: 18,
    paddingRight: 14,
    paddingTop: 22,
    paddingBottom: 18,
    gap: 28,
  },
  messageListStartsWithUser: { paddingTop: 31 },
  emptyList: { flexGrow: 1, paddingHorizontal: 18 },
  emptyReady: { flex: 1 },
  loadingState: { flex: 1, alignItems: 'center', justifyContent: 'center', paddingBottom: 38 },
  emptyError: { maxWidth: 286, textAlign: 'center', fontSize: 13, lineHeight: 20 },
  retryHistory: { minWidth: 84, minHeight: 42, alignItems: 'center', justifyContent: 'center' },
  retryHistoryText: { fontSize: 13, lineHeight: 18, fontWeight: '600' },
  userMessage: {
    alignSelf: 'flex-end',
    maxWidth: 300,
    borderRadius: 16,
    borderBottomRightRadius: 5,
    paddingHorizontal: 14,
    paddingVertical: 10,
  },
  userMessageText: { fontSize: 15, lineHeight: 24 },
  error: { marginHorizontal: 14, marginBottom: 8, borderRadius: 10, padding: 10, fontSize: 13 },
  composerArea: { paddingHorizontal: 14 },
  composer: {
    minHeight: 105,
    maxHeight: 176,
    borderRadius: 18,
    borderWidth: 1,
    paddingTop: 2,
    shadowOffset: { width: 0, height: 5 },
    shadowOpacity: 0.11,
    shadowRadius: 14,
    elevation: 3,
  },
  composerInput: {
    minHeight: 50,
    maxHeight: 118,
    paddingHorizontal: 14,
    paddingTop: 13,
    paddingBottom: 6,
    fontSize: 16,
    lineHeight: 22,
  },
  composerToolbar: {
    minHeight: 49,
    paddingHorizontal: 8,
    paddingBottom: 7,
    flexDirection: 'row',
    alignItems: 'flex-end',
    justifyContent: 'space-between',
  },
  toolGroup: { height: 36, flexDirection: 'row', alignItems: 'center' },
  attachTool: { width: 28, height: 36, alignItems: 'center', justifyContent: 'center' },
  knowledgeTool: {
    width: 32,
    height: 32,
    marginLeft: 6,
    borderRadius: 9,
    borderWidth: 1,
    alignItems: 'center',
    justifyContent: 'center',
  },
  webTool: { width: 32, height: 36, marginLeft: 13, alignItems: 'center', justifyContent: 'center' },
  webGlyph: { fontSize: 18, lineHeight: 22 },
  sendButton: { width: 40, height: 40, borderRadius: 13, alignItems: 'center', justifyContent: 'center' },
  disclaimer: { height: 18, paddingTop: 4, textAlign: 'center', fontSize: 10, lineHeight: 13 },
  toolDisabled: { opacity: 0.52 },
  pressed: { opacity: 0.64 },
  disabled: { opacity: 0.38 },
});
