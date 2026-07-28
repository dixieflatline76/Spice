#ifndef AUTOSTART_NATIVE_H
#define AUTOSTART_NATIVE_H

// Status codes returned by spiceLoginItemStatus. These mirror the Go Status
// constants in autostart.go.
#define SPICE_LOGIN_UNKNOWN      0
#define SPICE_LOGIN_NOTREGISTERED 1
#define SPICE_LOGIN_ENABLED      2
#define SPICE_LOGIN_REQUIRESAPPROVAL 3
#define SPICE_LOGIN_NOTFOUND     4
#define SPICE_LOGIN_UNSUPPORTED  5

// spiceLoginItemAvailable reports whether SMAppService exists on this OS.
// Returns 1 on macOS 13+, 0 otherwise.
int spiceLoginItemAvailable(void);

// spiceLoginItemStatus returns one of the SPICE_LOGIN_* codes above.
int spiceLoginItemStatus(void);

// spiceLoginItemRegister registers the running app bundle as a login item.
// Returns 0 on success. On failure it returns the NSError code (or -1 when
// the API is unavailable) and copies a NUL-terminated description into errBuf.
//
// Unlike nativeSetWallpaper, these calls report failures synchronously and
// verbatim: a rejected registration is precisely the condition the UI needs
// to explain to the user.
int spiceLoginItemRegister(char *errBuf, int errLen);

// spiceLoginItemUnregister removes the login item. Same return contract as
// spiceLoginItemRegister.
int spiceLoginItemUnregister(char *errBuf, int errLen);

// spiceOpenLoginItemsSettings opens System Settings at the Login Items pane.
// Returns 0 on success, -1 if the URL could not be opened.
int spiceOpenLoginItemsSettings(void);

#endif
