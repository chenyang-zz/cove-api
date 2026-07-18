# Cove Mobile

React Native migration spike for Cove, built with Expo SDK 57 and React Native 0.86.

## Phase 1 scope

- Native stack navigation for login → register, authentication → chat, and chat → profile.
- Protected-route cleanup so authentication screens cannot be reached after login.
- SecureStore-backed JWT persistence, startup restoration, refresh rotation, and logout.
- `expo/fetch` streaming support for `POST /api/chat/stream`.
- Native Fabric Markdown rendering without a WebView.
- Automatic light/dark colors and iOS safe-area/keyboard handling.

The existing Wails frontend remains the production desktop/WebView client during the migration.

## Environment

Copy `.env.example` to an uncommitted environment file and set:

```dotenv
EXPO_PUBLIC_API_BASE_URL=https://api.example.com
```

When unset, the development fallback is `http://localhost:8000`. The iOS Simulator can access a server running on the host Mac through `localhost`; a physical device needs a device-reachable HTTPS address.

## Commands

```bash
pnpm install
pnpm typecheck
pnpm test
pnpm exec expo prebuild --platform ios
pnpm ios
```

`react-native-enriched-markdown` contains native code, so use the generated development build rather than Expo Go.

## iOS toolchain note

Xcode 26.2 / Swift 6.2.3 reports an overload ambiguity in `expo-modules-jsi@57.0.3` while building JavaScript date decoding. The reproducible pnpm patch in `patches/` replaces the ambiguous global `abs` call with `Double.magnitude`; do not edit `node_modules` directly. Remove the patch after Expo publishes the equivalent upstream fix and a clean Simulator build succeeds without it.
