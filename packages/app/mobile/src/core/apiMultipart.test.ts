import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ApiError, authenticatedMultipartRequest } from './api';
import type { StoredSession } from './types';

const network = vi.hoisted(() => ({ fetch: vi.fn() }));
const sessionStore = vi.hoisted(() => ({
  clearStoredSession: vi.fn(),
  loadStoredSession: vi.fn(),
  saveStoredSession: vi.fn(),
}));

vi.mock('expo/fetch', () => ({ fetch: network.fetch }));
vi.mock('./config', () => ({ API_BASE_URL: 'http://api.test' }));
vi.mock('./session', () => sessionStore);

const initialSession: StoredSession = {
  accessToken: 'access-old',
  refreshToken: 'refresh-old',
  user: {
    id: 'user-1',
    username: 'tester',
    nickname: '测试用户',
    email: null,
    avatar: null,
  },
};

function jsonResponse(status: number, payload: unknown): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: vi.fn().mockResolvedValue(payload),
  } as unknown as Response;
}

describe('authenticated multipart request', () => {
  beforeEach(() => {
    network.fetch.mockReset();
    sessionStore.clearStoredSession.mockReset();
    sessionStore.loadStoredSession.mockReset().mockResolvedValue(initialSession);
    sessionStore.saveStoredSession.mockReset().mockImplementation(async (session: StoredSession) => session);
  });

  it('posts a caller-created FormData body without overriding its content type', async () => {
    network.fetch.mockResolvedValue(jsonResponse(200, {
      code: 0,
      message: 'ok',
      data: { id: 'document-1' },
    }));
    const body = new FormData();
    const createBody = vi.fn(() => body);

    await expect(authenticatedMultipartRequest('/api/document/upload', createBody)).resolves.toEqual({
      id: 'document-1',
    });
    expect(createBody).toHaveBeenCalledTimes(1);
    expect(network.fetch).toHaveBeenCalledWith('http://api.test/api/document/upload', {
      method: 'POST',
      headers: { Authorization: 'Bearer access-old' },
      body,
    });
    expect(network.fetch.mock.calls[0]?.[1]?.headers).not.toHaveProperty('Content-Type');
  });

  it('refreshes once after an initial 401 and rebuilds the multipart body for retry', async () => {
    network.fetch
      .mockResolvedValueOnce(jsonResponse(401, { code: 401, message: 'token expired' }))
      .mockResolvedValueOnce(jsonResponse(200, {
        code: 0,
        message: 'ok',
        data: {
          user_id: 'user-1',
          username: 'tester',
          access_token: 'access-new',
          refresh_token: 'refresh-new',
        },
      }))
      .mockResolvedValueOnce(jsonResponse(200, {
        code: 0,
        message: 'ok',
        data: { id: 'document-2' },
      }));
    const bodies = [new FormData(), new FormData()];
    const createBody = vi.fn(() => bodies[createBody.mock.calls.length - 1] ?? new FormData());

    await expect(authenticatedMultipartRequest('/api/document/upload', createBody)).resolves.toEqual({
      id: 'document-2',
    });
    expect(createBody).toHaveBeenCalledTimes(2);
    expect(network.fetch).toHaveBeenNthCalledWith(2, 'http://api.test/api/auth/refresh', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ refresh_token: 'refresh-old' }),
    }));
    expect(network.fetch).toHaveBeenNthCalledWith(3, 'http://api.test/api/document/upload', {
      method: 'POST',
      headers: { Authorization: 'Bearer access-new' },
      body: bodies[1],
    });
  });

  it('maps connectivity failures to the standard API error', async () => {
    network.fetch.mockRejectedValue(new Error('offline'));

    await expect(authenticatedMultipartRequest(
      '/api/document/upload',
      () => new FormData(),
      false,
    )).rejects.toMatchObject({
      status: 0,
      message: '无法连接到服务器，请检查网络后重试。',
    } satisfies Partial<ApiError>);
  });
});
