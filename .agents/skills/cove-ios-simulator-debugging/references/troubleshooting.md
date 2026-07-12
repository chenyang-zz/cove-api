# Cove iOS Simulator Troubleshooting

## Contents

- [Canonical paths](#canonical-paths)
- [Launch and bundle diagnosis](#launch-and-bundle-diagnosis)
- [Connectivity and ATS](#connectivity-and-ats)
- [Authentication checkpoints](#authentication-checkpoints)
- [Keyboard and visual validation](#keyboard-and-visual-validation)
- [Evidence checklist](#evidence-checklist)

## Canonical paths

- Preferred live-development command: `wails3 task ios:dev`
- Preferred generated bundle: `bin/cove.ios-dev.app`
- Expected generated bundle ID: `io.github.chenyangzz.cove`
- Vite default: `http://127.0.0.1:9245`
- API configuration: `frontend/.env.development.local`, key `VITE_API_BASE_URL`
- Generated development Plist: `build/ios/xcode/main/Info.plist`
- Legacy Plist used by `ios:run`: `build/ios/Info.dev.plist`

The API URL belongs in an uncommitted environment file. Never hard-code an environment host into TypeScript.

## Launch and bundle diagnosis

Run:

```bash
.agents/skills/cove-ios-simulator-debugging/scripts/ios_qa.sh preflight
.agents/skills/cove-ios-simulator-debugging/scripts/ios_qa.sh dev
.agents/skills/cove-ios-simulator-debugging/scripts/ios_qa.sh inspect-bundle
```

For a launch failure, compare all of these before rebuilding again:

| Check | Expected for `ios:dev` | Failure meaning |
|---|---|---|
| Bundle directory | `bin/cove.ios-dev.app` | Wrong packaging path |
| `CFBundleExecutable` | `cove` | Plist and copied binary disagree |
| Executable file | `bin/cove.ios-dev.app/cove` | Bundle is incomplete |
| `CFBundleIdentifier` | `io.github.chenyangzz.cove` | Launch command targets another app |
| Code signature | Ad-hoc valid | Simulator may reject installation |

Use `xcrun simctl listapps booted` when the installed identifier is unclear. Use wide logs only long enough to discover the real process, then return to the filtered log command.

The legacy `ios:run` path is not the normal fallback: `build/ios/Info.dev.plist` contains `CFBundleExecutable=check` and `CFBundleIdentifier=com.example.check.dev`, while its task launches `com.wails.cove.dev`. Diagnose or repair that path as a separate task.

## Connectivity and ATS

Classify the failure in this order:

1. Read `VITE_API_BASE_URL` from the active development environment file.
2. Test the endpoint from macOS with `curl`; this proves host reachability only.
3. Inspect Simulator logs while reproducing the request.
4. If logs show `NSURLErrorDomain Code=-1022`, diagnose ATS rather than server availability.
5. If native networking succeeds, check CORS, HTTP status, token refresh, and application error rendering.

Common distinctions:

| Signal | Interpretation | Next action |
|---|---|---|
| macOS `curl` cannot connect | Host, port, route, or service problem | Fix endpoint/service first |
| macOS succeeds; Simulator logs `-1022` | WKWebView ATS policy blocked HTTP | Prefer HTTPS or add a development-only WebKit exception |
| OPTIONS/CORS fails | Server CORS policy | Verify allowed origin and headers |
| Initial request returns 401 | Authentication/token issue | Exercise the real refresh or login flow |
| UI says offline without native error | Frontend mapping or request logic | Inspect console/application logs |

Production must use HTTPS. If development must call a remote HTTP API, configure the source development Plist narrowly for WebKit, such as a justified domain exception or development-only `NSAllowsArbitraryLoadsInWebContent`. Do not rely on manually modifying `bin/*.app/Info.plist`; the next build replaces it.

## Authentication checkpoints

Do not fabricate sessions. If the target state requires authentication:

1. Launch the real app and navigate to login.
2. Ask the user to log in without requesting credentials in chat.
3. Wait for the user to confirm completion.
4. Re-read the visible screen or capture a screenshot before continuing.

For non-sensitive keyboard, drawer, and scrolling interactions, use the Computer Use workflow in [computer-use.md](computer-use.md) first. Ask the user only when automation remains unreliable or the action requires credentials, sensitive data, deletion, or external submission.

## Keyboard and visual validation

The accepted Cove chat behavior uses the visual viewport height as the layout boundary. The application root stays fixed at the top. Avoid `offsetTop` compensation and viewport `scroll` listeners: during iOS keyboard animation they can intermittently move the entire screen even when a final static screenshot looks correct.

Validate all of the following:

- Open and close the keyboard at least three times.
- Confirm the status bar and header never move upward or overlap.
- Confirm only the composer follows the keyboard.
- Confirm keyboard-open bottom padding does not retain the full Home Indicator gap.
- Confirm closing restores the normal safe area without a jump.
- Confirm the closed history drawer casts no left-edge shadow.
- Exercise long messages and scrolling while the keyboard is open.
- Check light and dark appearance, a second iPhone size, and rotation when the change affects layout.

Use recording for animation bugs:

```bash
.agents/skills/cove-ios-simulator-debugging/scripts/ios_qa.sh record /private/tmp/cove-keyboard-cycle.mp4
```

Stop with `Ctrl-C`. If `ffmpeg` is available, generate a contact sheet for distributed frames rather than judging only the first and last frame.

## Evidence checklist

Record these in the handoff:

- Simulator model and iOS runtime
- Appearance and orientation
- App bundle ID and executable
- Vite URL and API base URL, excluding secrets
- Whether login used a real account
- Exact keyboard/open-close sequence
- Screenshot and recording paths
- Test/build results
- Remaining environment-only exceptions or packaging mismatches
