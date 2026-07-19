import { useCallback, useEffect, useRef, useState } from 'react';
import {
  ActivityIndicator,
  FlatList,
  Pressable,
  RefreshControl,
  StyleSheet,
  Text,
  View,
} from 'react-native';
import { Stack, useLocalSearchParams, useRouter } from 'expo-router';
import * as DocumentPicker from 'expo-document-picker';
import { SymbolView } from 'expo-symbols';
import { SafeAreaView } from 'react-native-safe-area-context';

import { ApiError } from '@/core/api';
import {
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
  type DocumentStatusResponse,
  type DocumentUploadFile,
  type KnowledgeDocument,
} from '@/core/documents';
import { getKnowledgeBase, type KnowledgeBase } from '@/core/knowledge';
import { loadStoredSession } from '@/core/session';
import { useAuth } from '@/providers/AuthProvider';
import { usePalette, type Palette } from '@/theme/palette';

const PAGE_SIZE = 20;
const DOCUMENT_STATUS_POLL_INTERVAL = 1800;
const DOCUMENT_PICKER_TYPES = [
  'application/pdf',
  'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
  'text/plain',
  'text/markdown',
  'text/html',
];

type LoadState = 'loading' | 'ready' | 'error';
type RouteParams = {
  knowledgeBaseId?: string | string[];
  name?: string | string[];
  description?: string | string[];
  color?: string | string[];
  doc_count?: string | string[];
  chat_enabled?: string | string[];
  docCount?: string | string[];
  chatEnabled?: string | string[];
};

const EMPTY_DOCUMENTS: DocumentPaginationState = {
  items: [],
  total: 0,
  page: 1,
  pageSize: PAGE_SIZE,
};

