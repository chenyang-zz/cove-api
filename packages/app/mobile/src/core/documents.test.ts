import { beforeEach, describe, expect, it, vi } from 'vitest';

import {
  MAX_DOCUMENT_UPLOAD_SIZE,
  applyDocumentStatuses,
  canLoadMoreDocuments,
  documentUploadError,
  getDocumentStatus,
  listDocuments,
  mergeDocumentPage,
  prependUploadedDocument,
  refreshDocumentRecords,
  uploadDocument,
  type DocumentPaginationState,
} from './documents';

const api = vi.hoisted(() => ({
  authenticatedMultipartRequest: vi.fn(),
  authenticatedRequest: vi.fn(),
}));
const fileSystem = vi.hoisted(() => {
  const copy = vi.fn();
  const createDirectory = vi.fn();
  const deleteDirectory = vi.fn();

  class Directory {
    readonly uri: string;
    exists = true;

    constructor(...parts: (string | { uri: string })[]) {
      this.uri = parts.map((part) => typeof part === 'string' ? part : part.uri).join('/');
    }

    create(options?: unknown) {
      createDirectory(this.uri, options);
    }

    delete() {
      this.exists = false;
      deleteDirectory(this.uri);
    }
  }

  class File {
    readonly uri: string;

    constructor(...parts: (string | { uri: string })[]) {
      this.uri = parts.map((part) => typeof part === 'string' ? part : part.uri).join('/');
    }

    async copy(destination: File) {
      await copy(this.uri, destination.uri);
    }
  }

  return {
    Directory,
    File,
    Paths: { cache: { uri: 'file:///cache' } },
    copy,
    createDirectory,
    deleteDirectory,
  };
});

vi.mock('./api', () => ({
  authenticatedMultipartRequest: api.authenticatedMultipartRequest,
  authenticatedRequest: api.authenticatedRequest,
}));
vi.mock('expo-file-system', () => fileSystem);

const firstPage: DocumentPaginationState = {
  items: [{ id: 'document-1', file_name: 'one.md' }],
  total: 3,
  page: 1,
  pageSize: 20,
};

