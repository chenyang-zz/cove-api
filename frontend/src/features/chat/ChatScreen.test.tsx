// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { StoredSession } from '../auth/types'
import type { ChatStreamEvent } from './types'

const mocks = vi.hoisted(() => ({
  deleteConversation: vi.fn(),
  getAgentConfig: vi.fn(),
  listConversations: vi.fn(),
  listMessages: vi.fn(),
  renameConversation: vi.fn(),
  streamChat: vi.fn(),
}))

vi.mock('./api', () => ({
  deleteConversation: mocks.deleteConversation,
  getAgentConfig: mocks.getAgentConfig,
  listConversations: mocks.listConversations,
  listMessages: mocks.listMessages,
  renameConversation: mocks.renameConversation,
  streamChat: mocks.streamChat,
}))

import { ChatScreen } from './ChatScreen'

const session: StoredSession = {
  accessToken: 'access',
  refreshToken: 'refresh',
  user: {
    id: 'user-1',
    username: 'linhai',
    nickname: '林海',
    email: 'linhai@example.com',
    avatar: null,
  },
}

function conversation(id: string, title: string, updatedAt = '2026-07-11T08:00:00Z') {
  return {
    id,
    title,
    is_group: false,
    member_persona_ids: [],
    enable_tools: false,
    created_at: '2026-07-10T08:00:00Z',
    updated_at: updatedAt,
  }
}

function message(id: string, content: string, role = 'assistant') {
  return {
    id,
    role,
    content,
    meta_data: null,
    images: [],
    sender_persona_id: null,
    sender_name: null,
    feedback: null,
    created_at: '2026-07-11T08:01:00Z',
  }
}

beforeEach(() => {
  vi.restoreAllMocks()
  sessionStorage.clear()
  mocks.listConversations.mockReset().mockResolvedValue({ list: [], total: 0, page: 1, page_size: 20 })
  mocks.listMessages.mockReset().mockResolvedValue({ list: [], has_more: false })
  mocks.getAgentConfig.mockReset().mockResolvedValue({ enable_knowledge: false })
  mocks.renameConversation.mockReset()
  mocks.deleteConversation.mockReset()
  mocks.streamChat.mockReset()
  Element.prototype.scrollIntoView = vi.fn()
  Element.prototype.scrollTo = vi.fn()
  window.scrollTo = vi.fn()
})

afterEach(() => {
  cleanup()
})

