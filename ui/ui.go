package ui

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"github.com/disintegration/imaging"
	"github.com/dixieflatline76/Spice/v2/asset"
	"github.com/dixieflatline76/Spice/v2/config"
	"github.com/dixieflatline76/Spice/v2/pkg/api"
	"github.com/dixieflatline76/Spice/v2/pkg/hotkey"
	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/pkg/sysinfo"
	"github.com/dixieflatline76/Spice/v2/pkg/ui"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/schema"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/v2/util"
	utilLog "github.com/dixieflatline76/Spice/v2/util/log"
)

// SpiceApp represents the application
type SpiceApp struct {
	fyne.App
	assetMgr     *asset.Manager
	trayMenu     *fyne.Menu
	splash       fyne.Window   // Splash window for initial setup
	notifiers    []ui.Notifier // List of notifiers to activate
	plugins      []ui.Plugin   // List of plugins to activate
	os           OS            // Operating system interface
	appConfig    *config.AppConfig
	prefsWindow  fyne.Window        // Singleton preferences window
	prefsTabs    *container.AppTabs // Reference to the main tabs in preferences
	tabItems     map[string]*container.TabItem
	trayMu       sync.Mutex
	trayDebounce *time.Timer
}

// OS interface defines methods for transforming the application state
// to foreground or background. This is useful for managing the application
// behavior on different operating systems, such as macOS where background apps
// do not show a Dock icon.
type OS interface {
	// TransformToForeground changes the application to be a regular app with a Dock icon.
	TransformToForeground()
	// TransformToBackground changes the application to be a background-only app.
	TransformToBackground()
	// SetupLifecycle sets up OS-specific lifecycle hooks.
	// This is where Chrome OS "fake tray" logic resides.
	SetupLifecycle(app fyne.App, sa *SpiceApp)
}

var (
	saInstance *SpiceApp // Singleton instance of the application
	saOnce     sync.Once // Ensures the singleton is created only once
)

// GetPluginManager returns the singleton instance of the application as a UIPluginManager
func GetPluginManager() ui.PluginManager {
	return getInstance()
}

// Register registers a plugin with the application
func (sa *SpiceApp) Register(plugin ui.Plugin) {
	sa.plugins = append(sa.plugins, plugin)
	plugin.Init(sa)
}

// Deregister deregisters a plugin from the application
func (sa *SpiceApp) Deregister(plugin ui.Plugin) {
	// Implementation for deregistering a plugin
	for i, p := range sa.plugins {
		if p == plugin {
			sa.plugins = append(sa.plugins[:i], sa.plugins[i+1:]...)
			break
		}
	}
}

// OpenPreferences opens the preferences window
func (sa *SpiceApp) OpenPreferences(tab string) {
	// Must execute on Fyne's main thread, as this can be called from background goroutines
	// (e.g. the local API HTTP server triggered by the browser extension)
	// macOS explicitly forbids window creation from background threads.
	if sa.App != nil {
		fyne.Do(func() {
			sa.CreatePreferencesWindow(tab)
		})
	} else {
		// Fallback for tests or extreme edge cases before app is fully initialized
		sa.CreatePreferencesWindow(tab)
	}
}

// ... (Lines 91-348 unrelated mostly, but Line 176 calls CreatePreferencesWindow)

// Let's target ONLY `OpenPreferences` first (lines 87-89).
// Then `CreateTrayMenu` call site.
// Then `CreatePreferencesWindow` definition.

// GetPreferences returns the preferences for the application
func (sa *SpiceApp) GetPreferences() fyne.Preferences {
	return sa.Preferences()
}

// GetAssetManager returns the asset manager
func (sa *SpiceApp) GetAssetManager() *asset.Manager {
	return sa.assetMgr
}

// RegisterNotifier registers a notifier with the application
func (sa *SpiceApp) RegisterNotifier(notifier ui.Notifier) {
	sa.notifiers = append(sa.notifiers, notifier)
}

// GetApplication returns the singleton instance of the application
func GetApplication() ui.App {
	return getInstance()
}

// GetInstance returns the singleton instance of the application
func getInstance() *SpiceApp {
	// Create a new instance of the application if it doesn't exist
	saOnce.Do(func() {
		fmt.Fprintln(os.Stderr, "[Spice] getInstance: starting")

		// Initialize the wallpaper service for right OS
		currentOS := getOS()
		fmt.Fprintln(os.Stderr, "[Spice] getInstance: OS interface created")

		a := app.NewWithID(config.AppName)
		fmt.Fprintln(os.Stderr, "[Spice] getInstance: Fyne app created")

		if _, ok := a.(desktop.App); ok {
			fmt.Fprintln(os.Stderr, "[Spice] getInstance: desktop.App confirmed")

			saInstance = &SpiceApp{
				App:      a,
				assetMgr: asset.NewManager(),
				trayMenu: fyne.NewMenu(""),
				notifiers: []ui.Notifier{func(title, message string) {
					a.SendNotification(fyne.NewNotification(title, message))
				}},
				plugins:  []ui.Plugin{},
				os:       currentOS,
				tabItems: make(map[string]*container.TabItem),
			}
			saInstance.appConfig = config.NewAppConfig(saInstance.Preferences())
			fmt.Fprintln(os.Stderr, "[Spice] getInstance: app config loaded")

			// Apply saved debug logging preference
			if saInstance.appConfig.GetDebugLoggingEnabled() {
				utilLog.SetDebugEnabled(true)
			}

			// Apply saved language preference
			i18n.SetLanguage(saInstance.appConfig.GetLanguage())

			// Apply saved theme
			currentTheme := saInstance.appConfig.GetTheme()
			switch currentTheme {
			case "Dark":
				saInstance.Settings().SetTheme(&forcedVariantTheme{Theme: theme.DefaultTheme(), variant: theme.VariantDark})
			case "Light":
				saInstance.Settings().SetTheme(&forcedVariantTheme{Theme: theme.DefaultTheme(), variant: theme.VariantLight})
			default:
				saInstance.Settings().SetTheme(&spiceTheme{Theme: theme.DefaultTheme()})
			}
			fmt.Fprintln(os.Stderr, "[Spice] getInstance: theme applied")

			saInstance.verifyEULA()
			fmt.Fprintln(os.Stderr, "[Spice] getInstance: EULA verified")

			// Setup OS-specific lifecycle hooks (e.g. Chrome OS Pseudo-Tray)
			saInstance.os.SetupLifecycle(saInstance.App, saInstance)

			// Common Lifecycle: Sync Monitors on return to foreground (e.g. Settings opened)
			saInstance.Lifecycle().SetOnEnteredForeground(func() {
				utilLog.Debug("App entered foreground - synchronizing monitors...")
				wp := wallpaper.GetInstance()
				if wp != nil {
					// Low-CPU check
					wp.SyncMonitors(false)
				}
			})
			fmt.Fprintln(os.Stderr, "[Spice] getInstance: lifecycle hooks set")
		} else {
			fmt.Fprintln(os.Stderr, "[Spice] getInstance: FATAL — not a desktop.App")
			utilLog.Fatal("Spice not supported on this platform")
		}
	})
	return saInstance
}

// NotifyUser sends a notification to the user via all registered notifiers
func (sa *SpiceApp) NotifyUser(title, message string) {
	// Check if notifications are enabled
	if !sa.appConfig.GetAppNotificationsEnabled() {
		return
	}

	for _, notify := range sa.notifiers {
		notify(title, message)
	}
}