export default function KnowledgeDetailScreen() {
  const palette = usePalette();
  const router = useRouter();
  const params = useLocalSearchParams<RouteParams>();
  const { signOut } = useAuth();
  const knowledgeBaseId = firstParam(params.knowledgeBaseId);
  const cachedKnowledge = cachedKnowledgeBase(knowledgeBaseId, params);
  const [knowledgeBase, setKnowledgeBase] = useState<KnowledgeBase | null>(cachedKnowledge);
  const [knowledgeState, setKnowledgeState] = useState<LoadState>(cachedKnowledge ? 'ready' : 'loading');
  const [knowledgeError, setKnowledgeError] = useState('');
  const [documents, setDocuments] = useState<DocumentPaginationState>(EMPTY_DOCUMENTS);
  const [documentState, setDocumentState] = useState<LoadState>('loading');
  const [documentError, setDocumentError] = useState('');
  const [refreshing, setRefreshing] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [loadMoreError, setLoadMoreError] = useState('');
  const [uploading, setUploading] = useState(false);
  const [uploadError, setUploadError] = useState('');
  const knowledgeBaseRef = useRef<KnowledgeBase | null>(cachedKnowledge);
  const documentsRef = useRef<DocumentPaginationState>(EMPTY_DOCUMENTS);
  const generationRef = useRef(0);
  const refreshingRef = useRef(false);
  const loadingMoreRef = useRef(false);
  const uploadingRef = useRef(false);
  const pollingRef = useRef(false);

  const updateDocuments = useCallback((next: DocumentPaginationState) => {
    documentsRef.current = next;
    setDocuments(next);
  }, []);

  const handleAuthError = useCallback(async (caught: unknown) => {
    if (!(caught instanceof ApiError) || caught.status !== 401) {
      return;
    }
    const storedSession = await loadStoredSession();
    if (!storedSession) {
      await signOut();
    }
  }, [signOut]);

  const loadInitial = useCallback(async () => {
    const generation = ++generationRef.current;
    setKnowledgeError('');
    setDocumentError('');
    setLoadMoreError('');
    if (!knowledgeBaseRef.current) {
      setKnowledgeState('loading');
    }
    setDocumentState('loading');

    const [knowledgeResult, documentResult] = await Promise.allSettled([
      getKnowledgeBase(knowledgeBaseId),
      listDocuments(knowledgeBaseId, 1, PAGE_SIZE),
    ]);
    if (generation !== generationRef.current) {
      return;
    }

    if (knowledgeResult.status === 'fulfilled') {
      knowledgeBaseRef.current = knowledgeResult.value;
      setKnowledgeBase(knowledgeResult.value);
      setKnowledgeState('ready');
    } else {
      setKnowledgeError(errorMessage(knowledgeResult.reason, '知识库信息加载失败。'));
      setKnowledgeState(knowledgeBaseRef.current ? 'ready' : 'error');
      await handleAuthError(knowledgeResult.reason);
    }

    if (documentResult.status === 'fulfilled') {
      updateDocuments(mergeDocumentPage(EMPTY_DOCUMENTS, documentResult.value, true));
      setDocumentState('ready');
    } else {
      setDocumentError(errorMessage(documentResult.reason, '文档列表加载失败，请稍后重试。'));
      setDocumentState('error');
      await handleAuthError(documentResult.reason);
    }
  }, [handleAuthError, knowledgeBaseId, updateDocuments]);

  useEffect(() => {
    const timer = setTimeout(() => {
      void loadInitial();
    }, 0);
    return () => {
      clearTimeout(timer);
      generationRef.current += 1;
    };
  }, [loadInitial]);

  const refresh = useCallback(async () => {
    if (refreshingRef.current) {
      return;
    }
    refreshingRef.current = true;
    setRefreshing(true);
    setDocumentError('');
    setKnowledgeError('');
    setLoadMoreError('');
    const generation = ++generationRef.current;
    const [knowledgeResult, documentResult] = await Promise.allSettled([
      getKnowledgeBase(knowledgeBaseId),
      listDocuments(knowledgeBaseId, 1, PAGE_SIZE),
    ]);
    if (generation === generationRef.current) {
      if (knowledgeResult.status === 'fulfilled') {
        knowledgeBaseRef.current = knowledgeResult.value;
        setKnowledgeBase(knowledgeResult.value);
        setKnowledgeState('ready');
      } else {
        setKnowledgeError(errorMessage(knowledgeResult.reason, '知识库信息刷新失败。'));
        await handleAuthError(knowledgeResult.reason);
      }
      if (documentResult.status === 'fulfilled') {
        updateDocuments(mergeDocumentPage(EMPTY_DOCUMENTS, documentResult.value, true));
        setDocumentState('ready');
      } else {
        setDocumentError(errorMessage(documentResult.reason, '文档列表刷新失败。'));
        await handleAuthError(documentResult.reason);
      }
    }
    refreshingRef.current = false;
    setRefreshing(false);
  }, [handleAuthError, knowledgeBaseId, updateDocuments]);

  const loadMore = useCallback(async () => {
    const current = documentsRef.current;
    const busy = loadingMoreRef.current || refreshingRef.current || documentState !== 'ready';
    if (!canLoadMoreDocuments(current, busy)) {
      return;
    }
    loadingMoreRef.current = true;
    setLoadingMore(true);
    setLoadMoreError('');
    const generation = generationRef.current;
    try {
      const response = await listDocuments(knowledgeBaseId, current.page + 1, current.pageSize);
      if (generation !== generationRef.current) {
        return;
      }
      updateDocuments(mergeDocumentPage(current, response));
    } catch (caught) {
      if (generation === generationRef.current) {
        setLoadMoreError(errorMessage(caught, '更多文档加载失败。'));
        await handleAuthError(caught);
      }
    } finally {
      loadingMoreRef.current = false;
      setLoadingMore(false);
    }
  }, [documentState, handleAuthError, knowledgeBaseId, updateDocuments]);

  const selectAndUploadDocument = useCallback(async () => {
    if (uploadingRef.current) {
      return;
    }
    uploadingRef.current = true;
    setUploading(true);
    setUploadError('');
    const generation = generationRef.current;
    try {
      const result = await DocumentPicker.getDocumentAsync({
        type: DOCUMENT_PICKER_TYPES,
        copyToCacheDirectory: true,
        multiple: false,
      });
      if (result.canceled || generation !== generationRef.current) {
        return;
      }
      const asset = result.assets[0];
      if (!asset) {
        return;
      }
      const file: DocumentUploadFile = {
        uri: asset.uri,
        name: asset.name,
        mimeType: asset.mimeType,
        size: asset.size,
      };
      const validationError = documentUploadError(file);
      if (validationError) {
        setUploadError(validationError);
        return;
      }
      const uploaded = await uploadDocument(knowledgeBaseId, file);
      if (generation !== generationRef.current) {
        return;
      }
      updateDocuments(prependUploadedDocument(documentsRef.current, uploaded));
      setDocumentState('ready');
      setKnowledgeBase((current) => {
        if (!current) {
          return current;
        }
        const next = { ...current, doc_count: Math.max(0, current.doc_count ?? 0) + 1 };
        knowledgeBaseRef.current = next;
        return next;
      });
    } catch (caught) {
      if (generation === generationRef.current) {
        setUploadError(errorMessage(caught, '文档上传失败，请稍后重试。'));
        await handleAuthError(caught);
      }
    } finally {
      uploadingRef.current = false;
      setUploading(false);
    }
  }, [handleAuthError, knowledgeBaseId, updateDocuments]);

  const pollDocumentStatuses = useCallback(async () => {
    if (pollingRef.current) {
      return;
    }
    const activeDocuments = documentsRef.current.items.filter(isDocumentProcessing);
    if (activeDocuments.length === 0) {
      return;
    }
    pollingRef.current = true;
    const generation = generationRef.current;
    try {
      const results = await Promise.allSettled(
        activeDocuments.map((item) => getDocumentStatus(item.id)),
      );
      if (generation !== generationRef.current) {
        return;
      }
      const updates = new Map<string, DocumentStatusResponse>();
      results.forEach((result, index) => {
        if (result.status === 'fulfilled') {
          updates.set(activeDocuments[index].id, result.value);
        }
      });
      let nextDocuments = documentsRef.current;
      const completed = [...updates.entries()].filter(([, status]) => status.status === 'done');
      const nonCompletedUpdates = new Map(
        [...updates.entries()].filter(([, status]) => status.status !== 'done'),
      );
      if (nonCompletedUpdates.size > 0) {
        nextDocuments = applyDocumentStatuses(nextDocuments, nonCompletedUpdates);
      }
      if (completed.length > 0) {
        try {
          const refreshed = await listDocuments(knowledgeBaseId, 1, PAGE_SIZE);
          if (generation !== generationRef.current) {
            return;
          }
          nextDocuments = refreshDocumentRecords(
            applyDocumentStatuses(nextDocuments, new Map(completed)),
            refreshed,
          );
          setDocumentError('');
        } catch (caught) {
          setDocumentError(errorMessage(caught, '文档已处理完成，片段信息刷新失败。'));
          await handleAuthError(caught);
        }
      }
      if (updates.size > 0) {
        updateDocuments(nextDocuments);
      }
      const rejected = results.find((result) => result.status === 'rejected');
      if (rejected?.status === 'rejected') {
        await handleAuthError(rejected.reason);
      }
    } finally {
      pollingRef.current = false;
    }
  }, [handleAuthError, knowledgeBaseId, updateDocuments]);

  const hasProcessingDocuments = documents.items.some(isDocumentProcessing);
  useEffect(() => {
    if (!hasProcessingDocuments) {
      return;
    }
    const timer = setInterval(() => {
      void pollDocumentStatuses();
    }, DOCUMENT_STATUS_POLL_INTERVAL);
    return () => clearInterval(timer);
  }, [hasProcessingDocuments, pollDocumentStatuses]);

  const title = knowledgeBase?.name || firstParam(params.name) || '知识库详情';

  return (
    <SafeAreaView edges={['bottom']} style={[styles.safeArea, { backgroundColor: palette.page }]}>
      <Stack.Screen
        options={{
          title,
          headerBackVisible: false,
          headerShadowVisible: false,
          headerStyle: { backgroundColor: palette.page },
          headerTintColor: palette.accent,
          headerTitleStyle: { color: palette.text, fontSize: 17, fontWeight: '600' },
        }}
      />
      <Stack.Toolbar placement="left">
        <Stack.Toolbar.Button
          accessibilityLabel="返回知识库列表"
          icon="chevron.left"
          hidesSharedBackground={false}
          separateBackground
          tintColor={palette.accent}
          onPress={() => router.back()}
        />
      </Stack.Toolbar>
      <Stack.Toolbar placement="right">
        <Stack.Toolbar.Button
          accessibilityLabel={uploading ? '正在上传文档' : '上传文档'}
          disabled={uploading || knowledgeState !== 'ready'}
          icon="square.and.arrow.up"
          hidesSharedBackground={false}
          separateBackground
          tintColor={palette.accent}
          onPress={() => void selectAndUploadDocument()}
        />
      </Stack.Toolbar>
      <FlatList
        data={documents.items}
        keyExtractor={(item, index) => item.id || `document-${index}`}
        contentInsetAdjustmentBehavior="automatic"
        contentContainerStyle={styles.listContent}
        onEndReached={() => void loadMore()}
        onEndReachedThreshold={0.35}
        refreshControl={(
          <RefreshControl
            refreshing={refreshing}
            tintColor={palette.accent}
            onRefresh={() => void refresh()}
          />
        )}
        ListHeaderComponent={(
          <View>
            <KnowledgeSummary
              knowledgeBase={knowledgeBase}
              state={knowledgeState}
              error={knowledgeError}
              palette={palette}
              onRetry={() => void loadInitial()}
            />
            <View style={styles.documentHeading}>
              <Text style={[styles.documentTitle, { color: palette.text }]}>文档</Text>
              <Text style={[styles.documentCount, { color: palette.textMuted }]}>共 {documents.total} 篇</Text>
            </View>
          </View>
        )}
        ListEmptyComponent={(
          <DocumentEmptyState
            state={documentState}
            error={documentError}
            uploading={uploading}
            palette={palette}
            onRetry={() => void loadInitial()}
            onUpload={() => void selectAndUploadDocument()}
          />
        )}
        ItemSeparatorComponent={() => <View style={styles.separator} />}
        ListFooterComponent={(
          <DocumentFooter
            loading={loadingMore}
            error={loadMoreError}
            hasItems={documents.items.length > 0}
            hasMore={documents.items.length < documents.total}
            total={documents.total}
            palette={palette}
            onRetry={() => void loadMore()}
          />
        )}
        renderItem={({ item, index }) => (
          <DocumentRow item={item} index={index} palette={palette} />
        )}
      />
      {(knowledgeError || documentError)
        && knowledgeState === 'ready'
        && documentState === 'ready' ? (
        <View
          accessibilityLiveRegion="polite"
          style={[styles.notice, { backgroundColor: palette.dangerSurface }]}
          testID="knowledge-detail-refresh-error">
          <Text numberOfLines={2} style={[styles.noticeText, { color: palette.danger }]}>
            {knowledgeError || documentError}
          </Text>
        </View>
      ) : null}
      {uploading ? (
        <View
          accessibilityLiveRegion="polite"
          style={[styles.uploadNotice, { backgroundColor: palette.surface, borderColor: palette.border }]}
          testID="knowledge-document-uploading">
          <ActivityIndicator size="small" color={palette.accent} />
          <Text style={[styles.uploadNoticeText, { color: palette.textSecondary }]}>正在上传文档…</Text>
        </View>
      ) : uploadError ? (
        <Pressable
          accessibilityRole="button"
          accessibilityLabel={`${uploadError}，点击关闭`}
          onPress={() => setUploadError('')}
          style={[
            styles.uploadNotice,
            { backgroundColor: palette.dangerSurface, borderColor: palette.danger },
          ]}
          testID="knowledge-document-upload-error">
          <SymbolView name="exclamationmark.circle.fill" size={16} tintColor={palette.danger} weight="semibold" />
          <Text numberOfLines={2} style={[styles.uploadNoticeText, { color: palette.danger }]}>{uploadError}</Text>
        </Pressable>
      ) : null}
    </SafeAreaView>
  );
}

