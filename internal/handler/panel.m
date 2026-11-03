#import <Cocoa/Cocoa.h>
#import "handler.h"

extern void porteroAskDone(int id, int choice, int remember);

// The panel that asks where a link should open. Everything here runs on the main thread.

static const int kMaxPanels = 9;
static const CGFloat kPanelWidth = 380;

@interface PorteroPanel : NSPanel
@property (nonatomic) int askID;
@property (nonatomic) BOOL finished;
@property (nonatomic, strong) NSButton *remember;
@end

@interface PorteroAsk : NSObject <NSWindowDelegate>
@property (nonatomic) int askID;
@property (nonatomic, copy) NSString *title;
@property (nonatomic, copy) NSArray<NSString *> *options;
@property (nonatomic) BOOL canRemember;
- (void)present:(PorteroAsk *)a;
@end

static NSMutableArray<PorteroPanel *> *openPanels;
static NSMutableArray<PorteroAsk *> *queued;
static PorteroAsk *controller; // one delegate/target for every panel
// Set by porteroCancelAllPanels once the app is quitting: later asks are answered with the default at once.
static BOOL shuttingDown = NO;

static void porteroFinish(PorteroPanel *panel, int choice) {
    if (panel.finished) return;
    panel.finished = YES;
    int remember = (choice >= 0 && panel.remember.isEnabled && panel.remember.state == NSControlStateValueOn) ? 1 : 0;
    [openPanels removeObject:panel];
    panel.delegate = nil;
    [panel orderOut:nil];
    [panel close];
    porteroAskDone(panel.askID, choice, remember);
    // A slot is free: show the next queued link.
    if (!shuttingDown && queued.count > 0 && openPanels.count < kMaxPanels) {
        PorteroAsk *next = queued.firstObject;
        [queued removeObjectAtIndex:0];
        [controller present:next];
    }
}

@implementation PorteroPanel
- (BOOL)canBecomeKeyWindow { return YES; }
- (BOOL)canBecomeMainWindow { return NO; }
- (void)cancelOperation:(id)sender { porteroFinish(self, -1); } // Esc
- (void)keyDown:(NSEvent *)event {
    NSString *c = event.charactersIgnoringModifiers;
    if (c.length == 1 && !(event.modifierFlags & (NSEventModifierFlagCommand | NSEventModifierFlagControl | NSEventModifierFlagOption))) {
        unichar ch = [c characterAtIndex:0];
        if (ch >= '1' && ch <= '9') {
            NSInteger i = ch - '1';
            for (NSView *v in self.contentView.subviews) {
                if ([v isKindOfClass:[NSButton class]] && v.tag == i + 1000) {
                    porteroFinish(self, (int)i);
                    return;
                }
            }
            return; // a digit beyond the last option: ignored
        }
        if (ch == '\r' || ch == 3) return; // Return does nothing: no accidental choice
    }
    [super keyDown:event];
}
@end

@implementation PorteroAsk

- (void)chosen:(NSButton *)sender {
    PorteroPanel *panel = (PorteroPanel *)sender.window;
    porteroFinish(panel, (int)(sender.tag - 1000));
}

- (BOOL)windowShouldClose:(NSWindow *)sender {
    porteroFinish((PorteroPanel *)sender, -1);
    return NO; // porteroFinish closed it already
}