// CreateTrayMenu creates the tray menu for the application
func (sa *SpiceApp) CreateTrayMenu() {
	defer func() {
		if r := recover(); r != nil {
			utilLog.Printf("ERROR: PANIC in CreateTrayMenu: %v", r)
		}
	}()
	utilLog.Debug("CreateTrayMenu: Starting...")
	desk, ok := sa.App.(desktop.App)
	if !ok {
		return
	}

	items := []*fyne.MenuItem{}
	for i, plugin := range sa.plugins {
		utilLog.Debugf("CreateTrayMenu: Processing plugin %s...", plugin.Name())
		if i == 0 {
			items = append(items, plugin.CreateTrayMenuItems()...)
		} else {
			pluginSubmenu := fyne.NewMenuItem(plugin.Name(), nil)
			pluginSubmenu.ChildMenu = fyne.NewMenu(plugin.Name(), plugin.CreateTrayMenuItems()...)
			items = append(items, pluginSubmenu)
		}
	}

	items = append(items, fyne.NewMenuItemSeparator())
	items = append(items, sa.CreateMenuItem(i18n.T("Preferences"), func() {
		sa.CreatePreferencesWindow("")
	}, "prefs.png"))

	items = append(items, fyne.NewMenuItemSeparator())
	items = append(items, sa.CreateMenuItem(i18n.T("About Spice"), func() {
		sa.CreateAboutSplash()
	}, "tray.png"))

	quitItem := sa.CreateMenuItem(i18n.T("Quit"), func() {
		sa.os.TransformToForeground()     // Ensure the app is in the foreground before quitting
		time.Sleep(50 * time.Millisecond) // Small delay to ensure the OS processes the state change
		sa.deactivateAllPlugins()         // Deactivate all plugins before quitting
		time.Sleep(2 * time.Second)       // Small delay to ensure plugins are deactivate
		utilLog.Println("Quitting Spice application")
		sa.Quit()
	}, "quit.png")
	quitItem.IsQuit = true
	items = append(items, fyne.NewMenuItemSeparator(), quitItem)

	sa.trayMenu = fyne.NewMenu("", items...)
	trayIcon, err := sa.assetMgr.GetIcon("tray.png")
	if err != nil {
		utilLog.Printf("ERROR: Failed to load tray icon: %v", err)
	}

	sa.SetIcon(trayIcon)

	utilLog.Debug("Calling desk.SetSystemTrayMenu...")
	desk.SetSystemTrayMenu(sa.trayMenu)
	utilLog.Debug("desk.SetSystemTrayMenu finished.")

	if trayIcon != nil {
		utilLog.Debug("Calling desk.SetSystemTrayIcon...")
		desk.SetSystemTrayIcon(trayIcon)
		utilLog.Debug("desk.SetSystemTrayIcon finished.")
	} else {
		utilLog.Debug("Skipping desk.SetSystemTrayIcon (icon is nil)")
	}
	utilLog.Debug("System Tray Menu and Icon setup process completed.")
}

// DeactivateAllPlugins deactivates all plugins in the application
func (sa *SpiceApp) deactivateAllPlugins() {
	// Deactivate all plugins
	for _, plugin := range sa.plugins {
		plugin.Deactivate()
		utilLog.Printf("Deactivated plugin: %s", plugin.Name())
	}
}

// CreateMenuItem creates a menu item with the given label, action, and icon
func (sa *SpiceApp) CreateMenuItem(label string, action func(), iconName string) *fyne.MenuItem {
	mi := fyne.NewMenuItem(label, func() {
		fyne.Do(action)
	})
	if iconName == "" {
		return mi
	}
	icon, err := sa.assetMgr.GetIcon(iconName)
	if err != nil {
		utilLog.Printf("Failed to load icon: %v", err)
		return mi
	}
	mi.Icon = icon
	return mi
}

// CreateToggleMenuItem creates a toggle menu item with the given label, action, icon, and checked state
func (sa *SpiceApp) CreateToggleMenuItem(label string, action func(bool), iconName string, checked bool) *fyne.MenuItem {

	mi := fyne.NewMenuItem("", nil)

	if checked {
		mi.Label = fmt.Sprintf("%s ✔", label)
	} else {
		mi.Label = label
	}

	if iconName != "" {
		icon, err := sa.assetMgr.GetIcon(iconName)
		if err != nil {
			utilLog.Printf("Failed to load icon: %v", err)
			return mi
		}
		mi.Icon = icon
	}
	mi.Checked = checked
	mi.Action = func() {
		fyne.Do(func() {
			newChecked := !mi.Checked
			if newChecked {
				mi.Label = fmt.Sprintf("%s ✔", label)
			} else {
				mi.Label = label
			}
			mi.Checked = newChecked
			action(newChecked)
			sa.trayMenu.Refresh()
		})
	}

	return mi
}

// addVersionWatermark adds a version watermark to the given image.
func (sa *SpiceApp) addVersionWatermark(img image.Image) (image.Image, error) {

	versionString := fmt.Sprintf("Version: %s", config.AppVersion)

	// Create a watermark image
	watermark := imaging.New(img.Bounds().Dx(), img.Bounds().Dy(), color.Transparent)

	// Add the label directly to the watermark image
	col := color.RGBA{100, 50, 0, 200} // Dark brown with some transparency

	// Calculate the width of the text
	bounds, _ := font.BoundString(basicfont.Face7x13, versionString)
	textWidth := bounds.Max.X.Ceil()

	//nolint:gosec // G115: integer overflow conversion int -> int32. We assume reasonable usage for UI dimensions.
	point := fixed.Point26_6{
		X: fixed.Int26_6((img.Bounds().Dx() - textWidth - 10) * 64), // Offset from right edge, accounting for text width
		Y: fixed.Int26_6((img.Bounds().Dy() - 10) * 64),             // Offset from bottom edge
	}

	d := &font.Drawer{
		Dst:  watermark,
		Src:  image.NewUniform(col),
		Face: basicfont.Face7x13, // Use the basic font
		Dot:  point,
	}
	d.DrawString(versionString)

	// Overlay the watermark
	dst := imaging.Overlay(img, watermark, image.Pt(0, 0), 1)
	return dst, nil
}

