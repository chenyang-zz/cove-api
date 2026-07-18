import type { ConfigContext, ExpoConfig } from 'expo/config';

export default ({ config }: ConfigContext): ExpoConfig => {
  const allowInsecureHttp = process.env.EXPO_ALLOW_INSECURE_HTTP === 'true';

  return {
    ...config,
    name: config.name ?? 'Cove',
    slug: config.slug ?? 'cove-mobile',
    ios: {
      ...config.ios,
      infoPlist: {
        ...config.ios?.infoPlist,
        ...(allowInsecureHttp
          ? {
              NSAppTransportSecurity: {
                NSAllowsArbitraryLoads: true,
              },
            }
          : {}),
      },
    },
  };
};
