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
import { Stack, useRouter } from 'expo-router';
import { SymbolView } from 'expo-symbols';
import { SafeAreaView } from 'react-native-safe-area-context';

import { ApiError } from '@/core/api';
import { listKnowledgeBases, setDefaultKnowledgeBase, type KnowledgeBase } from '@/core/knowledge';
import { loadStoredSession } from '@/core/session';
import { useAuth } from '@/providers/AuthProvider';
import { usePalette, type Palette } from '@/theme/palette';

type PageState = 'loading' | 'ready' | 'error';

export default function KnowledgeScreen() {
  const palette = usePalette();
  const router = useRouter();
  const { signOut } = useAuth();
  const [items, setItems] = useState<KnowledgeBase[]>([]);
  const [state, setState] = useState<PageState>('loading');
  const [refreshing, setRefreshing] = useState(false);
  const [settingDefaultID, setSettingDefaultID] = useState<string | null>(null);
  const [error, setError] = useState('');
  const settingDefaultRef = useRef(false);

  const load = useCallback(async (refresh = false) => {
    if (refresh) {
      setRefreshing(true);
    } else {
      setState('loading');
    }
    setError('');
    try {
      const response = await listKnowledgeBases();
      setItems(response.list.filter((item) => item.id && item.name));
      setState('ready');
    } catch (caught) {
      const message = caught instanceof Error ? caught.message : '知识库加载失败，请稍后重试。';
      setError(message);
      if (!refresh) {
        setState('error');
      }
      if (caught instanceof ApiError && caught.status === 401) {
        const storedSession = await loadStoredSession();
        if (!storedSession) {
          await signOut();
        }
      }
    } finally {
      setRefreshing(false);
    }
  }, [signOut]);

  useEffect(() => {
    void load();
  }, [load]);

  const handleSetDefault = useCallback(async (item: KnowledgeBase) => {
    if (item.is_default || settingDefaultRef.current) {
      return;
    }
    settingDefaultRef.current = true;
    setSettingDefaultID(item.id);
    setError('');
    try {
      const updated = await setDefaultKnowledgeBase(item.id);
      setItems((current) => current.map((currentItem) => (
        currentItem.id === updated.id
          ? { ...currentItem, ...updated, is_default: true }
          : { ...currentItem, is_default: false }
      )));
    } catch (caught) {
      const message = caught instanceof Error ? caught.message : '设置默认知识库失败，请稍后重试。';
      setError(message);
      if (caught instanceof ApiError && caught.status === 401) {
        const storedSession = await loadStoredSession();
        if (!storedSession) {
          await signOut();
        }
      }
    } finally {
      settingDefaultRef.current = false;
      setSettingDefaultID(null);
    }
  }, [signOut]);

  return (
    <SafeAreaView edges={['bottom']} style={[styles.safeArea, { backgroundColor: palette.page }]}>
      <Stack.Toolbar placement="left">
        <Stack.Toolbar.Button
          accessibilityLabel="返回"
          icon="chevron.left"
          hidesSharedBackground={false}
          separateBackground
          tintColor={palette.accent}
          onPress={() => router.back()}
        />
      </Stack.Toolbar>
      {state === 'loading' ? (
        <KnowledgeSkeleton palette={palette} />
      ) : state === 'error' ? (
        <View style={styles.centerState}>
          <View style={[styles.stateIcon, { backgroundColor: palette.surfaceMuted }]}>
            <SymbolView name="wifi.exclamationmark" size={26} tintColor={palette.textSecondary} weight="medium" />
          </View>
          <Text style={[styles.stateTitle, { color: palette.text }]}>暂时无法加载</Text>
          <Text style={[styles.stateMessage, { color: palette.textMuted }]}>{error}</Text>
          <Pressable
            accessibilityRole="button"
            accessibilityLabel="重新加载知识库"
            onPress={() => void load()}
            style={({ pressed }) => [
              styles.retryButton,
              { backgroundColor: palette.accent },
              pressed && styles.pressed,
            ]}>
            <Text style={[styles.retryLabel, { color: palette.accentText }]}>重新加载</Text>
          </Pressable>
        </View>
      ) : (
        <FlatList
          data={items}
          keyExtractor={(item) => item.id}
          contentInsetAdjustmentBehavior="automatic"
          contentContainerStyle={items.length ? styles.listContent : styles.emptyContent}
          refreshControl={(
            <RefreshControl
              refreshing={refreshing}
              tintColor={palette.accent}
              onRefresh={() => void load(true)}
            />
          )}
          ListHeaderComponent={items.length ? (
            <View style={styles.summary}>
              <Text style={[styles.summaryTitle, { color: palette.text }]}>你的知识库</Text>
              <Text style={[styles.summaryCount, { color: palette.textMuted }]}>
                共 {items.length} 个
              </Text>
            </View>
          ) : null}
          ListEmptyComponent={<KnowledgeEmpty palette={palette} />}
          ItemSeparatorComponent={() => <View style={styles.separator} />}
          renderItem={({ item }) => (
            <KnowledgeRow
              item={item}
              palette={palette}
              isSettingDefault={settingDefaultID === item.id}
              defaultChangeDisabled={settingDefaultID !== null || refreshing}
              onSetDefault={() => void handleSetDefault(item)}
            />
          )}
        />
      )}
      {state === 'ready' && error ? (
        <View style={[styles.refreshError, { backgroundColor: palette.dangerSurface }]}>
          <Text numberOfLines={2} style={[styles.refreshErrorText, { color: palette.danger }]}>{error}</Text>
        </View>
      ) : null}
    </SafeAreaView>
  );
}

