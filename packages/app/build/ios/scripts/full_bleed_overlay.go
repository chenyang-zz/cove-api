// Command full_bleed_overlay creates an iOS-only Wails source override for
// Cove. The override is generated inside build/ so the project never mutates
// the machine-local Go module cache.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const wailsModule = "github.com/wailsapp/wails/v3"

var (
	upstreamLayout               = []byte("    CGFloat webTop = safe.top;\n    CGFloat webBottom = safe.bottom + tabH;\n    self.webView.frame = UIEdgeInsetsInsetRect(self.view.bounds, UIEdgeInsetsMake(webTop, safe.left, webBottom, safe.right));")
	fullBleedLayout              = []byte("    // Let the web content paint below the status bar and Home Indicator.\n    // Interactive elements remain protected by CSS safe-area insets.\n    self.webView.frame = UIEdgeInsetsInsetRect(self.view.bounds, UIEdgeInsetsMake(0, 0, tabH, 0));")
	viewControllerImplementation = []byte("@implementation WailsViewController")
	nativeNavigationDeclarations = []byte(`@interface WailsViewController (CoveNativeNavigation)
- (void)coveConfigureNativeNavigation:(WKWebViewConfiguration *)configuration;
- (void)coveInstallNativeNavigation;
- (void)coveTearDownNativeNavigation;
@end

@implementation WailsViewController`)
	coveKeyboardScrollLock = []byte(`@interface WailsViewController ()
@property (nonatomic, assign) BOOL coveKeyboardLocksScroll;
@property (nonatomic, assign) BOOL coveScrollEnabledBeforeKeyboard;
@property (nonatomic, assign) BOOL coveBounceEnabledBeforeKeyboard;
@property (nonatomic, assign) BOOL coveObservesContentOffset;
@end

@implementation WailsViewController

static void *CoveContentOffsetObservationContext = &CoveContentOffsetObservationContext;

- (void)coveSetKeyboardScrollLocked:(BOOL)locked {
    UIScrollView *scrollView = self.webView.scrollView;
    if (!scrollView) return;

    if (locked) {
        if (!self.coveKeyboardLocksScroll) {
            self.coveScrollEnabledBeforeKeyboard = scrollView.scrollEnabled;
            self.coveBounceEnabledBeforeKeyboard = scrollView.bounces;
        }
        self.coveKeyboardLocksScroll = YES;
        scrollView.scrollEnabled = NO;
        scrollView.bounces = NO;
        [scrollView setContentOffset:CGPointZero animated:NO];
        return;
    }

    if (!self.coveKeyboardLocksScroll) return;
    self.coveKeyboardLocksScroll = NO;
    [scrollView setContentOffset:CGPointZero animated:NO];
    scrollView.scrollEnabled = self.coveScrollEnabledBeforeKeyboard;
    scrollView.bounces = self.coveBounceEnabledBeforeKeyboard;
}

- (void)coveKeyboardWillChangeFrame:(NSNotification *)notification {
    NSValue *frameValue = notification.userInfo[UIKeyboardFrameEndUserInfoKey];
    if (!frameValue || !self.view.window) return;

    CGRect keyboardFrame = [self.view convertRect:frameValue.CGRectValue fromView:nil];
    CGRect overlap = CGRectIntersection(self.view.bounds, keyboardFrame);
    BOOL keyboardVisible = !CGRectIsNull(overlap) && !CGRectIsEmpty(overlap) && CGRectGetHeight(overlap) > 1;
    [self coveSetKeyboardScrollLocked:keyboardVisible];

    if (keyboardVisible) {
        NSTimeInterval duration = [notification.userInfo[UIKeyboardAnimationDurationUserInfoKey] doubleValue];
        UIViewAnimationOptions curve =
            ([notification.userInfo[UIKeyboardAnimationCurveUserInfoKey] integerValue] << 16) |
            UIViewAnimationOptionBeginFromCurrentState;
        [UIView animateWithDuration:duration delay:0 options:curve animations:^{
            [self.webView.scrollView setContentOffset:CGPointZero animated:NO];
        } completion:^(__unused BOOL finished) {
            if (self.coveKeyboardLocksScroll) {
                [self.webView.scrollView setContentOffset:CGPointZero animated:NO];
            }
        }];
    }
}

- (void)coveKeyboardWillHide:(NSNotification *)notification {
    [self coveSetKeyboardScrollLocked:NO];
}

- (void)observeValueForKeyPath:(NSString *)keyPath
                      ofObject:(id)object
                        change:(NSDictionary<NSKeyValueChangeKey, id> *)change
                       context:(void *)context {
    if (context == CoveContentOffsetObservationContext) {
        UIScrollView *scrollView = (UIScrollView *)object;
        if (self.coveKeyboardLocksScroll && !CGPointEqualToPoint(scrollView.contentOffset, CGPointZero)) {
            [scrollView setContentOffset:CGPointZero animated:NO];
        }
        return;
    }
    [super observeValueForKeyPath:keyPath ofObject:object change:change context:context];
}

- (void)dealloc {
    [self coveTearDownNativeNavigation];
    [[NSNotificationCenter defaultCenter] removeObserver:self];
    if (self.coveObservesContentOffset) {
        [self.webView.scrollView removeObserver:self
                                     forKeyPath:@"contentOffset"
                                        context:CoveContentOffsetObservationContext];
    }
}
`)
	scrollConfiguration = []byte(`    if (@available(iOS 11.0, *)) {
        sv.contentInsetAdjustmentBehavior = UIScrollViewContentInsetAdjustmentNever;
    }`)
	scrollObservation = []byte(`    if (@available(iOS 11.0, *)) {
        sv.contentInsetAdjustmentBehavior = UIScrollViewContentInsetAdjustmentNever;
    }
    [sv addObserver:self
         forKeyPath:@"contentOffset"
            options:NSKeyValueObservingOptionNew
            context:CoveContentOffsetObservationContext];
    self.coveObservesContentOffset = YES;`)
	viewDidLoadStart = []byte(`- (void)viewDidLoad {
    [super viewDidLoad];`)
	viewDidLoadWithKeyboardObservers = []byte(`- (void)viewDidLoad {
    [super viewDidLoad];
    NSNotificationCenter *notifications = [NSNotificationCenter defaultCenter];
    [notifications addObserver:self
                      selector:@selector(coveKeyboardWillChangeFrame:)
                          name:UIKeyboardWillChangeFrameNotification
                        object:nil];
    [notifications addObserver:self
                      selector:@selector(coveKeyboardWillHide:)
                          name:UIKeyboardWillHideNotification
                        object:nil];`)
	messageHandlerRegistration   = []byte(`    [config.userContentController addScriptMessageHandler:self.messageHandler name:@"wails"];`)
	messageHandlerWithNavigation = []byte(`    [config.userContentController addScriptMessageHandler:self.messageHandler name:@"wails"];
    [self coveConfigureNativeNavigation:config];`)
	webViewSubviewRegistration  = []byte(`    [self.view addSubview:self.webView];`)
	webViewWithNativeNavigation = []byte(`    [self.view addSubview:self.webView];
    [self coveInstallNativeNavigation];`)
)

