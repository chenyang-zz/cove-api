import { Stack } from 'expo-router';

import { usePalette } from '@/theme/palette';

export default function AppLayout() {
  const palette = usePalette();
  return (
    <Stack
      initialRouteName="chat"
      screenOptions={{
        animation: 'default',
        contentStyle: { backgroundColor: palette.page },
        headerBackButtonDisplayMode: 'default',
        headerBackTitle: '返回',
        headerTintColor: palette.accent,
        headerTitleStyle: { color: palette.text, fontSize: 17, fontWeight: '600' },
      }}>
      <Stack.Screen name="chat" options={{ headerShown: false }} />
      <Stack.Screen
        name="knowledge"
        options={{
          title: '知识库',
          headerBackVisible: false,
          headerShadowVisible: false,
          headerStyle: { backgroundColor: palette.page },
          headerTintColor: palette.accent,
          headerTitleStyle: { color: palette.text, fontSize: 17, fontWeight: '600' },
        }}
      />
      <Stack.Screen
        name="profile"
        options={{
          title: '个人信息',
          headerBackVisible: false,
          headerShadowVisible: false,
          headerTransparent: true,
        }}
      />
    </Stack>
  );
}
