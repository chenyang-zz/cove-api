---
name: cove-ios-simulator-debugging
description: Debug and visually validate Cove on an iOS Simulator, including Wails/Vite launch failures, bundle or Info.plist mismatches, ATS and server connectivity errors, authenticated chat flows, Computer Use-driven Simulator interaction, WKWebView keyboard movement, safe-area spacing, shadows, screenshots, and recordings. Use when requests mention Cove iOS, iPhone Simulator, “无法连接到服务器”, ATS -1022, an app that will not launch, keyboard-induced whole-screen movement, repeated keyboard gestures, or iOS visual QA.
---

# Cove iOS Simulator Debugging

Use the repository's real Cove application and real authenticated state. Do not invent mock data or patch an installed `.app` as the final fix.

## Start Here

1. Run `.agents/skills/cove-ios-simulator-debugging/scripts/ios_qa.sh preflight` from the repository root.
2. Record `git status --short` before building. Preserve every pre-existing change.
3. Use `.agents/skills/cove-ios-simulator-debugging/scripts/ios_qa.sh dev` as the default build/install/launch path. It delegates to `wails3 task ios:dev` and uses the generated Cove bundle.
4. Do not start with `wails3 task ios:run`. Its legacy `build/ios/Info.dev.plist` currently names `check` and `com.example.check.dev`, while the task tries `com.wails.cove.dev` and the executable is `cove`.
5. Run `.agents/skills/cove-ios-simulator-debugging/scripts/ios_qa.sh inspect-bundle` after packaging when launch identity or ATS is suspect.
6. Use `.agents/skills/cove-ios-simulator-debugging/scripts/ios_qa.sh logs` to classify native failures before changing application code.

Read [references/troubleshooting.md](references/troubleshooting.md) when diagnosing connectivity, launch identity, keyboard movement, safe areas, or visual regressions.
Read [references/computer-use.md](references/computer-use.md) before automating taps, scrolling, software-keyboard cycles, or other Simulator UI interactions.

## Debugging Order

Follow this order and stop when the first failing layer is identified:

1. Verify Xcode tools, a booted iPhone Simulator, Vite, API configuration, and the working tree.
2. Build and launch through `ios:dev`.
3. Compare the packaged executable, `CFBundleExecutable`, `CFBundleIdentifier`, expected bundle ID, and files inside the bundle.
4. Separate Mac reachability, Simulator/WebKit ATS, authentication, CORS, and frontend rendering failures using logs and direct requests.
5. Use Computer Use for safe, repeatable Simulator interactions when available. Request user interaction for real login, credentials, confirmation-gated actions, or gestures that remain unreliable after the documented fallbacks.
6. Capture screenshots and a recording. For intermittent keyboard issues, record at least three open/close cycles and inspect frames across the full animation.

## Simulator UI Automation

- Load the installed Computer Use skill before controlling Simulator and follow its bootstrap and confirmation policy.
- Prefer Simulator accessibility elements. WKWebView content often does not appear in the accessibility tree; when it does not, refresh the screenshot and use fresh coordinates.
- Focus the center of the textarea rather than its edge, then use `super+k` to toggle the iOS software keyboard. Refresh state after every open or close instead of issuing a tight unobserved loop.
- Record the device concurrently and inspect a distributed contact sheet. A final screenshot alone cannot prove animation stability.
- Never type or submit a real chat message merely to test focus. Ask the user before any interaction involving credentials, sensitive data, deletion, or external submission.

Do not claim that a server is unreachable until ATS and native logs have been ruled out. Treat HTTPS as the production solution. Limit any HTTP exception to a development Plist; never present an edit to a built bundle as persistent configuration.

## Keyboard and Safe-Area Invariants

For this Cove WKWebView chat layout:

- Track `window.visualViewport.height` on `resize`.
- Keep the application root fixed at `top: 0` and size it from the visual viewport height.
- Do not compensate with `visualViewport.offsetTop`.
- Do not subscribe to visual viewport `scroll` for layout correction.
- Move only the composer region when the keyboard changes the viewport.
- Reduce keyboard-open bottom padding and hide nonessential footer copy when it produces a second safe-area gap.
- Keep the header and status-bar relationship fixed throughout keyboard animation.

Validate behavior, not just the final frame: no intermittent whole-screen lift, no drawer shadow leaking from the closed left edge, no composer overlap, and no abrupt mismatch between repeated keyboard cycles.

## Evidence and Handoff

- Capture the resting screen with `.agents/skills/cove-ios-simulator-debugging/scripts/ios_qa.sh screenshot [path]`.
- Record interactions with `.agents/skills/cove-ios-simulator-debugging/scripts/ios_qa.sh record [path]`; stop with `Ctrl-C`.
- Report the exact Simulator model/runtime, bundle ID, API base URL, tested appearance, interaction sequence, and observed result.
- Re-run `git status --short` after builds because iOS generation can rewrite tracked Xcode files. Revert only confirmed build noise and never discard unrelated user changes.
- If a repository fix is requested, follow GitNexus impact-analysis rules before editing existing symbols and run change detection before committing.
