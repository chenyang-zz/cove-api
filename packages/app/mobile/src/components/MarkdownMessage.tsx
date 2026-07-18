import { useMemo } from 'react';
import * as Device from 'expo-device';
import { Linking, Platform, StyleSheet, useColorScheme } from 'react-native';
import { EnrichedMarkdownText, type MarkdownStyle } from 'react-native-enriched-markdown';

import { stripUnsupportedMarkdownEmoji } from '@/core/markdown';
import { usePalette } from '@/theme/palette';

export function markdownForCurrentDevice(markdown: string) {
  if (Platform.OS !== 'ios' || Device.isDevice) {
    return markdown;
  }
  return stripUnsupportedMarkdownEmoji(markdown);
}

export function MarkdownMessage({ markdown, streaming = false }: { markdown: string; streaming?: boolean }) {
  const palette = usePalette();
  const dark = useColorScheme() === 'dark';
  const renderedMarkdown = useMemo(() => markdownForCurrentDevice(markdown), [markdown]);
  const markdownStyle = useMemo<MarkdownStyle>(() => ({
    paragraph: {
      color: palette.text,
      fontSize: 15,
      lineHeight: 25,
      marginTop: 0,
      marginBottom: 13,
    },
    h1: {
      color: palette.text,
      fontSize: 22,
      lineHeight: 27,
      fontWeight: '600',
      marginTop: 0,
      marginBottom: 9,
    },
    h2: {
      color: palette.text,
      fontSize: 19,
      lineHeight: 25,
      fontWeight: '600',
      marginTop: 0,
      marginBottom: 9,
    },
    h3: {
      color: palette.text,
      fontSize: 17,
      lineHeight: 23,
      fontWeight: '600',
      marginTop: 0,
      marginBottom: 8,
    },
    list: {
      color: palette.text,
      fontSize: 15,
      lineHeight: 25,
      marginTop: 0,
      marginBottom: 10,
      marginLeft: 4,
      gapWidth: 5,
      bulletColor: palette.text,
      markerColor: palette.text,
    },
    blockquote: {
      color: palette.textMuted,
      borderColor: palette.accent,
      backgroundColor: palette.surfaceMuted,
    },
    link: { color: palette.accent, underline: true },
    code: {
      color: dark ? '#B9F3EC' : '#075F5E',
      backgroundColor: palette.surfaceMuted,
      borderColor: palette.border,
    },
    codeBlock: {
      color: palette.text,
      backgroundColor: palette.surfaceMuted,
      borderColor: palette.border,
      borderWidth: StyleSheet.hairlineWidth,
      borderRadius: 10,
      padding: 12,
    },
    table: {
      color: palette.text,
      fontSize: 12,
      lineHeight: 16,
      marginTop: 0,
      marginBottom: 16,
      borderColor: palette.border,
      borderWidth: StyleSheet.hairlineWidth,
      borderRadius: 13,
      cellPaddingHorizontal: 8,
      cellPaddingVertical: 8,
      headerBackgroundColor: palette.surface,
      headerTextColor: palette.textSecondary,
      rowEvenBackgroundColor: palette.surface,
      rowOddBackgroundColor: palette.surface,
    },
  }), [dark, palette]);

  return (
    <EnrichedMarkdownText
      markdown={renderedMarkdown}
      flavor="github"
      markdownStyle={markdownStyle}
      selectable
      streamingAnimation={streaming}
      onLinkPress={({ url }) => {
        void Linking.openURL(url);
      }}
    />
  );
}
