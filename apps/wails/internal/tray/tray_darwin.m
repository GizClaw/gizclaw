#import <Cocoa/Cocoa.h>

extern void gizclawGoTrayOpenWindow(void);
extern void gizclawGoTrayOpenPod(char *podID);
extern void gizclawGoTrayQuit(void);

@interface GizClawTrayTarget : NSObject
@property(nonatomic, copy) NSString *podID;
- (void)openWindow:(id)sender;
- (void)openPod:(id)sender;
- (void)quit:(id)sender;
@end

@implementation GizClawTrayTarget
- (void)openWindow:(id)sender { gizclawGoTrayOpenWindow(); }
- (void)openPod:(id)sender {
  const char *value = [self.podID UTF8String];
  gizclawGoTrayOpenPod((char *)value);
}
- (void)quit:(id)sender { gizclawGoTrayQuit(); }
@end

static NSStatusItem *gizclawStatusItem;
static NSMenu *gizclawMenu;
static GizClawTrayTarget *gizclawRootTarget;
static NSMutableArray<GizClawTrayTarget *> *gizclawTargets;

static void onMain(dispatch_block_t block) {
  if ([NSThread isMainThread]) block();
  else dispatch_async(dispatch_get_main_queue(), block);
}

void gizclawTrayStart(void) {
  onMain(^{
    if (gizclawStatusItem != nil) return;
    gizclawStatusItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSSquareStatusItemLength];
    gizclawStatusItem.button.title = @"◈";
    gizclawStatusItem.button.toolTip = @"GizClaw";
    gizclawMenu = [[NSMenu alloc] initWithTitle:@"GizClaw"];
    gizclawRootTarget = [[GizClawTrayTarget alloc] init];
    gizclawTargets = [[NSMutableArray alloc] init];
    gizclawStatusItem.menu = gizclawMenu;
  });
}

void gizclawTrayClear(const char *openWindowLabel) {
  NSString *openWindowTitle = [NSString stringWithUTF8String:openWindowLabel];
  onMain(^{
    [gizclawMenu removeAllItems];
    [gizclawTargets removeAllObjects];
    NSMenuItem *open = [[NSMenuItem alloc] initWithTitle:openWindowTitle action:@selector(openWindow:) keyEquivalent:@""];
    open.target = gizclawRootTarget;
    [gizclawMenu addItem:open];
    [gizclawMenu addItem:[NSMenuItem separatorItem]];
  });
}

void gizclawTrayAddPod(const char *podID, const char *label, const char *openPodLabel) {
  NSString *pod = [NSString stringWithUTF8String:podID];
  NSString *title = [NSString stringWithUTF8String:label];
  NSString *openPodTitle = [NSString stringWithUTF8String:openPodLabel];
  onMain(^{
	if (gizclawTargets.count > 0) [gizclawMenu addItem:[NSMenuItem separatorItem]];
    GizClawTrayTarget *target = [[GizClawTrayTarget alloc] init];
    target.podID = pod;
    [gizclawTargets addObject:target];
    NSMenuItem *parent = [[NSMenuItem alloc] initWithTitle:title action:nil keyEquivalent:@""];
    NSMenu *submenu = [[NSMenu alloc] initWithTitle:title];
    NSMenuItem *open = [[NSMenuItem alloc] initWithTitle:openPodTitle action:@selector(openPod:) keyEquivalent:@""];
    open.target = target;
    [submenu addItem:open];
    parent.submenu = submenu;
    [gizclawMenu addItem:parent];
  });
}

void gizclawTrayFinish(const char *quitLabel) {
  NSString *quitTitle = [NSString stringWithUTF8String:quitLabel];
  onMain(^{
    [gizclawMenu addItem:[NSMenuItem separatorItem]];
    NSMenuItem *quit = [[NSMenuItem alloc] initWithTitle:quitTitle action:@selector(quit:) keyEquivalent:@"q"];
    quit.target = gizclawRootTarget;
    [gizclawMenu addItem:quit];
  });
}

void gizclawTrayStop(void) {
  onMain(^{
    if (gizclawStatusItem != nil) [[NSStatusBar systemStatusBar] removeStatusItem:gizclawStatusItem];
    gizclawStatusItem = nil;
    gizclawMenu = nil;
    gizclawTargets = nil;
    gizclawRootTarget = nil;
  });
}