function KnowledgeSummary({
  knowledgeBase,
  state,
  error,
  palette,
  onRetry,
}: {
  knowledgeBase: KnowledgeBase | null;
  state: LoadState;
  error: string;
  palette: Palette;
  onRetry: () => void;
}) {
  if (!knowledgeBase && state === 'loading') {
    return <KnowledgeSummarySkeleton palette={palette} />;
  }
  if (!knowledgeBase && state === 'error') {
    return (
      <View style={[styles.summaryCard, styles.summaryError, { backgroundColor: palette.surface, borderColor: palette.border }]}>
        <SymbolView name="exclamationmark.circle" size={24} tintColor={palette.danger} weight="medium" />
        <Text style={[styles.summaryErrorText, { color: palette.textMuted }]}>{error}</Text>
        <Pressable
          accessibilityRole="button"
          accessibilityLabel="重新加载知识库信息"
          onPress={onRetry}
          testID="knowledge-detail-summary-retry"
          style={({ pressed }) => pressed && styles.pressed}>
          <Text style={[styles.inlineAction, { color: palette.accent }]}>重试</Text>
        </Pressable>
      </View>
    );
  }
  if (!knowledgeBase) {
    return null;
  }

  const iconColor = validColor(knowledgeBase.color) ? knowledgeBase.color : palette.accent;
  const documentCount = Math.max(0, knowledgeBase.doc_count ?? 0);
  return (
    <View style={[styles.summaryCard, { backgroundColor: palette.surface, borderColor: palette.border }]}>
      <View style={[styles.summaryIcon, { backgroundColor: `${iconColor}16`, borderColor: `${iconColor}2E` }]}>
        <SymbolView name="books.vertical.fill" size={28} tintColor={iconColor} weight="semibold" />
      </View>
      <View style={styles.summaryBody}>
        <View style={styles.summaryHeading}>
          <Text numberOfLines={2} style={[styles.summaryName, { color: palette.text }]}>{knowledgeBase.name}</Text>
          {knowledgeBase.is_default ? (
            <View style={[styles.defaultBadge, { backgroundColor: `${palette.accent}14` }]}>
              <SymbolView name="star.fill" size={10} tintColor={palette.accent} weight="semibold" />
              <Text style={[styles.defaultLabel, { color: palette.accent }]}>默认</Text>
            </View>
          ) : null}
        </View>
        <Text numberOfLines={3} style={[styles.summaryDescription, { color: palette.textMuted }]}>
          {knowledgeBase.description?.trim() || '暂无描述'}
        </Text>
        <View style={styles.summaryMeta}>
          <Text style={[styles.summaryMetaText, { color: palette.textSecondary }]}>{documentCount} 篇文档</Text>
          <View style={[styles.metaDot, { backgroundColor: palette.borderStrong }]} />
          <Text style={[styles.summaryMetaText, { color: palette.textSecondary }]}>
            {knowledgeBase.chat_enabled ? '聊天已启用' : '聊天未启用'}
          </Text>
        </View>
      </View>
    </View>
  );
}

