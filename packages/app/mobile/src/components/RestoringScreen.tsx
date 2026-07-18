import { ActivityIndicator, Image, StyleSheet, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { usePalette } from '@/theme/palette';

const coveIcon = require('../../assets/images/icon.png');

export function RestoringScreen() {
  const palette = usePalette();
  return (
    <SafeAreaView style={[styles.safeArea, { backgroundColor: palette.page }]}>
      <View style={styles.content}>
        <Image source={coveIcon} style={styles.icon} accessibilityIgnoresInvertColors />
        <Text style={[styles.brand, { color: palette.text }]}>Cove</Text>
        <ActivityIndicator color={palette.accent} style={styles.indicator} />
      </View>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safeArea: { flex: 1 },
  content: { flex: 1, alignItems: 'center', justifyContent: 'center' },
  icon: { width: 82, height: 82, borderRadius: 20 },
  brand: { marginTop: 16, fontSize: 24, fontWeight: '700', letterSpacing: 0.2 },
  indicator: { marginTop: 22 },
});
