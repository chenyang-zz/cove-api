import { describe, expect, it } from 'vitest';

import {
  appendTokenPart,
  appendToolPart,
  applyThinkEvent,
  messageTimeline,
  toChatMessages,
} from './chatMessages';
import type { ChatMessage, ChatMessageResponse } from './types';

function assistant(overrides: Partial<ChatMessage> = {}): ChatMessage {
  return {
    id: 'assistant-1',
    role: 'assistant',
    content: '',
    meta_data: null,
    images: [],
    sender_persona_id: null,
    feedback: null,
    created_at: '2026-07-15T00:00:00Z',
    ...overrides,
  };
}

describe('chat message timeline', () => {
  it('keeps history metadata so tool calls render before markdown text', () => {
    const history: ChatMessageResponse[] = [assistant({
      content: '# 北京天气预报',
      meta_data: {
        parts: [
          {
            type: 'tool_call',
            text: null,
            tool: 'maps_weather',
            input: { city: '北京' },
            observation: null,
            error: null,
            iteration: 1,
            tool_call_id: 'weather-1',
          },
          {
            type: 'tool_result',
            text: null,
            tool: 'maps_weather',
            input: null,
            observation: '晴',
            error: null,
            iteration: 1,
            tool_call_id: 'weather-1',
          },
          {
            type: 'text',
            text: '# 北京天气预报',
            tool: null,
            input: null,
            observation: null,
            error: null,
            iteration: null,
            tool_call_id: null,
          },
        ],
      },
    })];

    const [message] = toChatMessages(history);
    const timeline = messageTimeline(message);

    expect(timeline.map((item) => item.kind)).toEqual(['tool', 'text']);
    expect(timeline[0]).toMatchObject({ kind: 'tool', result: { observation: '晴' } });
  });

  it('preserves streaming tool and token order', () => {
    let message = assistant({ pending: true, parts: [] });
    message = { ...message, thinking: { active: true, iteration: 1 } };
    message = applyThinkEvent(message, { type: 'think', status: 'done', iteration: 1 });
    message = {
      ...message,
      parts: appendToolPart(message.parts ?? [], {
        type: 'tool_call',
        tool: 'maps_weather',
        iteration: 1,
        tool_call_id: 'weather-1',
      }),
    };
    message = { ...message, parts: appendTokenPart(message.parts ?? [], '天气晴朗') };

    expect(message.thinking?.active).toBe(false);
    expect(messageTimeline(message).map((item) => item.kind)).toEqual(['tool', 'text']);
  });
});