func main() {
	overlayPath := flag.String("overlay", "", "path to the generated Go build overlay")
	modfilePath := flag.String("modfile", "", "path to the generated Go module file")
	sumfilePath := flag.String("sumfile", "", "path to the generated Go module sum file")
	flag.Parse()
	if *overlayPath == "" || *modfilePath == "" || *sumfilePath == "" {
		fatalf("-overlay, -modfile and -sumfile are required")
	}

	absOverlayPath := absolutePath(*overlayPath)
	absModfilePath := absolutePath(*modfilePath)
	absSumfilePath := absolutePath(*sumfilePath)
	projectRoot := filepath.Clean(filepath.Join(filepath.Dir(absOverlayPath), "..", "..", ".."))
	upstreamModule := moduleDirectory()
	localModule := filepath.Join(filepath.Dir(absOverlayPath), "wails-full-bleed")

	if err := copyTree(upstreamModule, localModule); err != nil {
		fatalf("copy Wails source for iOS build: %v", err)
	}
	patchWebViewLayout(filepath.Join(localModule, "pkg", "application", "webview_window_ios.m"))
	installCoveNativeNavigationSource(projectRoot, localModule)
	writeModFiles(projectRoot, absModfilePath, absSumfilePath, localModule)
}

func absolutePath(path string) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		fatalf("resolve %q: %v", path, err)
	}
	return absPath
}

