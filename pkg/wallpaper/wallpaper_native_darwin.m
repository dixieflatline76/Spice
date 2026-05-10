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

            // Cache-bust: NSWorkspace caches desktop images by file URL.
            // When the file content changes but the path stays the same
            // (e.g. anchor reprocessing overwrites the derivative in-place),
            // macOS silently ignores the setDesktopImageURL call.
            // Fix: if the current wallpaper URL matches the new one, briefly
            // set to a different URL to invalidate the cache.
            NSURL *currentURL = [[NSWorkspace sharedWorkspace] desktopImageURLForScreen:screen];
            if (currentURL && [currentURL isEqual:imageURL]) {
                // Use macOS built-in solid color as cache-bust target.
                // This path exists on macOS 12+ (our minimum deployment target).
                NSString *bustPath = @"/System/Library/Desktop Pictures/Solid Colors/Black.png";
                if ([[NSFileManager defaultManager] fileExistsAtPath:bustPath]) {
                    [[NSWorkspace sharedWorkspace] setDesktopImageURL:[NSURL fileURLWithPath:bustPath]
                                                            forScreen:screen
                                                              options:@{}
                                                                error:nil];
                }
            }

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
