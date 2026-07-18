import { useEffect, useRef, useState } from 'react';
import {
  AccessibilityInfo,
  ActivityIndicator,
  Animated,
  Easing,
  StyleSheet,
  Text,
  View,
} from 'react-native';

import { BrandMark } from '@/components/BrandMark';
import { MarkdownMessage } from '@/components/MarkdownMessage';
import { messageTimeline } from '@/core/chatMessages';
import type { ChatMessage, MessagePart } from '@/core/types';
import { usePalette } from '@/theme/palette';

function ToolEvent({
  call,
  result,
  running,
}: {
  call: MessagePart | null;
  result: MessagePart | null;
  running: boolean;
}) {
  const palette = usePalette();
  const tool = call?.tool || result?.tool || '未知工具';
  const failed = Boolean(result?.error);

  return (
    <View
      accessibilityRole="text"
      accessibilityLabel={`${running ? '正在使用工具' : failed ? '工具调用失败' : '工具已完成'} ${tool}`}
      style={[
        styles.toolEvent,
        { backgroundColor: palette.surfaceMuted, borderColor: palette.border },
      ]}>
      {running ? (
        <ActivityIndicator color={palette.accent} size={14} />
      ) : (
        <View
          style={[
            styles.toolStatus,
            { borderColor: failed ? palette.danger : palette.accent },
          ]}>
          <Text style={[styles.toolStatusGlyph, { color: failed ? palette.danger : palette.accent }]}>
            {failed ? '!' : '✓'}
          </Text>
        </View>
      )}
      <Text numberOfLines={1} style={[styles.toolName, { color: palette.text }]}>
        {tool}
      </Text>
    </View>
  );
}

function ThinkingIndicator() {
  const palette = usePalette();
  const progress = useRef(new Animated.Value(0)).current;
  const [reduceMotion, setReduceMotion] = useState(false);

  useEffect(() => {
    let mounted = true;
    void AccessibilityInfo.isReduceMotionEnabled().then((enabled) => {
      if (mounted) {
        setReduceMotion(enabled);
      }
    });
    const subscription = AccessibilityInfo.addEventListener('reduceMotionChanged', setReduceMotion);
    return () => {
      mounted = false;
      subscription.remove();
    };
  }, []);

  useEffect(() => {
    progress.stopAnimation();
    progress.setValue(0);
    if (reduceMotion) {
      return;
    }
    const animation = Animated.loop(
      Animated.timing(progress, {
        toValue: 1,
        duration: 1050,
        easing: Easing.linear,
        useNativeDriver: true,
      }),
    );
    animation.start();
    return () => animation.stop();
  }, [progress, reduceMotion]);

  function dotStyle(index: number, distance: number) {
    const start = 0.04 + index * 0.14;
    const peak = start + 0.08;
    const end = start + 0.16;
    return {
      opacity: progress.interpolate({
        inputRange: [0, start, peak, end, 1],
        outputRange: [0.62, 0.62, 1, 0.62, 0.62],
      }),
      transform: [{
        translateY: progress.interpolate({
          inputRange: [0, start, peak, end, 1],
          outputRange: [0, 0, -distance, 0, 0],
        }),
      }],
    };
  }

  return (
    <View accessibilityRole="text" accessibilityLabel="Cove 正在思考" style={styles.thinking}>
      <Animated.Text
        style={[
          styles.thinkingLabel,
          { color: palette.accent },
          reduceMotion ? undefined : dotStyle(0, 2.5),
        ]}>
        Think
      </Animated.Text>
      <View style={styles.thinkingDots}>
        {[0, 1, 2].map((index) => (
          <Animated.View
            key={index}
            style={[
              styles.thinkingDot,
              { backgroundColor: palette.accent },
              reduceMotion ? undefined : dotStyle(index + 1, 3.5),
            ]}
          />
        ))}
      </View>
    </View>
  );
}

export function AssistantMessage({ message, streaming }: { message: ChatMessage; streaming: boolean }) {
  const palette = usePalette();
  const timeline = messageTimeline(message);
  const isThinking = Boolean(message.thinking?.active) || (message.pending === true && timeline.length === 0);
  const compact = message.pending === true;

  return (
    <View style={styles.row}>
      <BrandMark size={compact ? 28 : 36} />
      <View style={[styles.content, !compact && styles.contentCompleted]}>
        {timeline.map((item, index) => {
          const previous = timeline[index - 1];
          const spacing = index === 0
            ? undefined
            : item.kind === 'text' && previous?.kind === 'tool'
              ? styles.textAfterTool
              : styles.partSpacing;
          return item.kind === 'text' ? (
            <View key={item.key} style={spacing}>
              <MarkdownMessage markdown={item.part.text ?? ''} streaming={message.pending} />
            </View>
          ) : (
            <View key={item.key} style={spacing}>
              <ToolEvent
                call={item.call}
                result={item.result}
                running={message.pending === true && streaming && !item.result}
              />
            </View>
          );
        })}
        {isThinking ? (
          <View style={timeline.length ? styles.thinkingAfterContent : undefined}>
            <ThinkingIndicator />
          </View>
        ) : null}
        {message.meta_data?.interrupted ? (
          <Text accessibilityRole="text" style={[styles.interrupted, { color: palette.textMuted }]}>
            回复已中断
          </Text>
        ) : null}
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  row: { width: '100%', flexDirection: 'row', alignItems: 'flex-start', gap: 8 },
  content: { minWidth: 0, flex: 1 },
  contentCompleted: { paddingTop: 2 },
  partSpacing: { marginTop: 12 },
  textAfterTool: { marginTop: 12 },
  toolEvent: {
    alignSelf: 'flex-start',
    height: 38,
    maxWidth: '100%',
    paddingHorizontal: 11,
    borderRadius: 11,
    borderWidth: StyleSheet.hairlineWidth,
    flexDirection: 'row',
    alignItems: 'center',
    gap: 7,
  },
  toolStatus: {
    width: 14,
    height: 14,
    borderRadius: 7,
    borderWidth: 1.5,
    alignItems: 'center',
    justifyContent: 'center',
  },
  toolStatusGlyph: { fontSize: 8, lineHeight: 10, fontWeight: '700' },
  toolName: { flexShrink: 1, fontSize: 13, lineHeight: 16 },
  thinkingAfterContent: { marginTop: 10 },
  thinking: { height: 28, flexDirection: 'row', alignItems: 'center', gap: 4 },
  thinkingDots: { flexDirection: 'row', gap: 4 },
  thinkingDot: { width: 5, height: 5, borderRadius: 2.5 },
  thinkingLabel: { fontSize: 13, lineHeight: 18 },
  interrupted: { marginTop: 8, fontSize: 12, lineHeight: 18 },
});
