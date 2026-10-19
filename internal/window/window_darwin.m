// 맥 설정 창: NSWindow + WKWebView. 편집 메뉴가 있어야 입력칸에서 ⌘C·⌘V 가 된다.
#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

@interface GSDelegate : NSObject <NSApplicationDelegate, WKNavigationDelegate, WKUIDelegate>
@property(strong) NSNumber *port;
@end

@implementation GSDelegate
- (BOOL)applicationShouldTerminateAfterLastWindowClosed:(NSApplication *)sender {
	return YES;
}

// 설정 화면 밖으로 나가는 링크는 기본 브라우저로 연다
- (void)webView:(WKWebView *)webView
    decidePolicyForNavigationAction:(WKNavigationAction *)action
                    decisionHandler:(void (^)(WKNavigationActionPolicy))handler {
	NSURL *u = action.request.URL;
	if ([u.host isEqualToString:@"127.0.0.1"] && [u.port isEqualToNumber:self.port]) {
		handler(WKNavigationActionPolicyAllow);
		return;
	}
	[[NSWorkspace sharedWorkspace] openURL:u];
	handler(WKNavigationActionPolicyCancel);
}

// target="_blank" 링크
- (WKWebView *)webView:(WKWebView *)webView
    createWebViewWithConfiguration:(WKWebViewConfiguration *)configuration
               forNavigationAction:(WKNavigationAction *)action
                    windowFeatures:(WKWindowFeatures *)features {
	[[NSWorkspace sharedWorkspace] openURL:action.request.URL];
	return nil;
}

- (void)webView:(WKWebView *)webView
    runJavaScriptAlertPanelWithMessage:(NSString *)message
                      initiatedByFrame:(WKFrameInfo *)frame
                     completionHandler:(void (^)(void))completionHandler {
	NSAlert *a = [NSAlert new];
	a.messageText = message;
	[a runModal];
	completionHandler();
}

- (void)webView:(WKWebView *)webView
    runJavaScriptConfirmPanelWithMessage:(NSString *)message
                        initiatedByFrame:(WKFrameInfo *)frame
                       completionHandler:(void (^)(BOOL))completionHandler {
	NSAlert *a = [NSAlert new];
	a.messageText = message;
	[a addButtonWithTitle:@"확인"];
	[a addButtonWithTitle:@"취소"];
	completionHandler([a runModal] == NSAlertFirstButtonReturn);
}
@end

static GSDelegate *gsDelegate;
static NSWindow *gsWindow;

static NSMenuItem *item(NSString *title, SEL action, NSString *key) {
	return [[NSMenuItem alloc] initWithTitle:title action:action keyEquivalent:key];
}

static void buildMenu(void) {
	NSMenu *bar = [NSMenu new];

	NSMenuItem *appItem = [NSMenuItem new];
	NSMenu *appMenu = [NSMenu new];
	[appMenu addItem:item(@"설정 창 닫기", @selector(terminate:), @"q")];
	appItem.submenu = appMenu;
	[bar addItem:appItem];

	NSMenuItem *editItem = [NSMenuItem new];
	NSMenu *edit = [[NSMenu alloc] initWithTitle:@"편집"];
	[edit addItem:item(@"실행 취소", @selector(undo:), @"z")];
	[edit addItem:item(@"다시 실행", @selector(redo:), @"Z")];
	[edit addItem:[NSMenuItem separatorItem]];
	[edit addItem:item(@"잘라내기", @selector(cut:), @"x")];
	[edit addItem:item(@"복사", @selector(copy:), @"c")];
	[edit addItem:item(@"붙여넣기", @selector(paste:), @"v")];
	[edit addItem:item(@"모두 선택", @selector(selectAll:), @"a")];
	editItem.submenu = edit;
	[bar addItem:editItem];

	NSMenuItem *winItem = [NSMenuItem new];
	NSMenu *win = [[NSMenu alloc] initWithTitle:@"윈도우"];
	[win addItem:item(@"닫기", @selector(performClose:), @"w")];
	[win addItem:item(@"최소화", @selector(performMiniaturize:), @"m")];
	[win addItem:item(@"새로 고침", @selector(reload:), @"r")];
	winItem.submenu = win;
	[bar addItem:winItem];

	NSApp.mainMenu = bar;
}

void gsRunWindow(const char *curl, const char *ctitle, int width, int height) {
	@autoreleasepool {
		NSApplication *app = [NSApplication sharedApplication];
		[app setActivationPolicy:NSApplicationActivationPolicyRegular];
		NSURL *url = [NSURL URLWithString:[NSString stringWithUTF8String:curl]];
		gsDelegate = [GSDelegate new];
		gsDelegate.port = url.port;
		app.delegate = gsDelegate;
		buildMenu();

		NSWindowStyleMask style = NSWindowStyleMaskTitled | NSWindowStyleMaskClosable |
		                          NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable;
		gsWindow = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, width, height)
		                                       styleMask:style
		                                         backing:NSBackingStoreBuffered
		                                           defer:NO];
		gsWindow.title = [NSString stringWithUTF8String:ctitle];
		gsWindow.releasedWhenClosed = NO;
		gsWindow.minSize = NSMakeSize(420, 420);
		[gsWindow center];
		[gsWindow setFrameAutosaveName:@"GodseonSyncSettings"];

		WKWebView *web = [[WKWebView alloc] initWithFrame:gsWindow.contentView.bounds
		                                    configuration:[WKWebViewConfiguration new]];
		web.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
		web.navigationDelegate = gsDelegate;
		web.UIDelegate = gsDelegate;
		[web setValue:@NO forKey:@"drawsBackground"]; // 불러오는 동안 흰 화면 대신 창 배경색
		[gsWindow.contentView addSubview:web];
		[web loadRequest:[NSURLRequest requestWithURL:url]];

		[gsWindow makeKeyAndOrderFront:nil];
		[app activateIgnoringOtherApps:YES];
		[app run];
	}
}

void gsFocusWindow(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		[NSApp activateIgnoringOtherApps:YES];
		[gsWindow makeKeyAndOrderFront:nil];
	});
}

void gsCloseWindow(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		[NSApp terminate:nil];
	});
}
