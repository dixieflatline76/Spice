# Spice User Guide

**Spice** is a premium wallpaper manager for Windows and macOS that keeps your desktop fresh, dynamic, and beautiful. It runs in the background as a system tray application — no windows to manage, just great wallpapers.

---

## Table of Contents
1. [The System Tray](#the-system-tray)
2. [Keyboard Shortcuts](#keyboard-shortcuts)
3. [Preferences: App Tab](#preferences-app-tab)
4. [Preferences: Wallpaper Tab](#preferences-wallpaper-tab)
   - [General](#general-settings)
   - [Online Sources](#online-sources)
   - [Local Sources](#local-sources)
5. [Favorites](#favorites)
6. [Multi-Display Setup](#multi-display-setup)
7. [Browser Extension](#browser-extension)

---

## The System Tray

Spice lives entirely in your system tray (Windows) or menu bar (macOS). Click the Spice icon to open the menu.

### Single Display Menu

When one display is connected, the tray menu shows actions directly:

| Item | Description |
| :--- | :--- |
| **Next Wallpaper** | Immediately advance to the next wallpaper |
| **Prev Wallpaper** | Go back to the previous wallpaper |
| **Pause Play** | Pause automatic rotation (changes to **Resume Play** when paused) |
| *(separator)* | |
| **Source:** | Shows the image provider (e.g., *Wallhaven*, *Met Museum*) |
| **By:** | Shows attribution — click to open the original image on the web |
| **Add to Favorites** | Save the current wallpaper locally *(only visible when Favorites is enabled)* |
| **Delete And Block** | Delete the image from cache and prevent it from ever appearing again |
| *(separator)* | |
| **Preferences** | Open the settings window |
| **About Spice** | Version info and credits |
| **Quit** | Exit Spice completely |

### Multiple Displays

When more than one display is connected, Display 1 (the primary) keeps its controls at the **top level** of the menu. Every additional display gets its own **submenu** named *Display 2*, *Display 3*, etc. (with the device name appended if available, e.g., *Display 2 (DELL U2723D)*).

Each display submenu contains the same full set of controls — Next, Prev, Pause Play, Source, Attribution, Add to Favorites, and Delete And Block — all acting independently on that specific display.

> **Tip:** When Spice starts with multiple monitors, it staggers wallpaper changes by a random offset so they don't all flash at the same time (configurable in Settings).

---

## Keyboard Shortcuts

Spice registers global hotkeys so you can control wallpapers without ever leaving your current app.

> **Note for macOS:** On macOS, the `Command` key is used for targeted actions (similar to Windows `Alt`). Global actions use `Command` + `Control` (similar to Windows `Ctrl` + `Alt`).

### Global Actions — All Displays At Once

| Action | Windows | macOS |
| :--- | :--- | :--- |
| Next Wallpaper | `Ctrl` + `Alt` + `→` | `Cmd` + `Control` + `→` |
| Previous Wallpaper | `Ctrl` + `Alt` + `←` | `Cmd` + `Control` + `←` |
| Sync / Detect Displays | `Ctrl` + `Alt` + `D` | `Cmd` + `Control` + `D` |
| Open Preferences | `Ctrl` + `Alt` + `O` | `Cmd` + `Control` + `O` |

### Targeted Actions — One Display At A Time

Hold a **number key (1–9)** alongside the modifier to target a specific display. Display 1 = key `1`, Display 2 = key `2`, and so on. These work with both the top-row number keys and the numeric keypad.

| Action | Windows | macOS |
| :--- | :--- | :--- |
| Next Wallpaper on Display *N* | `Alt` + `N` + `→` | `Command` + `N` + `→` |
| Previous Wallpaper on Display *N* | `Alt` + `N` + `←` | `Command` + `N` + `←` |
| Block Image on Display *N* | `Alt` + `N` + `↓` | `Command` + `N` + `↓` |
| Add to Favorites on Display *N* | `Alt` + `N` + `↑` | `Command` + `N` + `↑` |
| Pause Play on Display *N* | `Alt` + `N` + `P` | `Command` + `N` + `P` |

**Example:** To go to the next wallpaper on Display 2, press and hold `Alt` (Windows) or `Command` (macOS), then while still holding it, press `2` and `→` simultaneously.

> [!TIP]
> **Browser Navigation Conflicts:** The Targeted Actions (e.g., `Alt + Arrow` on Windows or `Command + Arrow` on macOS) are common shortcuts for browser navigation. If you find Spice is "hijacking" your browser's back/forward buttons, you can disable targeted shortcuts independently in the **App Settings**.

> **macOS Note:** Targeted shortcuts require Accessibility permissions. macOS will prompt you the first time Spice tries to detect which number key is held.

---

## Preferences: App Tab

Open via **Tray → Preferences** or `Ctrl` + `Alt` (Windows) / `Cmd` + `Control` (macOS) + `O`.

The **App** tab controls application-wide behaviour, independent of any wallpaper source.

| Setting | Description |
| :--- | :--- |
| **Enable System Notifications** | Toggle desktop toast notifications (e.g., "Paused Play", "Next Wallpaper"). Useful to turn off if they become distracting. |
| **Enable New Version Check** | Spice checks for updates once on startup and once per day. A tray indicator appears when a newer version is available. |
| **Enable global shortcuts** | Master switch for all keyboard hotkeys. Disable if the shortcuts conflict with another application. |
| **Enable Targeted Shortcuts** | Enable or disable targeted shortcuts (`Alt + 1-9 + Arrow` on Windows / `Cmd + 1-9 + Arrow` on macOS). Recommended to disable if they conflict with browser navigation. |
| **Theme** | Choose between *System* (follows OS dark/light mode), *Dark*, or *Light*. Changes apply immediately. |

---

## Preferences: Wallpaper Tab

The **Wallpaper** plugin tab appears immediately after the App tab. It contains four sub-sections accessed via a side navigation bar: **General**, **Online**, **Local**, and **AI**.

### General Settings

| Setting | Description |
| :--- | :--- |
| **Wallpaper Change Frequency** | How often Spice automatically rotates. Options range from *Every 5 Minutes* to *Daily*. Set to *Never* to disable automatic rotation entirely (you can still change manually via the tray or hotkeys). |
| **Cache Size** | How many images to keep on disk. A larger cache means faster display and fewer network requests at startup. Set to *None* to disable caching (images are fetched fresh each time). |
| **Smart Fit Mode** | Controls how Spice fits images to your screen — see below. |
| **Enable Face Crop** | When Smart Fit is active, the cropper aggressively centers on the largest detected face. **Note**: This setting is automatically disabled if Smart Fit Mode is "Disabled". |
| **Enable Face Boost** | When Smart Fit is active, the cropper *hints* toward faces but also considers overall composition. **Note**: This setting is automatically disabled if Smart Fit Mode is "Disabled". |
| **Stagger monitor changes** | Adds a small random delay between wallpaper changes on each display during automatic rotation, preventing a distracting simultaneous flash across all screens. |
| **Change wallpaper on start** | When enabled, Spice immediately changes the wallpaper when the app launches. Disable this to show the last-seen wallpaper until the timer fires. |
| **Refresh wallpapers nightly** | Spice quietly re-fetches images from all active sources once per night, keeping the cache fresh with new content without interrupting your day. |
| **Display Configuration → Refresh Displays** | Manually tell Spice to re-detect all connected monitors. Use this if you plug in or unplug a display while Spice is running. |
| **Clear Wallpaper Cache** | Deletes all downloaded images from disk. You will need an internet connection before new wallpapers appear again. Requires confirmation. |
| **Blocked Images → Reset** | Clears the list of blocked images, allowing previously deleted images to be re-downloaded. Requires confirmation. |

#### Smart Fit Modes

| Mode | Behaviour |
| :--- | :--- |
| **Disabled** | Images are used as-is — no processing. Fastest. |
| **Quality** | Rejects images whose aspect ratio doesn't match your monitor. No black bars, no stretched photos. May skip some images. |
| **Flexibility** | Accepts high-resolution images even if their aspect ratio differs from your screen, then crops intelligently. More variety. |

---

### Online Sources

The **Online** tab lists each cloud and institutional image provider as an expandable accordion card. Click a provider name to expand its settings.

#### Wallhaven
 
Wallhaven requires a free API key for NSFW content and higher search quotas. 
- **Verification**: Enter your key and click **Verify & Connect**. Spice will perform a live check to ensure the key is valid.
- **Persistence**: Once verified, the key is automatically saved and **locked** for your security. 
- **Clearance**: To change or remove it, use the **Clear API Key** button.
- **Favorites Sync**: Enter your **Wallhaven Username** and click **Verify Username** to enable the "Keep Favorites Synced" feature.

**Account Reset:**
Clicking the **Clear API Key** button performs a "Full Reset" of your Wallhaven integration. This will:
- Clear your API Key and Wallhaven Username.
- Disable "Keep Favorites Synced".
- **Remove all managed collections** (synced from your favorites) from your list.
- *Note: Manually added Wallhaven queries are preserved.*
#### Pexels

Like Wallhaven, Pexels requires an API key for access.
- **Verification**: Enter your key and click **Verify & Connect**.
- **Security**: The key is masked and locked once successfully verified.
 **Discovery & Sync:**
Spice can automatically discover all your public Wallhaven collections.
1. Enter your **Wallhaven Username** in the settings panel.
2. Click **Sync Collections Now**.
3. Spice will fetch your public collections and add them to the query list automatically.
4. New collections appear as **Managed** (synchronized) queries.

**Adding a manual collection:**
1. Go to [wallhaven.cc](https://wallhaven.cc) and search for a topic you like.
2. Copy the URL from your browser (e.g., `https://wallhaven.cc/search?q=nature&categories=110`).
3. Paste the URL into the **Add Collection** field, give it a name, and click **Add**.
4. The new source appears in the list with an **Active** checkbox.
5. Click **Apply** to save.

#### Pexels
 
Requires a Pexels API key (free). Enter it in the Pexels settings panel. Like Wallhaven, the key is **masked** and **locked** once saved; use the **Clear API Key** button to reset it.

Paste a Pexels search URL or collection URL and give it a name. Adding a collection follows the same pattern as Wallhaven.

#### Wikimedia Commons

Wikimedia Commons is a vast, dynamic repository of freely-licensed media. Unlike static museum collections, you can add your own personalized queries to Spice.

**Configuration Guide:**
- **Search terms**: Enter `search:nature` or paste a MediaSearch URL to fetch images matching a specific topic.
- **Categories**: Enter `category:Deep space` or paste a Category URL to rotate through all images in a specific Commons category.
- **Specific Files**: Enter `file:File:Earth_Eastern_Hemisphere.jpg` or a direct File URL to persistently display a single masterpiece.

> **Tip:** Wikimedia Commons is a community-driven project. You can support their mission via the **Donate to Wikimedia** link in the provider settings.

#### Museum Sources (Online)

Spice integrates with world-class museums, bringing curated artistic experiences directly to your desktop. These are high-resolution, open-access collections (CC0) that allow you to explore the world's greatest creative achievements.

**Available Museums:**
- **Metropolitan Museum of Art** (New York City, USA)
- **Art Institute of Chicago** (Chicago, IL, USA)

**How to Use:**
1. Open **Preferences → Wallpaper → Online**.
2. Expand a museum card (e.g., *The Metropolitan Museum of Art*).
3. Use the **Map** link to explore the museum's location or the **Donate** link to support their open-access initiatives.
4. Check the boxes next to curated collections (e.g., *Highights*, *European Paintings*) and click **Apply**.

#### Google Photos *(Beta)*

Google Photos integration uses the **Google Photos Picker API**, which lets you grant Spice access to specific albums without giving it access to your entire library. No Google API key is needed — authorization is handled through your browser.

> **This feature is currently in beta.** To request access, please [open an issue on GitHub](https://github.com/dixieflatline76/Spice/issues) and include your Google account email. Beta testers will be added to the allowlist.

Once access is granted:
1. Open **Preferences → Online → Google Photos**.
2. Click **Sign in with Google** and authorize via the Picker flow (only the albums you select are accessible).
3. Choose the albums you want to use and click **Apply**.

---

### Local Sources

The **Local** tab contains your **Favorites** library and any local folder sources you have added.

---

## Favorites

Favorites lets you permanently save copies of any wallpaper you love, so they keep appearing even if the original source is removed or a query is changed.

### How Favorites Work

- When you **Add to Favorites** (via the tray menu or `Alt` (Windows) / `Command` (macOS) + `N` + `↑`), Spice copies the current wallpaper into a local folder on your machine.
- Spice stores up to **200 favorites** (FIFO — the oldest is pruned when the limit is reached).
- Favorites are stored as independent image files. They persist even if the original collection is deleted or disabled.
- Favorites has its own **Active** toggle in **Local → Favorites → Wallpaper Sources**. When active, Spice will include your favorite images in the rotation alongside online sources.

### Removing a Favorite

- **Unfavoriting from the tray menu:** If the currently displayed wallpaper is a favorite image, the "Add to Favorites" item toggles to remove it. The image is deleted from disk immediately.
- **Clear All Favorites:** In **Local → Favorites**, a **Clear All Favorites** button wipes the entire collection. This cannot be undone.
- **Open Favorites Folder:** Lets you browse and manage the raw files in your system's file manager.

> Favorites images are stored in your system temp directory under `spice/favorite_images/`. You can back up this folder to preserve your collection.

---

## Multi-Display Setup

Spice fully supports any number of connected monitors. Each display is managed independently.

### Automatic Detection

When Spice starts, it detects all connected displays automatically. If you connect or disconnect a display while Spice is running:

1. Open **Preferences → Wallpaper → General → Refresh Displays**, or
2. Use the hotkey `Ctrl` + `Alt` + `D` (Windows) / `Cmd` + `Control` + `D` (macOS).

### Per-Display Control

- **Each display gets its own wallpaper queue.** Spice fetches from your active sources and distributes images across monitors.
- **Pausing is per-display.** You can pause Display 2 while Display 1 keeps rotating.
- **Favorites and Delete are per-display.** The action always applies to the wallpaper currently shown on the targeted display.

### Tray Menu with Multiple Displays

- **Display 1** controls appear at the **top level** of the tray menu (no submenu needed).
- **Display 2, 3, …** each have their own **submenu** with identical controls.

### Stagger Setting

With the **Stagger monitor changes** option enabled (recommended for 2+ displays), Spice introduces a random delay between each display's automatic rotation. This prevents all screens from flashing at the same moment, creating a more pleasant, natural feel.

---

## Browser Extension

The Spice browser extension brings wallpaper discovery directly into your web browser, allowing you to sync images to your desktop without ever leaving the page. It is available for **Chrome**, **Firefox**, and **Safari**.

### Installation

| Browser | How to Install |
| :--- | :--- |
| **Chrome** | Install from the [Chrome Web Store](https://chromewebstore.google.com) — search for *Spice Wallpaper* |
| **Firefox** | Install from [Firefox Add-ons (AMO)](https://addons.mozilla.org) — search for *Spice Wallpaper* |
| **Safari** | Open the Spice DMG and drag **both** the Spice app and the Spice Safari Extension into your **Applications** folder. Then enable it in **Safari → Settings → Extensions**. |

> **Pro Tip:** After installation, **pin the extension** to your browser toolbar so you can always see the status icon.

### How to Use the Extension

The extension acts as an intelligent "remote control" for the Spice app on your computer.

1.  **Automatic Detection:** As you browse supported sites (like *Wallhaven* or *Google Photos*), the extension automatically scans for high-resolution images.
2.  **The Pulsing Icon:** When a compatible image is found, the Spice icon in your toolbar will **pulse green**. This is your signal that the image can be synced.
3.  **One-Click Sync:** Simply click the pulsing icon. Spice will instantly download the image and set it as your wallpaper across all displays.
4.  **Instant Execution:** Thanks to **LiveSync Technology**, the communication between your browser and the desktop app is instantaneous. Your desktop updates the second you click.

### Supported Sites

The extension works out of the box with the following premier sources:
- **Wallhaven**
- **Pexels**
- **Wikimedia Commons**
- **Google Photos** (when viewing albums)

### Requirements & Privacy

- **Spice must be running:** The extension requires the Spice desktop application to be open and active in the system tray/menu bar.
- **Local Connection:** All communication happens over a secure local WebSocket (`localhost`). Your browsing data and selected wallpapers never leave your system and are never sent to a cloud server.
- **Always Responsive:** The extension includes a "Keep-Alive" system to ensure it stays ready even if your browser snoozes background tabs.


---

*That's everything you need to get the most out of Spice. If you run into issues or have questions, visit the [GitHub repository](https://github.com/dixieflatline76/Spice).*
