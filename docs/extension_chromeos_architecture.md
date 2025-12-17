# Spice Extension & Chrome OS Architecture

This document details the architecture of the **Spice Wallpaper Manager Extension** and the unique **Hybrid Driver** model used to support Chrome OS.

## 1. High-Level Overview

Spice uses a local Companion Extension to achieve two goals:
1.  **Smart Clipper**: Detects supported wallpaper URLs (Wallhaven, etc.) in the browser and offers a one-click "Add to Spice" button.
2.  **Chrome OS Driver**: On Chrome OS, the Go application cannot set the wallpaper directly (due to running in a Linux container). The extension acts as a bridge, receiving commands from the Go app and calling the native `chrome.wallpaper` API.

---

## 2. Communication Protocol

Communication occurs via a **Local WebSocket** connection.

*   **Server**: Spice App (Go) listens on `127.0.0.1:49452`.
*   **Client**: Chrome Extension (`background.js`) connects to this port.
*   **Security**: The extension only connects to localhost. The server accepts connections only from localhost.

### Payload Format (JSON)

**1. Heartbeat (Keep-alive)**
```json
{ "type": "heartbeat" }
```

**2. Set Wallpaper (App -> Extension)**
Used mainly on Chrome OS.
```json
{
  "type": "set_wallpaper",
  "url": "https://example.com/image.jpg",
  "layout": "CENTER_CROPPED"
}
```

**3. Add Collection (Extension -> App)**
Used when user clicks "Add to Spice".
```json
{
  "type": "add_collection",
  "url": "https://wallhaven.cc/tag/cyberpunk",
  "description": "Wallhaven: Cyberpunk"
}
```
*Note: In v1.3+, this triggers the UI to open the "Add Query" dialog rather than adding silently.*

---

## 3. Chrome Extension Architecture

The extension is built on **Manifest V3**.

### Core Components
1.  **Background Service Worker (`background.js`)**:
    *   Maintains the WebSocket connection.
    *   Implements a robust **Backoff Reconnection Strategy** (starts at 1s, caps at 10s) to handle cases where the App is closed.
    *   Monitors Tab URL changes (`chrome.tabs.onUpdated`, `onActivated`, `onFocusChanged`) to detect supported sites.
    *   **Animation**: Runs the "Icon Pulse" (switching between `icon_128.png` and `icon_anim_128.png`) when a supported site is detected.

2.  **Popup UI (`popup.html` / `popup.js`)**:
    *   Simple interface that appears when clicking the icon.
    *   If on a supported site: Shows "Add to Spice" flow.
    *   If not: Shows status (Connected/Disconnected) and "Open App" link.

### Cross-Browser Support
*   **Chrome / Edge**: Native support.
*   **Firefox**: Code compatible, requires minor `manifest.json` tweaks (e.g. browser specific ID).
*   **Safari**: Requires wrapping in a macOS App via Xcode (`xcrun safari-web-extension-converter`).

---

## 4. Chrome OS "Hybrid" Implementation

Chrome OS presents unique challenges because Linux apps (Crostini) run in a container and lack direct access to the host's desktop wallpaper APIs and System Tray.

### A. Wallpaper Setting (The "Bridge")
The App uses a **Delegated Driver** pattern.

1.  **Detection**: `pkg/wallpaper/linux.go` checks for the existence of `/dev/.cros_milestone`.
2.  **Driver Selection**: If found, `pkg/wallpaper/os_factory.go` returns the `ChromeOS` struct instead of the standard Linux one.
3.  **Execution**:
    *   The `ChromeOS` driver does *not* call a system command (like `feh` or `gsettings`).
    *   Instead, it sends the image URL/Path via the **WebSocket** to the connected Extension.
    *   The Extension receives the message and calls `chrome.wallpaper.setWallpaper()`.

### B. The "Pseudo-Tray" UI
Linux apps on Chrome OS do not support the System Tray / Notification Area icons. To provide a comparable experience, we implement a **Dock-Toggle Window**.

*   **Implementation**: `ui/linux.go`.
*   **Lifecycle Hook**: We hook into Fyne's `app.Lifecycle().SetOnEnteredForeground()`.
*   **Behavior**:
    1.  User clicks the Spice icon in the Chrome OS Shelf (Dock).
    2.  This triggers the `OnEnteredForeground` event.
    3.  Spice creates (or toggles) a **borderless, undecorated window**.
    4.  The window acts as a menu, containing the same buttons as the standard System Tray (Next Wallpaper, Pause, etc.).
    5.  **Positioning**: Attempts to center or place bottom-right (though Wayland limits absolute positioning).

---

## 5. Build & Distribution

*   **Windows**: Standard `.exe`.
*   **macOS**: Application Bundle (`.app`) inside `.dmg`. Can include the Safari Extension wrapper.
*   **Linux (Desktop)**: Binary or Package.
*   **Chrome OS**: Custom Build Targets (`make build-linux-amd64` / `arm64`) using `GOOS=linux`.
    *   User installs the Chrome Extension from the Web Store.
    *   User runs the Linux binary in the Crostini container.
