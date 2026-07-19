# Cove iOS Maestro E2E

This package-owned Maestro workspace turns high-value Cove App scenarios into repeatable iOS Simulator regression coverage. It complements the repository `ios-simulator` skill: the skill owns discovery, build, launch, local-stack diagnosis, and evidence handling; Maestro owns deterministic semantic UI interaction.

## Verified toolchain

- Maestro CLI 2.6.1, installed from the official `mobile-dev-inc/tap/maestro` Homebrew formula
- Java 17 or newer (verified with Java 21)
- Xcode and an installed Cove development build with bundle ID `io.github.chenyangzz.cove.mobile`
- Expo SDK 57 / React Native 0.86 using native accessibility identifiers from stable `testID` props

Install the local CLI on macOS:

```bash
brew tap mobile-dev-inc/tap
brew install mobile-dev-inc/tap/maestro
maestro --version
```

## Profile/password flow

Start the disposable OrbStack dependencies, migrate and start the real local API, then launch Metro with the run-owned API URL as documented in the root `e2e/README.md`. Install or launch the Cove development build on one explicit Simulator before running this driver.

Provide only synthetic, run-owned test data through environment variables:

```bash
IOS_SIMULATOR_UDID=<simulator-udid> \
E2E_RUN_ID=<run-id> \
MAESTRO_EXPO_DEV_CLIENT_URL='exp+cove-mobile://expo-development-client/?url=<encoded-metro-url>' \
MAESTRO_E2E_USERNAME=<synthetic-username> \
MAESTRO_E2E_OLD_PASSWORD=<current-password> \
MAESTRO_E2E_WRONG_PASSWORD=<known-wrong-password> \
MAESTRO_E2E_NEW_PASSWORD=<replacement-password> \
MAESTRO_E2E_NICKNAME=<updated-nickname> \
MAESTRO_E2E_EMAIL=<updated-email> \
make app-mobile-e2e-profile-password
```

The flow clears App state and the Simulator keychain, reconnects the development client through the run-owned `MAESTRO_EXPO_DEV_CLIENT_URL`, logs in with the current password, updates profile data, verifies a wrong-current-password rejection, changes the password, logs out, rejects the old login, and accepts the new login. Encode the Metro URL query value (for example, `http%3A%2F%2F192.0.2.10%3A8081`) and do not commit a machine-specific address. Screenshots, the JUnit report, and Maestro debug output are written below `output/ios-simulator/runs/<run-id>/`. On exit, the wrapper deletes Maestro's generated `commands-*.json` files and redacts injected passwords from retained text artifacts.

Never use a persistent or remote account. The password rotation is destructive for the supplied account and is safe only for a disposable fixture in the run-owned local database. The flow also clears the selected Simulator's keychain and App state, so use a dedicated disposable Simulator rather than one containing developer-owned test sessions.

## Chat persistence flow

Start `make e2e-app-backend` in a separate terminal, then start Metro with the run-owned Mac LAN API URL as documented in the root `e2e/README.md`. Use the loopback API/provider URLs below only for host-side fixture setup; the App bundle itself must use the LAN-reachable API URL.

```bash
IOS_SIMULATOR_UDID=<simulator-udid> \
E2E_RUN_ID=<run-id> \
MAESTRO_EXPO_DEV_CLIENT_URL='exp+cove-mobile://expo-development-client/?url=<encoded-metro-url>' \
MAESTRO_E2E_API_URL=http://127.0.0.1:58000 \
MAESTRO_E2E_LLM_BASE_URL=http://127.0.0.1:58001/v1 \
MAESTRO_E2E_USERNAME=<synthetic-username> \
MAESTRO_E2E_PASSWORD=<synthetic-password> \
MAESTRO_E2E_CHAT_PROMPT=<unique-prompt-at-most-20-characters> \
MAESTRO_E2E_CHAT_ANSWER='Local chat reply persisted.' \
make app-mobile-e2e-chat-persistence
```

The wrapper registers the user and creates a uniquely named default chat model through public APIs. The flow clears App state and keychain, authenticates, disables knowledge retrieval, sends the prompt, asserts the visible streaming state and terminal answer, terminates the native process, relaunches without clearing state, and verifies the same prompt/answer/title plus the drawer entry. The prompt limit keeps the Server-generated title exact. Use a fresh username for each run and keep the expected answer aligned with `COVE_E2E_LLM_ANSWER` when overriding the root provider.

## Navigation/native lifecycle flow

Keep `make e2e-app-backend` and the run-owned Metro process active, then provide a fresh synthetic user and non-secret draft values:

