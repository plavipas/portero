#import <Cocoa/Cocoa.h>
#import <os/log.h>
#import "handler.h"

extern void porteroHandleURL(char *url, char *opener);

// generation invalidates older exit timers; inflight counts links still being routed, so a fast link can
// never terminate the app while a slower one (a browser still starting) is in progress.
static int generation = 0;
static int inflight = 0;
// terminating: applicationShouldTerminate answered NSTerminateLater and waits for inflight to reach 0.
static BOOL terminating = NO;
// graceOver: the short delay after a quit request has passed; Apple events already queued were delivered.
static BOOL graceOver = NO;

static os_log_t porteroLog(void) {
    static os_log_t log;
    static dispatch_once_t once;
    dispatch_once(&once, ^{ log = os_log_create("dev.plavipas.portero", "handler"); });
    return log;
}

// Threading: -[NSApplication terminate:] with an NSTerminateLater answer runs a nested run loop until the reply
// comes. A libdispatch main-queue block that is itself running cannot be re-entered, so calling terminate: from
// one (and waiting for work queued on the main queue) deadlocks. Therefore nothing in the quit path uses
// dispatch_*(main queue): every deferred step is a run-loop timer or a CFRunLoopPerformBlock, registered in the
// common modes and NSModalPanelRunLoopMode (the mode AppKit waits in), and terminate: is only ever called from
// such a timer, never from a main-queue block. Both are serviced by the nested loop, so the grace timer, the
// 10 s cap, porteroDone and the panel blocks keep running while AppKit waits for the reply.
static NSArray<NSRunLoopMode> *porteroModes(void) {
    return @[NSRunLoopCommonModes, NSModalPanelRunLoopMode];
}

// Run block on the main run loop (any thread may call).
void porteroRunOnMain(void (^block)(void)) {
    CFRunLoopRef loop = CFRunLoopGetMain();
    CFRunLoopPerformBlock(loop, (__bridge CFArrayRef)porteroModes(), block);
    CFRunLoopWakeUp(loop);
}

// Run block on the main run loop after ms milliseconds (main thread only).
static void porteroAfterMs(int ms, void (^block)(void)) {
    NSTimer *t = [NSTimer timerWithTimeInterval:ms / 1000.0 repeats:NO block:^(NSTimer *timer) { block(); }];
    for (NSRunLoopMode m in porteroModes()) [[NSRunLoop mainRunLoop] addTimer:t forMode:m];
}

static void porteroReplyTerminate(void) {
    if (!terminating) return;
    terminating = NO;
    graceOver = NO;
    [NSApp replyToApplicationShouldTerminate:YES];
}

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
    if (inflight == 0 && generation == 0) {
        os_log(porteroLog(), "launched without link");
        porteroExitAfter(3000);
    }
}

// Quit requests (logout, restart, Quit, the idle quit): never NSTerminateNow. A link whose Apple event is
// already queued when the request arrives would be lost with the process, so the reply is deferred by a
// short grace so that the run loop delivers pending events first. Pending panels open their link in the
// default target; the reply then waits (at most 10 s) until every link in flight is done.
- (NSApplicationTerminateReply)applicationShouldTerminate:(NSApplication *)sender {
    os_log(porteroLog(), "terminating, inflight=%d", inflight);
    porteroCancelAllPanels();
    terminating = YES;
    graceOver = NO;
    porteroAfterMs(150, ^{
        if (!terminating) return;
        graceOver = YES;
        os_log(porteroLog(), "terminate grace over, inflight=%d", inflight);
        if (inflight == 0) porteroReplyTerminate();
    });
    porteroAfterMs(10000, ^{ porteroReplyTerminate(); });
    return NSTerminateLater;
}

// Documents (Open With, drag onto the icon): HTML files declared in Info.plist arrive here as file:// URLs.
- (void)application:(NSApplication *)app openURLs:(NSArray<NSURL *> *)urls {
    for (NSURL *u in urls) {
        os_log(porteroLog(), "link received (opener=-) %{private}@", u.absoluteString);
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
    os_log(porteroLog(), "link received (opener=%{public}@) %{private}@", opener.length ? opener : @"-", url);
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
    porteroAfterMs(ms, ^{
        if (mine == generation && inflight == 0) [NSApp terminate:nil];
    });
}

// Called from Go when one link has been routed (or failed); the app quits 2 s after the last one.
void porteroDone(void) {
    porteroRunOnMain(^{
        if (inflight > 0) inflight--;
        if (inflight == 0) {
            if (terminating) { if (graceOver) porteroReplyTerminate(); }
            else porteroExitAfter(2000);
        }
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
