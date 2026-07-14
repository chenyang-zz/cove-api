export type Conversation = {
  id: string
  title: string
  is_group: boolean
  member_persona_ids: string[]
  enable_tools: boolean
  created_at: string
  updated_at: string
}

export type AgentConfig = {
  enable_knowledge: boolean
}

export type ChatAttachment = {
  file_name: string
  text: string
}

export type MessageMetadata = {
  image_keys?: string[] | null
  sender_name?: string | null
  parts?: MessagePart[] | null
  interrupted?: boolean
}

export type MessagePart = {
  type: 'text' | 'tool_call' | 'tool_result' | string
  text: string | null
  tool: string | null
  input: Record<string, unknown> | null
  observation: string | null
  error: string | null
  iteration: number | null
  tool_call_id: string | null
}

export type ChatMessage = {
  id: string
  role: 'user' | 'assistant' | 'system' | string
  content: string
  meta_data: MessageMetadata | null
  images: string[]
  sender_persona_id: string | null
  sender_name: string | null
  feedback: string | null
  created_at: string
  pending?: boolean
  parts?: MessagePart[]
  thinking?: ChatThinkingState
}

export type ChatThinkingState = {
  active: boolean
  iteration: number
}

export type ChatStreamRequest = {
  conversation_id?: string
  message: string
  greeting?: string
  skill_id?: string
  image_keys?: string[]
  attachments?: ChatAttachment[]
  enable_knowledge?: boolean
  enable_memory?: boolean
  enable_web_search?: boolean
}

export type ChatMetaEvent = {
  type: 'meta'
  conversation_id: string
  title: string
}

export type ChatTextEvent = {
  type: 'token' | 'done'
  text: string
}

export type ChatErrorEvent = {
  type: 'error'
  content: string
}

export type ChatToolEvent = {
  type: 'tool_call' | 'tool_result'
  tool: string
  input?: Record<string, unknown>
  observation?: string
  error?: string
  iteration: number
  tool_call_id: string
}

export type ChatThinkEvent = {
  type: 'think'
  status: 'thinking' | 'done'
  iteration: number
}

export type ChatUnknownEvent = {
  type: string
  [key: string]: unknown
}

export type ChatStreamEvent =
  | ChatMetaEvent
  | ChatTextEvent
  | ChatErrorEvent
  | ChatToolEvent
  | ChatThinkEvent
  | ChatUnknownEvent

export type ResourceState = 'idle' | 'loading' | 'ready' | 'error'

export type ChatSubmission = {
  message: string
  attachments: ChatAttachment[]
  enableKnowledge: boolean
}

export type StreamState =
  | { status: 'idle' }
  | { status: 'streaming' }
  | { status: 'error'; message: string; submission: ChatSubmission }
