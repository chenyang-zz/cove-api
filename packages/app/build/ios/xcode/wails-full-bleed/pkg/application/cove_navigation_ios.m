#import "webview_window_ios.h"

#import <objc/runtime.h>

@class CoveProfileViewController;
@class CoveRegisterViewController;
@class CoveAuthenticatedChatViewController;

@interface WailsViewController (CoveNativeNavigationInternal)
- (void)covePushNativeRegister;
- (void)covePrepareNativeRegisterForURL:(NSURL *)sourceURL;
- (void)coveDiscardPreparedRegister;
- (void)covePopNativeRegister;
- (void)coveRegisterDidBecomeReady;
- (void)coveAuthenticationDidComplete;
- (void)coveActivateAndPushRegister;
- (void)covePrepareNativeChatForURL:(NSURL *)sourceURL;
- (void)coveDiscardPreparedChat;
- (void)coveChatDidBecomeReady;
- (void)coveChatSessionDidBecomeReady;
- (void)coveChatDidLogout;
- (void)coveNotifyChatAuthentication;
- (void)coveActivateAuthenticatedChat;
- (void)covePushNativeProfile;
- (void)coveDiscardPreparedProfile;
- (void)covePopNativeProfile;
- (void)coveProfileDidBecomeReady;
- (void)coveSetProfileNavigationLocked:(BOOL)locked;
- (void)coveProfileSessionChanged;
- (void)coveProfileDidLogout;
- (void)coveNavigationDidShowViewController:(UIViewController *)viewController;
- (void)coveActivateAndPushProfile;
@end

@interface CoveNavigationMessageHandler : NSObject <WKScriptMessageHandler>
@property (nonatomic, weak) WailsViewController *owner;
- (instancetype)initWithOwner:(WailsViewController *)owner;
@end

@interface CoveNavigationDelegate : NSObject <UINavigationControllerDelegate>
@property (nonatomic, weak) WailsViewController *owner;
@end

@interface CoveWebView : WKWebView
@end

@interface CoveProfileViewController : UIViewController <WKNavigationDelegate>
@property (nonatomic, strong) WKWebView *webView;
@property (nonatomic, strong) WailsSchemeHandler *schemeHandler;
@property (nonatomic, strong) NSURL *sourceURL;
@property (nonatomic, strong) CoveNavigationMessageHandler *messageHandler;
@property (nonatomic, assign) unsigned int windowID;
@property (nonatomic, assign, getter=isReady) BOOL ready;
@property (nonatomic, assign) BOOL coveKeyboardLocksScroll;
@property (nonatomic, assign) BOOL coveScrollEnabledBeforeKeyboard;
@property (nonatomic, assign) BOOL coveBounceEnabledBeforeKeyboard;
@property (nonatomic, assign) BOOL coveObservesContentOffset;
- (instancetype)initWithSourceURL:(NSURL *)sourceURL
                         windowID:(unsigned int)windowID
                   messageHandler:(CoveNavigationMessageHandler *)messageHandler;
@end

@interface CoveRegisterViewController : CoveProfileViewController
@end

@interface CoveAuthenticatedChatViewController : CoveProfileViewController
@end

static const void *CoveNavigationControllerKey = &CoveNavigationControllerKey;
static const void *CoveChatControllerKey = &CoveChatControllerKey;
static const void *CoveProfileControllerKey = &CoveProfileControllerKey;
static const void *CoveNavigationHandlerKey = &CoveNavigationHandlerKey;
static const void *CoveNavigationDelegateKey = &CoveNavigationDelegateKey;
static const void *CoveProfilePendingKey = &CoveProfilePendingKey;
static const void *CoveProfileLockedKey = &CoveProfileLockedKey;
static const void *CoveRegisterControllerKey = &CoveRegisterControllerKey;
static const void *CoveRegisterPendingKey = &CoveRegisterPendingKey;
static const void *CoveAuthenticatedChatControllerKey = &CoveAuthenticatedChatControllerKey;
static const void *CoveChatPendingKey = &CoveChatPendingKey;
static const void *CoveChatSessionReadyKey = &CoveChatSessionReadyKey;
static void *CoveRouteContentOffsetObservationContext = &CoveRouteContentOffsetObservationContext;

