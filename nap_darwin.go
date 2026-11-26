package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation
#import <Foundation/Foundation.h>

// While a timer runs, opt out of App Nap so the tray countdown and the alarm stay on time
// when the widget is hidden. Idle system sleep is still allowed.
static void *beginTimingActivity(void) {
	@autoreleasepool {
		id a = [[NSProcessInfo processInfo]
			beginActivityWithOptions:NSActivityUserInitiatedAllowingIdleSystemSleep
			reason:@"Counting down a focus timer"];
		return (void *)[a retain];
	}
}

static void endTimingActivity(void *a) {
	[[NSProcessInfo processInfo] endActivity:(id)a];
	[(id)a release];
}
*/
import "C"

import "unsafe"

var activity unsafe.Pointer

// stayAwake is called from the core with its lock held, so calls never overlap.
func stayAwake(on bool) {
	switch {
	case on && activity == nil:
		activity = C.beginTimingActivity()
	case !on && activity != nil:
		C.endTimingActivity(activity)
		activity = nil
	}
}