```bash
IOS_SIMULATOR_UDID=<simulator-udid> \
E2E_RUN_ID=<run-id> \
MAESTRO_EXPO_DEV_CLIENT_URL='exp+cove-mobile://expo-development-client/?url=<encoded-metro-url>' \
MAESTRO_E2E_API_URL=http://127.0.0.1:58000 \
MAESTRO_E2E_LLM_BASE_URL=http://127.0.0.1:58001/v1 \
MAESTRO_E2E_USERNAME=<synthetic-username> \
MAESTRO_E2E_EMAIL=<synthetic-email> \
MAESTRO_E2E_PASSWORD=<synthetic-password> \
MAESTRO_E2E_CHAT_DRAFT=<non-secret-draft> \
MAESTRO_E2E_UNSAVED_NICKNAME=<non-secret-nickname> \
make app-mobile-e2e-native-lifecycle
```

The wrapper creates only the disposable user fixture through the public registration API. The App flow proves anonymous protected-route rejection, App login, keyboard-backed chat draft state, drawer navigation to the real default knowledge base, the iOS interactive-pop gesture, preservation of the mounted chat draft, cancellation of an unsaved profile sheet edit, process termination/relaunch, and SecureStore session restoration without persisting page-local draft or profile state. It records the complete Simulator flow to `evidence/native-lifecycle.mp4`, captures seven checkpoints, and sanitizes retained text artifacts on exit.

## Native knowledge upload flow

Keep the run-owned API, worker, dependencies, and Metro process active. Use a fresh synthetic user and the host-side loopback URL only for public-API fixture setup:

```bash
IOS_SIMULATOR_UDID=<simulator-udid> \
E2E_RUN_ID=<run-id> \
MAESTRO_EXPO_DEV_CLIENT_URL='exp+cove-mobile://expo-development-client/?url=<encoded-metro-url>' \
MAESTRO_E2E_API_URL=http://127.0.0.1:58000 \
MAESTRO_E2E_LLM_BASE_URL=http://127.0.0.1:58001/v1 \
MAESTRO_E2E_USERNAME=<synthetic-username> \
MAESTRO_E2E_EMAIL=<synthetic-email> \
MAESTRO_E2E_PASSWORD=<synthetic-password> \
make app-mobile-e2e-knowledge-upload
```

The wrapper copies the controlled Markdown fixture into the selected Simulator's temporary directory, hands it to the public iOS document-import save sheet with `simctl openurl`, and uses Maestro to save it into Files. It does not write to a private App or Files container. A pre-staged runner may set `MAESTRO_E2E_FILES_READY=1` and optionally `MAESTRO_E2E_UPLOAD_FILE_STEM`; otherwise a short hash of the run ID creates a collision-free filename that remains fully addressable in the iOS Files accessibility tree. Explicit stems are limited to 40 characters for the same reason. The wrapper then registers the disposable user through `/api/auth/register`, creates a run-owned local embedding configuration, and verifies through public APIs that the user has exactly one empty default knowledge base. On every success or failure after automated staging, cleanup removes both the Simulator temporary file and the exact run-owned Files document; pre-staged user-owned fixtures are left untouched.

Maestro then performs a real App login, enters the default knowledge-base detail through normal navigation, opens Expo's real iOS DocumentPicker, selects the controlled Markdown file, observes `等待处理` or `解析中`, and waits for `已就绪` plus a non-zero visible chunk count. It never uploads through setup APIs or uses a deep link to bypass native selection. The worker deadline defaults to 120 seconds and can be bounded with `MAESTRO_E2E_PROCESSING_TIMEOUT_MS` from 1000 through 600000 milliseconds. The wrapper records `knowledge-upload.mp4`, captures the empty, picker, processing, and ready checkpoints, preserves the first Maestro failure, and sanitizes retained text artifacts on every exit.

## CI and maintenance

A CI runner needs macOS, Java 17+, a compatible Xcode/iOS Simulator runtime, the Maestro CLI, an iOS Simulator development build, Metro configured for the disposable local API, and the root OrbStack E2E lifecycle. The suite remains serial while it shares one database, deterministic provider, and Simulator. The native upload flow stages its controlled file through the public iOS document-import UI; this setup must not be replaced with an API upload. The registration UI remains a separate gap because iOS Password AutoFill requires a dedicated deterministic strategy; the navigation flow intentionally provisions its account at the public API boundary.

Maintain stable English kebab-case `testID` values when UI copy or layout changes. Update the flow when a user-visible contract changes, and verify Maestro compatibility when Expo, React Native, Xcode, iOS, or Maestro is upgraded. Keep native build/startup and dependency lifecycle logic out of this directory; those remain owned by the Simulator skill and root E2E harness.