static UIColor *CovePageColor(void) {
    if (@available(iOS 13.0, *)) {
        return [UIColor colorWithDynamicProvider:^UIColor *(UITraitCollection *traits) {
            if (traits.userInterfaceStyle == UIUserInterfaceStyleDark) {
                return [UIColor colorWithRed:11.0 / 255.0 green:32.0 / 255.0 blue:39.0 / 255.0 alpha:1.0];
            }
            return [UIColor colorWithRed:241.0 / 255.0 green:248.0 / 255.0 blue:248.0 / 255.0 alpha:1.0];
        }];
    }
    return [UIColor colorWithRed:241.0 / 255.0 green:248.0 / 255.0 blue:248.0 / 255.0 alpha:1.0];
}

static NSURL *CoveRouteURL(NSURL *sourceURL, NSString *route) {
    if (!sourceURL) return nil;
    NSURLComponents *components = [NSURLComponents componentsWithURL:sourceURL resolvingAgainstBaseURL:NO];
    if (!components) return sourceURL;
    NSMutableArray<NSURLQueryItem *> *items = [NSMutableArray array];
    for (NSURLQueryItem *item in components.queryItems ?: @[]) {
        if (![item.name isEqualToString:@"coveRoute"]) {
            [items addObject:item];
        }
    }
    [items addObject:[NSURLQueryItem queryItemWithName:@"coveRoute" value:route]];
    components.queryItems = items;
    return components.URL ?: sourceURL;
}

static NSURL *CoveProfileURL(NSURL *sourceURL) {
    return CoveRouteURL(sourceURL, @"profile");
}

static NSURL *CoveRegisterURL(NSURL *sourceURL) {
    return CoveRouteURL(sourceURL, @"register");
}

static NSURL *CoveChatURL(NSURL *sourceURL) {
    return CoveRouteURL(sourceURL, @"chat");
}

@implementation CoveNavigationMessageHandler

- (instancetype)initWithOwner:(WailsViewController *)owner {
    self = [super init];
    if (self) {
        _owner = owner;
    }
    return self;
}

- (void)userContentController:(WKUserContentController *)userContentController
      didReceiveScriptMessage:(WKScriptMessage *)message {
    if (![message.body isKindOfClass:[NSDictionary class]]) return;
    NSDictionary *body = (NSDictionary *)message.body;
    NSString *action = [body[@"action"] isKindOfClass:[NSString class]] ? body[@"action"] : nil;
    WailsViewController *owner = self.owner;
    if (!owner || !action.length) return;

    if ([action isEqualToString:@"prepareChat"]) {
        [owner covePrepareNativeChatForURL:owner.webView.URL];
    } else if ([action isEqualToString:@"chatReady"]) {
        [owner coveChatDidBecomeReady];
    } else if ([action isEqualToString:@"chatSessionReady"]) {
        [owner coveChatSessionDidBecomeReady];
    } else if ([action isEqualToString:@"chatLogout"]) {
        [owner coveChatDidLogout];
    } else if ([action isEqualToString:@"prepareRegister"]) {
        [owner covePrepareNativeRegisterForURL:owner.webView.URL];
    } else if ([action isEqualToString:@"pushRegister"]) {
        [owner covePushNativeRegister];
    } else if ([action isEqualToString:@"popRegister"]) {
        [owner covePopNativeRegister];
    } else if ([action isEqualToString:@"registerReady"]) {
        [owner coveRegisterDidBecomeReady];
    } else if ([action isEqualToString:@"authCompleted"]) {
        [owner coveAuthenticationDidComplete];
    } else if ([action isEqualToString:@"pushProfile"]) {
        [owner covePushNativeProfile];
    } else if ([action isEqualToString:@"popProfile"]) {
        [owner covePopNativeProfile];
    } else if ([action isEqualToString:@"profileReady"]) {
        [owner coveProfileDidBecomeReady];
    } else if ([action isEqualToString:@"profileNavigationLock"]) {
        [owner coveSetProfileNavigationLocked:[body[@"locked"] boolValue]];
    } else if ([action isEqualToString:@"profileSessionChanged"]) {
        [owner coveProfileSessionChanged];
    } else if ([action isEqualToString:@"profileLogout"]) {
        [owner coveProfileDidLogout];
    }
}

