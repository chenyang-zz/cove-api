import { Stack } from 'expo-router';

import { usePalette } from '@/theme/palette';

export default function AuthLayout() {
  const palette = usePalette();
  return (
    <Stack
      initialRouteName="login"
      screenOptions={{
        animation: 'default',
        contentStyle: { backgroundColor: palette.page },
        headerBackButtonDisplayMode: 'minimal',
        headerShadowVisible: false,
        headerStyle: { backgroundColor: palette.page },
        headerTintColor: palette.accent,
        headerTitleStyle: { color: palette.text },
      }}>
      <Stack.Screen name="login" options={{ headerShown: false }} />
      <Stack.Screen name="register" options={{ headerShown: false }} />
    </Stack>
  );
}
