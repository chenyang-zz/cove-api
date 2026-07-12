import { ApiError, authenticatedCommand, authenticatedRequest, refreshSession } from '../auth/api'
import type { ApiEnvelope, StoredSession } from '../auth/types'
import type {
  AgentConfig,
  ChatMessage,
  ChatStreamEvent,
  ChatStreamRequest,
  Conversation,
} from './types'

const API_BASE_URL = (import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8000').replace(
  /\/+$/,
  '',
)
const SESSION_STORAGE_KEY = 'cove.auth.session.v1'

export type PageListResponse<T> = {
  list: T[]
  total: number
  page: number
  page_size: number
}

export type MessageListResponse = {
  list: ChatMessage[]
  has_more: boolean
}

function currentSession(): StoredSession | null {
  if (typeof window === 'undefined') {
    return null
  }
  const raw = window.localStorage.getItem(SESSION_STORAGE_KEY)
  if (!raw) {
    return null
  }
  try {
    return JSON.parse(raw) as StoredSession
  } catch {
    return null
  }
}

export function listConversations(
  page = 1,
  pageSize = 20,
): Promise<PageListResponse<Conversation>> {
  // The Gin route is registered as GET /api/conversation/. Requesting the
  // slashless form returns a CORS-less 301 that WKWebView rejects.
  const query = new URLSearchParams({ page: String(page), page_size: String(pageSize) })
  return authenticatedRequest<PageListResponse<Conversation>>(`/api/conversation/?${query}`)
}

export function listMessages(
  conversationId: string,
  options: { limit?: number; before?: string } = {},
): Promise<MessageListResponse> {
  const query = new URLSearchParams({ limit: String(options.limit ?? 30) })
  if (options.before) {
    query.set('before', options.before)
  }
  return authenticatedRequest<MessageListResponse>(
    `/api/conversation/${encodeURIComponent(conversationId)}/messages?${query}`,
  )
}

export function getAgentConfig(): Promise<AgentConfig> {
  return authenticatedRequest<AgentConfig>('/api/agent-config')
}

export function renameConversation(conversationId: string, title: string): Promise<Conversation> {
  return authenticatedRequest<Conversation>(
    `/api/conversation/${encodeURIComponent(conversationId)}`,
    { method: 'PATCH', body: JSON.stringify({ title }) },
  )
}

export function deleteConversation(conversationId: string): Promise<void> {
  return authenticatedCommand(`/api/conversation/${encodeURIComponent(conversationId)}`, {
    method: 'DELETE',
  })
}

function parseSseBlock(block: string): ChatStreamEvent | null {
  let eventName = ''
  const dataLines: string[] = []

  for (const line of block.split(/\r?\n/)) {
    if (!line || line.startsWith(':')) {
      continue
    }
    if (line.startsWith('event:')) {
      eventName = line.slice(6).trim()
    } else if (line.startsWith('data:')) {
      dataLines.push(line.slice(5).trimStart())
    }
  }

  if (!eventName || dataLines.length === 0) {
    return null
  }

  const parsed = JSON.parse(dataLines.join('\n')) as Record<string, unknown>
  return { ...parsed, type: typeof parsed.type === 'string' ? parsed.type : eventName } as ChatStreamEvent
}

export async function consumeChatStream(
  body: ReadableStream<Uint8Array>,
  onEvent: (event: ChatStreamEvent) => void,
): Promise<void> {
  const reader = body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  try {
    while (true) {
      const { value, done } = await reader.read()
      buffer += decoder.decode(value, { stream: !done })

      let boundary = buffer.match(/\r?\n\r?\n/)
      while (boundary?.index !== undefined) {
        const block = buffer.slice(0, boundary.index)
        buffer = buffer.slice(boundary.index + boundary[0].length)
        const event = parseSseBlock(block)
        if (event) {
          onEvent(event)
        }
        boundary = buffer.match(/\r?\n\r?\n/)
      }

      if (done) {
        const event = parseSseBlock(buffer)
        if (event) {
          onEvent(event)
        }
        break
      }
    }
  } finally {
    reader.releaseLock()
  }
}

async function streamResponse(
  input: ChatStreamRequest,
  signal: AbortSignal,
  retryAfterRefresh: boolean,
): Promise<Response> {
  const session = currentSession()
  if (!session) {
    throw new ApiError(401, '请先登录。')
  }

  let response: Response
  try {
    response = await fetch(`${API_BASE_URL}/api/chat/stream`, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${session.accessToken}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(input),
      signal,
    })
  } catch (error: unknown) {
    if (signal.aborted) {
      throw error
    }
    throw new ApiError(0, '无法连接到服务器，请检查网络后重试。')
  }

  if (response.status === 401 && retryAfterRefresh) {
    const refreshed = await refreshSession()
    if (!refreshed) {
      throw new ApiError(401, '登录状态已失效，请重新登录。')
    }
    return streamResponse(input, signal, false)
  }

  if (!response.ok) {
    let message = '请求失败，请稍后重试。'
    try {
      const envelope = (await response.json()) as ApiEnvelope<unknown>
      message = envelope.message || message
    } catch {
      // Keep the safe fallback when a proxy returns a non-JSON error page.
    }
    throw new ApiError(response.status, message)
  }

  if (!response.body) {
    throw new ApiError(response.status, '服务器未返回可读取的消息流。')
  }
  return response
}

export async function streamChat(
  input: ChatStreamRequest,
  signal: AbortSignal,
  onEvent: (event: ChatStreamEvent) => void,
): Promise<void> {
  const response = await streamResponse(input, signal, true)
  await consumeChatStream(response.body as ReadableStream<Uint8Array>, onEvent)
}
