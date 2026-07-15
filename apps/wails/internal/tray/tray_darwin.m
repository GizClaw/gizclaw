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

static NSImage *gizclawTrayImage(void) {
  NSImage *image = nil;
  if (@available(macOS 11.0, *)) {
    image = [NSImage imageWithSystemSymbolName:@"bolt.horizontal.circle.fill"
                     accessibilityDescription:@"GizClaw"];
  }
  if (image == nil) {
    image = [[NSImage alloc] initWithSize:NSMakeSize(18, 18)];
    [image lockFocus];
    [[NSColor blackColor] setStroke];
    NSBezierPath *ring = [NSBezierPath bezierPathWithOvalInRect:NSMakeRect(2.5, 2.5, 13, 13)];
    ring.lineWidth = 1.7;
    [ring stroke];
    NSBezierPath *bolt = [NSBezierPath bezierPath];
    [bolt moveToPoint:NSMakePoint(10.2, 4.7)];
    [bolt lineToPoint:NSMakePoint(6.3, 9.4)];
    [bolt lineToPoint:NSMakePoint(9.0, 9.4)];
    [bolt lineToPoint:NSMakePoint(7.8, 13.3)];
    [bolt lineToPoint:NSMakePoint(11.8, 8.2)];
    [bolt lineToPoint:NSMakePoint(9.2, 8.2)];
    [bolt closePath];
    [[NSColor blackColor] setFill];
    [bolt fill];
    [image unlockFocus];
  }
  image.template = YES;
  image.size = NSMakeSize(18, 18);
  return image;
}

static void onMain(dispatch_block_t block) {
  if ([NSThread isMainThread]) block();
  else dispatch_async(dispatch_get_main_queue(), block);
}

void gizclawTrayStart(void) {
  onMain(^{
    if (gizclawStatusItem != nil) return;
    gizclawStatusItem = [[[NSStatusBar systemStatusBar]
        statusItemWithLength:NSVariableStatusItemLength] retain];
    gizclawStatusItem.button.title = @"";
    gizclawStatusItem.button.image = gizclawTrayImage();
    gizclawStatusItem.button.imagePosition = NSImageOnly;
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
    if (gizclawStatusItem != nil) {
      [[NSStatusBar systemStatusBar] removeStatusItem:gizclawStatusItem];
      [gizclawStatusItem release];
    }
    gizclawStatusItem = nil;
    gizclawMenu = nil;
    gizclawTargets = nil;
    gizclawRootTarget = nil;
  });
}