describe('ChatScreen', () => {
  it('tracks the visual viewport so the keyboard cannot scroll the whole app away', async () => {
    const listeners = new Map<string, EventListener>()
    const viewport = {
      height: 844,
      width: 390,
      offsetTop: 0,
      addEventListener: vi.fn((type: string, listener: EventListener) => listeners.set(type, listener)),
      removeEventListener: vi.fn((type: string) => listeners.delete(type)),
    }
    vi.stubGlobal('visualViewport', viewport)

    const { container, unmount } = render(<ChatScreen session={session} onLogout={vi.fn()} />)
    const app = container.querySelector<HTMLElement>('.chat-app')
    expect(container.querySelector('.message-scroll')?.classList.contains('message-scroll--empty')).toBe(true)
    expect(app?.dataset.keyboardOpen).toBe('false')
    expect(app?.style.getPropertyValue('--chat-keyboard-height')).toBe('0px')
    expect(app?.style.getPropertyValue('--chat-content-shift')).toBe('0px')

    viewport.height = 516
    viewport.offsetTop = 286
    act(() => listeners.get('resize')?.(new Event('resize')))
    expect(app?.dataset.keyboardOpen).toBe('true')
    expect(app?.style.getPropertyValue('--chat-keyboard-height')).toBe('328px')
    expect(app?.style.getPropertyValue('--chat-content-shift')).toBe('148px')

    viewport.height = 844
    act(() => listeners.get('resize')?.(new Event('resize')))
    expect(app?.dataset.keyboardOpen).toBe('false')
    expect(app?.style.getPropertyValue('--chat-content-shift')).toBe('0px')
    await waitFor(() => {
      expect(Element.prototype.scrollTo).toHaveBeenCalledWith({ top: 0 })
    })

    unmount()
    expect(document.documentElement.classList.contains('chat-document')).toBe(false)
    expect(viewport.removeEventListener).toHaveBeenCalledWith('resize', expect.any(Function))
    vi.unstubAllGlobals()
  })

  it('shows the personalized empty state and toggles the mobile drawer', async () => {
    const user = userEvent.setup()
    render(<ChatScreen session={session} onLogout={vi.fn()} />)

    expect(await screen.findByRole('heading', { name: '你好，林海' })).toBeTruthy()
    const drawer = screen.getByRole('complementary')
    expect(drawer.classList.contains('chat-drawer--open')).toBe(false)

    await user.click(screen.getByRole('button', { name: '打开会话列表' }))
    expect(drawer.classList.contains('chat-drawer--open')).toBe(true)
    await user.click(screen.getAllByRole('button', { name: '关闭会话列表' })[1])
    expect(drawer.classList.contains('chat-drawer--open')).toBe(false)
  })

  it('keeps the recent-conversations heading fixed and staggers loaded rows', async () => {
    mocks.listConversations.mockResolvedValue({
      list: [
        conversation('conversation-1', '第一条会话'),
        conversation('conversation-2', '第二条会话'),
      ],
      total: 2,
      page: 1,
      page_size: 20,
    })

    render(<ChatScreen session={session} onLogout={vi.fn()} />)

    await screen.findAllByText('第一条会话')
    const history = document.querySelector('.conversation-history') as HTMLElement
    const list = document.querySelector('.conversation-list') as HTMLElement
    const label = screen.getByText('最近对话')
    const rows = Array.from(document.querySelectorAll<HTMLElement>('.conversation-row'))

    expect(history.contains(label)).toBe(true)
    expect(list.contains(label)).toBe(false)
    expect(rows).toHaveLength(2)
    expect(rows[0].style.getPropertyValue('--conversation-index')).toBe('0')
    expect(rows[1].style.getPropertyValue('--conversation-index')).toBe('1')
  })

  it('uses a neutral user name when nickname is empty', async () => {
    render(
      <ChatScreen
        session={{ ...session, user: { ...session.user, nickname: null } }}
        onLogout={vi.fn()}
      />,
    )

    expect(await screen.findByRole('heading', { name: '你好，用户' })).toBeTruthy()
    expect(screen.queryByRole('heading', { name: '你好，linhai' })).toBeNull()
  })

  it('keeps account identity and logout in the drawer while the header action is disabled', async () => {
    const user = userEvent.setup()
    const onLogout = vi.fn()
    render(<ChatScreen session={session} onLogout={onLogout} />)
    await screen.findByRole('heading', { name: '你好，林海' })

    expect(screen.getByText('@linhai')).toBeTruthy()
    const placeholder = screen.getByRole('button', { name: '更多功能，暂不可用' })
    expect((placeholder as HTMLButtonElement).disabled).toBe(true)
    expect(screen.queryByRole('menu')).toBeNull()
    await user.click(screen.getByRole('button', { name: '退出登录' }))
    expect(onLogout).toHaveBeenCalledTimes(1)
  })

  it('focuses the composer without allowing WKWebView to scroll the page', async () => {
    const user = userEvent.setup()
    render(<ChatScreen session={session} onLogout={vi.fn()} />)
    const composer = await screen.findByRole('textbox', { name: '发送给 Cove 的消息' })
    const focus = vi.spyOn(composer, 'focus')

    await user.pointer({ target: composer, keys: '[MouseLeft]' })

    expect(focus).toHaveBeenCalledWith({ preventScroll: true })
  })

  it('protects form-edge taps when the textarea remains focused after the keyboard closes', async () => {
    render(<ChatScreen session={session} onLogout={vi.fn()} />)
    const textarea = await screen.findByRole('textbox', { name: '发送给 Cove 的消息' })
    const form = textarea.closest('form')
    expect(form).toBeTruthy()

    textarea.focus()
    const focus = vi.spyOn(textarea, 'focus')
    fireEvent.pointerDown(form as HTMLFormElement)

    expect(focus).toHaveBeenCalledWith({ preventScroll: true })
  })

  it('loads the latest conversation and its message history', async () => {
    mocks.listConversations.mockResolvedValue({
      list: [
        {
          id: 'conversation-1',
          title: '周末安排',
          is_group: false,
          member_persona_ids: [],
          enable_tools: false,
          created_at: '2026-07-10T08:00:00Z',
          updated_at: '2026-07-11T08:00:00Z',
        },
      ],
      total: 1,
      page: 1,
      page_size: 20,
    })
    mocks.listMessages.mockResolvedValue({
      list: [
        {
          id: 'message-1',
          role: 'assistant',
          content: '我们可以先安排上午。',
          meta_data: null,
          images: [],
          sender_persona_id: null,
          sender_name: null,
          feedback: null,
          created_at: '2026-07-11T08:01:00Z',
        },
      ],
      has_more: false,
    })

    render(<ChatScreen session={session} onLogout={vi.fn()} />)

    expect(await screen.findByText('我们可以先安排上午。')).toBeTruthy()
    expect(document.querySelector('.message-scroll')?.classList.contains('message-scroll--empty')).toBe(false)
    expect(mocks.listConversations).toHaveBeenCalledWith(1, 20)
    expect(mocks.listMessages).toHaveBeenCalledWith('conversation-1', { limit: 30 })
    expect(screen.getAllByText('周末安排')).toHaveLength(2)
  })

  it('shows the target conversation loading immediately and finishes after scrolling to its last message', async () => {
    const user = userEvent.setup()
    let resolveSecondHistory: ((value: { list: ReturnType<typeof message>[]; has_more: boolean }) => void) | undefined
    mocks.listConversations.mockResolvedValue({
      list: [
        conversation('conversation-1', '当前会话'),
        conversation('conversation-2', '目标会话', '2026-07-10T07:00:00Z'),
      ],
      total: 2,
      page: 1,
      page_size: 20,
    })
    mocks.listMessages.mockImplementation((conversationId: string) => {
      if (conversationId === 'conversation-1') {
        return Promise.resolve({ list: [message('message-1', '旧会话内容')], has_more: false })
      }
      return new Promise((resolve) => {
        resolveSecondHistory = resolve
      })
    })

    render(<ChatScreen session={session} onLogout={vi.fn()} />)
    await screen.findByText('旧会话内容')
    const scroll = document.querySelector('.message-scroll') as HTMLDivElement
    Object.defineProperties(scroll, {
      scrollHeight: {
        configurable: true,
        get: () => screen.queryByText('目标会话最后一条') ? 860 : 180,
      },
      clientHeight: { configurable: true, value: 280 },
    })
    vi.mocked(Element.prototype.scrollTo).mockClear()

    await user.click(screen.getByRole('button', { name: '打开会话列表' }))
    await user.click(screen.getByRole('button', { name: '目标会话7/10' }))

    expect(screen.getByLabelText('正在加载消息')).toBeTruthy()
    expect(screen.queryByText('旧会话内容')).toBeNull()
    expect(screen.getByText('目标会话', { selector: '.chat-header__title strong' })).toBeTruthy()

    act(() => resolveSecondHistory?.({
      list: [message('message-2', '目标会话最后一条')],
      has_more: false,
    }))

    expect(await screen.findByText('目标会话最后一条')).toBeTruthy()
    expect(Element.prototype.scrollTo).toHaveBeenCalledWith({ top: 580, behavior: 'auto' })
    expect(screen.queryByLabelText('正在加载消息')).toBeNull()
  })

  it('creates a conversation from meta and renders streamed markdown tokens', async () => {
    const user = userEvent.setup()
    mocks.streamChat.mockImplementation(
      async (
        _input: unknown,
        _signal: AbortSignal,
        onEvent: (event: ChatStreamEvent) => void,
      ) => {
        onEvent({ type: 'meta', conversation_id: 'conversation-2', title: '学习计划' })
        onEvent({ type: 'token', text: '**先确定目标**' })
        onEvent({ type: 'done', text: 'message-2' })
      },
    )
    render(<ChatScreen session={session} onLogout={vi.fn()} />)
    await screen.findByRole('heading', { name: '你好，林海' })

    const composer = screen.getByRole('textbox', { name: '发送给 Cove 的消息' })
    await user.type(composer, '帮我制定学习计划')
    await user.click(screen.getByRole('button', { name: '发送消息' }))

    expect(await screen.findByText('先确定目标')).toHaveProperty('tagName', 'STRONG')
    expect(mocks.streamChat).toHaveBeenCalledWith(
      { message: '帮我制定学习计划', enable_knowledge: false },
      expect.any(AbortSignal),
      expect.any(Function),
    )
    expect(screen.getAllByText('学习计划').length).toBeGreaterThan(0)
    expect(mocks.listMessages).not.toHaveBeenCalled()
  })

  it('blocks duplicate sends while a stream is pending and supports retry after failure', async () => {
    const user = userEvent.setup()
    let emit: ((event: ChatStreamEvent) => void) | undefined
    let finish: (() => void) | undefined
    mocks.streamChat
      .mockImplementationOnce(
        (_input: unknown, _signal: AbortSignal, onEvent: (event: ChatStreamEvent) => void) => {
          emit = onEvent
          return new Promise<void>((resolve) => {
            finish = resolve
          })
        },
      )
      .mockImplementationOnce(
        async (_input: unknown, _signal: AbortSignal, onEvent: (event: ChatStreamEvent) => void) => {
          onEvent({ type: 'done', text: 'message-retry' })
        },
      )

    render(<ChatScreen session={session} onLogout={vi.fn()} />)
    await screen.findByRole('heading', { name: '你好，林海' })
    const composer = screen.getByRole('textbox', { name: '发送给 Cove 的消息' })
    await user.type(composer, '请再试一次')
    await user.click(screen.getByRole('button', { name: '发送消息' }))

    expect((composer as HTMLTextAreaElement).disabled).toBe(true)
    expect(mocks.streamChat).toHaveBeenCalledTimes(1)
    act(() => {
      emit?.({ type: 'error', content: '服务暂时不可用' })
      finish?.()
    })
    const retry = await screen.findByRole('button', { name: '重新发送' })
    await user.click(retry)

    await waitFor(() => expect(mocks.streamChat).toHaveBeenCalledTimes(2))
  })

  it('attaches text files, toggles knowledge, removes attachments, and retries the full submission', async () => {
    const user = userEvent.setup()
    mocks.getAgentConfig.mockResolvedValue({ enable_knowledge: true })
    mocks.streamChat
      .mockImplementationOnce(
        async (_input: unknown, _signal: AbortSignal, onEvent: (event: ChatStreamEvent) => void) => {
          onEvent({ type: 'error', content: '暂时失败' })
        },
      )
      .mockImplementationOnce(
        async (_input: unknown, _signal: AbortSignal, onEvent: (event: ChatStreamEvent) => void) => {
          onEvent({ type: 'done', text: 'message-retry' })
        },
      )

    const { container } = render(<ChatScreen session={session} onLogout={vi.fn()} />)
    await screen.findByRole('heading', { name: '你好，林海' })
    const knowledge = await screen.findByRole('button', { name: '使用知识库' })
    await waitFor(() => expect(knowledge.getAttribute('aria-pressed')).toBe('true'))
    await user.click(knowledge)

    const input = container.querySelector<HTMLInputElement>('input[type="file"]')
    const firstFile = new File(['first'], 'first.md', { type: 'text/markdown' })
    Object.defineProperty(firstFile, 'text', { value: vi.fn().mockResolvedValue('first') })
    fireEvent.change(input as HTMLInputElement, { target: { files: [firstFile] } })
    expect(await screen.findByText('first.md')).toBeTruthy()
    await user.click(screen.getByRole('button', { name: '移除附件 first.md' }))
    expect(screen.queryByText('first.md')).toBeNull()

    const notesFile = new File(['notes'], 'notes.md', { type: 'text/markdown' })
    Object.defineProperty(notesFile, 'text', { value: vi.fn().mockResolvedValue('# Notes') })
    fireEvent.change(input as HTMLInputElement, { target: { files: [notesFile] } })
    expect(await screen.findByText('notes.md')).toBeTruthy()

    await user.type(screen.getByRole('textbox', { name: '发送给 Cove 的消息' }), '总结附件')
    await user.click(screen.getByRole('button', { name: '发送消息' }))
    await user.click(await screen.findByRole('button', { name: '重新发送' }))

    const expectedInput = {
      message: '总结附件',
      attachments: [{ file_name: 'notes.md', text: '# Notes' }],
      enable_knowledge: false,
    }
    expect(mocks.streamChat).toHaveBeenNthCalledWith(
      1,
      expectedInput,
      expect.any(AbortSignal),
      expect.any(Function),
    )
    expect(mocks.streamChat).toHaveBeenNthCalledWith(
      2,
      expectedInput,
      expect.any(AbortSignal),
      expect.any(Function),
    )
  })

  it('enforces attachment type, size, and count limits', async () => {
    const { container } = render(<ChatScreen session={session} onLogout={vi.fn()} />)
    await screen.findByRole('heading', { name: '你好，林海' })
    const input = container.querySelector<HTMLInputElement>('input[type="file"]')
    const files = ['one.md', 'two.md', 'three.md', 'four.md'].map((name) => {
      const file = new File(['text'], name, { type: 'text/markdown' })
      Object.defineProperty(file, 'text', { value: vi.fn().mockResolvedValue(name) })
      return file
    })

    fireEvent.change(input as HTMLInputElement, { target: { files } })

    expect((await screen.findByRole('alert')).textContent).toContain('最多添加 3 个附件。')
    expect(screen.getByText('one.md')).toBeTruthy()
    expect(screen.getByText('two.md')).toBeTruthy()
    expect(screen.getByText('three.md')).toBeTruthy()
    expect(screen.queryByText('four.md')).toBeNull()
  })

  it('loads more conversations at the drawer bottom and deduplicates page overlap', async () => {
    mocks.listConversations
      .mockReset()
      .mockResolvedValueOnce({
        list: [conversation('conversation-1', '第一页')],
        total: 2,
        page: 1,
        page_size: 20,
      })
      .mockResolvedValueOnce({
        list: [
          conversation('conversation-1', '第一页'),
          conversation('conversation-2', '第二页', '2026-07-10T07:00:00Z'),
        ],
        total: 2,
        page: 2,
        page_size: 20,
      })

    render(<ChatScreen session={session} onLogout={vi.fn()} />)
    await screen.findAllByText('第一页')
    const list = document.querySelector('.conversation-list') as HTMLDivElement
    Object.defineProperties(list, {
      scrollHeight: { configurable: true, value: 800 },
      clientHeight: { configurable: true, value: 300 },
      scrollTop: { configurable: true, writable: true, value: 500 },
    })

    fireEvent.scroll(list)

    expect(await screen.findByText('第二页')).toBeTruthy()
    expect(mocks.listConversations).toHaveBeenNthCalledWith(2, 2, 20)
    expect(screen.getAllByText('第一页')).toHaveLength(2)
  })

  it('prepends older messages at the top while preserving the visible scroll anchor', async () => {
    let resolveOlder: ((value: { list: ReturnType<typeof message>[]; has_more: boolean }) => void) | undefined
    mocks.listConversations.mockResolvedValue({
      list: [conversation('conversation-1', '历史记录')],
      total: 1,
      page: 1,
      page_size: 20,
    })
    mocks.listMessages
      .mockReset()
      .mockResolvedValueOnce({
        list: [message('message-2', '第二条'), message('message-3', '第三条')],
        has_more: true,
      })
      .mockImplementationOnce(
        () => new Promise((resolve) => {
          resolveOlder = resolve
        }),
      )

    render(<ChatScreen session={session} onLogout={vi.fn()} />)
    await screen.findByText('第三条')
    const scroll = document.querySelector('.message-scroll') as HTMLDivElement
    let scrollHeight = 600
    Object.defineProperties(scroll, {
      scrollHeight: { configurable: true, get: () => scrollHeight },
      clientHeight: { configurable: true, value: 240 },
      scrollTop: { configurable: true, writable: true, value: 20 },
    })

    fireEvent.scroll(scroll)
    await waitFor(() => {
      expect(mocks.listMessages).toHaveBeenNthCalledWith(2, 'conversation-1', {
        limit: 30,
        before: 'message-2',
      })
    })
    scrollHeight = 900
    act(() => resolveOlder?.({
      list: [message('message-1', '第一条'), message('message-2', '第二条')],
      has_more: false,
    }))

    expect(await screen.findByText('第一条')).toBeTruthy()
    expect(screen.getAllByText('第二条')).toHaveLength(1)
    expect(scroll.scrollTop).toBe(320)
  })

  it('renders ordered historical message parts and expandable tool details', async () => {
    const onLogout = vi.fn()
    mocks.listConversations.mockResolvedValue({
      list: [conversation('conversation-1', '事件记录')],
      total: 1,
      page: 1,
      page_size: 20,
    })
    mocks.listMessages.mockResolvedValue({
      list: [
        {
          ...message('message-1', '不应重复显示'),
          meta_data: {
            image_keys: [],
            sender_name: null,
            interrupted: true,
            parts: [
              { type: 'text', text: '**先查询**', tool: null, input: null, observation: null, error: null, iteration: null, tool_call_id: null },
              { type: 'tool_call', text: null, tool: 'current_time', input: { zone: 'Asia/Shanghai' }, observation: null, error: null, iteration: 1, tool_call_id: 'call-1' },
              { type: 'tool_result', text: null, tool: 'current_time', input: null, observation: '22:30', error: null, iteration: 1, tool_call_id: 'call-1' },
              { type: 'text', text: '查询完成。', tool: null, input: null, observation: null, error: null, iteration: null, tool_call_id: null },
            ],
          },
        },
      ],
      has_more: false,
    })

    render(<ChatScreen session={session} onLogout={onLogout} />)

    expect(await screen.findByText('先查询')).toHaveProperty('tagName', 'STRONG')
    expect(screen.getByText('查询完成。')).toBeTruthy()
    expect(screen.queryByText('不应重复显示')).toBeNull()
    const summary = screen.getByText('已使用 current_time').closest('summary')
    expect(summary).toBeTruthy()
    expect((summary?.parentElement as HTMLDetailsElement).open).toBe(false)
    fireEvent.click(summary as HTMLElement)
    expect((summary?.parentElement as HTMLDetailsElement).open).toBe(true)
    expect(screen.getByText('22:30')).toBeTruthy()
    expect(screen.getByText('回复已中断')).toBeTruthy()
  })

  it('falls back to Markdown content when persisted parts have no visible text', async () => {
    mocks.listConversations.mockResolvedValue({
      list: [conversation('conversation-1', 'Markdown 回退')],
      total: 1,
      page: 1,
      page_size: 20,
    })
    mocks.listMessages.mockResolvedValue({
      list: [
        {
          ...message('message-1', '**实际 Markdown 内容**'),
          pending: true,
          meta_data: {
            image_keys: [],
            sender_name: null,
            interrupted: false,
            parts: [
              { type: 'text', text: '   ', tool: null, input: null, observation: null, error: null, iteration: null, tool_call_id: null },
            ],
          },
        },
      ],
      has_more: false,
    })

    render(<ChatScreen session={session} onLogout={vi.fn()} />)

    expect(await screen.findByText('实际 Markdown 内容')).toHaveProperty('tagName', 'STRONG')
    expect(screen.queryByLabelText('Cove 正在思考')).toBeNull()
  })

  it('replaces unsupported Markdown emoji without changing code content', async () => {
    mocks.listConversations.mockResolvedValue({
      list: [conversation('conversation-1', 'Emoji 回退')],
      total: 1,
      page: 1,
      page_size: 20,
    })
    mocks.listMessages.mockResolvedValue({
      list: [message(
        'message-1',
        '地图信息。❓\n\n1. 🌍 **查询地点**\n\n😊 完成\n\n`const icon = "🌍"`',
      )],
      has_more: false,
    })

    render(<ChatScreen session={session} onLogout={vi.fn()} />)

    expect(await screen.findByText('地图信息。?')).toBeTruthy()
    expect(screen.getByText('查询地点')).toHaveProperty('tagName', 'STRONG')
    expect(screen.getByText('const icon = "🌍"')).toHaveProperty('tagName', 'CODE')
    const log = screen.getByRole('log')
    expect(log.textContent).not.toContain('😊')
    expect(log.textContent).not.toContain('1. 🌍')
  })

  it('wraps GFM tables in a horizontally scrollable region', async () => {
    mocks.listConversations.mockResolvedValue({
      list: [conversation('conversation-1', '天气表格')],
      total: 1,
      page: 1,
      page_size: 20,
    })
    mocks.listMessages.mockResolvedValue({
      list: [message(
        'message-1',
        '| 日期 | 白天天气 | 夜间天气 | 温度 | 风向 |\n| --- | --- | --- | --- | --- |\n| 7月12日 | 雷阵雨 | 雷阵雨 | 32°C | 东风 |',
      )],
      has_more: false,
    })

    render(<ChatScreen session={session} onLogout={vi.fn()} />)

    const region = await screen.findByRole('region', { name: '表格内容，可横向滚动' })
    expect(region.classList.contains('markdown-table-scroll')).toBe(true)
    expect(region.querySelector('table')).toBeTruthy()
    expect(screen.getByRole('columnheader', { name: '日期' })).toBeTruthy()
    expect(screen.getByRole('cell', { name: '7月12日' })).toBeTruthy()
  })

  it('keeps streamed text and tool events in their original timeline order', async () => {
    const user = userEvent.setup()
    mocks.streamChat.mockImplementation(
      async (_input: unknown, _signal: AbortSignal, onEvent: (event: ChatStreamEvent) => void) => {
        onEvent({ type: 'token', text: '查询前。' })
        onEvent({ type: 'tool_call', tool: 'search', input: { q: 'Cove' }, iteration: 1, tool_call_id: 'call-1' })
        onEvent({ type: 'tool_result', tool: 'search', observation: '找到结果', iteration: 1, tool_call_id: 'call-1' })
        onEvent({ type: 'token', text: '查询后。' })
        onEvent({ type: 'done', text: 'message-1' })
      },
    )
    render(<ChatScreen session={session} onLogout={vi.fn()} />)
    await screen.findByRole('heading', { name: '你好，林海' })
    await user.type(screen.getByRole('textbox', { name: '发送给 Cove 的消息' }), '查一下')
    await user.click(screen.getByRole('button', { name: '发送消息' }))

    const before = await screen.findByText('查询前。')
    const tool = screen.getByText('已使用 search')
    const after = screen.getByText('查询后。')
    expect(before.compareDocumentPosition(tool) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(tool.compareDocumentPosition(after) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('renames and deletes conversations through the row menu and closes it outside', async () => {
    const user = userEvent.setup()
    const conversation = {
      id: 'conversation-1',
      title: '旧名称',
      is_group: false,
      member_persona_ids: [],
      enable_tools: false,
      created_at: '2026-07-10T08:00:00Z',
      updated_at: '2026-07-11T08:00:00Z',
    }
    mocks.listConversations.mockResolvedValue({ list: [conversation], total: 1, page: 1, page_size: 20 })
    mocks.renameConversation.mockResolvedValue({ ...conversation, title: '新名称' })
    mocks.deleteConversation.mockResolvedValue(undefined)

    render(<ChatScreen session={session} onLogout={vi.fn()} />)
    await screen.findAllByText('旧名称')
    const manage = screen.getByRole('button', { name: '管理会话：旧名称' })
    const conversationList = document.querySelector('.conversation-list') as HTMLDivElement
    const drawer = document.querySelector('.chat-drawer') as HTMLElement
    vi.spyOn(manage, 'getBoundingClientRect').mockReturnValue({
      top: 700,
      bottom: 736,
      left: 264,
      right: 300,
      width: 36,
      height: 36,
      x: 264,
      y: 700,
      toJSON: () => ({}),
    })
    vi.spyOn(conversationList, 'getBoundingClientRect').mockReturnValue({
      top: 100,
      bottom: 760,
      left: 0,
      right: 310,
      width: 310,
      height: 660,
      x: 0,
      y: 100,
      toJSON: () => ({}),
    })
    vi.spyOn(drawer, 'getBoundingClientRect').mockReturnValue({
      top: 0,
      bottom: 844,
      left: 0,
      right: 310,
      width: 310,
      height: 844,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    })
    await user.click(manage)
    const bottomMenu = screen.getByRole('menu')
    expect(bottomMenu.parentElement).toBe(document.body)
    expect(bottomMenu.style.top).toBe('602px')
    expect(bottomMenu.style.left).toBe('164px')
    fireEvent.scroll(conversationList)
    expect(screen.queryByRole('menu')).toBeNull()

    await user.click(manage)
    await user.click(screen.getByRole('heading', { name: '你好，林海' }))
    expect(screen.queryByRole('menu')).toBeNull()

    await user.click(manage)
    await user.click(screen.getByRole('menuitem', { name: '重命名' }))
    const titleInput = screen.getByRole('textbox', { name: '会话名称' })
    await user.clear(titleInput)
    await user.type(titleInput, '新名称')
    await user.click(screen.getByRole('button', { name: '保存' }))
    await waitFor(() => expect(mocks.renameConversation).toHaveBeenCalledWith('conversation-1', '新名称'))

    await user.click(await screen.findByRole('button', { name: '管理会话：新名称' }))
    await user.click(screen.getByRole('menuitem', { name: '删除' }))
    await user.click(screen.getByRole('button', { name: '删除' }))

    await waitFor(() => expect(mocks.deleteConversation).toHaveBeenCalledWith('conversation-1'))
    expect(await screen.findByRole('heading', { name: '你好，林海' })).toBeTruthy()
    expect(screen.queryByText('新名称')).toBeNull()
  })
})
