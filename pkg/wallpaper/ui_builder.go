package wallpaper

import (
	"fmt"
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/dixieflatline76/Spice/pkg/ui/setting"
	utilLog "github.com/dixieflatline76/Spice/util/log"
)

// PrefsPanelBuilder constructs the preferences UI.
type PrefsPanelBuilder struct {
	plugin *Plugin
	sm     setting.SettingsManager

	// State for mutual exclusion logic
	faceCropCheck  *widget.Check
	faceBoostCheck *widget.Check
}

func NewPrefsPanelBuilder(p *Plugin, sm setting.SettingsManager) *PrefsPanelBuilder {
	return &PrefsPanelBuilder{
		plugin: p,
		sm:     sm,
	}
}

// BuildGeneralTab creates the General settings tab content.
func (b *PrefsPanelBuilder) BuildGeneralTab() fyne.CanvasObject {
	generalContainer := container.NewVBox()

	// 1. Wallpaper Cycle & Cache
	b.addFrequencySetting(generalContainer)
	b.addCacheSizeSetting(generalContainer)

	// 2. Smart Fit Section
	b.addSmartFitSection(generalContainer)

	// 3. Stagger & OnStart
	b.addToggleSettings(generalContainer)

	// 4. Nightly Refresh
	b.addNightlyRefreshSetting(generalContainer)

	// 5. Action Buttons (Sync, Clear, Reset)
	b.addActionButtons(generalContainer)

	return container.NewVScroll(generalContainer)
}

func (b *PrefsPanelBuilder) addFrequencySetting(c *fyne.Container) {
	config := setting.SelectConfig{
		Name:         "changeFrequency",
		Options:      setting.StringOptions(GetFrequencies()),
		InitialValue: int(b.plugin.cfg.GetWallpaperChangeFrequency()),
		Label:        b.sm.CreateSettingTitleLabel("Wallpaper Change Frequency:"),
		HelpContent:  b.sm.CreateSettingDescriptionLabel("Set how often the wallpaper changes. Set to \"Never\" to disable wallpaper changes."),
	}
	config.ApplyFunc = func(val interface{}) {
		freq := Frequency(val.(int))
		b.plugin.cfg.SetWallpaperChangeFrequency(freq)
		b.plugin.ChangeWallpaperFrequency(freq)
		config.InitialValue = int(freq)
	}
	b.sm.CreateSelectSetting(&config, c)
}

func (b *PrefsPanelBuilder) addCacheSizeSetting(c *fyne.Container) {
	config := setting.SelectConfig{
		Name:         "cacheSize",
		Options:      setting.StringOptions(GetCacheSizes()),
		InitialValue: int(b.plugin.cfg.GetCacheSize()),
		Label:        b.sm.CreateSettingTitleLabel("Cache Size:"),
		HelpContent:  b.sm.CreateSettingDescriptionLabel("Set how many images to cache for faster startup and less network usage. Set to \"None\" to disable caching."),
	}
	config.ApplyFunc = func(val interface{}) {
		size := CacheSize(val.(int))
		b.plugin.cfg.SetCacheSize(size)
		config.InitialValue = int(size)
	}
	b.sm.CreateSelectSetting(&config, c)
}

func (b *PrefsPanelBuilder) addSmartFitSection(c *fyne.Container) {
	// Smart Fit Mode
	config := setting.SelectConfig{
		Name:         "smartFitMode",
		Options:      GetSmartFitModes(),
		InitialValue: int(b.plugin.cfg.GetSmartFitMode()),
		Label:        b.sm.CreateSettingTitleLabel("Smart Fit Mode:"),
		HelpContent:  b.sm.CreateSettingDescriptionLabel("Control how images are fitted to your screen:\n- Disabled: Original image.\n- Quality: Rejects images with mismatched aspect ratio.\n- Flexibility: Allows high-res images to crop aggressively."),
	}
	config.ApplyFunc = func(val interface{}) {
		mode := SmartFitMode(val.(int))
		b.plugin.cfg.SetSmartFitMode(mode)
		config.InitialValue = int(mode)
	}
	config.NeedsRefresh = true

	config.OnChanged = func(s string, val interface{}) {
		mode := SmartFitMode(val.(int))
		b.updateFaceOptionsState(mode)
	}
	b.sm.CreateSelectSetting(&config, c)

	// Face Options (Crop / Boost)
	b.addFaceOptions(c)

	// Initialize state based on current mode
	b.updateFaceOptionsState(b.plugin.cfg.GetSmartFitMode())
}

