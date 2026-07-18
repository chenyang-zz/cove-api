---
name: cove-ios-native-navigation
description: Design, implement, refactor, and debug Cove native iOS page navigation. Use for the current Expo Router native Stack in mobile/ or, when explicitly requested, the legacy UIKit UINavigationController with route-specific WKWebViews. Covers login/register/chat/profile pushes and pops, interactive Back, screen lifecycle, authentication history cleanup, sheets, transition performance, preloading, and transitions that look like React or CSS swaps instead of native iOS navigation.
---

# Cove iOS Native Navigation

Treat a transition as native only when the platform navigation stack changes the visible native screen. The current app uses Expo Router `Stack`; the old Wails app uses UIKit controllers and WKWebViews.

## Select the Architecture First

- Use `mobile/` and Expo Router by default.
- Use the UIKit/WKWebView bridge only when the request explicitly targets `frontend/`, `build/ios/`, Wails, or the legacy wrapper.
- Never mix evidence between the two implementations.

Name the exact edge before changing code: login → register, register → login, authentication → chat, chat → profile, profile → chat, or logout → login. Read [references/architecture.md](references/architecture.md) before modifying layouts, authentication guards, screen lifecycle, the legacy bridge, or native build integration.

## Required Workflow

1. Read `AGENTS.md`. Run GitNexus upstream impact analysis before editing each existing symbol and warn before HIGH or CRITICAL changes.
2. Identify the source screen, destination screen, owning layout/stack, state that must survive, and state that must be destroyed.
3. Preserve the platform fallback only when that fallback is still in product scope.
4. Make the smallest navigation-owner change; do not build a second router inside page components.
5. Run focused tests, TypeScript checks, an Expo iOS export or native build, and Simulator validation.
6. Record the exact edge and its Back path. Inspect intermediate frames, not only the destination screenshot.

## Expo Router Rules

- Define page navigation in the relevant Expo Router `Stack` under `mobile/src/app/**/_layout.tsx`. Expo Router, React Native Screens, and the native stack own transition animation and interactive Back.
- Use `router.push()` for a forward page, `router.back()` for a pop, and `router.replace()` only when history must be removed, such as an authentication boundary.
- Keep login, register, chat, and profile as independent route components. Do not imitate page navigation with conditional rendering, `Animated.View`, CSS transforms, or a modal overlay.
- Keep local form state local. Login → register creates a registration screen; Back pops and destroys it. Re-entering registration must not restore old fields, validation errors, focus, or pending state.
- Keep chat under profile in the native stack. Profile → Back reveals the same chat instance, preserving conversation, draft, scroll, and drawer state.
- Let `AuthProvider` and `Stack.Protected` own anonymous/authenticated availability. After login or registration, protected-route changes must remove authentication screens from reachable Back history. Logout must make chat/profile unreachable immediately.
- Close a page-owned sheet before popping its screen when product behavior requires that priority. Do not disable the native Back gesture globally to work around sheet logic.
- Give root navigation, screen content, headers, splash, and sheets the same dynamic palette background. Native transition frames must not expose white or transparent gaps in dark mode.
- Do not prewarm hidden React Native pages by mounting extra route copies. Native-stack creation is already lightweight. If a measured delay remains, optimize module evaluation, asset decoding, or destination data fetching; do not keep login, register, chat, and profile all mounted.
- Prefetch code or route data only after profiling proves it useful, and only for the currently reachable destination. Prefetch must not retain popped form state.

## Legacy UIKit/WKWebView Rules

For an explicitly legacy task:

- Every native destination owns an independent `UIViewController` and `WKWebView`.
- Use `UINavigationController.pushViewController(_:animated:)` and pop APIs for the visible transition.
- Share `WKWebsiteDataStore.defaultDataStore` for the localStorage session; never copy tokens through bridge payloads.
- Synchronize the TypeScript action union, Objective-C handler, route-specific React entry, readiness messages, tests, and architecture reference.
- Preload at most one justified off-stack destination. Release unreachable registration and authenticated controllers at the lifecycle points documented in [references/architecture.md](references/architecture.md).
- Give each controller a native root surface using the dynamic page color and keep an unready WKWebView transparent to prevent white first frames.
- Edit source-of-truth files, not generated copies in `build/ios/xcode/wails-full-bleed`.

## Invariants

- A React state change, query change, CSS transform, or final screenshot alone does not prove native navigation.
- Authentication screens are not reachable after authentication; protected screens are not reachable after logout.
- Popped registration state is destroyed. Chat state survives a profile round trip.
- Page Back honors sheet/save locking without permanently breaking the interactive gesture.
- No generic full-screen loading page hides a cold navigation bug.
- No destination is kept alive without a documented lifecycle reason.

## Acceptance Evidence

Use the project `ios-simulator` skill and select its React Native or legacy command path as appropriate.

Require all of the following:

1. The installed build comes from current source, not a cached browser or stale development client.
2. A Simulator recording contains intermediate native right-to-left push and left-to-right pop frames.
3. The native Back button/gesture works where allowed.
4. Register → Back → register shows a fresh form.
5. Chat → profile → Back restores the same chat state.
6. Authentication and logout clean unreachable history.
7. Light/dark backgrounds and safe areas remain continuous through the transition.

Never claim completion merely because navigation symbols exist, a unit test changed a route, or the destination eventually appeared.