function KnowledgeSummarySkeleton({ palette }: { palette: Palette }) {
  return (
    <View accessibilityLabel="正在加载知识库信息" style={[styles.summaryCard, { backgroundColor: palette.surface, borderColor: palette.border }]}>
      <View style={[styles.summaryIcon, { backgroundColor: palette.surfaceMuted }]} />
      <View style={styles.summaryBody}>
        <View style={[styles.skeletonTitle, { backgroundColor: palette.surfaceMuted }]} />
        <View style={[styles.skeletonLine, { backgroundColor: palette.surfaceMuted }]} />
        <View style={[styles.skeletonMeta, { backgroundColor: palette.surfaceMuted }]} />
      </View>
    </View>
  );
}

function DocumentEmptyState({
  state,
  error,
  uploading,
  palette,
  onRetry,
  onUpload,
}: {
  state: LoadState;
  error: string;
  uploading: boolean;
  palette: Palette;
  onRetry: () => void;
  onUpload: () => void;
}) {
  if (state === 'loading') {
    return <DocumentSkeleton palette={palette} />;
  }
  if (state === 'error') {
    return (
      <View style={styles.emptyState}>
        <View style={[styles.emptyIcon, { backgroundColor: palette.dangerSurface }]}>
          <SymbolView name="wifi.exclamationmark" size={25} tintColor={palette.danger} weight="medium" />
        </View>
        <Text style={[styles.emptyTitle, { color: palette.text }]}>暂时无法加载文档</Text>
        <Text style={[styles.emptyMessage, { color: palette.textMuted }]}>{error}</Text>
        <Pressable
          accessibilityRole="button"
          accessibilityLabel="重新加载文档"
          onPress={onRetry}
          testID="knowledge-documents-retry"
          style={({ pressed }) => [
            styles.retryButton,
            { backgroundColor: palette.accent },
            pressed && styles.pressed,
          ]}>
          <Text style={[styles.retryLabel, { color: palette.accentText }]}>重新加载</Text>
        </Pressable>
      </View>
    );
  }
  return (
    <View style={styles.emptyState}>
      <View style={[styles.emptyIcon, { backgroundColor: palette.surfaceMuted, borderColor: palette.border }]}>
        <SymbolView name="doc.text" size={27} tintColor={palette.accent} weight="medium" />
      </View>
      <Text style={[styles.emptyTitle, { color: palette.text }]}>还没有文档</Text>
      <Text style={[styles.emptyMessage, { color: palette.textMuted }]}>上传资料后，Cove 会自动解析并加入这个知识库。</Text>
      <Pressable
        accessibilityRole="button"
        accessibilityLabel="选择要上传的文档"
        accessibilityState={{ disabled: uploading, busy: uploading }}
        disabled={uploading}
        onPress={onUpload}
        style={({ pressed }) => [
          styles.uploadButton,
          { backgroundColor: palette.accent },
          pressed && !uploading && styles.pressed,
        ]}
        testID="knowledge-document-upload-empty">
        {uploading ? (
          <ActivityIndicator size="small" color={palette.accentText} />
        ) : (
          <SymbolView name="square.and.arrow.up" size={15} tintColor={palette.accentText} weight="semibold" />
        )}
        <Text style={[styles.uploadButtonLabel, { color: palette.accentText }]}>选择文档</Text>
      </Pressable>
    </View>
  );
}