func (b *PrefsPanelBuilder) addFaceOptions(c *fyne.Container) {
	var cropConfig, boostConfig setting.BoolConfig

	// Face Crop
	cropConfig = setting.BoolConfig{
		Name:         "faceCrop",
		InitialValue: b.plugin.cfg.GetFaceCropEnabled(),
		Label:        b.sm.CreateSettingTitleLabel("Enable Face Crop:"),
		HelpContent:  b.sm.CreateSettingDescriptionLabel("Aggressively crops the image to center on the largest face found. Good for portraits."),
		ApplyFunc: func(val bool) {
			b.plugin.cfg.SetFaceCropEnabled(val)
			if val {
				b.plugin.cfg.SetFaceBoostEnabled(false)
				boostConfig.InitialValue = false
			}
			cropConfig.InitialValue = val
		},
		NeedsRefresh: true,
	}

	// Face Boost
	boostConfig = setting.BoolConfig{
		Name:         "faceBoost",
		InitialValue: b.plugin.cfg.GetFaceBoostEnabled(),
		Label:        b.sm.CreateSettingTitleLabel("Enable Face Boost:"),
		HelpContent:  b.sm.CreateSettingDescriptionLabel("Uses face detection to hint the smart cropper. Keeps faces in frame but balances with other image details."),
		ApplyFunc: func(val bool) {
			b.plugin.cfg.SetFaceBoostEnabled(val)
			if val {
				b.plugin.cfg.SetFaceCropEnabled(false)
				cropConfig.InitialValue = false
			}
			boostConfig.InitialValue = val
		},
		NeedsRefresh: true,
	}

	subSettingsContainer := container.NewVBox()
	b.faceCropCheck = b.sm.CreateBoolSetting(&cropConfig, subSettingsContainer)
	b.faceBoostCheck = b.sm.CreateBoolSetting(&boostConfig, subSettingsContainer)

	indentation := widget.NewLabel("      ")
	indentedWrapper := container.NewBorder(nil, nil, indentation, nil, subSettingsContainer)
	c.Add(indentedWrapper)

	// Mutual Exclusion Wiring
	b.wireFaceOptionsMutex()
}

func (b *PrefsPanelBuilder) wireFaceOptionsMutex() {
	originalCropHandler := b.faceCropCheck.OnChanged
	originalBoostHandler := b.faceBoostCheck.OnChanged

	b.faceCropCheck.OnChanged = func(val bool) {
		utilLog.Debugf("UI: Face Crop Toggled: %v", val)
		if val {
			b.faceBoostCheck.SetChecked(false)
		}
		if originalCropHandler != nil {
			originalCropHandler(val)
		}
	}

	b.faceBoostCheck.OnChanged = func(val bool) {
		utilLog.Debugf("UI: Face Boost Toggled: %v", val)
		if val {
			b.faceCropCheck.SetChecked(false)
		}
		if originalBoostHandler != nil {
			originalBoostHandler(val)
		}
	}
}

func (b *PrefsPanelBuilder) updateFaceOptionsState(mode SmartFitMode) {
	if b.faceCropCheck == nil || b.faceBoostCheck == nil {
		return
	}

	if mode == SmartFitOff {
		b.faceCropCheck.SetChecked(false)
		b.faceCropCheck.Disable()
		b.faceBoostCheck.SetChecked(false)
		b.faceBoostCheck.Disable()
	} else {
		b.faceCropCheck.Enable()
		b.faceBoostCheck.Enable()
	}
	b.faceCropCheck.Refresh()
	b.faceBoostCheck.Refresh()
}

func (b *PrefsPanelBuilder) addToggleSettings(c *fyne.Container) {
	// Stagger
	staggerConfig := setting.BoolConfig{
		Name:         "staggerChanges",
		InitialValue: b.plugin.cfg.GetStaggerMonitorChanges(),
		Label:        b.sm.CreateSettingTitleLabel("Stagger monitor changes:"),
		HelpContent:  b.sm.CreateSettingDescriptionLabel("Introduces a random delay when changing wallpapers across multiple screens to prevent a jarring simultaneous flash."),
		ApplyFunc: func(val bool) {
			b.plugin.cfg.SetStaggerMonitorChanges(val)
		},
	}
	b.sm.CreateBoolSetting(&staggerConfig, c)

	// Change on Start
	startConfig := setting.BoolConfig{
		Name:         "chgImgOnStart",
		InitialValue: b.plugin.cfg.GetChgImgOnStart(),
		Label:        b.sm.CreateSettingTitleLabel("Change wallpaper on start:"),
		HelpContent:  b.sm.CreateSettingDescriptionLabel("Disable if you prefer the wallpaper to change only based on its timer or a manual refresh."),
		ApplyFunc: func(val bool) {
			b.plugin.cfg.SetChgImgOnStart(val)
		},
	}
	b.sm.CreateBoolSetting(&startConfig, c)
}

func (b *PrefsPanelBuilder) addNightlyRefreshSetting(c *fyne.Container) {
	config := setting.BoolConfig{
		Name:         "nightlyRefresh",
		InitialValue: b.plugin.cfg.GetNightlyRefresh(),
		Label:        b.sm.CreateSettingTitleLabel("Refresh wallpapers nightly:"),
		HelpContent:  b.sm.CreateSettingDescriptionLabel("Useful when using image queries with random elements. Toggling this will start or stop the nightly refresh process."),
	}
	config.ApplyFunc = func(val bool) {
		b.plugin.cfg.SetNightlyRefresh(val)
		config.InitialValue = val
		if val {
			b.plugin.StartNightlyRefresh()
		} else {
			b.plugin.StopNightlyRefresh()
		}
	}
	b.sm.CreateBoolSetting(&config, c)
}

