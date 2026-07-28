#import "autostart_native.h"
#import <AppKit/AppKit.h>
#import <Foundation/Foundation.h>
#import <ServiceManagement/ServiceManagement.h>

// copyError writes an NSError description into the caller's buffer.
// Safe with a NULL or zero-length buffer.
static void copyError(NSError *error, char *errBuf, int errLen) {
    if (errBuf == NULL || errLen <= 0) {
        return;
    }
    NSString *desc = error ? [error localizedDescription] : @"unknown error";
    if (desc == nil) {
        desc = @"unknown error";
    }
    strncpy(errBuf, [desc UTF8String], (size_t)errLen - 1);
    errBuf[errLen - 1] = '\0';
}

int spiceLoginItemAvailable(void) {
    if (@available(macOS 13.0, *)) {
        return 1;
    }
    return 0;
}

int spiceLoginItemStatus(void) {
    if (@available(macOS 13.0, *)) {
        @autoreleasepool {
            SMAppService *service = [SMAppService mainAppService];
            switch ([service status]) {
                case SMAppServiceStatusNotRegistered:
                    return SPICE_LOGIN_NOTREGISTERED;
                case SMAppServiceStatusEnabled:
                    return SPICE_LOGIN_ENABLED;
                case SMAppServiceStatusRequiresApproval:
                    return SPICE_LOGIN_REQUIRESAPPROVAL;
                case SMAppServiceStatusNotFound:
                    return SPICE_LOGIN_NOTFOUND;
                default:
                    return SPICE_LOGIN_UNKNOWN;
            }
        }
    }
    return SPICE_LOGIN_UNSUPPORTED;
}

int spiceLoginItemRegister(char *errBuf, int errLen) {
    if (@available(macOS 13.0, *)) {
        @autoreleasepool {
            SMAppService *service = [SMAppService mainAppService];
            NSError *error = nil;

            if ([service registerAndReturnError:&error]) {
                return 0;
            }

            // Already-registered is reported as an error by the framework but
            // is exactly the state the caller asked for.
            if (error && [error code] == kSMErrorAlreadyRegistered) {
                return 0;
            }

            copyError(error, errBuf, errLen);
            NSLog(@"Spice: SMAppService register failed: %@", error);
            return error ? (int)[error code] : -1;
        }
    }

    copyError(nil, errBuf, errLen);
    return -1;
}

int spiceLoginItemUnregister(char *errBuf, int errLen) {
    if (@available(macOS 13.0, *)) {
        @autoreleasepool {
            SMAppService *service = [SMAppService mainAppService];
            NSError *error = nil;

            if ([service unregisterAndReturnError:&error]) {
                return 0;
            }

            // Nothing registered means we are already in the desired state.
            if (error && ([error code] == kSMErrorJobNotFound ||
                          [error code] == kSMErrorServiceUnavailable)) {
                return 0;
            }

            copyError(error, errBuf, errLen);
            NSLog(@"Spice: SMAppService unregister failed: %@", error);
            return error ? (int)[error code] : -1;
        }
    }

    copyError(nil, errBuf, errLen);
    return -1;
}

int spiceOpenLoginItemsSettings(void) {
    __block BOOL ok = NO;
    void (^open)(void) = ^{
        @autoreleasepool {
            NSURL *url = [NSURL URLWithString:@"x-apple.systempreferences:com.apple.LoginItems-Settings.extension"];
            ok = [[NSWorkspace sharedWorkspace] openURL:url];
        }
    };

    if ([NSThread isMainThread]) {
        open();
    } else {
        dispatch_sync(dispatch_get_main_queue(), open);
    }

    return ok ? 0 : -1;
}
