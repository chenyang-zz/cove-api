import type { ChatStreamEvent } from './types';

export function parseSseBlock(block: string): ChatStreamEvent | null {
  let eventName = '';
  const dataLines: string[] = [];

  for (const line of block.split(/\r?\n/)) {
    if (!line || line.startsWith(':')) {
      continue;
    }
    if (line.startsWith('event:')) {
      eventName = line.slice(6).trim();
    } else if (line.startsWith('data:')) {
      dataLines.push(line.slice(5).trimStart());
    }
  }

  if (!eventName || dataLines.length === 0) {
    return null;
  }
  const parsed = JSON.parse(dataLines.join('\n')) as Record<string, unknown>;
  return {
    ...parsed,
    type: typeof parsed.type === 'string' ? parsed.type : eventName,
  } as ChatStreamEvent;
}

export async function consumeSseStream(
  body: ReadableStream<Uint8Array>,
  onEvent: (event: ChatStreamEvent) => void,
): Promise<void> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';

  try {
    while (true) {
      const { value, done } = await reader.read();
      buffer += decoder.decode(value, { stream: !done });
      let boundary = buffer.match(/\r?\n\r?\n/);
      while (boundary?.index !== undefined) {
        const block = buffer.slice(0, boundary.index);
        buffer = buffer.slice(boundary.index + boundary[0].length);
        const event = parseSseBlock(block);
        if (event) {
          onEvent(event);
        }
        boundary = buffer.match(/\r?\n\r?\n/);
      }
      if (done) {
        const event = parseSseBlock(buffer);
        if (event) {
          onEvent(event);
        }
        break;
      }
    }
  } finally {
    reader.releaseLock();
  }
}