describe('knowledge document API and pagination', () => {
  beforeEach(() => {
    api.authenticatedMultipartRequest.mockReset();
    api.authenticatedRequest.mockReset();
    fileSystem.copy.mockReset();
    fileSystem.createDirectory.mockReset();
    fileSystem.deleteDirectory.mockReset();
  });

  it('maps the documented page, page_size and encoded kb_id query through authenticated request', async () => {
    const response = { list: [], total: 0, page: 2, page_size: 20 };
    api.authenticatedRequest.mockResolvedValue(response);

    await expect(listDocuments('knowledge / 1', 2, 20)).resolves.toBe(response);
    expect(api.authenticatedRequest).toHaveBeenCalledWith(
      '/api/document?page=2&page_size=20&kb_id=knowledge+%2F+1',
    );
  });

  it('merges subsequent pages without duplicate document ids and preserves order', () => {
    expect(mergeDocumentPage(firstPage, {
      list: [
        { id: 'document-1', file_name: 'duplicate.md' },
        { id: 'document-2', file_name: 'two.md' },
      ],
      total: 3,
      page: 2,
      page_size: 20,
    })).toEqual({
      items: [
        { id: 'document-1', file_name: 'one.md' },
        { id: 'document-2', file_name: 'two.md' },
      ],
      total: 3,
      page: 2,
      pageSize: 20,
    });
  });

  it('resets pagination to the refreshed first page', () => {
    expect(mergeDocumentPage(firstPage, {
      list: [{ id: 'document-new', file_name: 'new.md' }],
      total: 1,
      page: 1,
      page_size: 20,
    }, true)).toEqual({
      items: [{ id: 'document-new', file_name: 'new.md' }],
      total: 1,
      page: 1,
      pageSize: 20,
    });
  });

  it('guards load more while busy, empty or fully loaded', () => {
    expect(canLoadMoreDocuments(firstPage, false)).toBe(true);
    expect(canLoadMoreDocuments(firstPage, true)).toBe(false);
    expect(canLoadMoreDocuments({ ...firstPage, items: [] }, false)).toBe(false);
    expect(canLoadMoreDocuments({ ...firstPage, total: 1 }, false)).toBe(false);
  });

  it('creates a fresh multipart upload body with the knowledge id and native file descriptor', async () => {
    class TestFormData {
      readonly parts: { name: string; value: unknown; fileName?: string }[] = [];

      append(name: string, value: unknown, fileName?: string) {
        this.parts.push({ name, value, fileName });
      }
    }

    api.authenticatedMultipartRequest.mockResolvedValue({
      id: 'document-uploaded',
      file_name: 'guide.md',
      status: 'pending',
    });
    const originalFormData = globalThis.FormData;
    vi.stubGlobal('FormData', TestFormData);
    try {
      await uploadDocument('knowledge-1', {
        uri: 'file:///tmp/guide.md',
        name: 'guide.md',
        mimeType: 'text/markdown',
        size: 42,
      });
      expect(api.authenticatedMultipartRequest).toHaveBeenCalledWith(
        '/api/document/upload',
        expect.any(Function),
      );
      const createBody = api.authenticatedMultipartRequest.mock.calls[0]?.[1] as () => TestFormData;
      expect(createBody().parts).toEqual([
        { name: 'kb_id', value: 'knowledge-1', fileName: undefined },
        {
          name: 'file',
          value: expect.objectContaining({ uri: expect.stringMatching(/\/guide\.md$/) }),
          fileName: 'guide.md',
        },
      ]);
      expect(fileSystem.copy).toHaveBeenCalledWith(
        'file:///tmp/guide.md',
        expect.stringMatching(/\/guide\.md$/),
      );
      expect(fileSystem.deleteDirectory).toHaveBeenCalledOnce();
    } finally {
      vi.stubGlobal('FormData', originalFormData);
    }
  });

  it('loads an encoded document processing status through authenticated request', async () => {
    const response = { status: 'parsing', progress: 0.5 };
    api.authenticatedRequest.mockResolvedValue(response);

    await expect(getDocumentStatus('document / 1')).resolves.toBe(response);
    expect(api.authenticatedRequest).toHaveBeenCalledWith(
      '/api/document/document%20%2F%201/status',
    );
  });

  it('removes the temporary upload directory when copying the selected file fails', async () => {
    fileSystem.copy.mockRejectedValueOnce(new Error('copy failed'));

    await expect(uploadDocument('knowledge-1', {
      uri: 'file:///tmp/guide.md',
      name: 'guide.md',
    })).rejects.toThrow('copy failed');
    expect(api.authenticatedMultipartRequest).not.toHaveBeenCalled();
    expect(fileSystem.deleteDirectory).toHaveBeenCalledOnce();
  });

  it('validates supported file types and the 50 MiB upload limit', () => {
    expect(documentUploadError({ uri: 'file://guide.md', name: 'guide.md' })).toBe('');
    expect(documentUploadError({ uri: 'file://page.HTML', name: 'page.HTML' })).toBe('');
    expect(documentUploadError({ uri: 'file://photo.png', name: 'photo.png' })).toContain('仅支持');
    expect(documentUploadError({
      uri: 'file://large.pdf',
      name: 'large.pdf',
      size: MAX_DOCUMENT_UPLOAD_SIZE + 1,
    })).toBe('文件不能超过 50 MB。');
  });

  it('prepends one optimistic upload and applies processing status updates without duplicates', () => {
    const uploaded = { id: 'document-2', file_name: 'two.md', status: 'pending' };
    const withUpload = prependUploadedDocument(firstPage, uploaded);
    expect(withUpload.items.map((item) => item.id)).toEqual(['document-2', 'document-1']);
    expect(withUpload.total).toBe(4);
    expect(prependUploadedDocument(withUpload, { ...uploaded, status: 'parsing' }).total).toBe(4);

    const updates = new Map([
      ['document-2', { status: 'done', progress: 1, error_msg: null }],
    ]);
    expect(applyDocumentStatuses(withUpload, updates).items[0]).toMatchObject({
      id: 'document-2',
      status: 'done',
      progress: 1,
    });
  });

  it('refreshes completed document records without discarding already loaded pages', () => {
    const state: DocumentPaginationState = {
      items: [
        { id: 'document-1', file_name: 'one.md', status: 'done', chunk_num: 0 },
        { id: 'document-2', file_name: 'two.md', status: 'done', chunk_num: 2 },
      ],
      total: 2,
      page: 2,
      pageSize: 1,
    };

    expect(refreshDocumentRecords(state, {
      list: [{ id: 'document-1', file_name: 'one.md', status: 'done', chunk_num: 3 }],
      total: 2,
      page: 1,
      page_size: 20,
    })).toEqual({
      ...state,
      items: [
        { id: 'document-1', file_name: 'one.md', status: 'done', chunk_num: 3 },
        state.items[1],
      ],
    });
  });
});
