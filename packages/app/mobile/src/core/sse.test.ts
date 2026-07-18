import { describe, expect, it } from 'vitest';

import { consumeSseStream, parseSseBlock } from './sse';

describe('SSE parsing', () => {
  it('parses named JSON events', () => {
    expect(parseSseBlock('event: token\ndata: {"text":"你好"}')).toEqual({
      type: 'token',
      text: '你好',
    });
  });

  it('prefers an explicit payload type', () => {
    expect(parseSseBlock('event: message\ndata: {"type":"done","text":"m1"}')).toEqual({
      type: 'done',
      text: 'm1',
    });
  });

  it('consumes blocks split across byte chunks', async () => {
    const encoder = new TextEncoder();
    const events: unknown[] = [];
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(encoder.encode('event: token\ndata: {"text":"海'));
        controller.enqueue(encoder.encode('湾"}\n\nevent: done\ndata: {"text":"m1"}\n\n'));
        controller.close();
      },
    });

    await consumeSseStream(body, (event) => events.push(event));
    expect(events).toEqual([
      { type: 'token', text: '海湾' },
      { type: 'done', text: 'm1' },
    ]);
  });
});