// CreateSplashScreen creates a splash screen for the application
func (sa *SpiceApp) CreateSplashScreen(seconds int) {
	// Guard: skip splash if OpenGL is unavailable (Fyne GLFW would crash).
	if !sysinfo.CanCreateWindows() {
		utilLog.Println("OpenGL unavailable — skipping splash screen")
		sa.os.TransformToBackground()
		return
	}

	defer func() {
		if r := recover(); r != nil {
			errStr := fmt.Sprintf("%v", r)
			utilLog.Printf("Recovered from splash screen creation panic: %v", errStr)
			if strings.Contains(strings.ToLower(errStr), "apiunavailable") || strings.Contains(strings.ToLower(errStr), "wgl") || strings.Contains(strings.ToLower(errStr), "opengl") {
				ShowNativeFallbackAlert(i18n.T("Graphics Error"), i18n.T("Spice requires OpenGL 2.0+ or hardware acceleration to show windows. The application will continue to run in the background. Please try rebooting your machine to restore graphics functionality."))
			}
		}
	}()

	if sa.splash == nil {
		// Create a splash screen with the application icon
		drv, ok := sa.Driver().(desktop.Driver)
		if !ok {
			utilLog.Println("Splash screen not supported")
			return // Splash screen not supported
		}

		splashWindow := drv.CreateSplashWindow()
		if splashWindow == nil {
			utilLog.Println("Failed to create splash window (driver returned nil)")
			ShowNativeFallbackAlert(i18n.T("Graphics Error"), i18n.T("Spice requires OpenGL 2.0+ or hardware acceleration to show windows. The application will continue to run in the background. Please try rebooting your machine to restore graphics functionality."))
			return
		}

		// Load the splash image
		splashImg, err := sa.assetMgr.GetImage("splash.png")
		if err != nil {
			utilLog.Fatalf("Failed to load splash image: %v", err)
		}

		// Create a watermark image
		watermarkSplashImg, err := sa.addVersionWatermark(splashImg)
		if err == nil {
			splashImg = watermarkSplashImg
		}

		// Create an image canvas object
		img := canvas.NewImageFromImage(splashImg)
		img.FillMode = canvas.ImageFillOriginal // Ensure the image keeps its original size

		// Set the splash window content and show it
		splashWindow.SetContent(img)
		splashWindow.Resize(fyne.NewSize(300, 300))
		splashWindow.CenterOnScreen()
		sa.splash = splashWindow
	}

	// Show the splash screen immediately after creation
	sa.splash.Show()

	// Hide the splash screen after the animation loop
	// CRITICAL: We must NOT sleep on the main thread (fyne.Do), or the UI freezes and GIF won't animate.
	go func() {
		duration := time.Duration(seconds) * time.Second
		time.Sleep(duration)
		fyne.Do(func() {
			if sa.splash != nil {
				sa.splash.Hide() // Close the splash window
				sa.splash = nil
			}
			sa.os.TransformToBackground() // Transform the app to background state
		})
	}()
}

// CreatePreferencesWindow creates and displays the preferences window.
// If the window is already open, it brings it to the front.
func (sa *SpiceApp) CreatePreferencesWindow(initialTab string) {
	// Guard: skip if OpenGL is unavailable (Fyne GLFW would crash process)
	if !sysinfo.CanCreateWindows() {
		utilLog.Println("OpenGL unavailable — preferences window cannot be created")
		ShowNativeFallbackAlert(i18n.T("Graphics Error"), i18n.T("Spice requires OpenGL 2.0+ or hardware acceleration to show windows. The application will continue to run in the background. Please try rebooting your machine to restore graphics functionality."))
		return
	}

	sa.os.TransformToForeground()

	// If window already exists, just show it and switch tab if requested
	if sa.prefsWindow != nil {
		sa.RebuildPreferencesContent(initialTab) // Rebuild to pick up any new pendingAddUrls
		if initialTab != "" && sa.tabItems != nil {
			if item, ok := sa.tabItems[initialTab]; ok {
				sa.prefsTabs.Select(item)
			}
		}
		sa.prefsWindow.Show()
		sa.prefsWindow.RequestFocus()
		return
	}

	// Create a new window for the preferences
	var prefsWindow fyne.Window
	func() {
		defer func() {
			if r := recover(); r != nil {
				errStr := fmt.Sprintf("%v", r)
				utilLog.Printf("Recovered from preferences window creation panic: %v", errStr)
				if strings.Contains(strings.ToLower(errStr), "apiunavailable") || strings.Contains(strings.ToLower(errStr), "wgl") || strings.Contains(strings.ToLower(errStr), "opengl") {
					ShowNativeFallbackAlert(i18n.T("Graphics Error"), i18n.T("Spice requires OpenGL 2.0+ or hardware acceleration to show windows. The application will continue to run in the background. Please try rebooting your machine to restore graphics functionality."))
				}
			}
		}()
		prefsWindow = sa.NewWindow(fmt.Sprintf("%s %s", config.AppName, i18n.T("Preferences")))
	}()

	if prefsWindow == nil {
		utilLog.Println("Preferences window creation failed (returned nil or recovered)")
		ShowNativeFallbackAlert(i18n.T("Graphics Error"), i18n.T("Spice requires OpenGL 2.0+ or hardware acceleration to show windows. The application will continue to run in the background. Please try rebooting your machine to restore graphics functionality."))
		return
	}

	// Wrap the entire window setup in recovery — Fyne can return a non-nil but broken
	// window when OpenGL is unavailable (e.g. Remote Desktop). The actual crash often
	// happens at Show() or Content().MinSize(), not at NewWindow().
	func() {
		defer func() {
			if r := recover(); r != nil {
				utilLog.Printf("Recovered from preferences window setup panic: %v", r)
				ShowNativeFallbackAlert(i18n.T("Graphics Error"), i18n.T("Spice requires OpenGL 2.0+ or hardware acceleration to show windows. The application will continue to run in the background. Please try rebooting your machine to restore graphics functionality."))
				// Clean up the broken window reference
				sa.prefsWindow = nil
				sa.prefsTabs = nil
			}
		}()

		sa.prefsWindow = prefsWindow // Store reference

		// Build and bind the UI contents first, so the layout engine knows existing elements.
		sa.RebuildPreferencesContent(initialTab)

		// Fyne is notoriously stubborn about window minimum sizes.
		// Since UI scales dynamically, we cannot rely on hardcoded logical or physical sizes.
		// We read the layout engine's absolute Minimum Size for this content:
		minSize := prefsWindow.Content().MinSize()

		// The user concluded that ~700 logical pixels at 175% Windows scaling was the perfect physical width.
		// 700 * 1.75 = 1225 physical pixels.
		// To prevent the window from being "microscopic" when Windows is set to 100% scaling,
		// we calculate the required Fyne logical width to maintain a ~1225px physical footprint on screen.
		osScale := sysinfo.GetOSDisplayScale()
		if osScale <= 0 {
			osScale = 1.0
		}

		targetWidth := float32(1225) / osScale

		// Protect against Fyne's layout engine crushing the form fields if the text size demands more space.
		if targetWidth < minSize.Width*1.25 {
			targetWidth = minSize.Width * 1.25
		}

		// Derive height from actual screen height so it's always proportional to the display,
		// regardless of scaling factor. Target 85% of logical screen height.
		_, physHeight, err := sysinfo.GetScreenDimensions()
		logicalScreenHeight := float32(physHeight) / osScale
		targetHeight := targetWidth * 1.0 // Default: square (fallback if screen detection fails)

		if err == nil && logicalScreenHeight > 0 {
			targetHeight = logicalScreenHeight * 0.85

			// Don't let the window be taller than it is wide
			if targetHeight > targetWidth {
				targetHeight = targetWidth
			}
		}
		prefsWindow.SetOnClosed(func() {
			sa.os.TransformToBackground()
			sa.prefsWindow = nil // Clear reference on close
			sa.prefsTabs = nil   // Clear tabs reference
		})

		// Now that targetWidth and targetHeight are guaranteed to be >= MinSize,
		// we can safely call Resize before Show. This ensures macOS and Wayland
		// allocate the correct framebuffer size before the first paint, preventing
		// viewport clipping and the need for manual resizing/refresh hacks.
		prefsWindow.Resize(fyne.NewSize(targetWidth, targetHeight))
		prefsWindow.CenterOnScreen()
		prefsWindow.Show()
	}()
}