function KnowledgeRow({
  item,
  palette,
  isSettingDefault,
  defaultChangeDisabled,
  onSetDefault,
}: {
  item: KnowledgeBase;
  palette: Palette;
  isSettingDefault: boolean;
  defaultChangeDisabled: boolean;
  onSetDefault: () => void;
}) {
  const documentCount = Math.max(0, item.doc_count ?? 0);
  const description = item.description?.trim();
  const iconColor = safeColor(item.color, palette.accent, palette.page);
  const chatStateColor = item.chat_enabled ? palette.accent : palette.textMuted;

  return (
    <View style={[styles.card, { backgroundColor: palette.surface, borderColor: palette.border }]}>
      <View
        style={[
          styles.iconTile,
          { backgroundColor: `${iconColor}16`, borderColor: `${iconColor}2E` },
        ]}>
        <SymbolView name="books.vertical.fill" size={22} tintColor={iconColor} weight="semibold" />
      </View>
      <View style={styles.cardBody}>
        <View style={styles.cardHeading}>
          <Text numberOfLines={1} style={[styles.cardTitle, { color: palette.text }]}>{item.name}</Text>
          {item.is_default ? (
            <View style={[styles.defaultBadge, { backgroundColor: `${palette.accent}14` }]}>
              <SymbolView name="star.fill" size={10} tintColor={palette.accent} weight="semibold" />
              <Text style={[styles.defaultLabel, { color: palette.accent }]}>默认</Text>
            </View>
          ) : (
            <Pressable
              accessibilityRole="button"
              accessibilityLabel={`将${item.name}设为默认知识库`}
              accessibilityHint="设为默认后，聊天会优先使用这个知识库"
              accessibilityState={{ disabled: defaultChangeDisabled, busy: isSettingDefault }}
              disabled={defaultChangeDisabled}
              hitSlop={8}
              onPress={onSetDefault}
              testID={`knowledge-${item.id}-set-default`}
              style={({ pressed }) => [
                styles.defaultAction,
                defaultChangeDisabled && styles.defaultActionDisabled,
                pressed && !defaultChangeDisabled && styles.defaultActionPressed,
              ]}>
              <View style={[styles.defaultActionSurface, { backgroundColor: `${palette.accent}12` }]}>
                {isSettingDefault ? (
                  <ActivityIndicator size="small" color={palette.accent} />
                ) : (
                  <SymbolView name="star" size={16} tintColor={palette.accent} weight="semibold" />
                )}
              </View>
            </Pressable>
          )}
        </View>
        <Text numberOfLines={2} style={[styles.cardDescription, { color: palette.textMuted }]}>
          {description || '暂无描述'}
        </Text>
        <View style={styles.cardMeta}>
          <View style={styles.metaItem}>
            <SymbolView name="doc.text.fill" size={12} tintColor={palette.textMuted} weight="medium" />
            <Text style={[styles.metaText, { color: palette.textSecondary }]}>{documentCount} 篇文档</Text>
          </View>
          <View style={styles.chatState}>
            <SymbolView
              name={item.chat_enabled ? 'checkmark.circle.fill' : 'minus.circle.fill'}
              size={13}
              tintColor={chatStateColor}
              weight="semibold"
            />
            <Text style={[styles.metaText, { color: palette.textSecondary }]}>
              {item.chat_enabled ? '聊天已启用' : '聊天未启用'}
            </Text>
          </View>
        </View>
      </View>
    </View>
  );
}

