//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

// activatePID brings the app that owns this process to the front. No AppleScript, so
// macOS never asks for permission to control another app.
static int activatePID(int pid) {
  NSRunningApplication *a = [NSRunningApplication runningApplicationWithProcessIdentifier:pid];
  if (a == nil) return 0;
  return [a activateWithOptions:NSApplicationActivateAllWindows] ? 1 : 0;
}
*/
import "C"

func activateApp(pid int) bool { return C.activatePID(C.int(pid)) == 1 }