func (b *PrefsPanelBuilder) addActionButtons(c *fyne.Container) {
	// Sync
	syncConfig := setting.ButtonWithConfirmationConfig{
		Label:       b.sm.CreateSettingTitleLabel("Display Configuration:"),
		HelpContent: b.sm.CreateSettingDescriptionLabel("Synchronize Spice with currently connected monitors. Use this if you plugged or unplugged a monitor while Spice was running."),
		ButtonText:  "Refresh Displays",
		OnPressed:   func() { b.plugin.SyncMonitors(true) },
	}
	b.sm.CreateButtonWithConfirmationSetting(&syncConfig, c)

	// Clear Cache
	clearConfig := setting.ButtonWithConfirmationConfig{
		Label:          b.sm.CreateSettingTitleLabel("Clear Wallpaper Cache:"),
		HelpContent:    b.sm.CreateSettingDescriptionLabel("Delete all downloaded wallpapers (Source and Derivatives). This is a safety feature."),
		ButtonText:     "Clear Cache",
		ConfirmTitle:   "Clear Cache?",
		ConfirmMessage: "Are you sure? This will delete ALL downloaded images from disk. You will need internet to see new wallpapers.",
		OnPressed:      b.plugin.ClearCache,
	}
	b.sm.CreateButtonWithConfirmationSetting(&clearConfig, c)

	// Reset Avoid Set
	resetConfig := setting.ButtonWithConfirmationConfig{
		Label:          b.sm.CreateSettingTitleLabel("Blocked Images:"),
		HelpContent:    b.sm.CreateSettingDescriptionLabel("Clear the blocked images list. Blocked images may be downloaded next time wallpapers are refreshed."),
		ButtonText:     "Reset",
		ConfirmTitle:   "Please Confirm",
		ConfirmMessage: "This cannot be undone. Are you sure?",
		OnPressed:      b.plugin.cfg.ResetAvoidSet,
	}
	b.sm.CreateButtonWithConfirmationSetting(&resetConfig, c)
}

// BuildProviderTabs creates the provider accordions (Online, Local).
func (b *PrefsPanelBuilder) BuildProviderTabs() (fyne.CanvasObject, fyne.CanvasObject, int) {
	var onlineItems []accordionItem
	var localItems []accordionItem
	targetTabIndex := 0

	// Get ordered provider names
	var names []string
	for n := range b.plugin.providers {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, name := range names {
		p := b.plugin.providers[name]

		// Logic for pending URL / Focus
		isPending := false
		pendingURL := ""

		if b.plugin.pendingAddUrl != "" {
			if _, err := p.ParseURL(b.plugin.pendingAddUrl); err == nil {
				pendingURL = b.plugin.pendingAddUrl
				isPending = true
				b.plugin.pendingAddUrl = "" // Consume
			}
		}
		if b.plugin.focusProviderName == name {
			isPending = true
			b.plugin.focusProviderName = "" // Consume
		}

		if isPending {
			switch p.Type() {
			case provider.TypeLocal:
				targetTabIndex = 2
			case provider.TypeAI:
				targetTabIndex = 3
			default:
				targetTabIndex = 1
			}
		}

		// Create Panels
		settingsPanel := p.CreateSettingsPanel(b.sm)
		queryPanel := p.CreateQueryPanel(b.sm, pendingURL)

		if settingsPanel == nil && queryPanel == nil {
			continue
		}

		// Combined Content
		var content fyne.CanvasObject
		if settingsPanel != nil && queryPanel != nil {
			content = container.NewBorder(settingsPanel, nil, nil, nil, queryPanel)
		} else if settingsPanel != nil {
			content = settingsPanel
		} else {
			content = queryPanel
		}

		// Title Func
		titleFunc := func() string {
			title := p.Title()
			if title == "" {
				title = "Image Sources (" + p.Name() + ")"
			}
			activeCount := 0
			for _, q := range b.plugin.cfg.GetQueries() {
				if q.Provider == p.Name() && q.Active {
					activeCount++
				}
			}
			if activeCount > 0 {
				if activeCount == 1 {
					return fmt.Sprintf("%s (1 active)", title)
				}
				return fmt.Sprintf("%s (%d active)", title, activeCount)
			}
			return title
		}

		item := accordionItem{
			Title:     titleFunc(),
			TitleFunc: titleFunc,
			Content:   content,
			Open:      isPending,
			Icon:      p.GetProviderIcon(),
		}

		if p.Type() == provider.TypeLocal {
			localItems = append(localItems, item)
		} else if p.Type() == provider.TypeOnline {
			onlineItems = append(onlineItems, item)
		}
	}

	onlineTab, refreshOnline := createAccordion(onlineItems)
	localTab, refreshLocal := createAccordion(localItems)

	// Callback registration
	b.sm.RegisterOnSettingsSaved(func() {
		if refreshOnline != nil {
			refreshOnline()
		}
		if refreshLocal != nil {
			refreshLocal()
		}
	})

	return onlineTab, localTab, targetTabIndex
}