// RebuildPreferencesContent rebuilds the content of the preferences window.
// This allows refreshing the UI (e.g. to show "Add Query" dialogs) even if the window is already open.
func (sa *SpiceApp) RebuildPreferencesContent(initialTab string) {
	if sa.prefsWindow == nil {
		return
	}

	sm := NewSettingsManager(sa.prefsWindow)
	sm.RegisterOnSettingsSaved(func() {
		sa.RebuildTrayMenu()
	})

	// --- General Tab ---
	// Theme Selection
	themeOptions := []string{i18n.T("System"), i18n.T("Dark"), i18n.T("Light")}
	currentTheme := sa.appConfig.GetTheme()
	initialThemeIndex := 0
	for i, t := range themeOptions {
		if t == currentTheme {
			initialThemeIndex = i
			break
		}
	}

	// Language Selection
	langOptions := i18n.GetLanguageNames()
	langDisplayOptions := i18n.GetLanguageNames()
	langDisplayOptions[0] = i18n.T("System Default")
	currentLang := sa.appConfig.GetLanguage()
	initialLangIndex := 0
	for idx, l := range langOptions {
		if l == currentLang {
			initialLangIndex = idx
			break
		}
	}

	generalSchema := schema.PanelSchema{
		Sections: []schema.SectionSchema{
			{
				Title: i18n.T("General Application Settings"),
				Items: []schema.ItemSchema{
					schema.ButtonItem{
						Name:       "manageWindowsStartup",
						Label:      i18n.T("Start with Windows:"),
						Help:       i18n.T("Spice registers itself to start with Windows. Click to open Windows Settings to enable or disable this feature."),
						ButtonText: i18n.T("Manage in Windows Settings"),
						OnPressed: func() {
							u, _ := url.Parse("ms-settings:startupapps")
							if u != nil {
								_ = sa.OpenURL(u)
							}
						},
						VisibleIf: func() bool {
							return runtime.GOOS == "windows"
						},
					},
					schema.BoolItem{
						Name:         "enableNotifications",
						InitialValue: sa.appConfig.GetAppNotificationsEnabled(),
						Label:        i18n.T("Enable System Notifications:"),
						Help:         i18n.T("Enable or disable system notifications from Spice."),
						ApplyFunc: func(b bool) {
							sa.appConfig.SetAppNotificationsEnabled(b)
						},
					},
					schema.BoolItem{
						Name:         "enableUpdateCheck",
						InitialValue: sa.appConfig.GetUpdateCheckEnabled(),
						Label:        i18n.T("Enable New Version Check:"),
						Help:         i18n.T("Automatically check for new versions of Spice on startup."),
						ApplyFunc: func(b bool) {
							sa.appConfig.SetUpdateCheckEnabled(b)
						},
						VisibleIf: func() bool { return !config.IsStoreDistribution() },
					},
					schema.BoolItem{
						Name:         "enableShortcuts",
						InitialValue: !wallpaper.GetInstance().GetShortcutsDisabled(),
						Label:        i18n.T("Enable global shortcuts:"),
						Help:         i18n.T("Use keyboard shortcuts to control wallpapers. Disable if they conflict with other apps."),
						ApplyFunc: func(val bool) {
							wallpaper.GetInstance().SetShortcutsDisabled(!val)
							hotkey.StartListeners(GetPluginManager())
						},
					},
					schema.BoolItem{
						Name:         "enableTargetedShortcuts",
						InitialValue: !wallpaper.GetInstance().GetTargetedShortcutsDisabled(),
						Label:        i18n.T("Enable Display Specific Shortcuts (Alt + Arrow + 1-9):"),
						Help:         i18n.T("Disable this if Alt+Arrow conflicts with your browser or other apps."),
						ApplyFunc: func(val bool) {
							wallpaper.GetInstance().SetTargetedShortcutsDisabled(!val)
							hotkey.StartListeners(GetPluginManager())
						},
						EnabledIf: func() bool {
							val := sm.GetValue("enableShortcuts")
							if val == nil {
								return true
							}
							return val.(bool)
						},
					},
					schema.HyperlinkItem{
						ID:   "shortcutsLink",
						Text: i18n.T("View all shortcuts →"),
						URL:  "https://github.com/dixieflatline76/Spice/blob/main/docs/user_guide.md#keyboard-shortcuts",
					},
					schema.SelectItem{
						Name:         "theme",
						Options:      themeOptions,
						InitialValue: initialThemeIndex,
						Label:        i18n.T("Theme:"),
						Help:         i18n.T("Select the application theme."),
						ApplyFunc: func(val interface{}) {
							selectedIndex := val.(int)
							if selectedIndex < 0 || selectedIndex >= len(themeOptions) {
								return
							}
							selectedTheme := themeOptions[selectedIndex]
							sa.appConfig.SetTheme(selectedTheme)

							switch selectedTheme {
							case "Dark":
								sa.Settings().SetTheme(&forcedVariantTheme{Theme: theme.DefaultTheme(), variant: theme.VariantDark})
							case "Light":
								sa.Settings().SetTheme(&forcedVariantTheme{Theme: theme.DefaultTheme(), variant: theme.VariantLight})
							default:
								sa.Settings().SetTheme(&spiceTheme{Theme: theme.DefaultTheme()})
							}
						},
					},
					schema.SelectItem{
						Name:         "language",
						Options:      langDisplayOptions,
						InitialValue: initialLangIndex,
						Label:        i18n.T("Language:"),
						Help:         i18n.T("Select the application language. Restart may be required for full effect."),
						NeedsRefresh: true,
						ApplyFunc: func(val interface{}) {
							selectedIndex := val.(int)
							if selectedIndex < 0 || selectedIndex >= len(langOptions) {
								return
							}
							selectedLang := langOptions[selectedIndex]
							sa.appConfig.SetLanguage(selectedLang)
							i18n.SetLanguage(selectedLang)

							if srv := api.GetServer(); srv != nil {
								if err := srv.BroadcastLanguage(i18n.GetLanguage()); err != nil {
									utilLog.Printf("Failed to broadcast language change to extensions: %v", err)
								}
							}

							activeID := "App"
							selected := sa.prefsTabs.Selected()
							for id, item := range sa.tabItems {
								if item == selected {
									activeID = id
									break
								}
							}
							sa.RebuildPreferencesContent(activeID)
							sa.RebuildTrayMenu()
						},
					},
					schema.BoolItem{
						Name:         "enableDebugLogging",
						InitialValue: sa.appConfig.GetDebugLoggingEnabled(),
						Label:        i18n.T("Enable Debug Logging:"),
						Help:         i18n.T("Write verbose debug entries to the log file. Useful for troubleshooting."),
						ApplyFunc: func(b bool) {
							sa.appConfig.SetDebugLoggingEnabled(b)
							utilLog.SetDebugEnabled(b)
						},
					},
				},
			},
		},
	}

	generalContainer := sm.RenderSchema(generalSchema)

	// Initialize/Reset tab mapping
	sa.tabItems = make(map[string]*container.TabItem)

	generalTabItem := container.NewTabItem(i18n.T("App"), generalContainer)
	sa.tabItems["App"] = generalTabItem

	// --- Plugin Tabs ---
	var pluginTabItems []*container.TabItem
	for _, plugin := range sa.plugins {
		// Create a tab for each plugin
		pluginContainer := plugin.CreatePrefsPanel(sm)
		item := container.NewTabItem(plugin.Name(), pluginContainer)
		sa.tabItems[plugin.ID()] = item
		pluginTabItems = append(pluginTabItems, item)
	}

	// Combine all tabs
	tabs := container.NewAppTabs(generalTabItem)
	sa.prefsTabs = tabs // Store reference for dynamic switching
	for _, item := range pluginTabItems {
		tabs.Append(item)
	}

	// Select the initial tab if specified
	if initialTab != "" {
		if item, ok := sa.tabItems[initialTab]; ok {
			tabs.Select(item)
		}
	}

	closeButton := widget.NewButton(i18n.T("Close"), func() {
		sa.prefsWindow.Close()
	})

	// Layout: Tabs take up the main space, Apply/Close buttons at the bottom
	prefsWindowLayout := container.NewBorder(nil, container.NewVBox(sm.GetApplySettingsButton(), container.NewHBox(layout.NewSpacer(), closeButton)), nil, nil, tabs)

	// Wrap in a minSizeOverride so Fyne doesn't enforce the full content height
	// as the window's minimum size. Without this, Fyne silently ignores Resize()
	// when the content's MinSize exceeds the target (e.g., accordion panels at 1352px
	// on a 1440px screen at 100% scaling).
	sa.prefsWindow.SetContent(newMinSizeContainer(prefsWindowLayout))
	sm.GetCheckAndEnableApplyFunc()() // Trigger initial UI dependency refresh
}

