import { forwardRef, useState } from 'react';
import {
  Pressable,
  StyleSheet,
  Text,
  TextInput,
  View,
  type TextInputProps,
} from 'react-native';

import { usePalette } from '@/theme/palette';

type FieldProps = TextInputProps & {
  label: string;
  optional?: string;
  error?: string;
  password?: boolean;
};

export const Field = forwardRef<TextInput, FieldProps>(function Field(
  { label, optional, error, password = false, style, onBlur, onFocus, ...inputProps },
  ref,
) {
  const palette = usePalette();
  const [visible, setVisible] = useState(false);
  const [focused, setFocused] = useState(false);

  return (
    <View style={styles.group}>
      <View style={styles.labelRow}>
        <Text style={[styles.label, { color: palette.text }]}>{label}</Text>
        {optional ? <Text style={[styles.optional, { color: palette.textMuted }]}>{optional}</Text> : null}
      </View>
      <View
        style={[
          styles.inputShell,
          {
            backgroundColor: palette.input,
            borderColor: error ? palette.danger : focused ? palette.accent : palette.borderStrong,
          },
        ]}>
        <TextInput
          ref={ref}
          {...inputProps}
          onBlur={(event) => {
            setFocused(false);
            onBlur?.(event);
          }}
          onFocus={(event) => {
            setFocused(true);
            onFocus?.(event);
          }}
          secureTextEntry={password && !visible}
          placeholderTextColor={palette.textMuted}
          selectionColor={palette.accent}
          style={[styles.input, { color: palette.text }, style]}
        />
        {password ? (
          <Pressable
            accessibilityRole="button"
            accessibilityLabel={visible ? '隐藏密码' : '显示密码'}
            hitSlop={10}
            onPress={() => setVisible((current) => !current)}
            style={({ pressed }) => [styles.reveal, pressed && styles.pressed]}>
            <Text style={[styles.revealText, { color: palette.accent }]}>
              {visible ? '隐藏' : '显示'}
            </Text>
          </Pressable>
        ) : null}
      </View>
      {error ? (
        <Text accessibilityLiveRegion="polite" style={[styles.error, { color: palette.danger }]}>
          {error}
        </Text>
      ) : null}
    </View>
  );
});

const styles = StyleSheet.create({
  group: { gap: 8 },
  labelRow: { minHeight: 18, flexDirection: 'row', alignItems: 'center', gap: 18 },
  label: { fontSize: 14, lineHeight: 18, fontWeight: '600' },
  optional: { fontSize: 12, lineHeight: 15, fontWeight: '500' },
  inputShell: {
    height: 52,
    flexDirection: 'row',
    alignItems: 'center',
    borderWidth: 1,
    borderRadius: 15,
  },
  input: { minWidth: 0, flex: 1, paddingHorizontal: 13, paddingVertical: 12, fontSize: 16 },
  reveal: { alignSelf: 'stretch', justifyContent: 'center', paddingHorizontal: 14 },
  revealText: { fontSize: 13, fontWeight: '600' },
  error: { marginHorizontal: 2, fontSize: 13, lineHeight: 18 },
  pressed: { opacity: 0.55 },
});
