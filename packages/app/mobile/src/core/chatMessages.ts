import type {
  ChatMessage,
  ChatMessageResponse,
  ChatThinkEvent,
  ChatToolEvent,
  MessagePart,
} from './types';

export type MessageTimelineItem =
  | { kind: 'text'; part: MessagePart; key: string }
  | { kind: 'tool'; call: MessagePart | null; result: MessagePart | null; key: string };

export function textPart(text: string): MessagePart {
  return {
    type: 'text',
    text,
    tool: null,
    input: null,
    observation: null,
    error: null,
    iteration: null,
    tool_call_id: null,
  };
}

export function toChatMessages(messages: ChatMessageResponse[]): ChatMessage[] {
  return messages
    .filter((message) => message.role === 'user' || message.role === 'assistant')
    .map((message) => ({ ...message }));
}

export function appendTokenPart(parts: MessagePart[], text: string): MessagePart[] {
  const next = [...parts];
  const last = next[next.length - 1];
  if (last?.type === 'text') {
    next[next.length - 1] = { ...last, text: `${last.text ?? ''}${text}` };
    return next;
  }
  return [...next, textPart(text)];
}

export function appendToolPart(parts: MessagePart[], event: ChatToolEvent): MessagePart[] {
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
  ];
}

export function applyThinkEvent(message: ChatMessage, event: ChatThinkEvent): ChatMessage {
  if (event.status === 'thinking') {
    if (message.thinking && message.thinking.iteration > event.iteration) {
      return message;
    }
    return { ...message, thinking: { active: true, iteration: event.iteration } };
  }
  if (message.thinking?.iteration !== event.iteration) {
    return message;
  }
  return { ...message, thinking: { active: false, iteration: event.iteration } };
}

export function messageTimeline(message: ChatMessage): MessageTimelineItem[] {
  const source = message.parts?.length
    ? message.parts
    : message.meta_data?.parts?.length
      ? message.meta_data.parts
      : message.content
        ? [textPart(message.content)]
        : [];
  const timeline: MessageTimelineItem[] = [];
  const tools = new Map<string, Extract<MessageTimelineItem, { kind: 'tool' }>>();

  source.forEach((part, index) => {
    if (part.type === 'text') {
      if (part.text?.trim()) {
        timeline.push({ kind: 'text', part, key: `text-${index}` });
      }
      return;
    }
    if (part.type === 'tool_call') {
      const key = part.tool_call_id || `tool-${index}`;
      const item: Extract<MessageTimelineItem, { kind: 'tool' }> = {
        kind: 'tool',
        call: part,
        result: null,
        key,
      };
      timeline.push(item);
      tools.set(key, item);
      return;
    }
    if (part.type === 'tool_result') {
      const key = part.tool_call_id || `tool-result-${index}`;
      const existing = tools.get(key);
      if (existing) {
        existing.result = part;
      } else {
        timeline.push({ kind: 'tool', call: null, result: part, key });
      }
    }
  });

  if (message.content.trim() && !timeline.some((item) => item.kind === 'text')) {
    timeline.push({ kind: 'text', part: textPart(message.content), key: 'content-fallback' });
  }
  return timeline;
}
