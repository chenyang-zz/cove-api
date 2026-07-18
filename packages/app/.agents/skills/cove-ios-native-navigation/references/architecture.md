# Cove Native Navigation Architecture

## Current Architecture: Expo React Native

Use this section by default. The active mobile product is the Expo React Native application under `mobile/`.

### Source of truth

| Responsibility | Source |
| --- | --- |
| Root native stack and authentication guards | `mobile/src/app/_layout.tsx` |
| Restoration entry and redirect | `mobile/src/app/index.tsx` |
| Anonymous stack | `mobile/src/app/(auth)/_layout.tsx` |
| Login and registration screens | `mobile/src/app/(auth)/login.tsx`, `register.tsx` |
| Authenticated stack | `mobile/src/app/(app)/_layout.tsx` |
| Chat and profile screens | `mobile/src/app/(app)/chat.tsx`, `profile.tsx` |
| Session state and auth actions | `mobile/src/providers/AuthProvider.tsx` |
| Dynamic navigation colors | `mobile/src/theme/palette.ts` |

### Ownership boundary

Expo Router/native stack owns:

- native route stack membership
- right-to-left push and left-to-right pop animation
- native header/Back behavior and interactive Back gesture
- mounting a pushed screen and unmounting a popped screen
- removing routes made unavailable by `Stack.Protected`

Screen components own:

- content, accessibility, API data, and local form state
- drawers, alerts, and bottom sheets contained within the page
- closing page-owned overlays before requesting page Back
- restoring or intentionally refreshing page data on focus

`AuthProvider` owns the session. It must not also retain screen-local form or navigation state.

### Route map and lifecycle

| Route | Role | Expected lifetime |
| --- | --- | --- |
| `/` | Session restoration and redirect | Temporary entry only |
| `/(auth)/login` | Anonymous root | Present while anonymous |
| `/(auth)/register` | Forward auth page | New instance on push; destroyed on pop |
| `/(app)/chat` | Authenticated root | Preserved underneath profile |
| `/(app)/profile` | Independent detail page | Pushed from chat; destroyed on pop |

Use `router.push()` for login → register and chat → profile. Use `router.back()` for register/profile Back. Use `router.replace()` only when a history entry must be made unreachable. Root `Stack.Protected` guards are the primary authentication boundary.

### Required sequences

#### Login → Register → Back

```text
login calls router.push('/(auth)/register')
  → native stack mounts a fresh register screen
  → native push animation runs
  → Back pops register
  → register component unmounts and local form state is discarded
  → the existing login screen is revealed
```

Do not keep registration fields, errors, focus, or request state in a provider or module singleton. Do not mount a hidden registration page as “preload.” If entry is slow, profile module evaluation, asset decoding, and data work first.

#### Authentication → Chat

```text
login/register API establishes the secure session
  → AuthProvider status becomes authenticated
  → anonymous Stack.Protected branch becomes unavailable
  → authenticated branch becomes available
  → index/route redirect resolves to chat
```

After success, Back must not reveal login or registration. A loading/restoration screen may appear only while session status is genuinely unknown; it must not be used to hide transition latency.

#### Chat → Profile → Back

```text
chat calls router.push('/(app)/profile')
  → profile is pushed above chat
  → chat remains mounted underneath
  → profile closes an open sheet before page Back when required
  → router.back() or native gesture pops profile
  → the same chat instance is revealed
```

Chat draft, selected conversation, scroll position, and drawer state should survive the round trip unless product requirements explicitly reset them.

#### Logout

```text
user confirms logout
  → AuthProvider clears secure session
  → status becomes anonymous
  → authenticated Stack.Protected branch is removed
  → login becomes the anonymous root
```

The native Back gesture must never reopen chat or profile after logout.

### Performance and preloading

React Native native-stack navigation does not require one prebuilt WebView per destination. Follow these rules:

- Do not mount hidden route copies to “warm” every screen.
- Do not preserve popped registration state in global stores.
- Preserve only screens that remain in the native stack, such as chat underneath profile.
- If a destination is slow, measure JavaScript module evaluation, synchronous render work, image decoding, and first data fetch independently.
- Lazy-load or prefetch code/data only after measurement, and only for a destination reachable from the active screen.
- Prefetching must not change navigation semantics or keep a popped form instance alive.

### Native acceptance evidence

Require a Simulator recording with intermediate push/pop frames, native Back behavior, and lifecycle checks:

1. Login → register → Back → register starts with an empty new form.
2. Chat → profile → Back preserves the same chat state.
3. Login/register cannot be reached after authentication.
4. Chat/profile cannot be reached after logout.
5. Light/dark page colors remain continuous during the transition.

A unit test or final screenshot is necessary but not sufficient proof of native navigation.

## Legacy Architecture: Wails + UIKit + WKWebView

Use this section only when the request explicitly targets the legacy application.

### Legacy source of truth

| Responsibility | Source |
| --- | --- |
| Typed Web → native actions and route detection | `frontend/src/app/nativeNavigation.ts` |
| Authentication entry and Web fallback | `frontend/src/app/App.tsx` |
| Route-specific React entry selection | `frontend/src/main.tsx` |
| Authenticated chat entry and session handshake | `frontend/src/app/NativeChatApp.tsx` |
| Registration entry | `frontend/src/app/NativeRegisterApp.tsx` |
| Profile entry and navigation lock | `frontend/src/app/NativeProfileApp.tsx` |
| UIKit stack and script-message handler | `build/ios/cove_navigation_ios.m` |
| Generated Xcode integration | `build/ios/scripts/full_bleed_overlay.go` |

Do not edit copied output in `build/ios/xcode/wails-full-bleed`; regeneration replaces it.

UIKit owns controller membership, push/pop animation, interactive Back, and controller/WebView allocation. React owns page content, form/API state, session persistence, sheet state, and the non-iOS fallback. Every route-specific WebView uses `WKWebsiteDataStore.defaultDataStore` so the session is shared without bridge token payloads.

### Legacy bridge protocol

Keep `NativeNavigationAction`, the Objective-C handler, senders, receivers, tests, and this table synchronized.

| Action | Meaning |
| --- | --- |
| `prepareRegister` / `pushRegister` / `registerReady` / `popRegister` | Registration preload, push, readiness, and pop |
| `prepareChat` / `chatReady` / `authCompleted` / `chatSessionReady` | Authenticated chat preparation and shared-session handshake |
| `cove:native-chat-authenticated` | Native event asking chat to reread shared storage |
| `pushProfile` / `profileReady` / `popProfile` | Profile preparation, readiness, and pop |
| `cove:native-profile-activate` | Native event asking profile to refresh before display |
| `profileNavigationLock` | Disable or enable interactive pop for sheets/saves |
| `profileSessionChanged` | Notify other roots to reread session state |
| `chatLogout` / `profileLogout` | Reset stack to authentication and release protected pages |

### Legacy lifecycle rules

- Registration may be preloaded while login is idle, but the popped controller must be released only after UIKit confirms the pop. Re-entering creates a fresh empty controller.
- Authenticated chat waits until the shared session is readable before it becomes visible. After success, replace authentication history with chat.
- Keep chat alive underneath profile. Release profile after pop and release all authenticated controllers on logout.
- Keep at most the active controller, its reachable parent, and one justified off-stack preload.
- Give every controller a dynamic native page-color surface and keep an unready WKWebView transparent so a cold push never flashes WebKit white.
- Remove script handlers and delegates during teardown to avoid retain cycles.

Legacy acceptance still requires distinct controller instances, intermediate UIKit transition frames, correct interactive Back, authentication/logout stack cleanup, and verification that the installed bundle is current.