@end

@implementation CoveNavigationDelegate

- (void)navigationController:(UINavigationController *)navigationController
       didShowViewController:(UIViewController *)viewController
                    animated:(BOOL)animated {
    [self.owner coveNavigationDidShowViewController:viewController];
}

@end

@implementation CoveWebView

- (UIView *)inputAccessoryView {
    return nil;
}

@end

@implementation CoveProfileViewController

- (instancetype)initWithSourceURL:(NSURL *)sourceURL
                         windowID:(unsigned int)windowID
                   messageHandler:(CoveNavigationMessageHandler *)messageHandler {
    self = [super init];
    if (self) {
        _sourceURL = CoveProfileURL(sourceURL);
        _windowID = windowID;
        _messageHandler = messageHandler;
        _ready = NO;
    }
    return self;
}

- (void)loadView {
    UIView *pageView = [[UIView alloc] initWithFrame:CGRectZero];
    pageView.backgroundColor = CovePageColor();

    WKWebViewConfiguration *configuration = [[WKWebViewConfiguration alloc] init];
    configuration.websiteDataStore = WKWebsiteDataStore.defaultDataStore;
    if (@available(iOS 14.0, *)) {
        configuration.defaultWebpagePreferences.allowsContentJavaScript = YES;
    }
    if ([self.sourceURL.scheme.lowercaseString isEqualToString:@"wails"]) {
        self.schemeHandler = [[WailsSchemeHandler alloc] initWithWindowID:self.windowID];
        [configuration setURLSchemeHandler:self.schemeHandler forURLScheme:@"wails"];
    }
    [configuration.userContentController addScriptMessageHandler:self.messageHandler name:@"coveNavigation"];

    self.webView = [[CoveWebView alloc] initWithFrame:CGRectZero configuration:configuration];
    self.webView.autoresizingMask = UIViewAutoresizingFlexibleWidth | UIViewAutoresizingFlexibleHeight;
    self.webView.navigationDelegate = self;
    self.webView.opaque = NO;
    self.webView.alpha = 0.0;
    self.webView.backgroundColor = CovePageColor();
    self.webView.scrollView.backgroundColor = CovePageColor();
    self.webView.scrollView.contentInset = UIEdgeInsetsZero;
    self.webView.scrollView.scrollIndicatorInsets = UIEdgeInsetsZero;
    self.webView.scrollView.bounces = NO;
    self.webView.scrollView.alwaysBounceHorizontal = NO;
    self.webView.scrollView.showsHorizontalScrollIndicator = NO;
    if (@available(iOS 11.0, *)) {
        self.webView.scrollView.contentInsetAdjustmentBehavior = UIScrollViewContentInsetAdjustmentNever;
    }
    if (@available(iOS 16.4, *)) {
        self.webView.inspectable = YES;
    }
    [pageView addSubview:self.webView];
    self.view = pageView;
    if (self.sourceURL) {
        [self.webView loadRequest:[NSURLRequest requestWithURL:self.sourceURL]];
    }
}

