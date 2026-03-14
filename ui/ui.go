package ui

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"net/url"
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
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"github.com/disintegration/imaging"
	"github.com/dixieflatline76/Spice/v2/asset"
	"github.com/dixieflatline76/Spice/v2/config"
	"github.com/dixieflatline76/Spice/v2/pkg/api"
	"github.com/dixieflatline76/Spice/v2/pkg/hotkey"
	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/pkg/sysinfo"
	"github.com/dixieflatline76/Spice/v2/pkg/ui"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
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
	sa.CreatePreferencesWindow(tab)
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
		// Initialize the wallpaper service for right OS
		currentOS := getOS()

		a := app.NewWithID(config.AppName)
		if _, ok := a.(desktop.App); ok {

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
				saInstance.Settings().SetTheme(theme.DefaultTheme())
			}

			saInstance.verifyEULA()

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
		} else {
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
	mi := fyne.NewMenuItem(label, action)
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
		newChecked := !mi.Checked
		if newChecked {
			mi.Label = fmt.Sprintf("%s ✔", label)
		} else {
			mi.Label = label
		}
		mi.Checked = newChecked
		action(newChecked)
		sa.trayMenu.Refresh()
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
	if sa.splash == nil {
		// Create a splash screen with the application icon
		drv, ok := sa.Driver().(desktop.Driver)
		if !ok {
			utilLog.Println("Splash screen not supported")
			return // Splash screen not supported
		}

		splashWindow := drv.CreateSplashWindow()

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
	sa.os.TransformToForeground()

	// If window already exists, just show it and switch tab if requested
	if sa.prefsWindow != nil {
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
	prefsWindow := sa.NewWindow(fmt.Sprintf("%s %s", config.AppName, i18n.T("Preferences")))
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

	// We want a portrait/square look, so we tie the height to the width.
	// Target slightly taller than wide (1.0) to shave off those final few vertical pixels.
	targetHeight := targetWidth * 1.0

	// Safety check: ensure the resulting height actually fits the vertical content
	// Adding minimal vertical padding provides some room for scrolling.
	if targetHeight < minSize.Height*1.05 {
		targetHeight = minSize.Height * 1.05
	}

	// Prevent the window from expanding off the top/bottom on smaller physical screens.
	// Since GetScreenDimensions() currently returns physical pixels on Windows,
	// we translate that explicitly into Fyne's logical bounds.
	_, physHeight, err := sysinfo.GetScreenDimensions()
	if err == nil {
		logicalScreenHeight := float32(physHeight) / osScale
		maxSafeHeight := logicalScreenHeight * 0.90 // Keep 10% breathing room for the taskbar/titlebar

		if targetHeight > maxSafeHeight {
			targetHeight = maxSafeHeight

			// Maintain aspect ratio if we compress the height hard against screen edge
			if targetWidth > targetHeight {
				targetWidth = targetHeight
			}
		}
	}

	prefsWindow.Resize(fyne.NewSize(targetWidth, targetHeight))

	prefsWindow.CenterOnScreen()
	prefsWindow.SetOnClosed(func() {
		sa.os.TransformToBackground()
		sa.prefsWindow = nil // Clear reference on close
		sa.prefsTabs = nil   // Clear tabs reference
	})

	prefsWindow.Show()
}

// RebuildPreferencesContent rebuilds the content of the preferences window.
// This allows refreshing the UI (e.g. to show "Add Query" dialogs) even if the window is already open.
func (sa *SpiceApp) RebuildPreferencesContent(initialTab string) {
	if sa.prefsWindow == nil {
		return
	}

	sm := NewSettingsManager(sa.prefsWindow)

	// --- General Tab ---
	generalContainer := container.NewVBox()
	generalContainer.Add(sm.CreateSectionTitleLabel(i18n.T("General Application Settings")))

	// Enable System Notifications
	var notificationsConfig setting.BoolConfig
	notificationsConfig = setting.BoolConfig{
		Name:         "enableNotifications",
		InitialValue: sa.appConfig.GetAppNotificationsEnabled(),
		Label:        sm.CreateSettingTitleLabel(i18n.T("Enable System Notifications:")),
		HelpContent:  sm.CreateSettingDescriptionLabel(i18n.T("Enable or disable system notifications from Spice.")),
		ApplyFunc: func(b bool) {
			sa.appConfig.SetAppNotificationsEnabled(b)
			notificationsConfig.InitialValue = b
		},
	}
	sm.CreateBoolSetting(&notificationsConfig, generalContainer)

	// Enable New Version Check
	var updateCheckConfig setting.BoolConfig
	updateCheckConfig = setting.BoolConfig{
		Name:         "enableUpdateCheck",
		InitialValue: sa.appConfig.GetUpdateCheckEnabled(),
		Label:        sm.CreateSettingTitleLabel(i18n.T("Enable New Version Check:")),
		HelpContent:  sm.CreateSettingDescriptionLabel(i18n.T("Automatically check for new versions of Spice on startup.")),
		ApplyFunc: func(b bool) {
			sa.appConfig.SetUpdateCheckEnabled(b)
			updateCheckConfig.InitialValue = b
		},
	}
	sm.CreateBoolSetting(&updateCheckConfig, generalContainer)

	// Enable Global Shortcuts
	shortcutConfig := setting.BoolConfig{
		Name:         "enableShortcuts",
		InitialValue: !wallpaper.GetInstance().GetShortcutsDisabled(),
		Label:        sm.CreateSettingTitleLabel(i18n.T("Enable global shortcuts:")),
		HelpContent:  sm.CreateSettingDescriptionLabel(i18n.T("Use keyboard shortcuts to control wallpapers. Disable if they conflict with other apps.")),
		ApplyFunc: func(val bool) {
			wallpaper.GetInstance().SetShortcutsDisabled(!val)
			hotkey.StartListeners(GetPluginManager())
		},
	}
	sm.CreateBoolSetting(&shortcutConfig, generalContainer)

	targetedInner := container.NewVBox()

	targetedShortcutConfig := setting.BoolConfig{
		Name:         "enableTargetedShortcuts",
		InitialValue: !wallpaper.GetInstance().GetTargetedShortcutsDisabled(),
		Label:        sm.CreateSettingTitleLabel(i18n.T("Enable Display Specific Shortcuts (Alt + Arrow + 1-9):")),
		HelpContent:  sm.CreateSettingDescriptionLabel(i18n.T("Disable this if Alt+Arrow conflicts with your browser or other apps.")),
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
	}
	sm.CreateBoolSetting(&targetedShortcutConfig, targetedInner)

	indentation := widget.NewLabel("      ")

	shortcutsGuideURL, _ := url.Parse("https://github.com/dixieflatline76/Spice/blob/main/docs/user_guide.md#keyboard-shortcuts")
	shortcutsLink := widget.NewHyperlink(i18n.T("View all shortcuts →"), shortcutsGuideURL)
	targetedAndLinkContainer := container.NewVBox(
		targetedInner,
		container.NewHBox(shortcutsLink),
	)

	indentedWrapper := container.NewBorder(nil, nil, indentation, nil, targetedAndLinkContainer)
	generalContainer.Add(indentedWrapper)

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

	themeConfig := setting.SelectConfig{
		Name:         "theme",
		Options:      setting.StringOptions(util.StringsToStringers(themeOptions)),
		InitialValue: initialThemeIndex,
		Label:        sm.CreateSettingTitleLabel(i18n.T("Theme:")),
		HelpContent:  sm.CreateSettingDescriptionLabel(i18n.T("Select the application theme.")),
	}
	themeConfig.ApplyFunc = func(val interface{}) {
		selectedIndex := val.(int)
		if selectedIndex < 0 || selectedIndex >= len(themeOptions) {
			return // Safety check
		}
		selectedTheme := themeOptions[selectedIndex]
		sa.appConfig.SetTheme(selectedTheme)
		themeConfig.InitialValue = selectedIndex

		// Apply Theme
		switch selectedTheme {
		case "Dark":
			sa.Settings().SetTheme(&forcedVariantTheme{Theme: theme.DefaultTheme(), variant: theme.VariantDark})
		case "Light":
			sa.Settings().SetTheme(&forcedVariantTheme{Theme: theme.DefaultTheme(), variant: theme.VariantLight})
		default:
			sa.Settings().SetTheme(theme.DefaultTheme())
		}
	}
	sm.CreateSelectSetting(&themeConfig, generalContainer)

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

	langConfig := setting.SelectConfig{
		Name:         "language",
		Options:      setting.StringOptions(util.StringsToStringers(langDisplayOptions)),
		InitialValue: initialLangIndex,
		Label:        sm.CreateSettingTitleLabel(i18n.T("Language:")),
		HelpContent:  sm.CreateSettingDescriptionLabel(i18n.T("Select the application language. Restart may be required for full effect.")),
		NeedsRefresh: true,
	}
	langConfig.ApplyFunc = func(val interface{}) {
		selectedIndex := val.(int)
		if selectedIndex < 0 || selectedIndex >= len(langOptions) {
			return
		}
		selectedLang := langOptions[selectedIndex]
		sa.appConfig.SetLanguage(selectedLang)
		i18n.SetLanguage(selectedLang)
		langConfig.InitialValue = selectedIndex

		// Broadcast to extensions
		if srv := api.GetServer(); srv != nil {
			if err := srv.BroadcastLanguage(i18n.GetLanguage()); err != nil {
				utilLog.Printf("Failed to broadcast language change to extensions: %v", err)
			}
		}
	}
	sm.CreateSelectSetting(&langConfig, generalContainer)

	// Enable Debug Logging
	var debugLogConfig setting.BoolConfig
	debugLogConfig = setting.BoolConfig{
		Name:         "enableDebugLogging",
		InitialValue: sa.appConfig.GetDebugLoggingEnabled(),
		Label:        sm.CreateSettingTitleLabel(i18n.T("Enable Debug Logging:")),
		HelpContent:  sm.CreateSettingDescriptionLabel(i18n.T("Write verbose debug entries to the log file. Useful for troubleshooting.")),
		ApplyFunc: func(b bool) {
			sa.appConfig.SetDebugLoggingEnabled(b)
			utilLog.SetDebugEnabled(b)
			debugLogConfig.InitialValue = b
		},
	}
	sm.CreateBoolSetting(&debugLogConfig, generalContainer)

	// Initialize/Reset tab mapping
	sa.tabItems = make(map[string]*container.TabItem)

	generalTabItem := container.NewTabItem(i18n.T("App"), container.NewVScroll(generalContainer))
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

	// Register refresh functions for global UI elements (e.g. language change)
	sm.RegisterRefreshFunc(func() {
		// Find current ID by searching our map
		activeID := "App"
		selected := tabs.Selected()
		for id, item := range sa.tabItems {
			if item == selected {
				activeID = id
				break
			}
		}
		sa.RebuildPreferencesContent(activeID)
		sa.RebuildTrayMenu()
	})

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

	sa.prefsWindow.SetContent(prefsWindowLayout)
	sm.GetCheckAndEnableApplyFunc()() // Trigger initial UI dependency refresh
}

// forcedVariantTheme is a theme that forces a specific variant (Dark or Light)
type forcedVariantTheme struct {
	fyne.Theme
	variant fyne.ThemeVariant
}

func (t *forcedVariantTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	return t.Theme.Color(name, t.variant)
}

// verifyEULA checks if the End User License Agreement has been accepted. If not, it will show the EULA and prompt the user to accept it.
// If the user declines, the application will quit.
// If the EULA has been accepted, the application will proceed to setup.
func (sa *SpiceApp) verifyEULA() {
	// Check if the EULA has been accepted
	if util.HasAcceptedEULA(sa.Preferences()) {
		sa.CreateSplashScreen(startupSplashTime) // Show the splash screen if the EULA has been accepted
	} else {
		sa.displayEULAAcceptance() // Show the EULA if it hasn't been accepted
	}
}

// displayEULAAcceptance displays the End User License Agreement and prompts the user
// to accept it. If the user declines, the application will quit.
func (sa *SpiceApp) displayEULAAcceptance() {
	eulaText, err := sa.assetMgr.GetText("eula.txt")
	if err != nil {
		utilLog.Fatalf("Error loading EULA: %v", err)
	}

	// Create a new window for the EULA
	sa.os.TransformToForeground() // Ensure the app is in the foreground before showing the EULA
	eulaWindow := sa.NewWindow(i18n.T("Spice EULA"))
	eulaWindow.SetOnClosed(sa.os.TransformToBackground) // Set the close action to transform to background
	eulaWindow.Resize(fyne.NewSize(800, 600))
	eulaWindow.CenterOnScreen()
	eulaWindow.SetCloseIntercept(func() {
		// Prevent the window from being closed
	})

	// Create a scrollable text widget for the EULA content
	eulaWdgt := widget.NewRichTextWithText(eulaText)
	eulaWdgt.Wrapping = fyne.TextWrapWord
	eulaScroll := container.NewVScroll(eulaWdgt)
	eulaDialog := dialog.NewCustomConfirm(i18n.T("To continue using Spice, please review and accept the End User License Agreement."), i18n.T("Accept"), i18n.T("Decline"), eulaScroll, func(accepted bool) {
		if accepted {
			// Mark the EULA as accepted
			util.MarkEULAAccepted(sa.Preferences())
			eulaWindow.Close()
			sa.CreateSplashScreen(startupSplashTime) // Show the splash screen after user accepts the EULA
		} else {
			// Stop the service before quitting the application
			sa.Quit()
		}
	}, eulaWindow)

	eulaDialog.Resize(fyne.NewSize(795, 595)) // Resize the dialog to fit the window
	eulaDialog.Show()
	eulaWindow.Show()
}

// Start activates all plugins and runs the Fyne application
func (sa *SpiceApp) Start() {

	// Create the tray menu
	saInstance.CreateTrayMenu()
	go sa.StartPeriodicUpdateCheck()

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

// StartPeriodicUpdateCheck starts a goroutine to check for updates on startup and then periodically.
func (sa *SpiceApp) StartPeriodicUpdateCheck() {
	utilLog.Print("Starting periodic application update checker...")

	performCheck := func() {
		if !sa.appConfig.GetUpdateCheckEnabled() {
			utilLog.Print("Update check disabled by user.")
			return
		}
		utilLog.Print("Checking for application updates...")
		updateInfo, err := util.CheckForUpdates(nil)
		if err != nil {
			utilLog.Printf("Update check failed: %v", err)
			return
		}
		sa.updateTrayMenu(updateInfo)
	}

	time.Sleep(1 * time.Minute)
	performCheck()

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		performCheck()
	}
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
			utilLog.Printf("ERROR: PANIC in CreateAboutSplash (likely stale GLFW context): %v", r)
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
