// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  authenticatedCommand: vi.fn(),
  authenticatedRequest: vi.fn(),
  refreshSession: vi.fn(),
}))

vi.mock('../auth/api', () => ({
  ApiError: class ApiError extends Error {
    status: number
    constructor(status: number, message: string) {
      super(message)
      this.status = status
    }
  },
  authenticatedRequest: mocks.authenticatedRequest,
  authenticatedCommand: mocks.authenticatedCommand,
  refreshSession: mocks.refreshSession,
}))

import {
  consumeChatStream,
  deleteConversation,
  getAgentConfig,
  listConversations,
  listMessages,
  renameConversation,
  streamChat,
} from './api'
import type { ChatStreamEvent } from './types'

function chunkedStream(chunks: string[]): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder()
  return new ReadableStream({
    start(controller) {
      for (const chunk of chunks) {
        controller.enqueue(encoder.encode(chunk))
      }
      controller.close()
    },
  })
}

beforeEach(() => {
  vi.restoreAllMocks()
  mocks.authenticatedRequest.mockReset()
  mocks.authenticatedCommand.mockReset()
  mocks.refreshSession.mockReset()
  window.localStorage.clear()
})

describe('chat API', () => {
  it('passes conversation and message requests through authenticatedRequest', async () => {
    const conversations = {
      list: [{ id: 'conversation-1' }],
      total: 41,
      page: 2,
      page_size: 20,
    }
    mocks.authenticatedRequest.mockResolvedValueOnce(conversations)
    await expect(listConversations(2, 20)).resolves.toEqual(conversations)
    expect(mocks.authenticatedRequest).toHaveBeenNthCalledWith(
      1,
      '/api/conversation/?page=2&page_size=20',
    )

    const messages = { list: [{ id: 'message-1' }], has_more: true }
    mocks.authenticatedRequest.mockResolvedValueOnce(messages)
    await expect(
      listMessages('conversation / 1', { limit: 30, before: 'message-before' }),
    ).resolves.toEqual(messages)
    expect(mocks.authenticatedRequest).toHaveBeenNthCalledWith(
      2,
      '/api/conversation/conversation%20%2F%201/messages?limit=30&before=message-before',
    )
  })

  it('connects agent config and conversation mutation endpoints', async () => {
    mocks.authenticatedRequest
      .mockResolvedValueOnce({ enable_knowledge: true })
      .mockResolvedValueOnce({ id: 'conversation-1', title: '新名称' })
    mocks.authenticatedCommand.mockResolvedValueOnce(undefined)

    await expect(getAgentConfig()).resolves.toEqual({ enable_knowledge: true })
    expect(mocks.authenticatedRequest).toHaveBeenNthCalledWith(1, '/api/agent-config')

    await renameConversation('conversation / 1', '新名称')
    expect(mocks.authenticatedRequest).toHaveBeenNthCalledWith(
      2,
      '/api/conversation/conversation%20%2F%201',
      { method: 'PATCH', body: JSON.stringify({ title: '新名称' }) },
    )

    await deleteConversation('conversation / 1')
    expect(mocks.authenticatedCommand).toHaveBeenCalledWith(
      '/api/conversation/conversation%20%2F%201',
      { method: 'DELETE' },
    )
  })

  it('parses split SSE frames, multiline data, comments, and error events', async () => {
    const events: ChatStreamEvent[] = []
    const stream = chunkedStream([
      ': ping\n\n',
      'event: meta\ndata: {"type":"meta","conversation_id":"c1",',
      '"title":"新对话"}\n\n',
      'event: token\ndata: {"type":"token",\ndata: "text":"你',
      '好"}\n\n',
      'event: think\ndata: {"type":"think","status":"think',
      'ing","iteration":2}\n\n',
      'event: error\ndata: {"type":"error","content":"失败"}\n\n',
    ])

    await consumeChatStream(stream, (event) => events.push(event))

    expect(events).toEqual([
      { type: 'meta', conversation_id: 'c1', title: '新对话' },
      { type: 'token', text: '你好' },
      { type: 'think', status: 'thinking', iteration: 2 },
      { type: 'error', content: '失败' },
    ])
  })

  it('refreshes once after an initial 401 and streams the retried response', async () => {
    window.localStorage.setItem(
      'cove.auth.session.v1',
      JSON.stringify({ accessToken: 'old', refreshToken: 'refresh', user: { id: 'u1' } }),
    )
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(new Response('', { status: 401 }))
      .mockImplementationOnce(() => {
        return Promise.resolve(
          new Response(chunkedStream(['event: done\ndata: {"type":"done","text":"m1"}\n\n']), {
            status: 200,
            headers: { 'Content-Type': 'text/event-stream' },
          }),
        )
      })
    vi.stubGlobal('fetch', fetchMock)
    mocks.refreshSession.mockImplementation(async () => {
      window.localStorage.setItem(
        'cove.auth.session.v1',
        JSON.stringify({ accessToken: 'new', refreshToken: 'refresh-2', user: { id: 'u1' } }),
      )
      return { accessToken: 'new' }
    })
    const events: ChatStreamEvent[] = []

    await streamChat({ message: '你好' }, new AbortController().signal, (event) => events.push(event))

    expect(mocks.refreshSession).toHaveBeenCalledTimes(1)
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(new Headers(fetchMock.mock.calls[1][1]?.headers).get('Authorization')).toBe('Bearer new')
    expect(events).toEqual([{ type: 'done', text: 'm1' }])
  })

  it('sends text attachments and the explicit knowledge setting', async () => {
    window.localStorage.setItem(
      'cove.auth.session.v1',
      JSON.stringify({ accessToken: 'token', refreshToken: 'refresh', user: { id: 'u1' } }),
    )
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(chunkedStream(['event: done\ndata: {"type":"done","text":"m1"}\n\n']), {
        status: 200,
      }),
    )
    vi.stubGlobal('fetch', fetchMock)

    await streamChat(
      {
        message: '总结附件',
        attachments: [{ file_name: 'notes.md', text: '# Notes' }],
        enable_knowledge: true,
      },
      new AbortController().signal,
      vi.fn(),
    )

    expect(JSON.parse(String(fetchMock.mock.calls[0][1]?.body))).toEqual({
      message: '总结附件',
      attachments: [{ file_name: 'notes.md', text: '# Notes' }],
      enable_knowledge: true,
    })
  })

  it('propagates aborts without converting them to a network error', async () => {
    window.localStorage.setItem(
      'cove.auth.session.v1',
      JSON.stringify({ accessToken: 'token', refreshToken: 'refresh', user: { id: 'u1' } }),
    )
    const controller = new AbortController()
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation((_url: string, init: RequestInit) => {
        controller.abort()
        return Promise.reject(init.signal?.reason ?? new DOMException('Aborted', 'AbortError'))
      }),
    )

    await expect(streamChat({ message: '你好' }, controller.signal, vi.fn())).rejects.toMatchObject({
      name: 'AbortError',
    })
  })
})