func moduleDirectory() string {
	output, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", wailsModule).Output()
	if err != nil {
		fatalf("locate Wails module: %v", err)
	}
	directory := strings.TrimSpace(string(output))
	if directory == "" {
		fatalf("Wails module directory is empty")
	}
	return directory
}

func copyTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relativePath, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relativePath == ".git" || strings.HasPrefix(relativePath, ".git"+string(filepath.Separator)) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		targetPath := filepath.Join(destination, relativePath)
		if entry.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		return copyFile(path, targetPath)
	})
}

func copyFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()

	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	output, err := os.Create(destination)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func patchWebViewLayout(path string) {
	upstream, err := os.ReadFile(path)
	if err != nil {
		fatalf("read Wails iOS source: %v", err)
	}
	if !bytes.Contains(upstream, upstreamLayout) {
		fatalf("Wails iOS layout changed; update the full-bleed patch before building")
	}
	if !bytes.Contains(upstream, viewControllerImplementation) ||
		!bytes.Contains(upstream, scrollConfiguration) ||
		!bytes.Contains(upstream, viewDidLoadStart) ||
		!bytes.Contains(upstream, messageHandlerRegistration) ||
		!bytes.Contains(upstream, webViewSubviewRegistration) {
		fatalf("Wails iOS WebView lifecycle changed; update the keyboard scroll-lock patch before building")
	}

	override := bytes.Replace(upstream, viewControllerImplementation, coveKeyboardScrollLock, 1)
	override = bytes.Replace(override, viewControllerImplementation, nativeNavigationDeclarations, 1)
	override = bytes.Replace(override, viewDidLoadStart, viewDidLoadWithKeyboardObservers, 1)
	override = bytes.Replace(override, scrollConfiguration, scrollObservation, 1)
	override = bytes.Replace(override, messageHandlerRegistration, messageHandlerWithNavigation, 1)
	override = bytes.Replace(override, webViewSubviewRegistration, webViewWithNativeNavigation, 1)
	override = bytes.Replace(override, upstreamLayout, fullBleedLayout, 1)
	if err := os.WriteFile(path, override, 0o644); err != nil {
		fatalf("write Wails iOS override: %v", err)
	}
}

func installCoveNativeNavigationSource(projectRoot, localModule string) {
	source := filepath.Join(projectRoot, "build", "ios", "cove_navigation_ios.m")
	destination := filepath.Join(localModule, "pkg", "application", "cove_navigation_ios.m")
	if err := copyFile(source, destination); err != nil {
		fatalf("install Cove native navigation source: %v", err)
	}
}

func writeModFiles(projectRoot, modfilePath, sumfilePath, localModule string) {
	goMod, err := os.ReadFile(filepath.Join(projectRoot, "go.mod"))
	if err != nil {
		fatalf("read project go.mod: %v", err)
	}
	goSum, err := os.ReadFile(filepath.Join(projectRoot, "go.sum"))
	if err != nil {
		fatalf("read project go.sum: %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(modfilePath), 0o755); err != nil {
		fatalf("create generated module directory: %v", err)
	}
	localModulePath := filepath.ToSlash(localModule)
	generatedMod := append(bytes.TrimSpace(goMod), []byte("\n\nreplace "+wailsModule+" => "+localModulePath+"\n")...)
	if err := os.WriteFile(modfilePath, generatedMod, 0o644); err != nil {
		fatalf("write generated go.mod: %v", err)
	}
	if err := os.WriteFile(sumfilePath, goSum, 0o644); err != nil {
		fatalf("write generated go.sum: %v", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "full-bleed iOS override: "+format+"\n", args...)
	os.Exit(1)
}
