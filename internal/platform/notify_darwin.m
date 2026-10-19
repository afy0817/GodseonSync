// 맥 알림: UserNotifications. 앱 묶음 안에서만 쓸 수 있다 (밖에서 부르면 예외로 죽는다).
#import <Foundation/Foundation.h>
#import <UserNotifications/UserNotifications.h>

void gsNotifyClicked(void);
void gsNotifyFailed(char *msg);

@interface GSNotifyDelegate : NSObject <UNUserNotificationCenterDelegate>
@end

@implementation GSNotifyDelegate
// 설정 창이 맨 앞에 있어도 배너를 띄운다 (같은 앱 묶음이라 안 띄우면 묻힌다)
- (void)userNotificationCenter:(UNUserNotificationCenter *)center
       willPresentNotification:(UNNotification *)notification
         withCompletionHandler:(void (^)(UNNotificationPresentationOptions))handler {
	handler(UNNotificationPresentationOptionBanner | UNNotificationPresentationOptionList);
}

// 알림을 누르면 설정 창을 연다
- (void)userNotificationCenter:(UNUserNotificationCenter *)center
    didReceiveNotificationResponse:(UNNotificationResponse *)response
             withCompletionHandler:(void (^)(void))handler {
	if ([response.actionIdentifier isEqualToString:UNNotificationDefaultActionIdentifier]) {
		gsNotifyClicked();
	}
	handler();
}
@end

static GSNotifyDelegate *gsNotifyDelegate;
static BOOL gsNotifyReported;
// 처음 실행 때 허용 물음에 답하기 전에는 알림을 보내면 거절된다 → 답이 올 때까지 모아 둔다 (최근 것 몇 개만)
static BOOL gsAnswered;
static NSMutableArray<UNNotificationRequest *> *gsPending;
static const NSUInteger gsMaxPending = 5;

static void reportOnce(NSString *msg) {
	@synchronized([GSNotifyDelegate class]) {
		if (gsNotifyReported) return;
		gsNotifyReported = YES;
	}
	gsNotifyFailed((char *)msg.UTF8String);
}

// deliver 는 지금 권한을 보고 보낸다 (시스템 설정에서 나중에 바꿔도 따라가게 매번 본다)
static void deliver(UNNotificationRequest *r) {
	UNUserNotificationCenter *c = UNUserNotificationCenter.currentNotificationCenter;
	[c getNotificationSettingsWithCompletionHandler:^(UNNotificationSettings *s) {
		if (s.authorizationStatus == UNAuthorizationStatusDenied ||
		    s.authorizationStatus == UNAuthorizationStatusNotDetermined) {
			reportOnce(@"denied");
			return;
		}
		[c addNotificationRequest:r
		    withCompletionHandler:^(NSError *err) {
			    if (err != nil) reportOnce(err.localizedDescription);
		    }];
	}];
}

int gsNotifyInit(void) {
	NSBundle *b = NSBundle.mainBundle;
	if (b.bundleIdentifier == nil || ![b.bundlePath.pathExtension isEqualToString:@"app"]) {
		return 0;
	}
	UNUserNotificationCenter *c = UNUserNotificationCenter.currentNotificationCenter;
	gsNotifyDelegate = [GSNotifyDelegate new];
	gsPending = [NSMutableArray new];
	c.delegate = gsNotifyDelegate;
	// 이미 정해진 권한이면 바로 답이 오고, 처음이면 사용자가 허용·허용 안 함을 누를 때 온다
	[c requestAuthorizationWithOptions:UNAuthorizationOptionAlert
	                 completionHandler:^(BOOL granted, NSError *error) {
		                 NSArray<UNNotificationRequest *> *pending;
		                 @synchronized([GSNotifyDelegate class]) {
			                 gsAnswered = YES;
			                 pending = [gsPending copy];
			                 [gsPending removeAllObjects];
		                 }
		                 if (error != nil) reportOnce(error.localizedDescription);
		                 for (UNNotificationRequest *r in pending) deliver(r);
	                 }];
	return 1;
}

void gsNotifyPost(const char *ctitle, const char *cbody) {
	NSString *title = [NSString stringWithUTF8String:ctitle];
	UNMutableNotificationContent *n = [UNMutableNotificationContent new];
	// 알림 맨 위에 앱 이름이 이미 나오니 제목이 앱 이름과 같으면 빼고 내용만 쓴다
	if (![title isEqualToString:NSBundle.mainBundle.infoDictionary[@"CFBundleName"]]) {
		n.title = title;
	}
	n.body = [NSString stringWithUTF8String:cbody];
	UNNotificationRequest *r = [UNNotificationRequest requestWithIdentifier:NSUUID.UUID.UUIDString
	                                                                content:n
	                                                                trigger:nil];
	@synchronized([GSNotifyDelegate class]) {
		if (!gsAnswered) {
			[gsPending addObject:r];
			if (gsPending.count > gsMaxPending) [gsPending removeObjectAtIndex:0];
			return;
		}
	}
	deliver(r);
}
