#import "wallpaper_native.h"
#import <AppKit/AppKit.h>
#import <Foundation/Foundation.h>

// ensureMainThread runs a block on the main thread.
// If already on the main thread, it runs directly to avoid deadlock.
// If on a background thread, it dispatches synchronously.
static void ensureMainThread(void (^block)(void)) {
    if ([NSThread isMainThread]) {
        block();
    } else {
        dispatch_sync(dispatch_get_main_queue(), block);
    }
}

int nativeGetScreenCount(void) {
    __block int count = 0;
    ensureMainThread(^{
        count = (int)[[NSScreen screens] count];
    });
    return count;
}

int nativeGetScreenInfo(int index, NativeMonitorInfo *info) {
    __block int result = 0;
    ensureMainThread(^{
        @autoreleasepool {
            NSArray<NSScreen *> *screens = [NSScreen screens];
            if (index < 0 || index >= (int)[screens count]) {
                result = -1;
                return;
            }

            NSScreen *screen = [screens objectAtIndex:index];
            NSRect frame = [screen frame];
            CGFloat scale = [screen backingScaleFactor];

            info->index = index;
            info->width  = (int)(frame.size.width  * scale);
            info->height = (int)(frame.size.height * scale);

            // localizedName is available on macOS 10.15+; our min target is 12.0.
            NSString *name = @"Display";
            if (@available(macOS 10.15, *)) {
                name = [screen localizedName];
            }
            strncpy(info->name, [name UTF8String], sizeof(info->name) - 1);
            info->name[sizeof(info->name) - 1] = '\0';
        }
    });
    return result;
}

int nativeSetWallpaper(const char *imagePath, int screenIndex) {
    __block int result = 0;
    ensureMainThread(^{
        @autoreleasepool {
            NSString *path = [NSString stringWithUTF8String:imagePath];
            NSURL *imageURL = [NSURL fileURLWithPath:path];
            NSArray<NSScreen *> *screens = [NSScreen screens];

            if (screenIndex < 0 || screenIndex >= (int)[screens count]) {
                result = -1;
                return;
            }

            NSScreen *screen = [screens objectAtIndex:screenIndex];
            NSError *error = nil;
            BOOL success = [[NSWorkspace sharedWorkspace] setDesktopImageURL:imageURL
                                                                  forScreen:screen
                                                                    options:@{}
                                                                      error:&error];
            if (!success) {
                NSLog(@"Spice: NSWorkspace setDesktopImageURL failed: %@",
                      error ? error.localizedDescription : @"unknown error");
                result = -2;
            }
        }
    });
    return result;
}
