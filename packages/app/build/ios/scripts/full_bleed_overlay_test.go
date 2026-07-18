package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPatchWebViewLayoutAddsKeyboardScrollLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "webview_window_ios.m")
	upstream := bytes.Join([][]byte{
		viewControllerImplementation,
		viewDidLoadStart,
		messageHandlerRegistration,
		scrollConfiguration,
		webViewSubviewRegistration,
		upstreamLayout,
	}, []byte("\n"))
	if err := os.WriteFile(path, upstream, 0o644); err != nil {
		t.Fatalf("write upstream fixture: %v", err)
	}

	patchWebViewLayout(path)

	patched, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read patched source: %v", err)
	}
	for _, expected := range [][]byte{
		[]byte("coveSetKeyboardScrollLocked"),
		[]byte("UIKeyboardWillChangeFrameNotification"),
		[]byte("CoveContentOffsetObservationContext"),
		[]byte("coveConfigureNativeNavigation"),
		[]byte("coveInstallNativeNavigation"),
		fullBleedLayout,
	} {
		if !bytes.Contains(patched, expected) {
			t.Fatalf("patched source does not contain %q", expected)
		}
	}
	if bytes.Contains(patched, []byte("covePrepareNativeProfileForURL")) {
		t.Fatal("profile preloading must be owned by the authenticated navigation route")
	}
}

func TestRouteWebViewsLockOuterScrollWhileKeyboardIsVisible(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "cove_navigation_ios.m"))
	if err != nil {
		t.Fatalf("read native navigation source: %v", err)
	}

	for _, expected := range [][]byte{
		[]byte("CoveRouteContentOffsetObservationContext"),
		[]byte("UIKeyboardWillChangeFrameNotification"),
		[]byte("coveSetKeyboardScrollLocked"),
		[]byte("[scrollView setContentOffset:CGPointZero animated:NO]"),
	} {
		if !bytes.Contains(source, expected) {
			t.Fatalf("route WebView source does not contain %q", expected)
		}
	}
}