function DocumentSkeleton({ palette }: { palette: Palette }) {
  return (
    <View accessibilityLabel="正在加载文档" style={styles.skeletonList}>
      {[0, 1, 2].map((index) => (
        <View key={index} style={[styles.documentCard, { backgroundColor: palette.surface, borderColor: palette.border }]}>
          <View style={[styles.documentIcon, { backgroundColor: palette.surfaceMuted }]} />
          <View style={styles.documentBody}>
            <View style={[styles.documentSkeletonTitle, { width: index === 1 ? '58%' : '72%', backgroundColor: palette.surfaceMuted }]} />
            <View style={[styles.documentSkeletonMeta, { backgroundColor: palette.surfaceMuted }]} />
          </View>
        </View>
      ))}
    </View>
  );
}

function DocumentRow({ item, index, palette }: { item: KnowledgeDocument; index: number; palette: Palette }) {
  const status = documentStatus(item, palette);
  const details = documentDetails(item);
  return (
    <View
      accessible
      accessibilityLabel={`${item.file_name}，${status.label}，${details}`}
      style={[styles.documentCard, { backgroundColor: palette.surface, borderColor: palette.border }]}
      testID={`knowledge-document-${item.id || index}`}>
      <View style={[styles.documentIcon, { backgroundColor: `${status.color}12` }]}>
        <SymbolView name="doc.text.fill" size={21} tintColor={status.color} weight="medium" />
      </View>
      <View style={styles.documentBody}>
        <View style={styles.documentTopline}>
          <Text numberOfLines={1} style={[styles.documentName, { color: palette.text }]}>{item.file_name}</Text>
          <View style={[styles.statusBadge, { backgroundColor: `${status.color}12` }]}>
            {status.loading ? (
              <ActivityIndicator size="small" color={status.color} />
            ) : (
              <SymbolView name={status.icon} size={12} tintColor={status.color} weight="semibold" />
            )}
            <Text style={[styles.statusLabel, { color: status.color }]}>{status.label}</Text>
          </View>
        </View>
        <Text numberOfLines={1} style={[styles.documentMeta, { color: palette.textMuted }]}>{details}</Text>
        {status.kind === 'failed' && item.error_msg ? (
          <Text numberOfLines={2} style={[styles.documentError, { color: palette.danger }]}>{item.error_msg}</Text>
        ) : null}
        {status.kind === 'processing' && typeof item.progress === 'number' ? (
          <View style={[styles.progressTrack, { backgroundColor: palette.surfaceMuted }]}>
            <View style={[styles.progressFill, { width: `${progressPercent(item.progress)}%`, backgroundColor: palette.accent }]} />
          </View>
        ) : null}
      </View>
    </View>
  );
}

