#import <Cocoa/Cocoa.h>
#import "handler.h"

extern void porteroHandleURL(char *url, char *opener);

// generation invalidates older exit timers; inflight counts links still being routed, so a fast link can
// never terminate the app while a slower one (a browser still starting) is in progress.
static int generation = 0;
static int inflight = 0;

static void porteroArrived(void) {
    generation++;
    inflight++;
}

@interface PorteroDelegate : NSObject <NSApplicationDelegate>
@end

@implementation PorteroDelegate

- (void)applicationWillFinishLaunching:(NSNotification *)note {
    [[NSAppleEventManager sharedAppleEventManager]
        setEventHandler:self
            andSelector:@selector(handleGetURL:withReply:)
          forEventClass:kInternetEventClass
             andEventID:kAEGetURL];
}

- (void)applicationDidFinishLaunching:(NSNotification *)note {
    // Launched with no link (double-click): leave quietly. A document opened at launch arrives through
    // openURLs: BEFORE this callback, so the timer must not start when a link is already in flight.
    if (inflight == 0 && generation == 0) porteroExitAfter(3000);
}

// Documents (Open With, drag onto the icon): HTML files declared in Info.plist arrive here as file:// URLs.
- (void)application:(NSApplication *)app openURLs:(NSArray<NSURL *> *)urls {
    for (NSURL *u in urls) {
        porteroArrived();
        porteroHandleURL((char *)u.absoluteString.UTF8String, (char *)"");
    }
}

- (void)handleGetURL:(NSAppleEventDescriptor *)event withReply:(NSAppleEventDescriptor *)reply {
    NSString *url = [[event paramDescriptorForKeyword:keyDirectObject] stringValue] ?: @"";
    pid_t pid = [[event attributeDescriptorForKeyword:keySenderPIDAttr] int32Value];
    NSString *opener = @"";
    if (pid > 0) {
        NSRunningApplication *app = [NSRunningApplication runningApplicationWithProcessIdentifier:pid];
        opener = app.bundleIdentifier ?: @"";
    }
    porteroArrived();
    porteroHandleURL((char *)url.UTF8String, (char *)opener.UTF8String);
}

@end

void porteroRunApp(void) {
    @autoreleasepool {
        NSApplication *app = [NSApplication sharedApplication];
        PorteroDelegate *delegate = [PorteroDelegate new];
        app.delegate = delegate;
        [app setActivationPolicy:NSApplicationActivationPolicyAccessory];
        [app run];
    }
}

// Quit after ms milliseconds unless another link arrives or one is still being routed.
void porteroExitAfter(int ms) {
    int mine = generation;
    dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)ms * NSEC_PER_MSEC), dispatch_get_main_queue(), ^{
        if (mine == generation && inflight == 0) [NSApp terminate:nil];
    });
}

// Called from Go when one link has been routed (or failed); the app quits 2 s after the last one.
void porteroDone(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (inflight > 0) inflight--;
        if (inflight == 0) porteroExitAfter(2000);
    });
}

// Default handler for a scheme: bundle id of the app, or "".
char *porteroDefaultHandler(const char *scheme) {
    @autoreleasepool {
        NSURL *probe = [NSURL URLWithString:[NSString stringWithFormat:@"%s://example.com", scheme]];
        NSURL *appURL = [[NSWorkspace sharedWorkspace] URLForApplicationToOpenURL:probe];
        NSString *bid = appURL ? [NSBundle bundleWithURL:appURL].bundleIdentifier : nil;
        return strdup((bid ?: @"").UTF8String);
    }
}

// Path of the app that opens scheme links now, or "".
char *porteroDefaultAppPath(const char *scheme) {
    @autoreleasepool {
        NSURL *probe = [NSURL URLWithString:[NSString stringWithFormat:@"%s://example.com", scheme]];
        NSURL *appURL = [[NSWorkspace sharedWorkspace] URLForApplicationToOpenURL:probe];
        return strdup((appURL.path ?: @"").UTF8String);
    }
}

// "short\nbuild" of the bundle at appPath (empty parts when unreadable).
char *porteroBundleVersion(const char *appPath) {
    @autoreleasepool {
        NSURL *url = [NSURL fileURLWithPath:[NSString stringWithUTF8String:appPath]];
        NSDictionary *info = [NSBundle bundleWithURL:url].infoDictionary;
        NSString *s = info[@"CFBundleShortVersionString"] ?: @"";
        NSString *b = info[@"CFBundleVersion"] ?: @"";
        return strdup([NSString stringWithFormat:@"%@\n%@", s, b].UTF8String);
    }
}

// Ask macOS to make the app at appPath the default for scheme; macOS shows its own confirmation.
// Returns an error message or "".
char *porteroSetDefaultHandler(const char *appPath, const char *scheme) {
    @autoreleasepool {
        NSURL *appURL = [NSURL fileURLWithPath:[NSString stringWithUTF8String:appPath]];
        dispatch_semaphore_t done = dispatch_semaphore_create(0);
        // The completion block may run after a timeout: its state is an object the block retains, so a late
        // write never touches freed stack memory.
        NSMutableString *message = [NSMutableString string];
        [[NSWorkspace sharedWorkspace] setDefaultApplicationAtURL:appURL
                                              toOpenURLsWithScheme:[NSString stringWithUTF8String:scheme]
                                                 completionHandler:^(NSError *error) {
            if (error) {
                @synchronized (message) { [message setString:error.localizedDescription ?: @"unknown error"]; }
            }
            dispatch_semaphore_signal(done);
        }];
        // The completion runs on a background queue. On the main thread (the CLI) pump the main run loop while
        // waiting; on any other thread (portero serve) the main thread is not ours to pump, so just wait,
        // but not forever.
        if ([NSThread isMainThread]) {
            while (dispatch_semaphore_wait(done, dispatch_time(DISPATCH_TIME_NOW, 50 * NSEC_PER_MSEC))) {
                [[NSRunLoop mainRunLoop] runMode:NSDefaultRunLoopMode beforeDate:[NSDate dateWithTimeIntervalSinceNow:0.05]];
            }
        } else if (dispatch_semaphore_wait(done, dispatch_time(DISPATCH_TIME_NOW, 300LL * NSEC_PER_SEC)) != 0) {
            return strdup("macOS did not answer within 5 minutes");
        }
        @synchronized (message) { return strdup(message.UTF8String); }
    }
}

// Ask iCloud to download the file at path; returns at once. Returns an error message or "".
char *porteroStartDownload(const char *path) {
    @autoreleasepool {
        NSURL *u = [NSURL fileURLWithPath:[NSString stringWithUTF8String:path]];
        NSError *e = nil;
        if (![[NSFileManager defaultManager] startDownloadingUbiquitousItemAtURL:u error:&e]) {
            return strdup((e.localizedDescription ?: @"download not started").UTF8String);
        }
        return strdup("");
    }
}
