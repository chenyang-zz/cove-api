import { Directory, File, Paths } from 'expo-file-system';

import { authenticatedMultipartRequest, authenticatedRequest } from './api';

export const MAX_DOCUMENT_UPLOAD_SIZE = 50 * 1024 * 1024;

const SUPPORTED_DOCUMENT_EXTENSIONS = new Set([
  '.pdf',
  '.docx',
  '.md',
  '.markdown',
  '.txt',
  '.html',
  '.htm',
]);

export type KnowledgeDocument = {
  id: string;
  kb_id?: string | null;
  file_name: string;
  file_ext?: string | null;
  file_size?: number | null;
  source_type?: string | null;
  source_url?: string | null;
  status?: string | null;
  progress?: number | null;
  chunk_num?: number | null;
  error_msg?: string | null;
  tags?: string[] | null;
  created_at?: string | null;
  updated_at?: string | null;
};

export type DocumentListResponse = {
  list: KnowledgeDocument[];
  total: number;
  page: number;
  page_size: number;
};

export type DocumentPaginationState = {
  items: KnowledgeDocument[];
  total: number;
  page: number;
  pageSize: number;
};

export type DocumentUploadFile = {
  uri: string;
  name: string;
  mimeType?: string | null;
  size?: number | null;
};

export type DocumentStatusResponse = {
  status: string;
  progress?: number | null;
  error_msg?: string | null;
};

export function listDocuments(
  knowledgeBaseId: string,
  page = 1,
  pageSize = 20,
): Promise<DocumentListResponse> {
  const query = new URLSearchParams({
    page: String(page),
    page_size: String(pageSize),
    kb_id: knowledgeBaseId,
  });
  return authenticatedRequest<DocumentListResponse>(`/api/document?${query}`);
}

export async function uploadDocument(
  knowledgeBaseId: string,
  file: DocumentUploadFile,
): Promise<KnowledgeDocument> {
  const uploadDirectory = new Directory(
    Paths.cache,
    'cove-document-uploads',
    `${Date.now()}-${Math.random().toString(36).slice(2)}`,
  );
  uploadDirectory.create({ intermediates: true });
  const uploadFile = new File(uploadDirectory, file.name.replace(/[\\/]/g, '_'));

  try {
    await new File(file.uri).copy(uploadFile);
    return await authenticatedMultipartRequest<KnowledgeDocument>('/api/document/upload', () => {
      const body = new FormData();
      body.append('kb_id', knowledgeBaseId);
      body.append('file', uploadFile, file.name);
      return body;
    });
  } finally {
    try {
      if (uploadDirectory.exists) {
        uploadDirectory.delete();
      }
    } catch {
      // Cache cleanup must not turn a completed server upload into a visible failure.
    }
  }
}

export function getDocumentStatus(documentId: string): Promise<DocumentStatusResponse> {
  return authenticatedRequest<DocumentStatusResponse>(
    `/api/document/${encodeURIComponent(documentId)}/status`,
  );
}

export function documentUploadError(file: DocumentUploadFile): string {
  const name = file.name.trim();
  if (!name) {
    return '无法识别文件名，请重新选择。';
  }
  const dotIndex = name.lastIndexOf('.');
  const extension = dotIndex >= 0 ? name.slice(dotIndex).toLowerCase() : '';
  if (!SUPPORTED_DOCUMENT_EXTENSIONS.has(extension)) {
    return '仅支持 PDF、DOCX、Markdown、TXT 和 HTML 文件。';
  }
  if (typeof file.size === 'number' && file.size > MAX_DOCUMENT_UPLOAD_SIZE) {
    return '文件不能超过 50 MB。';
  }
  return '';
}

export function applyDocumentStatuses(
  state: DocumentPaginationState,
  updates: ReadonlyMap<string, DocumentStatusResponse>,
): DocumentPaginationState {
  let changed = false;
  const items = state.items.map((item) => {
    const update = updates.get(item.id);
    if (!update) {
      return item;
    }
    changed = true;
    return { ...item, ...update };
  });
  return changed ? { ...state, items } : state;
}

export function refreshDocumentRecords(
  state: DocumentPaginationState,
  response: DocumentListResponse,
): DocumentPaginationState {
  const refreshed = new Map(response.list.map((item) => [item.id, item]));
  return {
    ...state,
    items: state.items.map((item) => refreshed.get(item.id) ?? item),
    total: Math.max(0, response.total),
  };
}

export function prependUploadedDocument(
  state: DocumentPaginationState,
  document: KnowledgeDocument,
): DocumentPaginationState {
  const exists = state.items.some((item) => item.id === document.id);
  return {
    ...state,
    items: [document, ...state.items.filter((item) => item.id !== document.id)],
    total: exists ? state.total : state.total + 1,
  };
}

export function mergeDocumentPage(
  current: DocumentPaginationState,
  response: DocumentListResponse,
  reset = false,
): DocumentPaginationState {
  const merged = reset ? [] : [...current.items];
  const known = new Set(merged.map((item) => item.id));
  for (const item of response.list) {
    if (!item.id || known.has(item.id)) {
      continue;
    }
    known.add(item.id);
    merged.push(item);
  }
  return {
    items: merged,
    total: Math.max(0, response.total),
    page: Math.max(1, response.page),
    pageSize: Math.max(1, response.page_size),
  };
}

export function canLoadMoreDocuments(
  state: DocumentPaginationState,
  busy: boolean,
): boolean {
  return !busy && state.items.length > 0 && state.items.length < state.total;
}