// minSizeContainer wraps a CanvasObject and overrides its MinSize to prevent
// Fyne from enforcing the child's full content height as the window minimum.
// The child is still laid out at the full available size.
type minSizeContainer struct {
	widget.BaseWidget
	content fyne.CanvasObject
}

func newMinSizeContainer(content fyne.CanvasObject) *minSizeContainer {
	c := &minSizeContainer{content: content}
	c.ExtendBaseWidget(c)
	return c
}

func (m *minSizeContainer) CreateRenderer() fyne.WidgetRenderer {
	return &minSizeRenderer{content: m.content}
}

type minSizeRenderer struct {
	content fyne.CanvasObject
}

func (r *minSizeRenderer) Layout(size fyne.Size) {
	r.content.Resize(size)
	r.content.Move(fyne.NewPos(0, 0))
}

func (r *minSizeRenderer) MinSize() fyne.Size {
	// Report the content's width but a small height, so the window
	// can be resized shorter than the content's natural height.
	return fyne.NewSize(r.content.MinSize().Width, 100)
}

func (r *minSizeRenderer) Refresh() {
	r.content.Refresh()
}

func (r *minSizeRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.content}
}

func (r *minSizeRenderer) Destroy() {}

// spiceTheme wraps any Fyne theme to apply the Spice brand accent color.
type spiceTheme struct {
	fyne.Theme
}

func (t *spiceTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if name == theme.ColorNamePrimary {
		return color.NRGBA{R: 0xFF, G: 0x98, B: 0x00, A: 255} // Spice warm amber
	}
	return t.Theme.Color(name, variant)
}

// forcedVariantTheme forces a specific variant (Dark or Light) and applies the Spice accent.
type forcedVariantTheme struct {
	fyne.Theme
	variant fyne.ThemeVariant
}

func (t *forcedVariantTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	if name == theme.ColorNamePrimary {
		return color.NRGBA{R: 0xFF, G: 0x98, B: 0x00, A: 255} // Spice warm amber
	}
	return t.Theme.Color(name, t.variant)
}

// Start activates all plugins and runs the Fyne application
func (sa *SpiceApp) Start() {

	// Create the tray menu
	saInstance.CreateTrayMenu()
	if !config.IsStoreDistribution() {
		go sa.StartStartupUpdateCheck()
	}

	// Activate all plugins
	go func() {
		time.Sleep(500 * time.Millisecond) // Wait for the tray menu to be created and the ui to be ready
		for _, plugin := range sa.plugins {
			plugin.Activate()
			utilLog.Printf("Activated plugin: %s", plugin.Name())
		}
	}()

	// Run the Fyne application
	sa.Run()
}

// StartStartupUpdateCheck performs a one-time check for updates on startup,
// and registers the callback for the nightly background check.
func (sa *SpiceApp) StartStartupUpdateCheck() {
	// Register a self-contained callback that the nightly scheduler invokes.
	// It owns the full lifecycle: preference check → network call → tray update.
	if plugin := wallpaper.GetInstance(); plugin != nil {
		plugin.SetUpdateCallback(func() {
			if !sa.appConfig.GetUpdateCheckEnabled() {
				utilLog.Print("Nightly update check skipped: disabled by user.")
				return
			}
			utilLog.Print("Nightly Maintenance: Checking for application updates...")
			updateInfo, err := util.CheckForUpdates(nil)
			if err != nil {
				utilLog.Printf("Nightly Maintenance: Update check failed: %v", err)
				return
			}
			sa.updateTrayMenu(updateInfo)
		})
	}

	// Perform the initial check after a short delay
	time.Sleep(1 * time.Minute)
	if !sa.appConfig.GetUpdateCheckEnabled() {
		utilLog.Print("Startup update check disabled by user.")
		return
	}
	utilLog.Print("Checking for application updates on startup...")
	updateInfo, err := util.CheckForUpdates(nil)
	if err != nil {
		utilLog.Printf("Update check failed: %v", err)
		return
	}
	sa.updateTrayMenu(updateInfo)
}

// updateTrayMenu adds or removes the "Update Available" menu item based on check results.
func (sa *SpiceApp) updateTrayMenu(info *util.CheckForUpdatesResult) {
	fyne.Do(func() {
		// First, create a new slice of items that excludes any previous update item.
		newItems := make([]*fyne.MenuItem, 0)
		for _, item := range sa.trayMenu.Items {
			if !strings.HasPrefix(item.Label, updateMenuItemPrefix) {
				newItems = append(newItems, item)
			}
		}
		sa.trayMenu.Items = newItems

		// If no update is available, we're done.
		if !info.UpdateAvailable {
			sa.trayMenu.Refresh()
			return
		}

		// If an update IS available, create the new menu item.
		releaseURL, err := url.Parse(info.ReleaseURL)
		if err != nil {
			utilLog.Printf("Could not parse release URL for update menu item: %v", err)
			return
		}
		utilLog.Printf("New version available: %s", info.LatestVersion)

		updateItem := sa.CreateMenuItem(
			fmt.Sprintf("%s%s", updateMenuItemPrefix, info.LatestVersion),
			func() {
				if err := sa.OpenURL(releaseURL); err != nil {
					utilLog.Printf("Failed to open release URL: %v", err)
				}
			},
			"download.png",
		)

		// Find the insertion index (just before the separator above "About Spice").
		insertIndex := -1
		for i, item := range sa.trayMenu.Items {
			if item.Label == i18n.T("About Spice") {
				if i > 0 && sa.trayMenu.Items[i-1].IsSeparator {
					insertIndex = i - 1
				} else {
					insertIndex = i
				}
				break
			}
		}

		// Insert the new item at the correct position.
		if insertIndex != -1 {
			sa.trayMenu.Items = append(sa.trayMenu.Items[:insertIndex], append([]*fyne.MenuItem{updateItem}, sa.trayMenu.Items[insertIndex:]...)...)
		} else {
			// Fallback: add before the Quit button's separator if "About" isn't found.
			for i, item := range sa.trayMenu.Items {
				if item.Label == i18n.T("Quit") {
					insertIndex = i - 1
					break
				}
			}
			if insertIndex != -1 {
				sa.trayMenu.Items = append(sa.trayMenu.Items[:insertIndex], append([]*fyne.MenuItem{updateItem}, sa.trayMenu.Items[insertIndex:]...)...)
			}
		}

		// Notify user of new version available
		sa.NotifyUser(i18n.T("Spice: Update Available"), i18n.Tf("Click the tray icon to download version {{.Version}}.", map[string]any{"Version": info.LatestVersion}))

		sa.trayMenu.Refresh()
	})
}

