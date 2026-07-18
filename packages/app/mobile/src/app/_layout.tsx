import { DarkTheme, DefaultTheme, Stack, ThemeProvider } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { useColorScheme } from 'react-native';
import { GestureHandlerRootView } from 'react-native-gesture-handler';

import { AuthProvider, useAuth } from '@/providers/AuthProvider';
import { darkPalette, lightPalette, usePalette } from '@/theme/palette';

function Navigation() {
  const { status } = useAuth();
  const palette = usePalette();
  return (
    <Stack
      screenOptions={{
        animation: 'default',
        contentStyle: { backgroundColor: palette.page },
        headerBackButtonDisplayMode: 'minimal',
        headerShadowVisible: false,
        headerStyle: { backgroundColor: palette.page },
        headerTintColor: palette.accent,
        headerTitleStyle: { color: palette.text },
      }}>
      <Stack.Screen name="index" options={{ headerShown: false }} />
      <Stack.Protected guard={status === 'anonymous'}>
        <Stack.Screen name="(auth)" options={{ headerShown: false }} />
      </Stack.Protected>
      <Stack.Protected guard={status === 'authenticated'}>
        <Stack.Screen name="(app)" options={{ headerShown: false }} />
      </Stack.Protected>
    </Stack>
  );
}

export default function RootLayout() {
  const scheme = useColorScheme();
  const dark = scheme === 'dark';
  const palette = dark ? darkPalette : lightPalette;
  const baseTheme = dark ? DarkTheme : DefaultTheme;
  const navigationTheme = {
    ...baseTheme,
    colors: {
      ...baseTheme.colors,
      primary: palette.accent,
      background: palette.page,
      card: palette.page,
      text: palette.text,
      border: palette.border,
    },
  };

  return (
    <GestureHandlerRootView style={{ flex: 1, backgroundColor: palette.page }}>
      <ThemeProvider value={navigationTheme}>
        <StatusBar style={dark ? 'light' : 'dark'} />
        <AuthProvider>
          <Navigation />
        </AuthProvider>
      </ThemeProvider>
    </GestureHandlerRootView>
  );
}
