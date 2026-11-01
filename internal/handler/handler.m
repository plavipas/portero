#import <Cocoa/Cocoa.h>
#import "handler.h"

extern void porteroHandleURL(char *url, char *opener);

static int generation = 0;

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
    // Launched with no link (double-click): leave quietly.
    porteroExitAfter(3000);
}

// Documents (Open With, drag onto the icon): HTML files declared in Info.plist arrive here as file:// URLs.
- (void)application:(NSApplication *)app openURLs:(NSArray<NSURL *> *)urls {
    for (NSURL *u in urls) {
        generation++;
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
    generation++;
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

// Quit after ms milliseconds unless another link arrives in the meantime.
void porteroExitAfter(int ms) {
    int mine = generation;
    dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)ms * NSEC_PER_MSEC), dispatch_get_main_queue(), ^{
        if (mine == generation) [NSApp terminate:nil];
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

// Ask macOS to make the app at appPath the default for scheme; macOS shows its own confirmation.
// Returns an error message or "".
char *porteroSetDefaultHandler(const char *appPath, const char *scheme) {
    @autoreleasepool {
        NSURL *appURL = [NSURL fileURLWithPath:[NSString stringWithUTF8String:appPath]];
        dispatch_semaphore_t done = dispatch_semaphore_create(0);
        __block NSString *message = @"";
        [[NSWorkspace sharedWorkspace] setDefaultApplicationAtURL:appURL
                                              toOpenURLsWithScheme:[NSString stringWithUTF8String:scheme]
                                                 completionHandler:^(NSError *error) {
            if (error) message = error.localizedDescription;
            dispatch_semaphore_signal(done);
        }];
        // The completion runs on another thread; pump the main run loop while waiting.
        while (dispatch_semaphore_wait(done, dispatch_time(DISPATCH_TIME_NOW, 50 * NSEC_PER_MSEC))) {
            [[NSRunLoop mainRunLoop] runMode:NSDefaultRunLoopMode beforeDate:[NSDate dateWithTimeIntervalSinceNow:0.05]];
        }
        return strdup(message.UTF8String);
    }
}