// RefreshTrayMenu triggers a full rebuild of the tray menu structure.
// We use RebuildTrayMenu (debounced) instead of a simple Refresh() to ensure
// the system tray driver cleanly replaces the menu items, preventing duplicates
// on platforms like Windows when labels change.
func (sa *SpiceApp) RefreshTrayMenu() {
	sa.RebuildTrayMenu()
}

// RebuildTrayMenu rebuilds the tray menu list from scratch with debouncing.
func (sa *SpiceApp) RebuildTrayMenu() {
	sa.trayMu.Lock()
	defer sa.trayMu.Unlock()

	if sa.trayDebounce != nil {
		sa.trayDebounce.Stop()
	}

	sa.trayDebounce = time.AfterFunc(300*time.Millisecond, func() {
		fyne.Do(func() {
			sa.CreateTrayMenu()
		})
	})
}

// decodeGifToFrames decodes a GIF and pre-renders each frame onto a canvas
// handling disposal methods to prevent artifacts.
func decodeGifToFrames(data []byte) ([]image.Image, []time.Duration, error) {
	g, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return nil, nil, err
	}

	width, height := g.Config.Width, g.Config.Height
	numFrames := len(g.Image)
	frames := make([]image.Image, numFrames)
	delays := make([]time.Duration, numFrames)

	// Canvas to draw on (accumulates frames)
	canvasImg := image.NewRGBA(image.Rect(0, 0, width, height))

	for i, frame := range g.Image {
		// Handle disposal of previous frame
		// Disposal limits:
		// 1: Do not dispose (Keep) -> Default behavior of drawing over
		// 2: Restore to background -> Clear the area of the previous frame
		// 3: Restore to previous -> Not supported by standard image/draw efficiently, but rare.
		//    We approximated by handling 2.
		// Note: We process disposal BEFORE drawing current frame (Wait, spec says disposal happens AFTER display)
		// Actually, in a loop:
		// 1. Compose current frame onto canvas
		// 2. Save canvas as "Display Frame"
		// 3. Apply disposal to canvas for NEXT iteration

		// Standard "draw over"
		draw.Draw(canvasImg, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)

		// Save a copy for display
		// We must copy because canvasImg will change
		displayFrame := image.NewRGBA(canvasImg.Bounds())
		draw.Draw(displayFrame, canvasImg.Bounds(), canvasImg, image.Point{}, draw.Src)
		frames[i] = displayFrame

		// Calculate delay
		delay := time.Duration(g.Delay[i]) * 10 * time.Millisecond
		if delay == 0 {
			delay = 100 * time.Millisecond
		}
		delays[i] = delay

		// Handle Disposal for NEXT frame
		disposal := g.Disposal[i]
		if disposal == 2 { // Restore to background
			// Clear the *current frame's* bounds from the canvas
			draw.Draw(canvasImg, frame.Bounds(), image.Transparent, image.Point{}, draw.Src)
		}
	}

	return frames, delays, nil
}

// CreateAboutSplash creates an animated splash screen for the "About" dialog.
func (sa *SpiceApp) CreateAboutSplash() {
	defer func() {
		if r := recover(); r != nil {
			errStr := fmt.Sprintf("%v", r)
			utilLog.Printf("ERROR: PANIC in CreateAboutSplash (likely stale GLFW context): %v", errStr)
			if strings.Contains(strings.ToLower(errStr), "apiunavailable") || strings.Contains(strings.ToLower(errStr), "wgl") || strings.Contains(strings.ToLower(errStr), "opengl") {
				ShowNativeFallbackAlert(i18n.T("Graphics Error"), i18n.T("Spice requires OpenGL 2.0+ or hardware acceleration to show windows. The application will continue to run in the background. Please try rebooting your machine to restore graphics functionality."))
			}
		}
	}()
	if sa.splash != nil {
		return // Already showing
	}

	// Use the driver's native splash window creation
	drv, ok := sa.Driver().(desktop.Driver)
	if !ok {
		return // Not supported on this platform
	}
	splashWindow := drv.CreateSplashWindow()

	// Load the raw GIF data
	gifData, err := sa.assetMgr.GetRawImage("about_splash.gif")
	if err != nil {
		utilLog.Printf("Failed to load about_splash.gif: %v", err)
		return
	}

	// Pre-render frames to fix artifacts
	frames, delays, err := decodeGifToFrames(gifData)
	if err != nil {
		utilLog.Printf("Failed to decode GIF: %v", err)
		return
	}

	// Create initial image canvas
	img := canvas.NewImageFromImage(frames[0])
	img.FillMode = canvas.ImageFillContain
	img.ScaleMode = canvas.ImageScaleSmooth

	// Create Version Overlay
	versionLabel := canvas.NewText(config.AppVersion, color.RGBA{100, 50, 0, 200})
	versionLabel.TextSize = 18
	versionLabel.TextStyle = fyne.TextStyle{Bold: true}
	versionLabel.Alignment = fyne.TextAlignTrailing

	// Layout: Layer the version text over the image
	textContainer := container.NewPadded(container.NewBorder(nil, versionLabel, nil, nil))
	content := container.NewStack(img, textContainer)

	splashWindow.SetContent(content)
	splashWindow.Resize(fyne.NewSize(300, 300)) // Scaled to 300px (Original)
	splashWindow.CenterOnScreen()
	sa.splash = splashWindow
	sa.splash.Show()

	// Start manual animation loop with pre-rendered frames
	go func() {
		defer func() {
			fyne.Do(func() {
				if sa.splash != nil {
					sa.splash.Hide()
					sa.splash = nil
				}
				sa.os.TransformToBackground()
			})
		}()

		// One full cycle
		for i, frame := range frames {
			// Check if closed externally
			if sa.splash == nil {
				return
			}

			// Update frame
			currentFrame := frame
			fyne.Do(func() {
				img.Image = currentFrame
				img.Refresh()
			})

			time.Sleep(delays[i])
		}
	}()
}

// Anchor popup shared palette — theme-aware highlight colors.
// Active amber is consistent across themes; inactive/text adapt for readability.
func anchorIsDarkTheme() bool {
	settings := fyne.CurrentApp().Settings()
	// Check if Spice has forced a specific theme variant
	if ft, ok := settings.Theme().(*forcedVariantTheme); ok {
		return ft.variant == theme.VariantDark
	}
	// Fall back to system theme
	return settings.ThemeVariant() == theme.VariantDark
}

func anchorCellInactiveColor() color.NRGBA {
	if anchorIsDarkTheme() {
		return color.NRGBA{R: 50, G: 50, B: 58, A: 255} // dark slate
	}
	return color.NRGBA{R: 220, G: 220, B: 225, A: 255} // light gray
}

func anchorCellActiveColor() color.NRGBA {
	return color.NRGBA{R: 0xFF, G: 0x98, B: 0x00, A: 255} // warm amber (both themes)
}

func anchorTextActiveColor() color.NRGBA {
	return color.NRGBA{R: 30, G: 30, B: 35, A: 255} // dark text on amber (both themes)
}

func anchorTextInactiveColor() color.NRGBA {
	if anchorIsDarkTheme() {
		return color.NRGBA{R: 200, G: 200, B: 210, A: 255} // light text on dark
	}
	return color.NRGBA{R: 50, G: 50, B: 58, A: 255} // dark text on light
}