function KnowledgeEmpty({ palette }: { palette: Palette }) {
  return (
    <View style={styles.centerState}>
      <View style={[styles.emptyIcon, { backgroundColor: palette.surfaceMuted, borderColor: palette.border }]}>
        <SymbolView name="books.vertical" size={34} tintColor={palette.accent} weight="medium" />
      </View>
      <Text style={[styles.stateTitle, { color: palette.text }]}>还没有知识库</Text>
      <Text style={[styles.stateMessage, { color: palette.textMuted }]}>在桌面端添加资料后，会显示在这里并用于聊天。</Text>
    </View>
  );
}

function KnowledgeSkeleton({ palette }: { palette: Palette }) {
  return (
    <View accessibilityLabel="正在加载知识库" style={styles.skeletonContent}>
      <View style={styles.summary}>
        <View style={[styles.skeletonHeading, { backgroundColor: palette.surfaceMuted }]} />
        <View style={[styles.skeletonCount, { backgroundColor: palette.surfaceMuted }]} />
      </View>
      {[0, 1, 2].map((index) => (
        <View
          key={index}
          style={[styles.card, { backgroundColor: palette.surface, borderColor: palette.border }]}>
          <View style={[styles.iconTile, { backgroundColor: palette.surfaceMuted }]} />
          <View style={styles.cardBody}>
            <View style={[styles.skeletonTitle, { width: index === 1 ? '52%' : '64%', backgroundColor: palette.surfaceMuted }]} />
            <View style={[styles.skeletonLine, { width: index === 2 ? '72%' : '88%', backgroundColor: palette.surfaceMuted }]} />
            <View style={[styles.skeletonMeta, { backgroundColor: palette.surfaceMuted }]} />
          </View>
        </View>
      ))}
    </View>
  );
}

function safeColor(value: string | null | undefined, fallback: string, background: string): string {
  if (!value || !/^#[0-9a-f]{6}$/i.test(value) || !/^#[0-9a-f]{6}$/i.test(background)) {
    return fallback;
  }

  const luminance = (hex: string) => {
    const channels = [1, 3, 5].map((offset) => Number.parseInt(hex.slice(offset, offset + 2), 16) / 255);
    const linear = channels.map((channel) => (
      channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4
    ));
    return (0.2126 * linear[0]) + (0.7152 * linear[1]) + (0.0722 * linear[2]);
  };
  const foregroundLuminance = luminance(value);
  const backgroundLuminance = luminance(background);
  const contrast = (Math.max(foregroundLuminance, backgroundLuminance) + 0.05)
    / (Math.min(foregroundLuminance, backgroundLuminance) + 0.05);

  return contrast >= 2.4 ? value : fallback;
}