- (void)viewDidLoad {
    [super viewDidLoad];

    NSNotificationCenter *notifications = [NSNotificationCenter defaultCenter];
    [notifications addObserver:self
                      selector:@selector(coveKeyboardWillChangeFrame:)
                          name:UIKeyboardWillChangeFrameNotification
                        object:nil];
    [notifications addObserver:self
                      selector:@selector(coveKeyboardWillHide:)
                          name:UIKeyboardWillHideNotification
                        object:nil];

    [self.webView.scrollView addObserver:self
                              forKeyPath:@"contentOffset"
                                 options:NSKeyValueObservingOptionNew
                                 context:CoveRouteContentOffsetObservationContext];
    self.coveObservesContentOffset = YES;
}

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

    if (!keyboardVisible) return;
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

- (void)coveKeyboardWillHide:(NSNotification *)notification {
    [self coveSetKeyboardScrollLocked:NO];
}

- (void)observeValueForKeyPath:(NSString *)keyPath
                      ofObject:(id)object
                        change:(NSDictionary<NSKeyValueChangeKey, id> *)change
                       context:(void *)context {
    if (context == CoveRouteContentOffsetObservationContext) {
        UIScrollView *scrollView = (UIScrollView *)object;
        if (self.coveKeyboardLocksScroll && !CGPointEqualToPoint(scrollView.contentOffset, CGPointZero)) {
            [scrollView setContentOffset:CGPointZero animated:NO];
        }
        return;
    }
    [super observeValueForKeyPath:keyPath ofObject:object change:change context:context];
}

- (void)setReady:(BOOL)ready {
    _ready = ready;
    if (!ready || !self.isViewLoaded) {
        self.webView.alpha = 0.0;
        return;
    }

    // React reports readiness after mounting. Reveal on the next main-loop turn
    // so WebKit can commit that first rendered frame before UIKit exposes it.
    dispatch_async(dispatch_get_main_queue(), ^{
        if (self.isReady) {
            self.webView.alpha = 1.0;
        }
    });
}

- (void)dealloc {
    [[NSNotificationCenter defaultCenter] removeObserver:self];
    if (self.coveObservesContentOffset) {
        [self.webView.scrollView removeObserver:self
                                     forKeyPath:@"contentOffset"
                                        context:CoveRouteContentOffsetObservationContext];
    }
    [self.webView.configuration.userContentController removeScriptMessageHandlerForName:@"coveNavigation"];
}

@end

@implementation CoveRegisterViewController

- (instancetype)initWithSourceURL:(NSURL *)sourceURL
                         windowID:(unsigned int)windowID
                   messageHandler:(CoveNavigationMessageHandler *)messageHandler {
    self = [super initWithSourceURL:sourceURL windowID:windowID messageHandler:messageHandler];
    if (self) {
        self.sourceURL = CoveRegisterURL(sourceURL);
    }
    return self;
}

@end

@implementation CoveAuthenticatedChatViewController

- (instancetype)initWithSourceURL:(NSURL *)sourceURL
                         windowID:(unsigned int)windowID
                   messageHandler:(CoveNavigationMessageHandler *)messageHandler {
    self = [super initWithSourceURL:sourceURL windowID:windowID messageHandler:messageHandler];
    if (self) {
        self.sourceURL = CoveChatURL(sourceURL);
    }
    return self;
}

@end

@implementation WailsViewController (CoveNativeNavigation)

- (UINavigationController *)coveNavigationController {
    return objc_getAssociatedObject(self, CoveNavigationControllerKey);
}

- (UIViewController *)coveChatController {
    return objc_getAssociatedObject(self, CoveChatControllerKey);
}

- (CoveProfileViewController *)coveProfileController {
    return objc_getAssociatedObject(self, CoveProfileControllerKey);
}

- (CoveRegisterViewController *)coveRegisterController {
    return objc_getAssociatedObject(self, CoveRegisterControllerKey);
}

- (CoveAuthenticatedChatViewController *)coveAuthenticatedChatController {
    return objc_getAssociatedObject(self, CoveAuthenticatedChatControllerKey);
}

- (CoveNavigationMessageHandler *)coveNavigationHandler {
    return objc_getAssociatedObject(self, CoveNavigationHandlerKey);
}

