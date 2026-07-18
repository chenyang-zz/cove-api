import { beforeEach, describe, expect, it, vi } from 'vitest';

const api = vi.hoisted(() => ({ authenticatedRequest: vi.fn() }));

vi.mock('./api', () => ({ authenticatedRequest: api.authenticatedRequest }));

import { listKnowledgeBases, setDefaultKnowledgeBase } from './knowledge';

describe('knowledge base API', () => {
  beforeEach(() => {
    api.authenticatedRequest.mockReset();
  });

  it('loads the authenticated knowledge base list from the documented endpoint', async () => {
    const response = {
      list: [
        {
          id: 'knowledge-1',
          name: '产品资料',
          description: 'Cove 产品文档',
          doc_count: 12,
          chat_enabled: true,
        },
      ],
    };
    api.authenticatedRequest.mockResolvedValue(response);

    await expect(listKnowledgeBases()).resolves.toBe(response);
    expect(api.authenticatedRequest).toHaveBeenCalledWith('/api/knowledge-base/');
  });

  it('sets the selected knowledge base as default through the documented endpoint', async () => {
    const response = {
      id: 'knowledge-2',
      name: '团队资料',
      is_default: true,
    };
    api.authenticatedRequest.mockResolvedValue(response);

    await expect(setDefaultKnowledgeBase('knowledge-2')).resolves.toBe(response);
    expect(api.authenticatedRequest).toHaveBeenCalledWith(
      '/api/knowledge-base/knowledge-2/default',
      { method: 'POST' },
    );
  });
});
