# Computer Use for Cove Simulator QA

Use this workflow only after preflight confirms a booted Simulator and the real Cove app is running. Load the installed `computer-use:computer-use` skill first and obey its bootstrap and confirmation policy; do not copy a version-specific plugin path from an old session.

## Start a recorded session

Start device-native recording in a foreground shell session:

```bash
.agents/skills/cove-ios-simulator-debugging/scripts/ios_qa.sh record /private/tmp/cove-interaction.mp4
```

Stop it with `Ctrl-C` after the interaction sequence. Use the `ios_qa.sh screenshot` command for final device evidence; Computer Use screenshots include the macOS Simulator window frame and are primarily for locating controls.

## Bootstrap and inspect Simulator

Use `node_repl`, deriving `<computer-use-plugin-root>` from the installed Computer Use skill path:

```js
if (!globalThis.sky) {
  var { setupComputerUseRuntime } = await import(
    "<computer-use-plugin-root>/scripts/computer-use-client.mjs"
  );
  await setupComputerUseRuntime({ globals: globalThis });
}

var simulatorState = await sky.get_app_state({
  app: "Simulator",
  disableDiff: true,
});
nodeRepl.write(simulatorState.text);
```

Prefer `element_index` actions when the target appears in the accessibility tree. Simulator commonly exposes only its window chrome while Cove's WKWebView controls are absent. In that case, emit `simulatorState.screenshot`, visually locate the target, and use screenshot-relative coordinates. Refresh `get_app_state` after every action because coordinates and element indices become stale after keyboard, rotation, drawer, or window changes.

If an action returns `noWindowsAvailable`, reacquire state using the bundle identifier and use it for subsequent calls:

```js
simulatorState = await sky.get_app_state({
  app: "com.apple.iphonesimulator",
  disableDiff: true,
});
```

If state can be read but actions still fail, reacquire the Simulator window once more. Ask the user to perform the gesture if the failure repeats; do not switch to AppleScript or coordinate synthesis outside Computer Use.

## Exercise the software keyboard

1. Click the center of the composer textarea, never its border. Edge taps exercise a different WKWebView scroll path and can trigger selection or page movement.
2. Refresh the screenshot and confirm the textarea has focus.
3. If the software keyboard is hidden because the Simulator uses a hardware keyboard, press `super+k`.
4. For each of at least three cycles, toggle once, refresh state and inspect, toggle again, then refresh and inspect. Do not send all toggles in a tight loop.

Set `textareaX` and `textareaY` from the center of the latest Computer Use screenshot before running the action:

```js
var textareaPoint = { x: 0, y: 0 }; // replace both zeros with measured coordinates
if (textareaPoint.x <= 0 || textareaPoint.y <= 0) throw new Error("Measure textarea center first");
await sky.click({ app: "Simulator", x: textareaPoint.x, y: textareaPoint.y });
simulatorState = await sky.get_app_state({ app: "Simulator" });

await sky.press_key({ app: "Simulator", key: "super+k" });
simulatorState = await sky.get_app_state({ app: "Simulator" });
```

Repeat the two `press_key`/`get_app_state` steps for open and close. Verify in every sampled state that the status bar and header stay fixed, only the composer follows the keyboard, the message region remains the sole scroll container, and closing restores the normal safe area.

For drawer or long-content QA, use a fresh screenshot before coordinate clicks or drags. Do not type or submit a real message just to create content. Use existing authenticated data, or ask the user to prepare the required state.

## Inspect the whole animation

Check the recording duration, then sample frames across the entire clip rather than only its beginning:

```bash
ffprobe -v error -show_entries format=duration \
  -of default=noprint_wrappers=1 /private/tmp/cove-interaction.mp4

ffmpeg -y -i /private/tmp/cove-interaction.mp4 \
  -vf "fps=1/5,scale=260:-1,tile=5x4" \
  -frames:v 1 -update 1 /private/tmp/cove-interaction-sheet.jpg
```

Adjust the sampling interval and tile dimensions to cover the full duration. Inspect the contact sheet for intermittent whole-screen lift, header drift, composer jumps, stale safe-area gaps, keyboard animation discontinuity, and drawer shadow leakage. Keep the video as the source of truth when a contact-sheet frame is ambiguous.

Use `xcrun simctl ui booted appearance dark` for dark-mode evidence and restore the original appearance after capture. Theme commands do not replace keyboard-cycle recording.
