import { beforeEach, describe, expect, it, vi } from 'vitest';

const api = vi.hoisted(() => ({
  authenticatedCommand: vi.fn(),
  authenticatedRequest: vi.fn(),
}));

vi.mock('./api', () => ({
  ApiError: class ApiError extends Error {},
  authenticatedCommand: api.authenticatedCommand,
  authenticatedRequest: api.authenticatedRequest,
  refreshSession: vi.fn(),
}));

vi.mock('expo/fetch', () => ({ fetch: vi.fn() }));
vi.mock('./session', () => ({ loadStoredSession: vi.fn() }));
vi.mock('./sse', () => ({ consumeSseStream: vi.fn() }));

import { deleteConversation, renameConversation } from './chat';

describe('conversation actions', () => {
  beforeEach(() => {
    api.authenticatedCommand.mockReset();
    api.authenticatedRequest.mockReset();
  });

  it('renames an encoded conversation path with the documented payload', async () => {
    const updated = { id: 'conversation / 1', title: '新名称' };
    api.authenticatedRequest.mockResolvedValue(updated);

    await expect(renameConversation('conversation / 1', '新名称')).resolves.toBe(updated);
    expect(api.authenticatedRequest).toHaveBeenCalledWith(
      '/api/conversation/conversation%20%2F%201',
      { method: 'PATCH', body: JSON.stringify({ title: '新名称' }) },
    );
  });

  it('deletes an encoded conversation path without expecting response data', async () => {
    api.authenticatedCommand.mockResolvedValue(undefined);

    await expect(deleteConversation('conversation / 1')).resolves.toBeUndefined();
    expect(api.authenticatedCommand).toHaveBeenCalledWith(
      '/api/conversation/conversation%20%2F%201',
      { method: 'DELETE' },
    );
  });
});
