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
7. [Browser Extensions](#browser-extensions)

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
 
Spice is designed for power users who want total control without interrupting their flow. Our global hotkeys are instant, responsive, and cross-platform.

**Why it's cool:**
- **Zero-Touch Control**: Advance your wallpapers or save a favorite without ever leaving your IDE, browser, or game.
- **The "Magic Number" System**: Our unique targeting system lets you control specific monitors independently by simply holding a number key (1-9).
- **Universal Muscle Memory**: Whether you're on Windows or macOS, the shortcut logic remains consistent and intuitive.

> **Note for macOS:** Global actions use `Command` + `Control` (similar to Windows `Ctrl` + `Alt`). Targeted actions use `Command` (similar to Windows `Alt`).

### Global Actions — All Displays At Once
These shortcuts perform actions across every connected monitor simultaneously.

| Action | Windows | macOS |
| :--- | :--- | :--- |
| **Next Wallpaper** | `Ctrl` + `Alt` + `→` | `Cmd` + `Ctrl` + `→` |
| **Previous Wallpaper** | `Ctrl` + `Alt` + `←` | `Cmd` + `Ctrl` + `←` |
| **Sync / Detect Displays** | `Ctrl` + `Alt` + `D` | `Cmd` + `Ctrl` + `D` |
| **Open Preferences** | `Ctrl` + `Alt` + `O` | `Cmd` + `Ctrl` + `O` |

### Targeted Actions — Precision Monitor Control
Hold a **number key (1–9)** alongside the modifier to target a specific display. Display 1 = key `1`, Display 2 = key `2`, etc.

| Action | Windows | macOS |
| :--- | :--- | :--- |
| Next on Display *N* | `Alt` + `N` + `→` | `Command` + `N` + `→` |
| Previous on Display *N* | `Alt` + `N` + `←` | `Command` + `N` + `←` |
| **Block / Delete** | `Alt` + `N` + `↓` | `Command` + `N` + `↓` |
| **Add to Favorites** | `Alt` + `N` + `↑` | `Command` + `N` + `↑` |
| **Pause / Play** | `Alt` + `N` + `P` | `Command` + `N` + `P` |

**Example:** To skip the wallpaper on your second monitor, press and hold `Alt` (Windows) or `Command` (macOS), tap `2`, and press `→`. 

### Customizing & Disabling Shortcuts
 
Spice gives you granular control over how hotkeys interact with your system. If you find that Spice shortcuts conflict with other software (like your IDE or web browser), you have two options in **Preferences → App**:

1. **Disable All Shortcuts**: The "Enable global shortcuts" master switch completely unregisters Spice from your system's input loop. Use this if you prefer using the tray menu exclusively.
2. **Disable Targeted Shortcuts Only**: If you love the global rotation shortcuts (`Ctrl` + `Alt` + `Arrow`) but find that the Targeted Shortcuts (e.g., `Alt` + `1` + `Right`) interfere with your browser's "Go Back/Forward" actions, you can disable **only** the targeted modifiers.

> **macOS Permissions**: Targeted shortcuts require Accessibility permissions. macOS will prompt you once when Spice first detects a number key press. If shortcuts stop working, ensure Spice is still enabled in **System Settings → Privacy & Security → Accessibility**.

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
| **Language** | Select between 11 supported languages (English, German, Spanish, French, Italian, Portuguese, Japanese, Russian, Ukrainian, or Chinese). Choosing **System Default** follows your OS language. |

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
 
Wallhaven is a premier destination for high-quality wallpapers. Spice integrates deeply with the Wallhaven API to provide a seamless search and synchronization experience.

**Authentication:**
Entering your API Key is highly recommended as it enables access to your private favorite collections and higher search quotas.
- **Verification**: Paste your key and click **Verify & Connect**. Spice performs an immediate live check.
- **Security**: Once verified, the key is masked (dots) and permanently **locked**.
- **Change/Remove**: To update the key, you must use the **Clear API Key** button, which performs a [Full Reset](#account-reset).

**Favorites Synchronization:**
This is the "killer feature" for Wallhaven power users. Instead of manually adding individual search URLs, Spice can mirror your entire Wallhaven account.
- **Setting it up**:
  1. Enter your **Wallhaven Username**.
  2. Click **Verify Username**. (Note: An API Key must be verified first).
  3. Once verified, the **Keep Favorites Synced** checkbox will become available.
- **The Magic**: When "Keep Favorites Synced" is enabled, Spice will automatically discover all your public favorite collections and add them as **Managed** queries.
- **Live Sync**: As you add or remove favorite collections on the Wallhaven website, Spice will detect the changes and automatically update its source list to match.

**Adding Manual Queries:**
If you want to track a specific search that isn't in your favorites:
1. Go to [wallhaven.cc](https://wallhaven.cc) and search for a topic.
2. Copy the browser URL (e.g., `https://wallhaven.cc/search?q=nature&categories=110`).
3. Back in Spice, click **Add Wallhaven URL**, paste the link, and give it a descriptive name.

<a name="account-reset"></a>
**Account Reset:**
Clicking **Clear API Key** is a destructive but necessary action if you wish to change accounts. It will:
1. Clear the stored API Key and Username.
2. Disable the synchronization engine.
3. **Remove all Managed collections** from your list to prevent data orphans.
*Note: Your manually added queries are left untouched.*

#### Pexels
 
Pexels is a high-quality stock photography provider known for its vibrant, modern imagery. Spice leverages the Pexels API to bring these professional photos directly to your desktop.

**How to Use:**
- **Verification**: Paste your free API key and click **Verify & Connect**. Like Wallhaven, Spice ensures the key is active before locking it for security.
- **Adding Queries**: Use the **Add Pexels Search** button to paste a URL from the [Pexels website](https://pexels.com). You can track specific search terms (e.g., "Minimalist Interiors") or follow hand-picked collections from top photographers.

#### Wikimedia Commons
 
Wikimedia Commons is a vast, dynamic repository of freely-licensed media from millions of contributors. Unlike static museum collections, Commons allows you to tap into a live stream of real-world history and discovery.

**The Power of Discovery:**
- **Search**: Use `search:nature` or paste a MediaSearch URL to fetch images matching specific topics.
- **Category Power**: Follow deep categories like `category:Deep space` or `category:Impressionist paintings` for a focused rotation.
- **Specific Files**: Want to stick with a single masterpiece? Enter a direct File URL (e.g., `file:File:Earth_Eastern_Hemisphere.jpg`).

> **Tip:** Support the community! You can contribute to their mission via the **Donate to Wikimedia** link in the provider settings.

#### Museum Sources (Online)
 
Art has no borders. Spice integrates with the world’s leading cultural institutions to transform your screen into a rotating gallery of historical masterpieces, all provided under Open Access (CC0) licenses.

**Available Museums:**
- **Metropolitan Museum of Art** (New York City, USA)
- **Art Institute of Chicago** (Chicago, IL, USA)

**The "Director's Cut" Collections:**
Each museum provides curated collections designed to showcase institutional highlights:
- **Arts of Asia**: From ancient ceramics to modern prints.
- **The Impressionists**: Iconic works from Monet, Degas, and Renoir.
- **High Resolution Art**: Masterpieces chosen for their exceptional detail and scale.

**How to Use:**
1. Open **Preferences → Wallpaper → Online**.
2. Expand a museum card.
3. Use the **Map** link to "Plan a Visit" and explore the institution's location.
4. Toggle the collections you want and click **Apply**.

#### Google Photos
 
Google Photos is for your personal memories. Spice uses the **Google Photos Picker API**, ensuring a high-privacy integration.

**Why it's cool:**
- **Manual Picker**: Instead of giving Spice access to your entire library, the browser-based Picker flow lets you hand-select exactly which albums or media items are available.
- **Privacy First**: Spice never sees your credentials; authorization is handled securely through the official Google Auth flow in your default browser.
- **High Resolution**: Images are downloaded at their original resolution (where supported) to ensure your memories look stunning on high-DPI displays.

**How to Use:**
1. Open **Preferences → Wallpaper → Online → Google Photos**. (Note: May require beta allowlisting).
2. Click **Connect to Google Photos**.
3. In your browser, select the albums or specific photos you want to use.
4. Back in Spice, toggle your Google Photos source and click **Apply**.

---

### Local Sources
 
Sometimes the best wallpapers are the ones you already own or have carefully curated into your favorites. These can be managed in the **Local** tab of the Spice Preferences.

#### Local Favorites
The "Favorites" provider is the heart of Spice's localized content. It acts as a persistent archive of images you've "loved" from other providers.

**The "Killer" Features:**
- **Persistent Storage**: When you favorite an image (via the Tray Menu or Keyboard Shortcut), Spice downloads a permanent copy to your local storage. Even if the original collection on Wallhaven or Pexels is deleted, your favorite remains.
- **Auto-Syncing**: Your Favorites source is active by default. As soon as you "heart" an image, it's added to your rotation instantly.
- **Deep Integration**: Favorited images preserve their original metadata, so you can still use the "Open in Browser" feature to visit the original source.

**How to Use:**
- **Adding**: Click the Heart icon in the tray menu or use `Alt` + `N` + `↑` (Windows) / `Command` + `N` + `↑` (macOS).
- **Managing**: Go to the **Local** tab to see your library. You can toggle your favorites on/off or clear your entire collection using the **Clear** button.
- **Quick-Access**: Click the **"❤️ Personal Favorites"** name in the list to immediately open your local favorites folder in the OS file explorer.

#### Local Folders
Want to use your own photography or personal collection?
1. Open **Preferences → Wallpaper → Local**.
2. Click **Add Folder** to select a directory on your computer.
   - **Note for Windows Users:** Due to OS limitations, the folder picker requires you to select a specific file. Navigate to your desired folder, click on **any image file** inside it, and click "Open". Spice will automatically add the entire folder containing that image to your rotation, not just the single file.
3. Spice will search for high-resolution images (`.jpg`, `.png`, `.webp`) and add them to your rotation.
4. **Tip**: Click the folder name in the list to open that directory directly in your file explorer.

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

## Browser Extensions
 
The Spice Browser Extension is the ultimate companion for discovery. It bridges the gap between your web browser and your desktop, allowing you to sync new collections with zero configuration.

### Why it's cool:
- **LiveSync Discovery**: The extension is "alive." As you browse supported sites like Wallhaven or Pexels, it silently scans the page for compatible search and collection URLs.
- **The Pulsing Signal**: When a valid source is found, the Spice icon in your toolbar will **pulse green**. This is your immediate visual signal that a "Masterpiece Collection" has been detected.
- **One-Click Synchronization**: Clicking the extension opens a small popup where you can instantly add the discovered URL as a new Spice query. No copying, pasting, or manual setup required.
- **Zero Privacy Sacrifice**: The extension communicates with Spice over a secure local WebSocket (`localhost`). Your browsing habits and selected images are never sent to a cloud server.

### Installation
Spice is available for all major desktop browsers:
- **Chrome / Edge / Brave**: Install from the [Chrome Web Store](https://chromewebstore.google.com).
- **Firefox**: Install from [Firefox AMO](https://addons.mozilla.org).
- **Safari**: Included with the Spice macOS app. Activate it in **Safari → Settings → Extensions**.

### Supported Sites
The extension is pre-tuned for high-resolution discovery on:
- **Wallhaven**: Search results, top lists, and personal favorites.
- **Pexels**: Curated collections and modern photography searches.
- **Wikimedia Commons**: Specific categories and MediaSearch topics.

> **Pro Tip:** Keep Spice running in your system tray! The extension needs the desktop app to be open to receive its synchronization signals.
