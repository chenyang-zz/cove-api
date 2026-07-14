import {
  ArrowClockwise,
  ArrowUp,
  Books,
  CaretRight,
  DotsThree,
  GlobeHemisphereWest,
  List,
  Paperclip,
  PencilSimple,
  Plus,
  SignOut,
  Trash,
  WarningCircle,
  X,
} from '@phosphor-icons/react'
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type FormEvent,
  type KeyboardEvent,
  type MouseEvent as ReactMouseEvent,
  type CSSProperties,
  type UIEvent as ReactUIEvent,
} from 'react'
import { createPortal } from 'react-dom'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import type { StoredSession } from '../auth/types'
import {
  deleteConversation,
  getAgentConfig,
  listConversations,
  listMessages,
  renameConversation,
  streamChat,
} from './api'
import type {
  ChatAttachment,
  ChatMessage,
  ChatSubmission,
  ChatThinkEvent,
  ChatToolEvent,
  Conversation,
  MessagePart,
  ResourceState,
  StreamState,
} from './types'
import './ChatScreen.css'

const coveIcon = '/cove-mark.svg'
const maxAttachmentCount = 3
const maxAttachmentBytes = 1024 * 1024
const conversationPageSize = 20
const messagePageSize = 30
const paginationThreshold = 48
const conversationMenuWidth = 136
const conversationMenuHeight = 92
const conversationMenuGap = 6
const conversationMenuEdge = 8
const textFilePattern = /\.(?:txt|md|markdown|csv|json|log|ya?ml|xml|html?|css|jsx?|tsx?|py|go|rs|java|c|cpp|h|sh|sql)$/i

type ChatScreenProps = {
  session: StoredSession
  onLogout: () => void
  onOpenProfile?: () => void
  focusRequest?: number
}

type ConversationMenuPosition = {
  top: number
  left: number
}

function localMessage(role: 'user' | 'assistant', content: string): ChatMessage {
  return {
    id: `local-${role}-${Date.now()}-${Math.random().toString(16).slice(2)}`,
    role,
    content,
    meta_data: null,
    images: [],
    sender_persona_id: null,
    sender_name: null,
    feedback: null,
    created_at: new Date().toISOString(),
    pending: true,
    ...(role === 'assistant' ? { parts: [] } : {}),
  }
}

function upsertConversation(items: Conversation[], event: { conversation_id: string; title: string }) {
  const existing = items.find((item) => item.id === event.conversation_id)
  const timestamp = new Date().toISOString()
  const next: Conversation = existing
    ? { ...existing, title: event.title, updated_at: timestamp }
    : {
        id: event.conversation_id,
        title: event.title,
        is_group: false,
        member_persona_ids: [],
        enable_tools: false,
        created_at: timestamp,
        updated_at: timestamp,
      }
  return [next, ...items.filter((item) => item.id !== next.id)]
}

function textPart(text: string): MessagePart {
  return {
    type: 'text',
    text,
    tool: null,
    input: null,
    observation: null,
    error: null,
    iteration: null,
    tool_call_id: null,
  }
}

function appendTokenPart(parts: MessagePart[], text: string) {
  const next = [...parts]
  const last = next[next.length - 1]
  if (last?.type === 'text') {
    next[next.length - 1] = { ...last, text: `${last.text ?? ''}${text}` }
    return next
  }
  return [...next, textPart(text)]
}

function appendToolPart(parts: MessagePart[], event: ChatToolEvent) {
  return [
    ...parts,
    {
      type: event.type,
      text: null,
      tool: event.tool,
      input: event.input ?? null,
      observation: event.observation ?? null,
      error: event.error ?? null,
      iteration: event.iteration,
      tool_call_id: event.tool_call_id,
    },
  ]
}

function applyThinkEvent(message: ChatMessage, event: ChatThinkEvent): ChatMessage {
  if (event.status === 'thinking') {
    if (message.thinking && message.thinking.iteration > event.iteration) {
      return message
    }
    return {
      ...message,
      thinking: { active: true, iteration: event.iteration },
    }
  }
  if (message.thinking?.iteration !== event.iteration) {
    return message
  }
  return {
    ...message,
    thinking: { active: false, iteration: event.iteration },
  }
}

type TimelineItem =
  | { kind: 'text'; part: MessagePart; key: string }
  | { kind: 'tool'; call: MessagePart | null; result: MessagePart | null; key: string }

type MarkdownNode = {
  type: string
  value?: string
  children?: MarkdownNode[]
}

const markdownEmojiPattern = /(?:\p{Regional_Indicator}{2}|[#*0-9]\uFE0F?\u20E3|\p{Extended_Pictographic})(?:\uFE0F|\p{Emoji_Modifier})?(?:\u200D\p{Extended_Pictographic}(?:\uFE0F|\p{Emoji_Modifier})?)*/gu

function normalizeMarkdownEmoji(text: string) {
  return text.replace(/\u2753\uFE0F?/gu, '?').replace(markdownEmojiPattern, '')
}

function remarkEmojiFallback() {
  return (tree: MarkdownNode) => {
    function visit(node: MarkdownNode) {
      if (node.type === 'text' && typeof node.value === 'string') {
        node.value = normalizeMarkdownEmoji(node.value)
      }
      node.children?.forEach(visit)
    }
    visit(tree)
  }
}

function messageTimeline(message: ChatMessage): TimelineItem[] {
  const source = message.parts?.length
    ? message.parts
    : message.meta_data?.parts?.length
      ? message.meta_data.parts
      : message.content
        ? [textPart(message.content)]
        : []
  const timeline: TimelineItem[] = []
  const toolItems = new Map<string, Extract<TimelineItem, { kind: 'tool' }>>()

  source.forEach((part, index) => {
    if (part.type === 'text') {
      if (part.text?.trim()) {
        timeline.push({ kind: 'text', part, key: `text-${index}` })
      }
      return
    }
    if (part.type === 'tool_call') {
      const key = part.tool_call_id || `tool-${index}`
      const item: Extract<TimelineItem, { kind: 'tool' }> = {
        kind: 'tool',
        call: part,
        result: null,
        key,
      }
      timeline.push(item)
      toolItems.set(key, item)
      return
    }
    if (part.type === 'tool_result') {
      const key = part.tool_call_id || `tool-result-${index}`
      const existing = toolItems.get(key)
      if (existing) {
        existing.result = part
      } else {
        timeline.push({ kind: 'tool', call: null, result: part, key })
      }
      return
    }
    if (import.meta.env.DEV) {
      console.debug('Ignored chat message part', part)
    }
  })
  if (message.content.trim() && !timeline.some((item) => item.kind === 'text')) {
    timeline.push({ kind: 'text', part: textPart(message.content), key: 'content-fallback' })
  }
  return timeline
}

function MarkdownContent({ text }: { text: string }) {
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm, remarkEmojiFallback]}
      skipHtml
      components={{
        a: ({ href, children }) => (
          <a href={href} target="_blank" rel="noreferrer noopener">
            {children}
          </a>
        ),
        table: ({ children }) => (
          <div
            className="markdown-table-scroll"
            role="region"
            aria-label="表格内容，可横向滚动"
            tabIndex={0}
          >
            <table>{children}</table>
          </div>
        ),
      }}
    >
      {text}
    </ReactMarkdown>
  )
}

