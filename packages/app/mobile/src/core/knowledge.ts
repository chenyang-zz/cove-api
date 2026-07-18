import { authenticatedRequest } from './api';

export type KnowledgeBase = {
  id: string;
  name: string;
  description?: string | null;
  icon?: string | null;
  color?: string | null;
  doc_count?: number | null;
  image_count?: number | null;
  chat_enabled?: boolean | null;
  is_default?: boolean | null;
  created_at?: string | null;
  updated_at?: string | null;
};

type KnowledgeBaseListResponse = {
  list: KnowledgeBase[];
};

export function listKnowledgeBases(): Promise<KnowledgeBaseListResponse> {
  return authenticatedRequest<KnowledgeBaseListResponse>('/api/knowledge-base/');
}

export function setDefaultKnowledgeBase(knowledgeBaseId: string): Promise<KnowledgeBase> {
  return authenticatedRequest<KnowledgeBase>(
    `/api/knowledge-base/${encodeURIComponent(knowledgeBaseId)}/default`,
    { method: 'POST' },
  );
}