function DocumentFooter({
  loading,
  error,
  hasItems,
  hasMore,
  total,
  palette,
  onRetry,
}: {
  loading: boolean;
  error: string;
  hasItems: boolean;
  hasMore: boolean;
  total: number;
  palette: Palette;
  onRetry: () => void;
}) {
  if (!hasItems) {
    return null;
  }
  if (loading) {
    return <ActivityIndicator style={styles.footer} color={palette.accent} />;
  }
  if (error) {
    return (
      <Pressable
        accessibilityRole="button"
        accessibilityLabel="重试加载更多文档"
        onPress={onRetry}
        testID="knowledge-documents-load-more-retry"
        style={({ pressed }) => [styles.footer, pressed && styles.pressed]}>
        <Text style={[styles.footerError, { color: palette.danger }]}>{error} 点击重试</Text>
      </Pressable>
    );
  }
  if (!hasMore) {
    return <Text style={[styles.footerEnd, { color: palette.textMuted }]}>共 {total} 篇，已全部加载</Text>;
  }
  return null;
}

function cachedKnowledgeBase(knowledgeBaseId: string, params: RouteParams): KnowledgeBase | null {
  const name = firstParam(params.name);
  if (!knowledgeBaseId || !name) {
    return null;
  }
  return {
    id: knowledgeBaseId,
    name,
    description: firstParam(params.description) || null,
    color: firstParam(params.color) || null,
    doc_count: Math.max(0, Number(firstParam(params.doc_count) || firstParam(params.docCount)) || 0),
    chat_enabled: (firstParam(params.chat_enabled) || firstParam(params.chatEnabled)) === 'true',
  };
}

