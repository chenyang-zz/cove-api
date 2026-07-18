export type ApiFieldError = {
  field: string;
  tag: string;
  param: string;
  message: string;
};

export type ApiEnvelope<T> = {
  code: number;
  message: string;
  data?: T;
  errors?: ApiFieldError[];
};

export type AuthResponse = {
  user_id: string;
  username: string;
  email?: string | null;
  access_token: string;
  refresh_token: string;
};

export type UserResponse = {
  id: string;
  username: string;
  nickname: string | null;
  email: string | null;
  avatar: string | null;
  created_at: string;
  updated_at?: string;
};

export type SessionUser = Pick<
  UserResponse,
  'id' | 'username' | 'nickname' | 'email' | 'avatar'
>;

export type StoredSession = {
  accessToken: string;
  refreshToken: string;
  user: SessionUser;
};

export type LoginInput = {
  login: string;
  password: string;
};

export type RegisterInput = {
  username: string;
  email?: string;
  password: string;
};

export type ProfileUpdateInput = {
  nickname: string;
  email: string;
};

export type PasswordChangeInput = {
  old_password: string;
  new_password: string;
};

export type ChatStreamRequest = {
  conversation_id?: string;
  message: string;
  enable_knowledge?: boolean;
};

export type MessagePart = {
  type: 'text' | 'tool_call' | 'tool_result' | string;
  text: string | null;
  tool: string | null;
  input: Record<string, unknown> | null;
  observation: string | null;
  error: string | null;
  iteration: number | null;
  tool_call_id: string | null;
};

export type MessageMetadata = {
  image_keys?: string[] | null;
  sender_name?: string | null;
  parts?: MessagePart[] | null;
  interrupted?: boolean;
};

export type ChatThinkingState = {
  active: boolean;
  iteration: number;
};

export type ChatToolEvent = {
  type: 'tool_call' | 'tool_result';
  tool: string;
  input?: Record<string, unknown>;
  observation?: string;
  error?: string;
  iteration: number;
  tool_call_id: string;
};

export type ChatThinkEvent = {
  type: 'think';
  status: 'thinking' | 'done';
  iteration: number;
};

export type ChatStreamEvent =
  | { type: 'meta'; conversation_id: string; title: string }
  | { type: 'token' | 'done'; text: string }
  | { type: 'error'; content: string }
  | ChatToolEvent
  | ChatThinkEvent
  | { type: string; [key: string]: unknown };

export type ConversationResponse = {
  id: string;
  title: string;
  is_group: boolean;
  member_persona_ids: string[];
  enable_tools: boolean;
  created_at: string;
  updated_at: string;
};

export type ConversationListResponse = {
  list: ConversationResponse[];
  total: number;
  page: number;
  page_size: number;
};

export type ChatMessageResponse = {
  id: string;
  role: 'user' | 'assistant' | 'system' | string;
  content: string;
  meta_data: MessageMetadata | null;
  images: string[];
  sender_persona_id: string | null;
  feedback: string | null;
  created_at: string;
};

export type ChatMessage = ChatMessageResponse & {
  pending?: boolean;
  parts?: MessagePart[];
  thinking?: ChatThinkingState;
};

export type MessageListResponse = {
  list: ChatMessageResponse[];
  has_more: boolean;
};
