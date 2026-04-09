#ifndef WALLPAPER_NATIVE_H
#define WALLPAPER_NATIVE_H

// NativeMonitorInfo carries screen metadata from Objective-C to Go.
typedef struct {
    int index;
    int width;   // Physical pixels (frame.width * backingScaleFactor)
    int height;  // Physical pixels (frame.height * backingScaleFactor)
    char name[256];
} NativeMonitorInfo;

// nativeGetScreenCount returns the number of connected screens via [NSScreen screens].
int nativeGetScreenCount(void);

// nativeGetScreenInfo fills info for the screen at the given index.
// Returns 0 on success, -1 if index is out of bounds.
int nativeGetScreenInfo(int index, NativeMonitorInfo *info);

// nativeSetWallpaper sets the desktop wallpaper for the screen at the given index.
// Returns 0 on success, -1 if index is out of bounds, -2 if NSWorkspace fails.
int nativeSetWallpaper(const char *imagePath, int screenIndex);

#endif