- (BOOL)coveProfilePending {
    return [objc_getAssociatedObject(self, CoveProfilePendingKey) boolValue];
}

- (void)coveSetProfilePending:(BOOL)pending {
    objc_setAssociatedObject(self, CoveProfilePendingKey, @(pending), OBJC_ASSOCIATION_RETAIN_NONATOMIC);
}

- (BOOL)coveProfileLocked {
    return [objc_getAssociatedObject(self, CoveProfileLockedKey) boolValue];
}

- (BOOL)coveRegisterPending {
    return [objc_getAssociatedObject(self, CoveRegisterPendingKey) boolValue];
}

- (void)coveSetRegisterPending:(BOOL)pending {
    objc_setAssociatedObject(self, CoveRegisterPendingKey, @(pending), OBJC_ASSOCIATION_RETAIN_NONATOMIC);
}

- (BOOL)coveChatPending {
    return [objc_getAssociatedObject(self, CoveChatPendingKey) boolValue];
}

- (void)coveSetChatPending:(BOOL)pending {
    objc_setAssociatedObject(self, CoveChatPendingKey, @(pending), OBJC_ASSOCIATION_RETAIN_NONATOMIC);
}

- (BOOL)coveChatSessionReady {
    return [objc_getAssociatedObject(self, CoveChatSessionReadyKey) boolValue];
}

- (void)coveSetChatSessionReady:(BOOL)ready {
    objc_setAssociatedObject(self, CoveChatSessionReadyKey, @(ready), OBJC_ASSOCIATION_RETAIN_NONATOMIC);
}

