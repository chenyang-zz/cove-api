import { fetch } from 'expo/fetch';

import { ApiError, authenticatedCommand, authenticatedRequest, refreshSession } from './api';
import { API_BASE_URL } from './config';
import { loadStoredSession } from './session';
import { consumeSseStream } from './sse';
import type {
  ApiEnvelope,
  ChatStreamEvent,
  ChatStreamRequest,
  ConversationListResponse,
  ConversationResponse,
  MessageListResponse,
} from './types';

export function listConversations(page = 1, pageSize = 20): Promise<ConversationListResponse> {
  const query = new URLSearchParams({ page: String(page), page_size: String(pageSize) });
  return authenticatedRequest<ConversationListResponse>(`/api/conversation/?${query}`);
}

export function listMessages(conversationId: string, limit = 30): Promise<MessageListResponse> {
  const query = new URLSearchParams({ limit: String(limit) });
  return authenticatedRequest<MessageListResponse>(
    `/api/conversation/${encodeURIComponent(conversationId)}/messages?${query}`,
  );
}

export function renameConversation(
  conversationId: string,
  title: string,
): Promise<ConversationResponse> {
  return authenticatedRequest<ConversationResponse>(
    `/api/conversation/${encodeURIComponent(conversationId)}`,
    { method: 'PATCH', body: JSON.stringify({ title }) },
  );
}

export function deleteConversation(conversationId: string): Promise<void> {
  return authenticatedCommand(`/api/conversation/${encodeURIComponent(conversationId)}`, {
    method: 'DELETE',
  });
}

async function openStream(
  input: ChatStreamRequest,
  signal: AbortSignal,
  retryAfterRefresh: boolean,
): Promise<Response> {
  const session = await loadStoredSession();
  if (!session) {
    throw new ApiError(401, '请先登录。');
  }

  let response: Response;
  try {
    response = await fetch(`${API_BASE_URL}/api/chat/stream`, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${session.accessToken}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(input),
      signal,
    });
  } catch (error) {
    if (signal.aborted) {
      throw error;
    }
    throw new ApiError(0, '无法连接到服务器，请检查网络后重试。');
  }

  if (response.status === 401 && retryAfterRefresh) {
    const refreshed = await refreshSession();
    if (!refreshed) {
      throw new ApiError(401, '登录状态已失效，请重新登录。');
    }
    return openStream(input, signal, false);
  }

  if (!response.ok) {
    let message = '请求失败，请稍后重试。';
    try {
      const envelope = (await response.json()) as ApiEnvelope<unknown>;
      message = envelope.message || message;
    } catch {
      // Keep the safe fallback for a non-JSON proxy response.
    }
    throw new ApiError(response.status, message);
  }
  if (!response.body) {
    throw new ApiError(response.status, '服务器未返回可读取的消息流。');
  }
  return response;
}

export async function streamChat(
  input: ChatStreamRequest,
  signal: AbortSignal,
  onEvent: (event: ChatStreamEvent) => void,
): Promise<void> {
  const response = await openStream(input, signal, true);
  const body = response.body;
  if (!body) {
    throw new ApiError(response.status, '服务器未返回可读取的消息流。');
  }
  await consumeSseStream(body, onEvent);
}