const styles = StyleSheet.create({
  safeArea: { flex: 1 },
  listContent: { paddingHorizontal: 16, paddingTop: 18, paddingBottom: 28 },
  emptyContent: { flexGrow: 1, paddingHorizontal: 28 },
  summary: { minHeight: 36, marginBottom: 14, flexDirection: 'row', alignItems: 'center' },
  summaryTitle: { flex: 1, fontSize: 21, lineHeight: 27, fontWeight: '700', letterSpacing: -0.25 },
  summaryCount: { fontSize: 12, lineHeight: 17, fontWeight: '600' },
  separator: { height: 12 },
  card: {
    minHeight: 104,
    padding: 14,
    borderRadius: 20,
    borderWidth: StyleSheet.hairlineWidth,
    flexDirection: 'row',
    alignItems: 'flex-start',
  },
  iconTile: {
    width: 46,
    height: 46,
    borderRadius: 15,
    borderWidth: StyleSheet.hairlineWidth,
    alignItems: 'center',
    justifyContent: 'center',
  },
  cardBody: { minWidth: 0, flex: 1, marginLeft: 13 },
  cardHeading: { minHeight: 23, flexDirection: 'row', alignItems: 'center', gap: 8 },
  cardTitle: { minWidth: 0, flex: 1, fontSize: 16, lineHeight: 22, fontWeight: '700', letterSpacing: -0.15 },
  defaultBadge: {
    height: 24,
    paddingHorizontal: 8,
    borderRadius: 8,
    flexDirection: 'row',
    alignItems: 'center',
    gap: 4,
  },
  defaultLabel: { fontSize: 10, lineHeight: 13, fontWeight: '700', includeFontPadding: false },
  defaultAction: {
    width: 30,
    height: 30,
    alignItems: 'center',
    justifyContent: 'center',
  },
  defaultActionSurface: {
    width: 30,
    height: 30,
    borderRadius: 10,
    alignItems: 'center',
    justifyContent: 'center',
  },
  defaultActionDisabled: { opacity: 0.48 },
  defaultActionPressed: { opacity: 0.72, transform: [{ scale: 0.94 }] },
  cardDescription: { minHeight: 19, marginTop: 3, fontSize: 13, lineHeight: 19 },
  cardMeta: { marginTop: 9, flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between' },
  metaItem: { flexDirection: 'row', alignItems: 'center', gap: 5 },
  metaText: { fontSize: 11, lineHeight: 16, fontWeight: '500' },
  chatState: { marginLeft: 12, flexDirection: 'row', alignItems: 'center', gap: 5 },
  centerState: { flex: 1, paddingHorizontal: 24, alignItems: 'center', justifyContent: 'center' },
  stateIcon: { width: 62, height: 62, borderRadius: 20, alignItems: 'center', justifyContent: 'center' },
  emptyIcon: {
    width: 72,
    height: 72,
    borderRadius: 23,
    borderWidth: StyleSheet.hairlineWidth,
    alignItems: 'center',
    justifyContent: 'center',
  },
  stateTitle: { marginTop: 18, fontSize: 18, lineHeight: 24, fontWeight: '700', textAlign: 'center' },
  stateMessage: { maxWidth: 270, marginTop: 7, fontSize: 13, lineHeight: 20, textAlign: 'center' },
  retryButton: { minWidth: 108, height: 44, marginTop: 20, paddingHorizontal: 18, borderRadius: 14, alignItems: 'center', justifyContent: 'center' },
  retryLabel: { fontSize: 14, lineHeight: 20, fontWeight: '700' },
  pressed: { opacity: 0.7, transform: [{ scale: 0.98 }] },
  refreshError: { position: 'absolute', right: 16, bottom: 14, left: 16, paddingHorizontal: 12, paddingVertical: 9, borderRadius: 11 },
  refreshErrorText: { fontSize: 12, lineHeight: 17, textAlign: 'center' },
  skeletonContent: { flex: 1, paddingHorizontal: 16, paddingTop: 16 },
  skeletonHeading: { width: 94, height: 19, borderRadius: 7 },
  skeletonCount: { width: 45, height: 10, marginLeft: 'auto', borderRadius: 5 },
  skeletonTitle: { height: 14, marginTop: 3, borderRadius: 6 },
  skeletonLine: { height: 9, marginTop: 9, borderRadius: 5 },
  skeletonMeta: { width: 105, height: 8, marginTop: 11, borderRadius: 4 },
});