func anchorHeaderColor() color.NRGBA {
	if anchorIsDarkTheme() {
		return color.NRGBA{R: 220, G: 220, B: 225, A: 255}
	}
	return color.NRGBA{R: 40, G: 40, B: 45, A: 255}
}

func anchorDescColor() color.NRGBA {
	if anchorIsDarkTheme() {
		return color.NRGBA{R: 120, G: 120, B: 130, A: 255}
	}
	return color.NRGBA{R: 100, G: 100, B: 110, A: 255}
}

func anchorMonitorColor() color.NRGBA {
	if anchorIsDarkTheme() {
		return color.NRGBA{R: 150, G: 150, B: 160, A: 255}
	}
	return color.NRGBA{R: 70, G: 70, B: 80, A: 255}
}

// anchorCell is a tappable canvas-based grid cell for the anchor popup.
// It renders a colored rectangle background with a centered pixel art icon.
type anchorCell struct {
	widget.BaseWidget
	active   bool
	icon     image.Image
	onTapped func()
	bg       *canvas.Rectangle
	img      *canvas.Image
}

func newAnchorCell(icon image.Image, active bool, onTapped func()) *anchorCell {
	c := &anchorCell{
		icon:     icon,
		active:   active,
		onTapped: onTapped,
	}
	c.ExtendBaseWidget(c)
	return c
}

func (c *anchorCell) Tapped(_ *fyne.PointEvent) {
	if c.onTapped != nil {
		c.onTapped()
	}
}

func (c *anchorCell) CreateRenderer() fyne.WidgetRenderer {
	bgColor := anchorCellInactiveColor()
	if c.active {
		bgColor = anchorCellActiveColor()
	}

	c.bg = canvas.NewRectangle(bgColor)
	c.bg.CornerRadius = 6

	c.img = canvas.NewImageFromImage(c.icon)
	c.img.FillMode = canvas.ImageFillContain
	c.img.ScaleMode = canvas.ImageScalePixels // Preserve pixel art crispness

	return &anchorCellRenderer{
		cell:    c,
		objects: []fyne.CanvasObject{c.bg, c.img},
	}
}

type anchorCellRenderer struct {
	cell    *anchorCell
	objects []fyne.CanvasObject
}

func (r *anchorCellRenderer) Layout(size fyne.Size) {
	r.cell.bg.Resize(size)
	r.cell.bg.Move(fyne.NewPos(0, 0))

	// Center the icon with padding inside the cell
	pad := float32(8)
	iconSize := fyne.NewSize(size.Width-pad*2, size.Height-pad*2)
	r.cell.img.Resize(iconSize)
	r.cell.img.Move(fyne.NewPos(pad, pad))
}

func (r *anchorCellRenderer) MinSize() fyne.Size {
	return fyne.NewSize(64, 56)
}

func (r *anchorCellRenderer) Refresh() {
	bgColor := anchorCellInactiveColor()
	if r.cell.active {
		bgColor = anchorCellActiveColor()
	}
	r.cell.bg.FillColor = bgColor
	r.cell.bg.Refresh()
	r.cell.img.Refresh()
}

func (r *anchorCellRenderer) Destroy() {}

func (r *anchorCellRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

// tappableContainer is a simple tappable wrapper for any content.
type tappableContainer struct {
	widget.BaseWidget
	onTapped func()
	content  fyne.CanvasObject
}

func newTappableContainer(onTapped func(), content fyne.CanvasObject) *tappableContainer {
	t := &tappableContainer{onTapped: onTapped, content: content}
	t.ExtendBaseWidget(t)
	return t
}

func (t *tappableContainer) Tapped(_ *fyne.PointEvent) {
	if t.onTapped != nil {
		t.onTapped()
	}
}

func (t *tappableContainer) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.content)
}

// loadAnchorIcons derives all 9 directional icons from the existing next.png asset
// via imaging transforms. For center, a clean crosshair is drawn programmatically.
func (sa *SpiceApp) loadAnchorIcons(size int) [9]image.Image {
	var icons [9]image.Image

	// Load the existing next.png icon (glossy red arrow pointing →)
	res, err := sa.assetMgr.GetIcon("next.png")
	if err != nil {
		utilLog.Printf("[WARN] Failed to load next.png for anchor icons: %v", err)
		return icons
	}

	baseImg, _, err := image.Decode(bytes.NewReader(res.Content()))
	if err != nil {
		utilLog.Printf("[WARN] Failed to decode next.png: %v", err)
		return icons
	}

	// Resize to target size for crisp display
	arrow := imaging.Resize(baseImg, size, size, imaging.Lanczos)

	// Grid layout:  ↖(0)  ↑(1)  ↗(2)
	//               ←(3)  ●(4)  →(5)
	//               ↙(6)  ↓(7)  ↘(8)

	// Cardinal directions from the → base
	icons[5] = arrow                    // → = original
	icons[3] = imaging.FlipH(arrow)     // ← = flip horizontal
	icons[1] = imaging.Rotate90(arrow)  // ↑ = rotate 90° CCW
	icons[7] = imaging.Rotate270(arrow) // ↓ = rotate 90° CW

	// Diagonal directions: rotate the base arrow 45°
	diag := imaging.Rotate(arrow, 45, color.Transparent)
	// Rotate produces a larger canvas; crop back to size
	diag = imaging.CropCenter(diag, size, size)
	icons[2] = diag                               // ↗ = 45° CCW
	icons[0] = imaging.FlipH(diag)                // ↖ = flip horizontal
	icons[8] = imaging.FlipV(diag)                // ↘ = flip vertical
	icons[6] = imaging.FlipH(imaging.FlipV(diag)) // ↙ = flip both

	// Center: draw a clean crosshair programmatically
	icons[4] = drawCrosshair(size, color.NRGBA{R: 220, G: 60, B: 40, A: 255})

	return icons
}

// drawCrosshair renders a simple crosshair/target icon with real alpha transparency.
func drawCrosshair(size int, col color.NRGBA) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	cx, cy := size/2, size/2
	radius := size * 3 / 8
	thickness := max(size/12, 2)
	dotRadius := max(size/10, 2)

	// Draw circle outline
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := x - cx
			dy := y - cy
			dist := dx*dx + dy*dy
			outer := radius * radius
			inner := (radius - thickness) * (radius - thickness)
			if dist <= outer && dist >= inner {
				img.SetNRGBA(x, y, col)
			}
		}
	}

	// Draw center dot
	for y := cy - dotRadius; y <= cy+dotRadius; y++ {
		for x := cx - dotRadius; x <= cx+dotRadius; x++ {
			dx := x - cx
			dy := y - cy
			if dx*dx+dy*dy <= dotRadius*dotRadius {
				img.SetNRGBA(x, y, col)
			}
		}
	}

	return img
}

