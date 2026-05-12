#import "wallpaper_native.h"
#import <AppKit/AppKit.h>
#import <Foundation/Foundation.h>

// ensureMainThread runs a block on the main thread synchronously.
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
    // Copy the path string — the caller's memory may be freed before the
    // async block executes.
    NSString *path = [[NSString alloc] initWithUTF8String:imagePath];
    int idx = screenIndex;

    // Use dispatch_async instead of dispatch_sync to prevent deadlock:
    // The Go caller holds mc.mu.Lock() when calling SetWallpaper.
    // If Fyne's main thread is simultaneously blocked on mc.mu.RLock()
    // (e.g. the user opened the anchor popup), dispatch_sync would
    // deadlock: goroutine waits for main thread, main thread waits
    // for the lock the goroutine holds.
    //
    // dispatch_async releases the Go goroutine immediately, allowing
    // mc.mu.Unlock() to proceed and unblock the main thread.
    dispatch_async(dispatch_get_main_queue(), ^{
        @autoreleasepool {
            NSURL *imageURL = [NSURL fileURLWithPath:path];
            NSArray<NSScreen *> *screens = [NSScreen screens];

            if (idx < 0 || idx >= (int)[screens count]) {
                NSLog(@"Spice: Invalid screen index %d (count: %d)", idx, (int)[screens count]);
                return;
            }

            NSScreen *screen = [screens objectAtIndex:idx];
            NSError *error = nil;

            // WindowServer cache bypass:
            // macOS caches rendered wallpapers at multiple levels — both in
            // NSWorkspace (by URL) and in WindowServer (by rendered texture).
            // When a derivative file is overwritten in-place (same path, new
            // content — e.g. after anchor reprocessing), neither cache is
            // invalidated. The only reliable fix is presenting a genuinely
            // different file URL to WindowServer.
            //
            // Strategy: if the current wallpaper URL matches the new one,
            // create a hardlink with a ".spice_tmp" suffix. Hardlinks share
            // the same data (zero extra disk space) but have a unique URL.
            // The previous temp file is cleaned up on each call.
            NSURL *currentURL = [[NSWorkspace sharedWorkspace] desktopImageURLForScreen:screen];
            if (currentURL && [currentURL isEqual:imageURL]) {
                NSString *tmpPath = [NSString stringWithFormat:@"%@.spice_tmp", path];

                // Clean up previous temp hardlink
                [[NSFileManager defaultManager] removeItemAtPath:tmpPath error:nil];

                // Create hardlink: same inode data, unique directory entry
                if (link([path UTF8String], [tmpPath UTF8String]) == 0) {
                    imageURL = [NSURL fileURLWithPath:tmpPath];
                }
            }

            BOOL success = [[NSWorkspace sharedWorkspace] setDesktopImageURL:imageURL
                                                                   forScreen:screen
                                                                     options:@{}
                                                                       error:&error];
            if (!success) {
                NSLog(@"Spice: NSWorkspace setDesktopImageURL failed: %@",
                      error ? error.localizedDescription : @"unknown error");
            }
        }
    });

    // Return 0 optimistically — errors are logged asynchronously.
    // This is acceptable since the Go layer only logs SetWallpaper errors
    // and doesn't use the return value for recovery.
    return 0;
}
