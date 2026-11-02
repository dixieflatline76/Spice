//go:build darwin
// +build darwin

package activity

/*
#cgo LDFLAGS: -framework AppKit -framework CoreGraphics -framework Foundation
#import <AppKit/AppKit.h>
#import <CoreGraphics/CoreGraphics.h>
#import <Foundation/Foundation.h>
#include <stdlib.h>
#include <string.h>

typedef struct {
    char owner[128];
    int  layer;
    int  x, y, w, h;
} NativeWindowInfo;

typedef struct {
    int x, y, w, h;
} NativeDisplayRect;

static void copyFrontmostAppName(char *buf, int bufLen) {
    if (bufLen <= 0) return;
    buf[0] = '\0';
    @autoreleasepool {
        NSRunningApplication *app = [[NSWorkspace sharedWorkspace] frontmostApplication];
        if (app && app.localizedName) {
            strncpy(buf, [app.localizedName UTF8String], (size_t)(bufLen - 1));
            buf[bufLen - 1] = '\0';
        }
    }
}

static int getActiveDisplays(NativeDisplayRect *out, int maxCount) {
    uint32_t count = 0;
    CGDirectDisplayID ids[16];
    if (CGGetActiveDisplayList(16, ids, &count) != kCGErrorSuccess) {
        return 0;
    }
    int n = (int)count < maxCount ? (int)count : maxCount;
    for (int i = 0; i < n; i++) {
        CGRect r = CGDisplayBounds(ids[i]);
        out[i].x = (int)r.origin.x;
        out[i].y = (int)r.origin.y;
        out[i].w = (int)r.size.width;
        out[i].h = (int)r.size.height;
    }
    return n;
}

static int getOnScreenWindows(NativeWindowInfo *out, int maxCount) {
    int written = 0;
    @autoreleasepool {
        CFArrayRef list = CGWindowListCopyWindowInfo(
            kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements,
            kCGNullWindowID
        );
        if (!list) return 0;
        CFIndex total = CFArrayGetCount(list);
        for (CFIndex i = 0; i < total && written < maxCount; i++) {
            NSDictionary *entry = (__bridge NSDictionary *)CFArrayGetValueAtIndex(list, i);
            if (!entry) continue;

            NSNumber *layerNum = entry[(__bridge NSString *)kCGWindowLayer];
            NSString *ownerStr = entry[(__bridge NSString *)kCGWindowOwnerName];
            NSDictionary *boundsDict = entry[(__bridge NSString *)kCGWindowBounds];
            if (!layerNum || !ownerStr || !boundsDict) continue;

            CGRect rect = CGRectZero;
            if (!CGRectMakeWithDictionaryRepresentation((__bridge CFDictionaryRef)boundsDict, &rect)) {
                continue;
            }

            NativeWindowInfo *info = &out[written++];
            strncpy(info->owner, [ownerStr UTF8String], sizeof(info->owner) - 1);
            info->owner[sizeof(info->owner) - 1] = '\0';
            info->layer = [layerNum intValue];
            info->x = (int)rect.origin.x;
            info->y = (int)rect.origin.y;
            info->w = (int)rect.size.width;
            info->h = (int)rect.size.height;
        }
        CFRelease(list);
    }
    return written;
}
*/
import "C"

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

type darwinDetector struct{}

// New returns the platform Detector hook (macOS-backed on Darwin).
func New() Detector { return &darwinDetector{} }

func (d *darwinDetector) UserBusy() BusyState {
	var cDisplays [16]C.NativeDisplayRect
	dCount := int(C.getActiveDisplays(&cDisplays[0], 16))
	if dCount == 0 {
		return BusyNone
	}

	var cWins [128]C.NativeWindowInfo
	wCount := int(C.getOnScreenWindows(&cWins[0], 128))
	if wCount == 0 {
		return BusyNone
	}

	// CGWindowListCopyWindowInfo returns windows in front-to-back z-order.
	// For each display, only evaluate the topmost layer-0 user window located
	// on that display so background windows occluded behind a smaller active
	// window cannot falsely trigger BusyFullscreen.
	for j := 0; j < dCount; j++ {
		disp := cDisplays[j]
		dispRect := Rect{X: int(disp.x), Y: int(disp.y), W: int(disp.w), H: int(disp.h)}
		for i := 0; i < wCount; i++ {
			w := cWins[i]
			if int(w.layer) != 0 {
				continue
			}
			owner := strings.TrimSpace(C.GoString(&w.owner[0]))
			if owner == "" || strings.EqualFold(owner, "Spice") || macSystemOwners[strings.ToLower(owner)] {
				continue
			}
			winRect := Rect{X: int(w.x), Y: int(w.y), W: int(w.w), H: int(w.h)}
			inter := intersectArea(winRect, dispRect)
			if inter <= 0 || (inter*2 < winRect.area() && inter*2 < dispRect.area()) {
				continue
			}
			// This is the topmost user window on display j.
			if state := ClassifyWindow(owner, int(w.layer), winRect, dispRect, "Spice"); state != BusyNone {
				return state
			}
			break
		}
	}
	return BusyNone
}

func (d *darwinDetector) Battery() BatteryState {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, "pmset", "-g", "batt")
	out, err := cmd.Output()
	if err != nil {
		return BatteryState{}
	}
	return ParsePmsetOutput(string(out))
}

func (d *darwinDetector) ForegroundProcess() (string, bool) {
	var buf [128]C.char
	C.copyFrontmostAppName(&buf[0], 128)
	name := strings.TrimSpace(C.GoString(&buf[0]))
	return name, name != ""
}
