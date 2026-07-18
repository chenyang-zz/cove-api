import { Redirect } from 'expo-router';

import { RestoringScreen } from '@/components/RestoringScreen';
import { useAuth } from '@/providers/AuthProvider';

export default function IndexScreen() {
  const { status } = useAuth();
  if (status === 'restoring') {
    return <RestoringScreen />;
  }
  return <Redirect href={status === 'authenticated' ? '/(app)/chat' : '/(auth)/login'} />;
}