function ToolEventDetails({
  call,
  result,
  running,
}: {
  call: MessagePart | null
  result: MessagePart | null
  running: boolean
}) {
  const tool = call?.tool || result?.tool || '未知工具'
  const status = result ? (result.error ? 'error' : 'complete') : running ? 'running' : 'complete'
  const statusLabel = status === 'running' ? '正在使用工具' : status === 'error' ? '工具调用失败' : '工具已完成'

  return (
    <div className={`tool-event tool-event--${status}`} role="status" aria-label={`${statusLabel} ${tool}`}>
      <span className="tool-event__status" aria-hidden="true" />
      <span className="tool-event__name">{tool}</span>
    </div>
  )
}

function AssistantMessageContent({ message, streaming }: { message: ChatMessage; streaming: boolean }) {
  const timeline = messageTimeline(message)
  const hasText = timeline.some((item) => item.kind === 'text')
  const isThinking = Boolean(message.thinking?.active) || (message.pending === true && timeline.length === 0)

  return (
    <>
      {timeline.map((item) =>
        item.kind === 'text' ? (
          <div className="message-part message-part--text" key={item.key}>
            <MarkdownContent text={item.part.text ?? ''} />
          </div>
        ) : (
          <ToolEventDetails
            call={item.call}
            result={item.result}
            running={message.pending === true && streaming && message.id.startsWith('local-assistant-')}
            key={item.key}
          />
        ),
      )}
      {isThinking && (
        <span className="thinking-indicator" aria-label="Cove 正在思考">
          <span className="thinking-indicator__dots" aria-hidden="true"><i /><i /><i /></span>
          <span>Think...</span>
        </span>
      )}
      {message.meta_data?.interrupted && (
        <p className="message-interrupted" role="status">回复已中断</p>
      )}
      {message.pending && hasText && !isThinking && <span className="stream-cursor" aria-hidden="true" />}
    </>
  )
}

function mergeUniqueById<T extends { id: string }>(items: T[], incoming: T[]) {
  const seen = new Set(items.map((item) => item.id))
  return [...items, ...incoming.filter((item) => !seen.has(item.id))]
}

function prependUniqueById<T extends { id: string }>(items: T[], incoming: T[]) {
  const existing = new Set(items.map((item) => item.id))
  return [...incoming.filter((item) => !existing.has(item.id)), ...items]
}

type ScrollAnchor = {
  scrollHeight: number
  scrollTop: number
}

function isNearBottom(element: HTMLElement) {
  return element.scrollHeight - element.scrollTop - element.clientHeight <= paginationThreshold
}

function scrollToBottom(element: HTMLElement, behavior: ScrollBehavior = 'auto') {
  const maximumScrollTop = Math.max(0, element.scrollHeight - element.clientHeight)
  element.scrollTo({ top: maximumScrollTop, behavior })
}

function isTextAttachment(file: File) {
  return file.type.startsWith('text/') || file.type === 'application/json' || textFilePattern.test(file.name)
}

