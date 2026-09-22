//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c -Wno-deprecated-declarations
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

void goTrayOpen(void);
void goTrayRecord(void);

@interface LogTray : NSObject
@property (strong) NSStatusItem *item;
@end

@implementation LogTray
- (void)openLog:(id)sender { goTrayOpen(); }
- (void)recordNow:(id)sender { goTrayRecord(); }
- (void)quitLog:(id)sender { [NSApp terminate:nil]; }
@end

static LogTray *gTray = nil;
static NSMenuItem *gStreak = nil;

// trayRun draws the menu bar item and never returns. It reports what macOS gave it, so a
// missing item can be told apart from a missing program.
static void trayRun(const char *streak, const void *iconBytes, int iconLen) {
  @autoreleasepool {
    [NSApplication sharedApplication];
    [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
    gTray = [[LogTray alloc] init];
    gTray.item = [[NSStatusBar systemStatusBar] statusItemWithLength:NSSquareStatusItemLength];
    NSStatusBarButton *b = gTray.item.button;
    if (iconLen > 0) {
      NSImage *img = [[NSImage alloc] initWithData:[NSData dataWithBytes:iconBytes length:iconLen]];
      [img setSize:NSMakeSize(17, 17)];
      [img setTemplate:YES];
      b.image = img;
      b.imagePosition = NSImageLeft;
    }
    // no text: the narrowest possible item, so a crowded menu bar can still fit it
    gTray.item.length = NSSquareStatusItemLength;
    gTray.item.behavior = NSStatusItemBehaviorTerminationOnRemoval;
    gTray.item.autosaveName = @"LOG_menu";
    [gTray.item setVisible:YES];

    NSMenu *menu = [[NSMenu alloc] init];
    gStreak = [[NSMenuItem alloc] initWithTitle:[NSString stringWithUTF8String:streak] action:nil keyEquivalent:@""];
    [gStreak setEnabled:NO];
    [menu addItem:gStreak];
    [menu addItem:[NSMenuItem separatorItem]];
    NSMenuItem *o = [[NSMenuItem alloc] initWithTitle:@"Open LOG_" action:@selector(openLog:) keyEquivalent:@""];
    o.target = gTray; [menu addItem:o];
    NSMenuItem *r = [[NSMenuItem alloc] initWithTitle:@"Record now" action:@selector(recordNow:) keyEquivalent:@""];
    r.target = gTray; [menu addItem:r];
    [menu addItem:[NSMenuItem separatorItem]];
    NSMenuItem *q = [[NSMenuItem alloc] initWithTitle:@"Quit" action:@selector(quitLog:) keyEquivalent:@"q"];
    q.target = gTray; [menu addItem:q];
    gTray.item.menu = menu;
    dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 2 * NSEC_PER_SEC), dispatch_get_main_queue(), ^{
      NSRect f = b.window.frame;
      NSString *line = [NSString stringWithFormat:@"menu bar item: visible=%d at x=%.0f y=%.0f w=%.0f h=%.0f;",
                        (int)gTray.item.visible, f.origin.x, f.origin.y, f.size.width, f.size.height];
      for (NSScreen *s in [NSScreen screens]) {
        NSRect r = s.frame;
        line = [line stringByAppendingFormat:@" screen %@ x=%.0f y=%.0f %.0fx%.0f holds=%d;",
                s.localizedName, r.origin.x, r.origin.y, r.size.width, r.size.height,
                (int)NSPointInRect(NSMakePoint(NSMidX(f), NSMidY(f)), r)];
      }
      NSString *path = [NSHomeDirectory() stringByAppendingPathComponent:@"Library/Logs/LOG_.log"];
      NSFileHandle *fh = [NSFileHandle fileHandleForWritingAtPath:path];
      if (fh == nil) { [[NSFileManager defaultManager] createFileAtPath:path contents:nil attributes:nil]; fh = [NSFileHandle fileHandleForWritingAtPath:path]; }
      [fh seekToEndOfFile];
      [fh writeData:[[line stringByAppendingString:@"\n"] dataUsingEncoding:NSUTF8StringEncoding]];
      [fh closeFile];
    });

    [NSApp run];
  }
}

static void traySetStreak(const char *s) {
  NSString *t = [NSString stringWithUTF8String:s];
  dispatch_async(dispatch_get_main_queue(), ^{ if (gStreak) gStreak.title = t; });
}
*/
import "C"

import (
	"runtime"
	"time"
	"unsafe"
)

func init() { runtime.LockOSThread() } // the menu bar belongs to the first thread

var trayRoot string

// trayRunNative draws the menu bar item with Cocoa directly: the tray library available
// today never tells macOS the program is an interface app, so nothing appeared.
func trayRunNative(root string) error {
	trayRoot = root
	go func() {
		for range time.Tick(10 * time.Minute) {
			s := C.CString(trayStreak(trayRoot))
			C.traySetStreak(s)
			C.free(unsafe.Pointer(s))
		}
	}()
	streak := C.CString(trayStreak(root))
	defer C.free(unsafe.Pointer(streak))
	C.trayRun(streak, unsafe.Pointer(&trayTemplate[0]), C.int(len(trayTemplate)))
	return nil
}