- (void)coveConfigureNativeNavigation:(WKWebViewConfiguration *)configuration {
    if ([self coveNavigationHandler]) return;
    CoveNavigationMessageHandler *handler = [[CoveNavigationMessageHandler alloc] initWithOwner:self];
    objc_setAssociatedObject(self, CoveNavigationHandlerKey, handler, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
    [configuration.userContentController addScriptMessageHandler:handler name:@"coveNavigation"];
}

- (void)coveInstallNativeNavigation {
    if ([self coveNavigationController] || !self.webView) return;

    [self.webView removeFromSuperview];
    UIViewController *chatController = [[UIViewController alloc] init];
    chatController.view = self.webView;
    chatController.view.backgroundColor = CovePageColor();

    UINavigationController *navigationController =
        [[UINavigationController alloc] initWithRootViewController:chatController];
    [navigationController setNavigationBarHidden:YES animated:NO];
    navigationController.view.frame = self.view.bounds;
    navigationController.view.autoresizingMask = UIViewAutoresizingFlexibleWidth | UIViewAutoresizingFlexibleHeight;
    navigationController.view.backgroundColor = CovePageColor();

    CoveNavigationDelegate *navigationDelegate = [[CoveNavigationDelegate alloc] init];
    navigationDelegate.owner = self;
    navigationController.delegate = navigationDelegate;
    navigationController.interactivePopGestureRecognizer.delegate = nil;
    navigationController.interactivePopGestureRecognizer.enabled = NO;

    objc_setAssociatedObject(self, CoveChatControllerKey, chatController, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
    objc_setAssociatedObject(self, CoveNavigationControllerKey, navigationController, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
    objc_setAssociatedObject(self, CoveNavigationDelegateKey, navigationDelegate, OBJC_ASSOCIATION_RETAIN_NONATOMIC);

    [self addChildViewController:navigationController];
    [self.view addSubview:navigationController.view];
    [navigationController didMoveToParentViewController:self];
}

- (void)covePrepareNativeRegisterForURL:(NSURL *)sourceURL {
    [self coveDiscardPreparedChat];
    if ([self coveRegisterController] || !sourceURL) return;
    CoveRegisterViewController *registerController =
        [[CoveRegisterViewController alloc] initWithSourceURL:sourceURL
                                                    windowID:self.windowID
                                              messageHandler:[self coveNavigationHandler]];
    objc_setAssociatedObject(self, CoveRegisterControllerKey, registerController, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
    [registerController loadViewIfNeeded];
}

- (void)coveDiscardPreparedRegister {
    CoveRegisterViewController *registerController = [self coveRegisterController];
    UINavigationController *navigationController = [self coveNavigationController];
    BOOL registerInStack = registerController &&
        [navigationController.viewControllers containsObject:registerController];
    if (!registerController || registerInStack) return;

    [registerController.webView stopLoading];
    registerController.webView.navigationDelegate = nil;
    objc_setAssociatedObject(self, CoveRegisterControllerKey, nil, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
}

- (void)covePushNativeRegister {
    UINavigationController *navigationController = [self coveNavigationController];
    if (!navigationController || navigationController.topViewController != [self coveChatController]) return;
    [self coveSetRegisterPending:YES];
    if (![self coveRegisterController]) {
        [self covePrepareNativeRegisterForURL:self.webView.URL];
    }
    [self coveActivateAndPushRegister];
}

- (void)coveActivateAndPushRegister {
    CoveRegisterViewController *registerController = [self coveRegisterController];
    UINavigationController *navigationController = [self coveNavigationController];
    if (!registerController || !navigationController ||
        navigationController.topViewController != [self coveChatController]) return;

    [self coveSetRegisterPending:NO];
    dispatch_async(dispatch_get_main_queue(), ^{
        if (navigationController.topViewController == [self coveChatController]) {
            [navigationController pushViewController:registerController animated:YES];
        }
    });
}

- (void)covePopNativeRegister {
    UINavigationController *navigationController = [self coveNavigationController];
    if (navigationController.topViewController == [self coveRegisterController]) {
        [navigationController popViewControllerAnimated:YES];
    }
}

- (void)coveRegisterDidBecomeReady {
    [self coveRegisterController].ready = YES;
    if ([self coveRegisterPending]) {
        [self coveActivateAndPushRegister];
    }
}

- (void)coveAuthenticationDidComplete {
    [self coveSetRegisterPending:NO];
    [self coveSetChatPending:YES];
    [self coveSetChatSessionReady:NO];
    if (![self coveAuthenticatedChatController]) {
        [self covePrepareNativeChatForURL:self.webView.URL];
    }
    [self coveNotifyChatAuthentication];
}

- (void)covePrepareNativeChatForURL:(NSURL *)sourceURL {
    [self coveDiscardPreparedRegister];
    if ([self coveAuthenticatedChatController] || !sourceURL) return;
    CoveAuthenticatedChatViewController *chatController =
        [[CoveAuthenticatedChatViewController alloc] initWithSourceURL:sourceURL
                                                              windowID:self.windowID
                                                        messageHandler:[self coveNavigationHandler]];
    objc_setAssociatedObject(self, CoveAuthenticatedChatControllerKey, chatController, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
    [chatController loadViewIfNeeded];
}

- (void)coveDiscardPreparedChat {
    CoveAuthenticatedChatViewController *chatController = [self coveAuthenticatedChatController];
    UINavigationController *navigationController = [self coveNavigationController];
    BOOL chatInStack = chatController &&
        [navigationController.viewControllers containsObject:chatController];
    if (!chatController || chatInStack || [self coveChatPending]) return;

    [chatController.webView stopLoading];
    chatController.webView.navigationDelegate = nil;
    objc_setAssociatedObject(self, CoveAuthenticatedChatControllerKey, nil, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
    [self coveSetChatSessionReady:NO];
}

- (void)coveChatDidBecomeReady {
    [self coveAuthenticatedChatController].ready = YES;
    [self coveNotifyChatAuthentication];
}

- (void)coveNotifyChatAuthentication {
    CoveAuthenticatedChatViewController *chatController = [self coveAuthenticatedChatController];
    if (![self coveChatPending] || !chatController.isReady) return;
    [chatController.webView evaluateJavaScript:
        @"window.dispatchEvent(new CustomEvent('cove:native-chat-authenticated'));"
                                completionHandler:nil];
}

- (void)coveChatSessionDidBecomeReady {
    [self coveSetChatSessionReady:YES];
    if ([self coveChatPending]) {
        [self coveActivateAuthenticatedChat];
    }
}

- (void)coveActivateAuthenticatedChat {
    UINavigationController *navigationController = [self coveNavigationController];
    CoveAuthenticatedChatViewController *chatController = [self coveAuthenticatedChatController];
    if (!navigationController || !chatController || !chatController.isReady ||
        ![self coveChatPending] || ![self coveChatSessionReady]) return;

    UIViewController *topController = navigationController.topViewController;
    BOOL authenticationVisible =
        topController == [self coveChatController] || topController == [self coveRegisterController];
    if (!authenticationVisible) return;

    [self coveSetChatPending:NO];
    dispatch_async(dispatch_get_main_queue(), ^{
        [navigationController pushViewController:chatController animated:YES];
    });
}

- (void)coveChatDidLogout {
    [self coveSetChatPending:NO];
    [self coveSetChatSessionReady:NO];
    [self.webView evaluateJavaScript:
        @"window.dispatchEvent(new CustomEvent('cove:native-profile-logout'));"
                       completionHandler:nil];

    UINavigationController *navigationController = [self coveNavigationController];
    UIViewController *authenticationController = [self coveChatController];
    if (navigationController && authenticationController) {
        [navigationController setViewControllers:@[authenticationController] animated:YES];
    }

    [self coveDiscardPreparedProfile];

    [self coveAuthenticatedChatController].webView.navigationDelegate = nil;
    objc_setAssociatedObject(self, CoveAuthenticatedChatControllerKey, nil, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
}

- (void)covePrepareNativeProfileForURL:(NSURL *)sourceURL {
    if ([self coveProfileController] || !sourceURL) return;
    CoveProfileViewController *profileController =
        [[CoveProfileViewController alloc] initWithSourceURL:sourceURL
                                                    windowID:self.windowID
                                              messageHandler:[self coveNavigationHandler]];
    objc_setAssociatedObject(self, CoveProfileControllerKey, profileController, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
    [profileController loadViewIfNeeded];
}

- (void)coveDiscardPreparedProfile {
    CoveProfileViewController *profileController = [self coveProfileController];
    UINavigationController *navigationController = [self coveNavigationController];
    BOOL profileInStack = profileController &&
        [navigationController.viewControllers containsObject:profileController];
    if (!profileController || profileInStack) return;

    [profileController.webView stopLoading];
    profileController.webView.navigationDelegate = nil;
    objc_setAssociatedObject(self, CoveProfileControllerKey, nil, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
    [self coveSetProfilePending:NO];
}

- (void)covePushNativeProfile {
    UINavigationController *navigationController = [self coveNavigationController];
    UIViewController *topController = navigationController.topViewController;
    BOOL chatVisible =
        topController == [self coveChatController] || topController == [self coveAuthenticatedChatController];
    if (!navigationController || !chatVisible) return;
    [self coveSetProfilePending:YES];
    if (![self coveProfileController]) {
        [self covePrepareNativeProfileForURL:self.webView.URL];
    }
    if ([self coveProfileController].isReady) {
        [self coveActivateAndPushProfile];
    }
}

- (void)coveActivateAndPushProfile {
    CoveProfileViewController *profileController = [self coveProfileController];
    UINavigationController *navigationController = [self coveNavigationController];
    UIViewController *topController = navigationController.topViewController;
    BOOL chatVisible =
        topController == [self coveChatController] || topController == [self coveAuthenticatedChatController];
    if (!profileController || !navigationController || !chatVisible) return;

    [self coveSetProfilePending:NO];
    NSString *activate = @"window.dispatchEvent(new CustomEvent('cove:native-profile-activate'));";
    [profileController.webView evaluateJavaScript:activate completionHandler:^(__unused id result, __unused NSError *error) {
        dispatch_async(dispatch_get_main_queue(), ^{
            UIViewController *topController = navigationController.topViewController;
            if (topController == [self coveChatController] ||
                topController == [self coveAuthenticatedChatController]) {
                [navigationController pushViewController:profileController animated:YES];
            }
        });
    }];
}

- (void)covePopNativeProfile {
    UINavigationController *navigationController = [self coveNavigationController];
    if (navigationController.topViewController == [self coveProfileController]) {
        [navigationController popViewControllerAnimated:YES];
    }
}

- (void)coveProfileDidBecomeReady {
    [self coveProfileController].ready = YES;
    if ([self coveProfilePending]) {
        [self coveActivateAndPushProfile];
    }
}

- (void)coveSetProfileNavigationLocked:(BOOL)locked {
    objc_setAssociatedObject(self, CoveProfileLockedKey, @(locked), OBJC_ASSOCIATION_RETAIN_NONATOMIC);
    UINavigationController *navigationController = [self coveNavigationController];
    BOOL profileVisible = navigationController.topViewController == [self coveProfileController];
    navigationController.interactivePopGestureRecognizer.enabled = profileVisible && !locked;
}

- (void)coveProfileSessionChanged {
    [self.webView evaluateJavaScript:
        @"window.dispatchEvent(new CustomEvent('cove:native-profile-session-changed'));"
                       completionHandler:nil];
    [[self coveAuthenticatedChatController].webView evaluateJavaScript:
        @"window.dispatchEvent(new CustomEvent('cove:native-profile-session-changed'));"
                                                    completionHandler:nil];
}

- (void)coveProfileDidLogout {
    [self.webView evaluateJavaScript:
        @"window.dispatchEvent(new CustomEvent('cove:native-profile-logout'));"
                       completionHandler:nil];
    UINavigationController *navigationController = [self coveNavigationController];
    if ([self coveAuthenticatedChatController]) {
        [self coveChatDidLogout];
    } else {
        [navigationController popToRootViewControllerAnimated:YES];
    }
}

- (void)coveNavigationDidShowViewController:(UIViewController *)viewController {
    UINavigationController *navigationController = [self coveNavigationController];
    CoveRegisterViewController *registerController = [self coveRegisterController];
    BOOL profileVisible = viewController == [self coveProfileController];
    BOOL registerVisible = viewController == registerController;
    BOOL authenticatedChatVisible = viewController == [self coveAuthenticatedChatController];
    navigationController.interactivePopGestureRecognizer.enabled =
        registerVisible || (profileVisible && ![self coveProfileLocked]);
    if (authenticatedChatVisible && navigationController.viewControllers.count > 1) {
        [navigationController setViewControllers:@[viewController] animated:NO];
    }
    if (!profileVisible) {
        [[self coveProfileController].webView evaluateJavaScript:
            @"window.dispatchEvent(new CustomEvent('cove:native-profile-hidden'));"
                                             completionHandler:nil];
    }
    if (!registerVisible) {
        [registerController.webView evaluateJavaScript:
            @"window.dispatchEvent(new CustomEvent('cove:native-register-hidden'));"
                                              completionHandler:nil];
        [self coveDiscardPreparedRegister];
        if (viewController == [self coveChatController]) {
            [self covePrepareNativeRegisterForURL:self.webView.URL];
        }
    }
    if (authenticatedChatVisible) {
        NSURL *chatURL = [self coveAuthenticatedChatController].webView.URL ?: self.webView.URL;
        [self covePrepareNativeProfileForURL:chatURL];
    }
}

- (void)coveTearDownNativeNavigation {
    [self.webView.configuration.userContentController removeScriptMessageHandlerForName:@"coveNavigation"];
    [self coveProfileController].webView.navigationDelegate = nil;
    [self coveRegisterController].webView.navigationDelegate = nil;
    [self coveAuthenticatedChatController].webView.navigationDelegate = nil;
    [self coveNavigationController].delegate = nil;
}

@end