- (void)present:(PorteroAsk *)a {
    NSUInteger n = a.options.count;
    CGFloat pad = 14, btnH = 28, gap = 6, labelH = 18;
    CGFloat height = pad + labelH + 10 + n * (btnH + gap) + 6 + 20 + pad;

    PorteroPanel *panel = [[PorteroPanel alloc]
        initWithContentRect:NSMakeRect(0, 0, kPanelWidth, height)
                  styleMask:(NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskNonactivatingPanel | NSWindowStyleMaskUtilityWindow)
                    backing:NSBackingStoreBuffered
                      defer:NO];
    panel.askID = a.askID;
    panel.title = @"Open link in";
    // Keys 1..9 and Esc must work at once: the panel is non-activating (the app the user clicked in keeps
    // focus and stays active) but still becomes the key window, and the newest panel is ordered front as key
    // below. becomesKeyOnlyIfNeeded NO makes it key although it has no text field that needs input.
    panel.becomesKeyOnlyIfNeeded = NO;
    panel.hidesOnDeactivate = NO;
    panel.floatingPanel = YES;
    panel.level = NSFloatingWindowLevel;
    panel.collectionBehavior = NSWindowCollectionBehaviorCanJoinAllSpaces | NSWindowCollectionBehaviorFullScreenAuxiliary;
    panel.releasedWhenClosed = NO;
    panel.delegate = self;

    NSView *content = panel.contentView;
    // The link is data: plain text in a label, never a format string or markup.
    NSTextField *label = [NSTextField labelWithString:a.title ?: @""];
    label.lineBreakMode = NSLineBreakByTruncatingMiddle;
    label.frame = NSMakeRect(pad, height - pad - labelH, kPanelWidth - 2 * pad, labelH);
    label.toolTip = a.title;
    [content addSubview:label];

    CGFloat y = label.frame.origin.y - 10;
    for (NSUInteger i = 0; i < n; i++) {
        y -= btnH;
        NSButton *b = [NSButton buttonWithTitle:[NSString stringWithFormat:@"%lu  %@", (unsigned long)i + 1, a.options[i]]
                                         target:self
                                         action:@selector(chosen:)];
        b.tag = (NSInteger)i + 1000;
        b.bezelStyle = NSBezelStyleRounded;
        b.alignment = NSTextAlignmentLeft;
        b.lineBreakMode = NSLineBreakByTruncatingMiddle;
        b.frame = NSMakeRect(pad, y, kPanelWidth - 2 * pad, btnH);
        [content addSubview:b];
        y -= gap;
    }
    NSButton *rem = [NSButton checkboxWithTitle:@"Remember this choice for this meeting" target:nil action:nil];
    rem.state = NSControlStateValueOff;
    rem.enabled = a.canRemember;
    rem.frame = NSMakeRect(pad, y - 6 - 20, kPanelWidth - 2 * pad, 20);
    [content addSubview:rem];
    panel.remember = rem;

    // Near the pointer, kept inside the visible area of the screen the pointer is on; each further panel is
    // offset so that stacked panels stay reachable.
    NSPoint mouse = [NSEvent mouseLocation];
    NSScreen *screen = NSScreen.mainScreen;
    for (NSScreen *s in NSScreen.screens) {
        if (NSMouseInRect(mouse, s.frame, NO)) { screen = s; break; }
    }
    CGFloat off = 28.0 * openPanels.count;
    NSRect vf = screen.visibleFrame;
    NSSize sz = panel.frame.size;
    CGFloat x = mouse.x - sz.width / 2 + off;
    CGFloat py = mouse.y - sz.height - 16 - off;
    x = MAX(NSMinX(vf), MIN(x, NSMaxX(vf) - sz.width));
    py = MAX(NSMinY(vf), MIN(py, NSMaxY(vf) - sz.height));
    [panel setFrameOrigin:NSMakePoint(x, py)];

    [openPanels addObject:panel];
    [panel makeKeyAndOrderFront:nil];
}

@end

// Show a panel for link `askID` (called from Go on any thread). options is newline-separated.
void porteroAsk(int askID, const char *title, const char *options, int canRemember) {
    NSString *t = [NSString stringWithUTF8String:title] ?: @"";
    NSString *o = [NSString stringWithUTF8String:options] ?: @"";
    NSArray<NSString *> *opts = [o componentsSeparatedByString:@"\n"];
    dispatch_async(dispatch_get_main_queue(), ^{
        if (shuttingDown) {
            // Termination already cancelled every panel; a link dispatched before that must not open a new one.
            porteroAskDone(askID, -1, 0);
            return;
        }
        if (!openPanels) {
            openPanels = [NSMutableArray array];
            queued = [NSMutableArray array];
            controller = [PorteroAsk new];
        }
        PorteroAsk *a = [PorteroAsk new];
        a.askID = askID;
        a.title = t;
        a.options = opts;
        a.canRemember = canRemember != 0;
        if (openPanels.count >= kMaxPanels) {
            [queued addObject:a];
        } else {
            [controller present:a];
        }
    });
}

// Cancel every open and queued panel (the app is quitting); each answers with the default. Main thread.
int porteroCancelAllPanels(void) {
    shuttingDown = YES;
    int n = (int)(openPanels.count + queued.count);
    NSArray<PorteroAsk *> *q = [queued copy];
    [queued removeAllObjects];
    for (PorteroAsk *a in q) porteroAskDone(a.askID, -1, 0);
    for (PorteroPanel *p in [openPanels copy]) porteroFinish(p, -1);
    return n;
}