export function ChatScreen({ session, onLogout, onOpenProfile, focusRequest = 0 }: ChatScreenProps) {
  const [conversations, setConversations] = useState<Conversation[]>([])
  const [conversationState, setConversationState] = useState<ResourceState>('loading')
  const [conversationError, setConversationError] = useState('')
  const [conversationTotal, setConversationTotal] = useState(0)
  const [conversationNextPage, setConversationNextPage] = useState(2)
  const [conversationLoadingMore, setConversationLoadingMore] = useState(false)
  const [conversationMoreError, setConversationMoreError] = useState('')
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [messageState, setMessageState] = useState<ResourceState>('idle')
  const [messageError, setMessageError] = useState('')
  const [messageHasMore, setMessageHasMore] = useState(false)
  const [messageLoadingMore, setMessageLoadingMore] = useState(false)
  const [messageMoreError, setMessageMoreError] = useState('')
  const [streamState, setStreamState] = useState<StreamState>({ status: 'idle' })
  const [draft, setDraft] = useState('')
  const [attachments, setAttachments] = useState<ChatAttachment[]>([])
  const [attachmentError, setAttachmentError] = useState('')
  const [knowledgeEnabled, setKnowledgeEnabled] = useState(false)
  const [knowledgeState, setKnowledgeState] = useState<ResourceState>('loading')
  const [knowledgeError, setKnowledgeError] = useState('')
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [conversationMenuId, setConversationMenuId] = useState<string | null>(null)
  const [conversationMenuPosition, setConversationMenuPosition] = useState<ConversationMenuPosition | null>(null)
  const [renameTarget, setRenameTarget] = useState<Conversation | null>(null)
  const [renameTitle, setRenameTitle] = useState('')
  const [deleteTarget, setDeleteTarget] = useState<Conversation | null>(null)
  const [conversationActionError, setConversationActionError] = useState('')
  const [conversationActionPending, setConversationActionPending] = useState(false)
  const abortRef = useRef<AbortController | null>(null)
  const skipHistoryForRef = useRef<string | null>(null)
  const viewportRootRef = useRef<HTMLElement | null>(null)
  const conversationListRef = useRef<HTMLDivElement | null>(null)
  const conversationMenuRef = useRef<HTMLDivElement | null>(null)
  const messageScrollRef = useRef<HTMLDivElement | null>(null)
  const textareaRef = useRef<HTMLTextAreaElement | null>(null)
  const fileInputRef = useRef<HTMLInputElement | null>(null)
  const hasMessagesRef = useRef(false)
  const selectedIdRef = useRef<string | null>(null)
  const historyGenerationRef = useRef(0)
  const conversationLoadingMoreRef = useRef(false)
  const messageLoadingMoreRef = useRef(false)
  const messageAnchorRef = useRef<ScrollAnchor | null>(null)
  const initialMessageScrollRef = useRef(false)
  const autoFollowRef = useRef(true)
  const keyboardHeightRef = useRef(Number(sessionStorage.getItem('cove-keyboard-height')) || 0)
  const keyboardPreparationTimerRef = useRef<number | null>(null)

  const displayName = session.user.nickname?.trim() || '用户'
  const activeConversation = conversations.find((item) => item.id === selectedId)
  const menuConversation = conversations.find((item) => item.id === conversationMenuId)
  const isEmptyConversation = messageState === 'ready' && messages.length === 0

  const loadConversations = useCallback(async (selectFirst = false) => {
    setConversationState('loading')
    setConversationError('')
    setConversationMoreError('')
    try {
      const response = await listConversations(1, conversationPageSize)
      const sorted = [...response.list].sort(
        (a, b) => Date.parse(b.updated_at) - Date.parse(a.updated_at),
      )
      const total = Number.isFinite(response.total) ? response.total : sorted.length
      const page = Number.isFinite(response.page) ? response.page : 1
      setConversations(sorted)
      setConversationTotal(total)
      setConversationNextPage(page + 1)
      setConversationState('ready')
      if (selectFirst && sorted.length > 0) {
        setSelectedId((current) => {
          const next = current ?? sorted[0].id
          selectedIdRef.current = next
          return next
        })
      }
    } catch (error: unknown) {
      setConversationState('error')
      setConversationError(error instanceof Error ? error.message : '会话加载失败。')
    }
  }, [])

  const loadMoreConversations = useCallback(async () => {
    if (
      conversationLoadingMoreRef.current ||
      conversationState !== 'ready' ||
      conversations.length >= conversationTotal
    ) {
      return
    }
    conversationLoadingMoreRef.current = true
    setConversationLoadingMore(true)
    setConversationMoreError('')
    try {
      const response = await listConversations(conversationNextPage, conversationPageSize)
      setConversations((current) => mergeUniqueById(current, response.list))
      if (Number.isFinite(response.total)) {
        setConversationTotal(response.total)
      }
      setConversationNextPage(
        Number.isFinite(response.page) ? response.page + 1 : conversationNextPage + 1,
      )
    } catch (error: unknown) {
      setConversationMoreError(error instanceof Error ? error.message : '更多会话加载失败。')
    } finally {
      conversationLoadingMoreRef.current = false
      setConversationLoadingMore(false)
    }
  }, [conversationNextPage, conversationState, conversationTotal, conversations.length])

  const loadHistory = useCallback(async (conversationId: string) => {
    const generation = ++historyGenerationRef.current
    initialMessageScrollRef.current = false
    setMessageState('loading')
    setMessageError('')
    setMessageMoreError('')
    setMessageHasMore(false)
    setMessageLoadingMore(false)
    messageLoadingMoreRef.current = false
    try {
      const response = await listMessages(conversationId, { limit: messagePageSize })
      if (generation !== historyGenerationRef.current || selectedIdRef.current !== conversationId) {
        return
      }
      initialMessageScrollRef.current = true
      setMessages(response.list)
      setMessageHasMore(response.has_more)
      setMessageState('ready')
    } catch (error: unknown) {
      if (generation !== historyGenerationRef.current || selectedIdRef.current !== conversationId) {
        return
      }
      setMessageState('error')
      setMessageError(error instanceof Error ? error.message : '消息加载失败。')
    }
  }, [])

  const loadOlderHistory = useCallback(async () => {
    const conversationId = selectedId
    const oldestMessage = messages[0]
    const scroll = messageScrollRef.current
    if (
      !conversationId ||
      !oldestMessage ||
      !scroll ||
      !messageHasMore ||
      messageLoadingMoreRef.current ||
      messageState !== 'ready'
    ) {
      return
    }

    const generation = historyGenerationRef.current
    messageLoadingMoreRef.current = true
    setMessageLoadingMore(true)
    setMessageMoreError('')
    const anchor = { scrollHeight: scroll.scrollHeight, scrollTop: scroll.scrollTop }
    try {
      const response = await listMessages(conversationId, {
        limit: messagePageSize,
        before: oldestMessage.id,
      })
      if (generation !== historyGenerationRef.current || selectedIdRef.current !== conversationId) {
        return
      }
      messageAnchorRef.current = anchor
      setMessages((current) => prependUniqueById(current, response.list))
      setMessageHasMore(response.has_more)
    } catch (error: unknown) {
      if (generation === historyGenerationRef.current && selectedIdRef.current === conversationId) {
        setMessageMoreError(error instanceof Error ? error.message : '更早消息加载失败。')
      }
    } finally {
      if (generation === historyGenerationRef.current) {
        messageLoadingMoreRef.current = false
        setMessageLoadingMore(false)
      }
    }
  }, [messageHasMore, messageState, messages, selectedId])

  const loadKnowledgeConfig = useCallback(async () => {
    setKnowledgeState('loading')
    setKnowledgeError('')
    try {
      const config = await getAgentConfig()
      setKnowledgeEnabled(Boolean(config.enable_knowledge))
      setKnowledgeState('ready')
    } catch (error: unknown) {
      setKnowledgeState('error')
      setKnowledgeError(error instanceof Error ? error.message : '知识库配置加载失败。')
    }
  }, [])

  useEffect(() => {
    void loadConversations(true)
    void loadKnowledgeConfig()
  }, [loadConversations, loadKnowledgeConfig])

  useEffect(() => {
    if (!selectedId) {
      selectedIdRef.current = null
      historyGenerationRef.current += 1
      setMessages([])
      setMessageState('ready')
      setMessageHasMore(false)
      setMessageMoreError('')
      return
    }
    selectedIdRef.current = selectedId
    if (skipHistoryForRef.current === selectedId) {
      skipHistoryForRef.current = null
      setMessageState('ready')
      setMessageHasMore(false)
      return
    }
    void loadHistory(selectedId)
  }, [loadHistory, selectedId])

  useLayoutEffect(() => {
    hasMessagesRef.current = messages.length > 0
    const messageScroll = messageScrollRef.current
    if (!messageScroll) {
      return
    }
    if (messageAnchorRef.current) {
      const anchor = messageAnchorRef.current
      messageAnchorRef.current = null
      messageScroll.scrollTop = anchor.scrollTop + (messageScroll.scrollHeight - anchor.scrollHeight)
      return
    }
    if (initialMessageScrollRef.current) {
      initialMessageScrollRef.current = false
      scrollToBottom(messageScroll)
      autoFollowRef.current = true
      return
    }
    if (autoFollowRef.current) {
      scrollToBottom(messageScroll, streamState.status === 'streaming' ? 'auto' : 'smooth')
    }
  }, [messages, streamState.status])

  useEffect(() => {
    return () => {
      abortRef.current?.abort()
      if (keyboardPreparationTimerRef.current !== null) {
        window.clearTimeout(keyboardPreparationTimerRef.current)
      }
    }
  }, [])

  useEffect(() => {
    if (!conversationMenuId) {
      return
    }

    function closeConversationMenu(event: PointerEvent) {
      const target = event.target
      if (
        target instanceof Element &&
        target.closest('[data-conversation-menu-trigger]')?.getAttribute('data-conversation-menu-trigger') === conversationMenuId
      ) {
        return
      }
      if (!conversationMenuRef.current?.contains(event.target as Node)) {
        setConversationMenuId(null)
        setConversationMenuPosition(null)
      }
    }

    function closeConversationMenuWithEscape(event: globalThis.KeyboardEvent) {
      if (event.key === 'Escape') {
        setConversationMenuId(null)
        setConversationMenuPosition(null)
      }
    }

    function closeConversationMenuForLayoutChange() {
      setConversationMenuId(null)
      setConversationMenuPosition(null)
    }

    const conversationList = viewportRootRef.current?.querySelector('.conversation-list')
    document.addEventListener('pointerdown', closeConversationMenu)
    document.addEventListener('keydown', closeConversationMenuWithEscape)
    conversationList?.addEventListener('scroll', closeConversationMenuForLayoutChange)
    window.addEventListener('resize', closeConversationMenuForLayoutChange)
    return () => {
      document.removeEventListener('pointerdown', closeConversationMenu)
      document.removeEventListener('keydown', closeConversationMenuWithEscape)
      conversationList?.removeEventListener('scroll', closeConversationMenuForLayoutChange)
      window.removeEventListener('resize', closeConversationMenuForLayoutChange)
    }
  }, [conversationMenuId])

  useEffect(() => {
    if (!renameTarget && !deleteTarget) {
      return
    }
    function closeDialogWithEscape(event: globalThis.KeyboardEvent) {
      if (event.key === 'Escape' && !conversationActionPending) {
        setRenameTarget(null)
        setDeleteTarget(null)
        setConversationActionError('')
      }
    }
    document.addEventListener('keydown', closeDialogWithEscape)
    return () => document.removeEventListener('keydown', closeDialogWithEscape)
  }, [conversationActionPending, deleteTarget, renameTarget])

  useLayoutEffect(() => {
    document.documentElement.classList.add('chat-document')
    window.scrollTo(0, 0)
    return () => document.documentElement.classList.remove('chat-document')
  }, [])

  useLayoutEffect(() => {
    const root = viewportRootRef.current
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
      const contentShift = Math.round(Math.min(160, keyboardHeight * 0.45))
      const keyboardOpen = keyboardHeight > 20
      if (!keyboardOpen && activeViewport.height > layoutHeight) {
        layoutHeight = activeViewport.height
      }

      activeRoot.style.setProperty('--chat-keyboard-height', `${keyboardHeight}px`)
      activeRoot.style.setProperty('--chat-content-shift', `${contentShift}px`)
      activeRoot.dataset.keyboardOpen = String(keyboardOpen)
      if (keyboardOpen) {
        keyboardHeightRef.current = keyboardHeight
        sessionStorage.setItem('cove-keyboard-height', String(keyboardHeight))
        if (keyboardPreparationTimerRef.current !== null) {
          window.clearTimeout(keyboardPreparationTimerRef.current)
          keyboardPreparationTimerRef.current = null
        }
        window.requestAnimationFrame(() => {
          const messageScroll = messageScrollRef.current
          messageScroll?.scrollTo({ top: messageScroll.scrollHeight })
        })
      } else if (!hasMessagesRef.current) {
        window.requestAnimationFrame(() => {
          messageScrollRef.current?.scrollTo({ top: 0 })
        })
      }
    }

    syncVisualViewport()
    activeViewport.addEventListener('resize', syncVisualViewport)
    return () => {
      activeViewport.removeEventListener('resize', syncVisualViewport)
    }
  }, [])

  function focusComposerWithoutScroll(textarea: HTMLTextAreaElement) {
    const root = viewportRootRef.current
    const viewport = window.visualViewport
    if (root && viewport && viewport.width < 900) {
      const anticipatedHeight =
        keyboardHeightRef.current || Math.min(360, Math.max(260, window.innerHeight * 0.38))
      const anticipatedContentShift = Math.round(Math.min(160, anticipatedHeight * 0.45))
      const heightBeforeFocus = viewport.height
      root.style.setProperty('--chat-keyboard-height', `${anticipatedHeight}px`)
      root.dataset.keyboardOpen = 'true'
      void root.offsetHeight

      if (keyboardPreparationTimerRef.current !== null) {
        window.clearTimeout(keyboardPreparationTimerRef.current)
      }
      keyboardPreparationTimerRef.current = window.setTimeout(() => {
        if (viewport.height >= heightBeforeFocus - 20) {
          root.style.setProperty('--chat-keyboard-height', '0px')
          root.style.setProperty('--chat-content-shift', '0px')
          root.dataset.keyboardOpen = 'false'
        }
        keyboardPreparationTimerRef.current = null
      }, 650)
      window.requestAnimationFrame(() => {
        root.style.setProperty('--chat-content-shift', `${anticipatedContentShift}px`)
      })
    }
    textarea.focus({ preventScroll: true })
  }

  useEffect(() => {
    if (focusRequest === 0) {
      return
    }
    const focusFrame = window.requestAnimationFrame(() => {
      const textarea = textareaRef.current
      if (textarea) {
        focusComposerWithoutScroll(textarea)
      }
    })
    return () => window.cancelAnimationFrame(focusFrame)
  }, [focusRequest])

  function handleComposerSurfacePress(event: {
    target: EventTarget | null
    preventDefault: () => void
  }) {
    const root = viewportRootRef.current
    const textarea = textareaRef.current
    const target = event.target
    if (
      !root ||
      !textarea ||
      !(target instanceof Element) ||
      target.closest('button') ||
      root.dataset.keyboardOpen === 'true'
    ) {
      return
    }

    event.preventDefault()
    focusComposerWithoutScroll(textarea)
  }

  function startNewConversation() {
    abortRef.current?.abort()
    abortRef.current = null
    historyGenerationRef.current += 1
    selectedIdRef.current = null
    setSelectedId(null)
    setMessages([])
    setMessageState('ready')
    setMessageError('')
    setMessageHasMore(false)
    setMessageMoreError('')
    setStreamState({ status: 'idle' })
    setAttachments([])
    setAttachmentError('')
    setConversationMenuId(null)
    setConversationMenuPosition(null)
    setDrawerOpen(false)
    window.requestAnimationFrame(() => {
      const textarea = textareaRef.current
      if (textarea) {
        focusComposerWithoutScroll(textarea)
      }
    })
  }

  function selectConversation(conversationId: string) {
    if (conversationId === selectedId) {
      setDrawerOpen(false)
      return
    }
    abortRef.current?.abort()
    abortRef.current = null
    historyGenerationRef.current += 1
    initialMessageScrollRef.current = false
    selectedIdRef.current = conversationId
    setStreamState({ status: 'idle' })
    setAttachments([])
    setAttachmentError('')
    setConversationMenuId(null)
    setConversationMenuPosition(null)
    setMessages([])
    setMessageState('loading')
    setMessageError('')
    setMessageHasMore(false)
    setMessageMoreError('')
    setSelectedId(conversationId)
    setDrawerOpen(false)
  }

  function beginRename(conversation: Conversation) {
    setConversationMenuId(null)
    setConversationMenuPosition(null)
    setConversationActionError('')
    setRenameTitle(conversation.title || '新对话')
    setRenameTarget(conversation)
  }

  function toggleConversationMenu(
    event: ReactMouseEvent<HTMLButtonElement>,
    conversationId: string,
  ) {
    if (conversationMenuId === conversationId) {
      setConversationMenuId(null)
      setConversationMenuPosition(null)
      return
    }

    const triggerRect = event.currentTarget.getBoundingClientRect()
    const listRect = event.currentTarget.closest('.conversation-list')?.getBoundingClientRect()
    const drawerRect = event.currentTarget.closest('.chat-drawer')?.getBoundingClientRect()
    const boundaryTop = listRect?.top ?? conversationMenuEdge
    const boundaryBottom = listRect?.bottom ?? window.innerHeight - conversationMenuEdge
    const minimumTop = boundaryTop + conversationMenuEdge
    const maximumTop = Math.max(
      minimumTop,
      boundaryBottom - conversationMenuHeight - conversationMenuEdge,
    )
    const belowTop = triggerRect.bottom + conversationMenuGap
    const preferredTop = belowTop + conversationMenuHeight <= boundaryBottom - conversationMenuEdge
      ? belowTop
      : triggerRect.top - conversationMenuHeight - conversationMenuGap
    const minimumLeft = (drawerRect?.left ?? 0) + conversationMenuEdge
    const maximumLeft = Math.max(
      minimumLeft,
      (drawerRect?.right ?? window.innerWidth) - conversationMenuWidth - conversationMenuEdge,
    )

    setConversationMenuPosition({
      top: Math.min(maximumTop, Math.max(minimumTop, preferredTop)),
      left: Math.min(
        maximumLeft,
        Math.max(minimumLeft, triggerRect.right - conversationMenuWidth),
      ),
    })
    setConversationMenuId(conversationId)
  }

  async function submitRename(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const title = renameTitle.trim()
    if (!renameTarget || !title || title.length > 256 || conversationActionPending) {
      if (!title) {
        setConversationActionError('请输入会话名称。')
      } else if (title.length > 256) {
        setConversationActionError('会话名称不能超过 256 个字符。')
      }
      return
    }

    setConversationActionPending(true)
    setConversationActionError('')
    try {
      const updated = await renameConversation(renameTarget.id, title)
      setConversations((current) =>
        current.map((conversation) => (conversation.id === updated.id ? updated : conversation)),
      )
      setRenameTarget(null)
    } catch (error: unknown) {
      setConversationActionError(error instanceof Error ? error.message : '重命名失败。')
    } finally {
      setConversationActionPending(false)
    }
  }

  async function confirmDeleteConversation() {
    if (!deleteTarget || conversationActionPending) {
      return
    }
    setConversationActionPending(true)
    setConversationActionError('')
    try {
      await deleteConversation(deleteTarget.id)
      setConversations((current) => current.filter((conversation) => conversation.id !== deleteTarget.id))
      setConversationTotal((current) => Math.max(0, current - 1))
      if (selectedId === deleteTarget.id) {
        abortRef.current?.abort()
        abortRef.current = null
        historyGenerationRef.current += 1
        selectedIdRef.current = null
        setSelectedId(null)
        setMessages([])
        setMessageState('ready')
        setMessageHasMore(false)
        setMessageMoreError('')
        setStreamState({ status: 'idle' })
        setAttachments([])
        setAttachmentError('')
        setDrawerOpen(false)
      }
      setDeleteTarget(null)
    } catch (error: unknown) {
      setConversationActionError(error instanceof Error ? error.message : '删除会话失败。')
    } finally {
      setConversationActionPending(false)
    }
  }

  async function handleAttachmentSelection(files: FileList | null) {
    if (!files?.length) {
      return
    }
    const next = [...attachments]
    let errorMessage = ''
    for (const file of Array.from(files)) {
      if (next.length >= maxAttachmentCount) {
        errorMessage = `最多添加 ${maxAttachmentCount} 个附件。`
        break
      }
      if (!isTextAttachment(file)) {
        errorMessage = `${file.name} 不是支持的文本文件。`
        continue
      }
      if (file.size > maxAttachmentBytes) {
        errorMessage = `${file.name} 超过 1 MiB。`
        continue
      }
      if (next.some((attachment) => attachment.file_name === file.name)) {
        errorMessage = `${file.name} 已经添加。`
        continue
      }
      const text = await file.text()
      if (!text) {
        errorMessage = `${file.name} 是空文件。`
        continue
      }
      next.push({ file_name: file.name, text })
    }
    setAttachments(next)
    setAttachmentError(errorMessage)
    if (fileInputRef.current) {
      fileInputRef.current.value = ''
    }
  }

  function updateAssistant(id: string, updater: (message: ChatMessage) => ChatMessage) {
    setMessages((current) => current.map((message) => (message.id === id ? updater(message) : message)))
  }

  async function sendMessage(submission?: ChatSubmission) {
    const request: ChatSubmission = submission ?? {
      message: draft.trim(),
      attachments: attachments.map((attachment) => ({ ...attachment })),
      enableKnowledge: knowledgeEnabled,
    }
    const text = request.message.trim()
    if (!text || streamState.status === 'streaming') {
      return
    }

    const conversationIdAtSend = selectedId
    const userMessage = localMessage('user', text)
    const assistantMessage: ChatMessage = {
      ...localMessage('assistant', ''),
      thinking: { active: true, iteration: 0 },
    }
    const controller = new AbortController()
    abortRef.current = controller
    setDraft('')
    setAttachments([])
    setAttachmentError('')
    setMessageError('')
    setMessageState('ready')
    setStreamState({ status: 'streaming' })
    autoFollowRef.current = true
    setMessages((current) => [...current, userMessage, assistantMessage])
    let reachedTerminalEvent = false

    try {
      await streamChat(
        {
          ...(conversationIdAtSend ? { conversation_id: conversationIdAtSend } : {}),
          message: text,
          ...(request.attachments.length > 0 ? { attachments: request.attachments } : {}),
          enable_knowledge: request.enableKnowledge,
        },
        controller.signal,
        (event) => {
          if (event.type === 'meta') {
            const meta = event as { type: 'meta'; conversation_id: string; title: string }
            if (meta.conversation_id !== conversationIdAtSend) {
              skipHistoryForRef.current = meta.conversation_id
              selectedIdRef.current = meta.conversation_id
              setSelectedId(meta.conversation_id)
            }
            setConversations((current) => {
              if (!current.some((conversation) => conversation.id === meta.conversation_id)) {
                setConversationTotal((total) => total + 1)
              }
              return upsertConversation(current, meta)
            })
            return
          }
          if (event.type === 'token') {
            const token = event as { type: 'token'; text: string }
            updateAssistant(assistantMessage.id, (message) => ({
              ...message,
              content: message.content + token.text,
              parts: appendTokenPart(message.parts ?? [], token.text),
              thinking: message.thinking
                ? { ...message.thinking, active: false }
                : undefined,
            }))
            return
          }
          if (event.type === 'think') {
            const think = event as ChatThinkEvent
            if (
              (think.status !== 'thinking' && think.status !== 'done')
              || !Number.isInteger(think.iteration)
              || think.iteration < 0
            ) {
              if (import.meta.env.DEV) {
                console.debug('Ignored invalid chat think event', event)
              }
              return
            }
            updateAssistant(assistantMessage.id, (message) => applyThinkEvent(message, think))
            return
          }
          if (event.type === 'tool_call' || event.type === 'tool_result') {
            updateAssistant(assistantMessage.id, (message) => ({
              ...message,
              parts: appendToolPart(message.parts ?? [], event as ChatToolEvent),
              thinking: message.thinking
                ? { ...message.thinking, active: false }
                : undefined,
            }))
            return
          }
          if (event.type === 'done') {
            reachedTerminalEvent = true
            const done = event as { type: 'done'; text: string }
            updateAssistant(assistantMessage.id, (message) => ({
              ...message,
              id: done.text || message.id,
              pending: false,
              thinking: undefined,
            }))
            setMessages((current) =>
              current.map((message) =>
                message.id === userMessage.id ? { ...message, pending: false } : message,
              ),
            )
            setStreamState({ status: 'idle' })
            return
          }
          if (event.type === 'error') {
            reachedTerminalEvent = true
            const streamError = event as { type: 'error'; content: string }
            updateAssistant(assistantMessage.id, (message) => ({
              ...message,
              pending: false,
              thinking: undefined,
            }))
            setStreamState({ status: 'error', message: streamError.content, submission: request })
            return
          }
          if (import.meta.env.DEV) {
            console.debug('Ignored chat stream event', event)
          }
        },
      )
      if (!reachedTerminalEvent && !controller.signal.aborted) {
        updateAssistant(assistantMessage.id, (item) => ({
          ...item,
          pending: false,
          thinking: undefined,
        }))
        setStreamState({
          status: 'error',
          message: '消息流提前结束，请重新发送。',
          submission: request,
        })
      }
    } catch (error: unknown) {
      if (!controller.signal.aborted) {
        const message = error instanceof Error ? error.message : '回复中断，请稍后重试。'
        updateAssistant(assistantMessage.id, (item) => ({
          ...item,
          pending: false,
          thinking: undefined,
        }))
        setStreamState({ status: 'error', message, submission: request })
      }
    } finally {
      if (abortRef.current === controller) {
        abortRef.current = null
      }
    }
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    void sendMessage()
  }

  function handleComposerKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing) {
      event.preventDefault()
      void sendMessage()
    }
  }

  function handleDraftChange(value: string) {
    setDraft(value)
    const textarea = textareaRef.current
    if (textarea) {
      textarea.style.height = 'auto'
      textarea.style.height = `${Math.min(textarea.scrollHeight, 132)}px`
    }
  }

  function handleConversationListScroll(event: ReactUIEvent<HTMLDivElement>) {
    const element = event.currentTarget
    if (isNearBottom(element)) {
      void loadMoreConversations()
    }
  }

  function handleMessageScroll(event: ReactUIEvent<HTMLDivElement>) {
    const element = event.currentTarget
    autoFollowRef.current = isNearBottom(element)
    if (element.scrollTop <= paginationThreshold) {
      void loadOlderHistory()
    }
  }

  function openProfile() {
    if (document.activeElement instanceof HTMLElement) {
      document.activeElement.blur()
    }
    setConversationMenuId(null)
    setConversationMenuPosition(null)
    onOpenProfile?.()
  }

  return (
    <main className="chat-app" ref={viewportRootRef}>
      <button
        className={drawerOpen ? 'chat-drawer-scrim chat-drawer-scrim--visible' : 'chat-drawer-scrim'}
        type="button"
        aria-label="关闭会话列表"
        tabIndex={drawerOpen ? 0 : -1}
        onClick={() => setDrawerOpen(false)}
      />

      <aside className={drawerOpen ? 'chat-drawer chat-drawer--open' : 'chat-drawer'}>
        <header className="chat-drawer__header">
          <div className="brand-lockup">
            <img className="brand-lockup__icon" src={coveIcon} alt="" />
            <span>Cove</span>
          </div>
          <button className="icon-button chat-drawer__close" type="button" aria-label="关闭会话列表" onClick={() => setDrawerOpen(false)}>
            <X size={20} weight="bold" />
          </button>
        </header>

        <button className="new-chat-button" type="button" onClick={startNewConversation}>
          <Plus size={18} weight="bold" />
          <span>新对话</span>
        </button>

        <section className="conversation-history" aria-labelledby="recent-conversations-label">
          <p className="conversation-list__label" id="recent-conversations-label">最近对话</p>
          <div className="conversation-list-frame">
            <div
              className="conversation-list"
              ref={conversationListRef}
              aria-label="历史会话"
              onScroll={handleConversationListScroll}
            >
              {conversationState === 'loading' && (
                <div className="conversation-skeleton" aria-label="正在加载会话">
                  <span />
                  <span />
                  <span />
                </div>
              )}
              {conversationState === 'error' && (
                <div className="drawer-error" role="alert">
                  <p>{conversationError}</p>
                  <button type="button" onClick={() => void loadConversations(false)}>
                    <ArrowClockwise size={16} /> 重试
                  </button>
                </div>
              )}
              {conversationState === 'ready' && conversations.length === 0 && (
                <p className="conversation-list__empty">发送第一条消息后，会话会保存在这里。</p>
              )}
              {conversations.map((conversation, index) => (
                <div
                  className={conversation.id === selectedId ? 'conversation-row conversation-row--active' : 'conversation-row'}
                  key={conversation.id}
                  style={{ '--conversation-index': index % conversationPageSize } as CSSProperties}
                >
                  <button className="conversation-row__select" type="button" onClick={() => selectConversation(conversation.id)}>
                    <span>{conversation.title || '新对话'}</span>
                    <time dateTime={conversation.updated_at}>
                      {new Intl.DateTimeFormat('zh-CN', { month: 'numeric', day: 'numeric' }).format(
                        new Date(conversation.updated_at),
                      )}
                    </time>
                  </button>
                  <div className="conversation-row__menu-wrap">
                    <button
                      className="conversation-row__menu-trigger"
                      type="button"
                      aria-label={`管理会话：${conversation.title || '新对话'}`}
                      aria-expanded={conversationMenuId === conversation.id}
                      data-conversation-menu-trigger={conversation.id}
                      onClick={(event) => toggleConversationMenu(event, conversation.id)}
                    >
                      <DotsThree size={18} weight="bold" />
                    </button>
                  </div>
                </div>
              ))}
              <div className="conversation-list__sentinel" aria-hidden="true" />
              {conversationLoadingMore && (
                <p className="pagination-status" role="status">正在加载更多会话…</p>
              )}
              {conversationMoreError && (
                <div className="pagination-error" role="alert">
                  <span>{conversationMoreError}</span>
                  <button type="button" onClick={() => void loadMoreConversations()}>重试</button>
                </div>
              )}
            </div>
          </div>
        </section>

        <div className="drawer-account">
          <button className="drawer-account__profile" type="button" aria-label="查看个人信息" onClick={openProfile}>
            <span className="drawer-account__avatar">{displayName.slice(0, 1).toUpperCase()}</span>
            <span className="drawer-account__identity">
              <span className="drawer-account__name">{displayName}</span>
              <span className="drawer-account__username">@{session.user.username}</span>
            </span>
            <CaretRight className="drawer-account__chevron" size={17} weight="bold" />
          </button>
          <button className="drawer-account__logout" type="button" aria-label="退出登录" onClick={onLogout}>
            <SignOut size={18} />
          </button>
        </div>
      </aside>

      {menuConversation && conversationMenuPosition && createPortal(
        <div
          className="conversation-row__menu"
          ref={conversationMenuRef}
          role="menu"
          style={conversationMenuPosition}
        >
          <button
            type="button"
            role="menuitem"
            disabled={streamState.status === 'streaming' && selectedId === menuConversation.id}
            onClick={() => beginRename(menuConversation)}
          >
            <PencilSimple size={16} /> 重命名
          </button>
          <button
            className="conversation-row__delete"
            type="button"
            role="menuitem"
            disabled={streamState.status === 'streaming' && selectedId === menuConversation.id}
            onClick={() => {
              setConversationMenuId(null)
              setConversationMenuPosition(null)
              setConversationActionError('')
              setDeleteTarget(menuConversation)
            }}
          >
            <Trash size={16} /> 删除
          </button>
        </div>,
        document.body,
      )}

      {renameTarget && (
        <div
          className="conversation-dialog-backdrop"
          onClick={(event) => {
            if (event.target === event.currentTarget && !conversationActionPending) {
              setRenameTarget(null)
              setConversationActionError('')
            }
          }}
        >
          <form className="conversation-dialog" role="dialog" aria-modal="true" aria-labelledby="rename-conversation-title" onSubmit={submitRename}>
            <h2 id="rename-conversation-title">重命名会话</h2>
            <input
              autoFocus
              value={renameTitle}
              maxLength={256}
              aria-label="会话名称"
              disabled={conversationActionPending}
              onChange={(event) => setRenameTitle(event.target.value)}
            />
            {conversationActionError && <p role="alert">{conversationActionError}</p>}
            <div className="conversation-dialog__actions">
              <button type="button" disabled={conversationActionPending} onClick={() => { setRenameTarget(null); setConversationActionError('') }}>取消</button>
              <button className="conversation-dialog__primary" type="submit" disabled={conversationActionPending || !renameTitle.trim()}>保存</button>
            </div>
          </form>
        </div>
      )}

      {deleteTarget && (
        <div
          className="conversation-dialog-backdrop"
          onClick={(event) => {
            if (event.target === event.currentTarget && !conversationActionPending) {
              setDeleteTarget(null)
              setConversationActionError('')
            }
          }}
        >
          <section className="conversation-dialog" role="dialog" aria-modal="true" aria-labelledby="delete-conversation-title">
            <h2 id="delete-conversation-title">删除会话？</h2>
            <p>“{deleteTarget.title || '新对话'}”及其消息将被永久删除。</p>
            {conversationActionError && <p className="conversation-dialog__error" role="alert">{conversationActionError}</p>}
            <div className="conversation-dialog__actions">
              <button type="button" disabled={conversationActionPending} onClick={() => { setDeleteTarget(null); setConversationActionError('') }}>取消</button>
              <button className="conversation-dialog__danger" type="button" disabled={conversationActionPending} onClick={() => void confirmDeleteConversation()}>删除</button>
            </div>
          </section>
        </div>
      )}

      <section className="chat-workspace">
        <header className="chat-header">
          <button className="icon-button chat-header__menu" type="button" aria-label="打开会话列表" onClick={() => setDrawerOpen(true)}>
            <List size={22} />
          </button>
          <div className="chat-header__title">
            <strong>{activeConversation?.title || '新对话'}</strong>
            <span>{streamState.status === 'streaming' ? 'Cove 正在回复' : 'Cove AI'}</span>
          </div>
          <div className="account-menu">
            <button className="icon-button account-menu__placeholder" type="button" aria-label="更多功能，暂不可用" disabled>
              <DotsThree size={24} weight="bold" />
            </button>
          </div>
        </header>

        <div
          className={isEmptyConversation ? 'message-scroll message-scroll--empty' : 'message-scroll'}
          ref={messageScrollRef}
          aria-busy={messageState === 'loading'}
          onScroll={handleMessageScroll}
        >
          <div className="message-column" role="log" aria-live="polite" aria-relevant="additions text">
            <div className="message-history-sentinel" aria-hidden="true" />
            {messageLoadingMore && (
              <p className="pagination-status message-history-status" role="status">正在加载更早消息…</p>
            )}
            {messageMoreError && (
              <div className="pagination-error message-history-error" role="alert">
                <span>{messageMoreError}</span>
                <button type="button" onClick={() => void loadOlderHistory()}>重试</button>
              </div>
            )}
            {messageState === 'loading' && (
              <div className="message-skeleton" aria-label="正在加载消息">
                <span />
                <span />
                <span />
              </div>
            )}
            {messageState === 'error' && (
              <div className="message-error" role="alert">
                <WarningCircle size={24} />
                <p>{messageError}</p>
                {selectedId && (
                  <button type="button" onClick={() => void loadHistory(selectedId)}>
                    重新加载
                  </button>
                )}
              </div>
            )}
            {messageState === 'ready' && messages.length === 0 && (
              <div className="chat-empty">
                <img src={coveIcon} alt="" />
                <h1>你好，{displayName}</h1>
                <p>把正在思考的事告诉我，我们一起理清。</p>
                <div className="prompt-suggestions">
                  <button type="button" onClick={() => handleDraftChange('帮我把今天最重要的三件事理清楚')}>
                    梳理今天的重点
                  </button>
                  <button type="button" onClick={() => handleDraftChange('帮我制定一个可执行的学习计划')}>
                    制定学习计划
                  </button>
                </div>
              </div>
            )}
            {messageState === 'ready' && messages.map((message) => (
              <article className={`message message--${message.role}`} key={message.id}>
                {message.role === 'assistant' && (
                  <img className="message__avatar" src={coveIcon} alt="Cove" />
                )}
                <div className="message__body">
                  {message.role === 'assistant' ? (
                    <AssistantMessageContent message={message} streaming={streamState.status === 'streaming'} />
                  ) : (
                    <p>{message.content}</p>
                  )}
                </div>
              </article>
            ))}
            {streamState.status === 'error' && (
              <div className="stream-error" role="alert">
                <WarningCircle size={19} />
                <span>{streamState.message}</span>
                <button type="button" onClick={() => void sendMessage(streamState.submission)}>
                  重新发送
                </button>
              </div>
            )}
            <div />
          </div>
        </div>

        <footer className="composer-area">
          <form
            className="composer"
            onSubmit={handleSubmit}
            onTouchStartCapture={handleComposerSurfacePress}
            onPointerDownCapture={handleComposerSurfacePress}
          >
            <textarea
              ref={textareaRef}
              rows={1}
              value={draft}
              placeholder="问问 Cove..."
              aria-label="发送给 Cove 的消息"
              autoComplete="off"
              autoCorrect="off"
              autoCapitalize="sentences"
              spellCheck={false}
              enterKeyHint="send"
              disabled={streamState.status === 'streaming'}
              onTouchStart={(event) => {
                if (document.activeElement !== event.currentTarget) {
                  event.preventDefault()
                  focusComposerWithoutScroll(event.currentTarget)
                }
              }}
              onPointerDown={(event) => {
                if (document.activeElement !== event.currentTarget) {
                  event.preventDefault()
                  focusComposerWithoutScroll(event.currentTarget)
                }
              }}
              onChange={(event) => handleDraftChange(event.target.value)}
              onKeyDown={handleComposerKeyDown}
            />
            <input
              className="composer__file-input"
              ref={fileInputRef}
              type="file"
              multiple
              accept="text/*,.md,.markdown,.csv,.json,.log,.yaml,.yml,.xml,.html,.css,.js,.jsx,.ts,.tsx,.py,.go,.rs,.java,.c,.cpp,.h,.sh,.sql"
              tabIndex={-1}
              aria-hidden="true"
              onChange={(event) => void handleAttachmentSelection(event.target.files)}
            />
            {attachments.length > 0 && (
              <div className="composer-attachments" aria-label="已添加附件">
                {attachments.map((attachment) => (
                  <span className="composer-attachment" key={attachment.file_name}>
                    <Paperclip size={13} />
                    <span>{attachment.file_name}</span>
                    <button
                      type="button"
                      aria-label={`移除附件 ${attachment.file_name}`}
                      disabled={streamState.status === 'streaming'}
                      onClick={() => setAttachments((current) => current.filter((item) => item.file_name !== attachment.file_name))}
                    >
                      <X size={13} weight="bold" />
                    </button>
                  </span>
                ))}
              </div>
            )}
            {(attachmentError || knowledgeError) && (
              <p className="composer__error" role="alert">{attachmentError || knowledgeError}</p>
            )}
            <div className="composer__toolbar">
              <div>
                <button
                  className="composer-tool"
                  type="button"
                  aria-label="添加文本附件"
                  title="添加文本附件"
                  disabled={streamState.status === 'streaming' || attachments.length >= maxAttachmentCount}
                  onClick={() => fileInputRef.current?.click()}
                >
                  <Paperclip size={18} />
                </button>
                <button
                  className={knowledgeEnabled ? 'composer-tool composer-tool--active' : 'composer-tool'}
                  type="button"
                  aria-label={knowledgeState === 'error' ? '重新加载知识库配置' : '使用知识库'}
                  aria-pressed={knowledgeEnabled}
                  title={knowledgeState === 'error' ? '配置加载失败，点击重试' : knowledgeEnabled ? '知识库已开启' : '知识库已关闭'}
                  disabled={streamState.status === 'streaming' || knowledgeState === 'loading'}
                  onClick={() => {
                    if (knowledgeState === 'error') {
                      void loadKnowledgeConfig()
                    } else {
                      setKnowledgeEnabled((enabled) => !enabled)
                    }
                  }}
                >
                  <Books size={18} />
                </button>
                <button className="composer-tool" type="button" disabled aria-label="联网搜索，服务端暂未接入" title="联网搜索服务端暂未接入">
                  <GlobeHemisphereWest size={18} />
                </button>
              </div>
              <button className="send-button" type="submit" aria-label="发送消息" disabled={!draft.trim() || streamState.status === 'streaming'}>
                <ArrowUp size={19} weight="bold" />
              </button>
            </div>
          </form>
          <p>Cove 可能会出错，请核对重要信息。</p>
        </footer>
      </section>
    </main>
  )
}