function firstParam(value: string | string[] | undefined): string {
  return Array.isArray(value) ? (value[0] ?? '') : (value ?? '');
}

function errorMessage(caught: unknown, fallback: string): string {
  return caught instanceof Error ? caught.message : fallback;
}

function validColor(value: string | null | undefined): value is string {
  return Boolean(value && /^#[0-9a-f]{6}$/i.test(value));
}

function documentStatus(item: KnowledgeDocument, palette: Palette): {
  kind: 'waiting' | 'processing' | 'done' | 'failed';
  label: string;
  icon: 'clock.fill' | 'arrow.triangle.2.circlepath' | 'checkmark.circle.fill' | 'exclamationmark.circle.fill';
  color: string;
  loading: boolean;
} {
  switch (item.status) {
    case 'done':
      return { kind: 'done', label: '已就绪', icon: 'checkmark.circle.fill', color: palette.accent, loading: false };
    case 'failed':
      return { kind: 'failed', label: '处理失败', icon: 'exclamationmark.circle.fill', color: palette.danger, loading: false };
    case 'parsing':
      return {
        kind: 'processing',
        label: typeof item.progress === 'number' ? `解析中 ${progressPercent(item.progress)}%` : '解析中',
        icon: 'arrow.triangle.2.circlepath',
        color: palette.accent,
        loading: true,
      };
    default:
      return { kind: 'waiting', label: '等待处理', icon: 'clock.fill', color: palette.textMuted, loading: false };
  }
}

function documentDetails(item: KnowledgeDocument): string {
  const details: string[] = [];
  if (item.file_ext) {
    details.push(item.file_ext.replace(/^\./, '').toUpperCase());
  }
  if (typeof item.file_size === 'number' && item.file_size >= 0) {
    details.push(formatFileSize(item.file_size));
  }
  if (typeof item.chunk_num === 'number' && item.chunk_num > 0) {
    details.push(`${item.chunk_num} 个片段`);
  }
  return details.join(' · ') || '文档';
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) {
    return `${bytes} B`;
  }
  if (bytes < 1024 * 1024) {
    return `${(bytes / 1024).toFixed(bytes < 10 * 1024 ? 1 : 0)} KB`;
  }
  return `${(bytes / (1024 * 1024)).toFixed(bytes < 10 * 1024 * 1024 ? 1 : 0)} MB`;
}

function progressPercent(progress: number): number {
  const normalized = progress > 1 ? progress : progress * 100;
  return Math.round(Math.max(0, Math.min(100, normalized)));
}

function isDocumentProcessing(item: KnowledgeDocument): boolean {
  return item.status === 'pending' || item.status === 'parsing';
}