// ShowAnchorPopup displays the 3×3 crop anchor selection popup.
// This is the ADAPTER implementation — it owns window lifecycle and OpenGL recovery.
// The window stays open after selection, showing a processing overlay and redrawing
// with the updated active anchor when reprocessing completes.
func (sa *SpiceApp) ShowAnchorPopup(monitorID int, currentAnchor provider.CropAnchor, labels [9]string, values [9]provider.CropAnchor, onSelect func(anchor provider.CropAnchor, onDone func())) {
	// Guard: skip if OpenGL is unavailable.
	if !sysinfo.CanCreateWindows() {
		utilLog.Println("OpenGL unavailable — anchor popup cannot be created")
		ShowNativeFallbackAlert(i18n.T("Graphics Error"), i18n.T("Spice requires OpenGL 2.0+ or hardware acceleration to show windows. The application will continue to run in the background. Please try rebooting your machine to restore graphics functionality."))
		return
	}

	sa.os.TransformToForeground()

	title := fmt.Sprintf("%s — %s %d", i18n.T("Crop Anchor"), i18n.T("Display"), monitorID+1)

	// Create window with OpenGL panic recovery (matches CreatePreferencesWindow pattern)
	var w fyne.Window
	func() {
		defer func() {
			if r := recover(); r != nil {
				errStr := fmt.Sprintf("%v", r)
				utilLog.Printf("Recovered from anchor popup window creation panic: %v", errStr)
				if strings.Contains(strings.ToLower(errStr), "apiunavailable") || strings.Contains(strings.ToLower(errStr), "wgl") || strings.Contains(strings.ToLower(errStr), "opengl") {
					ShowNativeFallbackAlert(i18n.T("Graphics Error"), i18n.T("Spice requires OpenGL 2.0+ or hardware acceleration to show windows. The application will continue to run in the background. Please try rebooting your machine to restore graphics functionality."))
				}
			}
		}()
		w = sa.NewWindow(title)
	}()

	if w == nil {
		utilLog.Println("Anchor popup window creation failed (returned nil or recovered)")
		return
	}

	w.SetFixedSize(true)

	// -- Load icons from existing next.png asset --
	icons := sa.loadAnchorIcons(48)

	// -- Shared state --
	activeAnchor := currentAnchor
	processing := false

	// -- Mutable content container (swapped between grid and overlay) --
	contentStack := container.NewStack()

	// -- Build the interactive grid content --
	var buildContent func()
	buildContent = func() {
		// -- Header --
		headerText := canvas.NewText(i18n.T("Crop Anchor"), anchorHeaderColor())
		headerText.TextSize = 14
		headerText.TextStyle = fyne.TextStyle{Bold: true}
		headerText.Alignment = fyne.TextAlignCenter

		// -- Description label --
		descText := canvas.NewText(i18n.T("Anchor Description"), anchorDescColor())
		descText.TextSize = 10
		descText.Alignment = fyne.TextAlignCenter

		// -- Monitor label --
		monitorText := canvas.NewText(
			fmt.Sprintf("%s %d", i18n.T("Display"), monitorID+1),
			anchorMonitorColor(),
		)
		monitorText.TextSize = 11
		monitorText.TextStyle = fyne.TextStyle{Bold: true}
		monitorText.Alignment = fyne.TextAlignCenter

		// -- 3×3 grid of icon cells --
		gridContainer := container.NewGridWithColumns(3)
		for i := 0; i < 9; i++ {
			idx := i
			anchor := values[idx]
			isActive := anchor == activeAnchor

			cell := newAnchorCell(icons[idx], isActive, func() {
				if processing {
					return // Ignore clicks while processing
				}
				processing = true

				// Show processing overlay
				fyne.Do(func() {
					overlayText := canvas.NewText(i18n.T("Applying..."), color.NRGBA{R: 240, G: 240, B: 245, A: 255})
					overlayText.TextSize = 14
					overlayText.TextStyle = fyne.TextStyle{Bold: true}
					overlayText.Alignment = fyne.TextAlignCenter

					// Semi-transparent dark overlay
					overlayBg := canvas.NewRectangle(color.NRGBA{R: 30, G: 30, B: 35, A: 200})

					overlay := container.NewStack(
						overlayBg,
						container.NewCenter(overlayText),
					)
					contentStack.Objects = []fyne.CanvasObject{overlay}
					contentStack.Refresh()
				})

				// Dispatch anchor change with done callback
				onSelect(anchor, func() {
					activeAnchor = anchor
					processing = false
					// Redraw grid on UI thread with new active state
					fyne.Do(func() {
						buildContent()
					})
				})
			})
			gridContainer.Add(cell)
		}

		// -- Auto button (custom tappable — uses exact same highlight color as grid cells) --
		autoActive := activeAnchor == provider.AnchorAuto || activeAnchor == 0
		autoBgColor := anchorCellInactiveColor()
		autoTextColor := anchorTextInactiveColor()
		if autoActive {
			autoBgColor = anchorCellActiveColor()
			autoTextColor = anchorTextActiveColor()
		}

		autoBg := canvas.NewRectangle(autoBgColor)
		autoBg.CornerRadius = 6
		autoLabel := canvas.NewText(i18n.T("Auto"), autoTextColor)
		autoLabel.TextSize = 14
		autoLabel.TextStyle = fyne.TextStyle{Bold: true}
		autoLabel.Alignment = fyne.TextAlignCenter

		autoBtn := container.NewStack(autoBg, container.NewPadded(container.NewCenter(autoLabel)))

		autoTap := newTappableContainer(func() {
			if processing {
				return
			}
			processing = true

			fyne.Do(func() {
				overlayText := canvas.NewText(i18n.T("Applying..."), color.NRGBA{R: 240, G: 240, B: 245, A: 255})
				overlayText.TextSize = 14
				overlayText.TextStyle = fyne.TextStyle{Bold: true}
				overlayText.Alignment = fyne.TextAlignCenter

				overlayBg := canvas.NewRectangle(color.NRGBA{R: 30, G: 30, B: 35, A: 200})

				overlay := container.NewStack(
					overlayBg,
					container.NewCenter(overlayText),
				)
				contentStack.Objects = []fyne.CanvasObject{overlay}
				contentStack.Refresh()
			})

			onSelect(provider.AnchorAuto, func() {
				activeAnchor = provider.AnchorAuto
				processing = false
				fyne.Do(func() {
					buildContent()
				})
			})
		}, autoBtn)

		// -- Separator line --
		separator := canvas.NewRectangle(color.NRGBA{R: 70, G: 70, B: 80, A: 255})
		separator.SetMinSize(fyne.NewSize(0, 1))

		// -- Close button (same styling as Auto for consistency) --
		closeBg := canvas.NewRectangle(anchorCellInactiveColor())
		closeBg.CornerRadius = 6
		closeLabel := canvas.NewText(i18n.T("Close"), anchorTextInactiveColor())
		closeLabel.TextSize = 14
		closeLabel.TextStyle = fyne.TextStyle{Bold: true}
		closeLabel.Alignment = fyne.TextAlignCenter

		closeBtnContent := container.NewStack(closeBg, container.NewPadded(container.NewCenter(closeLabel)))
		closeTap := newTappableContainer(func() {
			w.Close()
		}, closeBtnContent)

		// -- Assemble layout --
		content := container.NewVBox(
			container.NewCenter(headerText),
			container.NewCenter(descText),
			layout.NewSpacer(),
			container.NewCenter(monitorText),
			gridContainer,
			layout.NewSpacer(),
			separator,
			autoTap,
			closeTap,
		)

		contentStack.Objects = []fyne.CanvasObject{content}
		contentStack.Refresh()
	}

	// Initial build
	buildContent()

	w.SetContent(container.NewPadded(contentStack))

	// Size to content — no dead space
	minSize := contentStack.MinSize()
	w.Resize(fyne.NewSize(minSize.Width+32, minSize.Height+24))
	w.CenterOnScreen()

	w.SetOnClosed(func() {
		sa.os.TransformToBackground()
	})

	w.Show()
}