const styles = StyleSheet.create({
  safeArea: { flex: 1 },
  listContent: { flexGrow: 1, paddingHorizontal: 16, paddingTop: 18, paddingBottom: 28 },
  summaryCard: {
    minHeight: 128,
    padding: 16,
    borderRadius: 22,
    borderWidth: StyleSheet.hairlineWidth,
    flexDirection: 'row',
    alignItems: 'flex-start',
  },
  summaryIcon: {
    width: 58,
    height: 58,
    borderRadius: 18,
    borderWidth: StyleSheet.hairlineWidth,
    alignItems: 'center',
    justifyContent: 'center',
  },
  summaryBody: { minWidth: 0, flex: 1, marginLeft: 14 },
  summaryHeading: { minHeight: 28, flexDirection: 'row', alignItems: 'flex-start', gap: 8 },
  summaryName: { minWidth: 0, flex: 1, fontSize: 20, lineHeight: 26, fontWeight: '700', letterSpacing: -0.3 },
  summaryDescription: { marginTop: 4, fontSize: 13, lineHeight: 19 },
  summaryMeta: { marginTop: 10, flexDirection: 'row', alignItems: 'center', gap: 7 },
  summaryMetaText: { fontSize: 11, lineHeight: 15, fontWeight: '600' },
  metaDot: { width: 3, height: 3, borderRadius: 2 },
  defaultBadge: { height: 24, paddingHorizontal: 8, borderRadius: 8, flexDirection: 'row', alignItems: 'center', gap: 4 },
  defaultLabel: { fontSize: 10, lineHeight: 13, fontWeight: '700', includeFontPadding: false },
  summaryError: { minHeight: 104, alignItems: 'center', justifyContent: 'center', gap: 8 },
  summaryErrorText: { flex: 1, fontSize: 13, lineHeight: 18 },
  inlineAction: { fontSize: 13, lineHeight: 18, fontWeight: '700' },
  skeletonTitle: { width: '56%', height: 16, marginTop: 3, borderRadius: 6 },
  skeletonLine: { width: '88%', height: 10, marginTop: 12, borderRadius: 5 },
  skeletonMeta: { width: 128, height: 9, marginTop: 13, borderRadius: 5 },
  documentHeading: { height: 51, paddingTop: 22, flexDirection: 'row', alignItems: 'center' },
  documentTitle: { flex: 1, fontSize: 19, lineHeight: 25, fontWeight: '700', letterSpacing: -0.2 },
  documentCount: { fontSize: 12, lineHeight: 17, fontWeight: '600' },
  separator: { height: 10 },
  documentCard: {
    minHeight: 78,
    padding: 13,
    borderRadius: 17,
    borderWidth: StyleSheet.hairlineWidth,
    flexDirection: 'row',
    alignItems: 'flex-start',
  },
  documentIcon: { width: 42, height: 42, borderRadius: 13, alignItems: 'center', justifyContent: 'center' },
  documentBody: { minWidth: 0, flex: 1, marginLeft: 12 },
  documentTopline: { minHeight: 24, flexDirection: 'row', alignItems: 'center', gap: 8 },
  documentName: { minWidth: 0, flex: 1, fontSize: 14, lineHeight: 20, fontWeight: '600' },
  documentMeta: { marginTop: 3, fontSize: 11, lineHeight: 15, fontWeight: '500' },
  statusBadge: {
    minHeight: 23,
    paddingHorizontal: 7,
    borderRadius: 8,
    flexDirection: 'row',
    alignItems: 'center',
    gap: 4,
  },
  statusLabel: { fontSize: 10, lineHeight: 13, fontWeight: '700', includeFontPadding: false },
  documentError: { marginTop: 5, fontSize: 11, lineHeight: 15 },
  progressTrack: { height: 3, marginTop: 8, borderRadius: 2, overflow: 'hidden' },
  progressFill: { height: 3, borderRadius: 2 },
  emptyState: { minHeight: 290, paddingHorizontal: 24, alignItems: 'center', justifyContent: 'center' },
  emptyIcon: {
    width: 62,
    height: 62,
    borderRadius: 20,
    borderWidth: StyleSheet.hairlineWidth,
    alignItems: 'center',
    justifyContent: 'center',
  },
  emptyTitle: { marginTop: 17, fontSize: 17, lineHeight: 23, fontWeight: '700', textAlign: 'center' },
  emptyMessage: { maxWidth: 270, marginTop: 6, fontSize: 13, lineHeight: 19, textAlign: 'center' },
  retryButton: { height: 42, marginTop: 18, paddingHorizontal: 17, borderRadius: 13, alignItems: 'center', justifyContent: 'center' },
  retryLabel: { fontSize: 13, lineHeight: 18, fontWeight: '700' },
  uploadButton: {
    minHeight: 42,
    marginTop: 18,
    paddingHorizontal: 17,
    borderRadius: 13,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 7,
  },
  uploadButtonLabel: { fontSize: 13, lineHeight: 18, fontWeight: '700' },
  skeletonList: { gap: 10 },
  documentSkeletonTitle: { height: 12, marginTop: 3, borderRadius: 6 },
  documentSkeletonMeta: { width: 110, height: 8, marginTop: 10, borderRadius: 4 },
  footer: { minHeight: 48, alignItems: 'center', justifyContent: 'center' },
  footerError: { paddingVertical: 15, fontSize: 12, lineHeight: 17, textAlign: 'center' },
  footerEnd: { paddingTop: 18, fontSize: 11, lineHeight: 16, textAlign: 'center' },
  notice: { position: 'absolute', right: 16, bottom: 14, left: 16, paddingHorizontal: 12, paddingVertical: 9, borderRadius: 11 },
  noticeText: { fontSize: 12, lineHeight: 17, textAlign: 'center' },
  uploadNotice: {
    position: 'absolute',
    right: 16,
    bottom: 14,
    left: 16,
    minHeight: 42,
    paddingHorizontal: 13,
    paddingVertical: 10,
    borderRadius: 13,
    borderWidth: StyleSheet.hairlineWidth,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 8,
  },
  uploadNoticeText: { flexShrink: 1, fontSize: 12, lineHeight: 17, fontWeight: '600' },
  pressed: { opacity: 0.6, transform: [{ scale: 0.98 }] },
});
