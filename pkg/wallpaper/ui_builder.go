package wallpaper

import (
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
)

// PrefsPanelBuilder constructs the preferences UI.
type PrefsPanelBuilder struct {
	plugin *Plugin
	sm     setting.SettingsManager
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
		Label:        b.sm.CreateSettingTitleLabel(i18n.T("Wallpaper Change Frequency:")),
		HelpContent:  b.sm.CreateSettingDescriptionLabel(i18n.T("Set how often the wallpaper changes. Set to \"Never\" to disable wallpaper changes.")),
	}
	config.ApplyFunc = func(val interface{}) {
		freq := Frequency(val.(int))
		b.plugin.cfg.SetWallpaperChangeFrequency(freq)
		b.plugin.ChangeWallpaperFrequency(freq, true)
		config.InitialValue = int(freq)
	}
	b.sm.CreateSelectSetting(&config, c)
}

func (b *PrefsPanelBuilder) addCacheSizeSetting(c *fyne.Container) {
	config := setting.SelectConfig{
		Name:         "cacheSize",
		Options:      setting.StringOptions(GetCacheSizes()),
		InitialValue: int(b.plugin.cfg.GetCacheSize()),
		Label:        b.sm.CreateSettingTitleLabel(i18n.T("Cache Size:")),
		HelpContent:  b.sm.CreateSettingDescriptionLabel(i18n.T("Set how many images to cache for faster startup and less network usage. Set to \"None\" to disable caching.")),
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
		Label:        b.sm.CreateSettingTitleLabel(i18n.T("Smart Fit Mode:")),
		HelpContent:  b.sm.CreateSettingDescriptionLabel(i18n.T("Control how images are fitted to your screen:\n- Disabled: Original image.\n- Quality: Rejects images with mismatched aspect ratio.\n- Flexibility: Allows high-res images to crop aggressively.")),
	}
	config.ApplyFunc = func(val interface{}) {
		mode := SmartFitMode(val.(int))
		b.plugin.cfg.SetSmartFitMode(mode)
		config.InitialValue = int(mode)
	}
	config.NeedsRefresh = true
	b.sm.CreateSelectSetting(&config, c)

	// Face Options (Crop / Boost)
	b.addFaceOptions(c)
}

func (b *PrefsPanelBuilder) addFaceOptions(c *fyne.Container) {
	var cropConfig, boostConfig setting.BoolConfig

	// Face Crop
	cropConfig = setting.BoolConfig{
		Name:         "faceCrop",
		InitialValue: b.plugin.cfg.GetFaceCropEnabled(),
		Label:        b.sm.CreateSettingTitleLabel(i18n.T("Enable Face Crop:")),
		HelpContent:  b.sm.CreateSettingDescriptionLabel(i18n.T("Aggressively crops the image to center on the largest face found. Good for portraits.")),
		ApplyFunc: func(val bool) {
			b.plugin.cfg.SetFaceCropEnabled(val)
			if val {
				b.plugin.cfg.SetFaceBoostEnabled(false)
				b.sm.SetValue("faceBoost", false) // Update the other setting's UI
			}
			cropConfig.InitialValue = val
		},
		NeedsRefresh: true,
		EnabledIf: func() bool {
			val := b.sm.GetValue("smartFitMode")
			if val == nil {
				return true
			}
			return SmartFitMode(val.(int)) != SmartFitOff
		},
	}

	// Face Boost
	boostConfig = setting.BoolConfig{
		Name:         "faceBoost",
		InitialValue: b.plugin.cfg.GetFaceBoostEnabled(),
		Label:        b.sm.CreateSettingTitleLabel(i18n.T("Enable Face Boost:")),
		HelpContent:  b.sm.CreateSettingDescriptionLabel(i18n.T("Uses face detection to hint the smart cropper. Keeps faces in frame but balances with other image details.")),
		ApplyFunc: func(val bool) {
			b.plugin.cfg.SetFaceBoostEnabled(val)
			if val {
				b.plugin.cfg.SetFaceCropEnabled(false)
				b.sm.SetValue("faceCrop", false) // Update the other setting's UI
			}
			boostConfig.InitialValue = val
		},
		NeedsRefresh: true,
		EnabledIf: func() bool {
			val := b.sm.GetValue("smartFitMode")
			if val == nil {
				return true
			}
			return SmartFitMode(val.(int)) != SmartFitOff
		},
	}

	subSettingsContainer := container.NewVBox()
	b.sm.CreateBoolSetting(&cropConfig, subSettingsContainer)
	b.sm.CreateBoolSetting(&boostConfig, subSettingsContainer)

	indentation := widget.NewLabel("      ")
	indentedWrapper := container.NewBorder(nil, nil, indentation, nil, subSettingsContainer)
	c.Add(indentedWrapper)
}

func (b *PrefsPanelBuilder) addToggleSettings(c *fyne.Container) {
	// Stagger
	staggerConfig := setting.BoolConfig{
		Name:         "staggerChanges",
		InitialValue: b.plugin.cfg.GetStaggerMonitorChanges(),
		Label:        b.sm.CreateSettingTitleLabel(i18n.T("Stagger monitor changes:")),
		HelpContent:  b.sm.CreateSettingDescriptionLabel(i18n.T("Introduces a random delay when changing wallpapers across multiple screens to prevent a jarring simultaneous flash.")),
		ApplyFunc: func(val bool) {
			b.plugin.cfg.SetStaggerMonitorChanges(val)
		},
	}
	b.sm.CreateBoolSetting(&staggerConfig, c)

	// Change on Start
	startConfig := setting.BoolConfig{
		Name:         "chgImgOnStart",
		InitialValue: b.plugin.cfg.GetChgImgOnStart(),
		Label:        b.sm.CreateSettingTitleLabel(i18n.T("Change wallpaper on start:")),
		HelpContent:  b.sm.CreateSettingDescriptionLabel(i18n.T("Disable if you prefer the wallpaper to change only based on its timer or a manual refresh.")),
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
		Label:        b.sm.CreateSettingTitleLabel(i18n.T("Refresh wallpapers nightly:")),
		HelpContent:  b.sm.CreateSettingDescriptionLabel(i18n.T("Useful when using image queries with random elements. Toggling this will start or stop the nightly refresh process.")),
	}
	config.ApplyFunc = func(val bool) {
		b.plugin.cfg.SetNightlyRefresh(val)
		config.InitialValue = val
		if val {
			go b.plugin.StartNightlyRefresh()
		} else {
			b.plugin.StopNightlyRefresh()
		}
	}
	b.sm.CreateBoolSetting(&config, c)
}

func (b *PrefsPanelBuilder) addActionButtons(c *fyne.Container) {
	// Sync
	syncConfig := setting.ButtonWithConfirmationConfig{
		Label:       b.sm.CreateSettingTitleLabel(i18n.T("Display Configuration:")),
		HelpContent: b.sm.CreateSettingDescriptionLabel(i18n.T("Synchronize Spice with currently connected monitors. Use this if you plugged or unplugged a monitor while Spice was running.")),
		ButtonText:  i18n.T("Refresh Displays"),
		OnPressed:   func() { b.plugin.SyncMonitors(true) },
	}
	b.sm.CreateButtonWithConfirmationSetting(&syncConfig, c)

	// Clear Cache
	clearConfig := setting.ButtonWithConfirmationConfig{
		Label:          b.sm.CreateSettingTitleLabel(i18n.T("Clear Wallpaper Cache:")),
		HelpContent:    b.sm.CreateSettingDescriptionLabel(i18n.T("Delete all downloaded wallpapers (Source and Derivatives). This is a safety feature.")),
		ButtonText:     i18n.T("Clear Cache"),
		ConfirmTitle:   i18n.T("Clear Cache?"),
		ConfirmMessage: i18n.T("Are you sure? This will delete ALL downloaded images from disk. You will need internet to see new wallpapers."),
		OnPressed:      b.plugin.ClearCache,
	}
	b.sm.CreateButtonWithConfirmationSetting(&clearConfig, c)

	// Reset Avoid Set
	resetConfig := setting.ButtonWithConfirmationConfig{
		Label:          b.sm.CreateSettingTitleLabel(i18n.T("Blocked Images:")),
		HelpContent:    b.sm.CreateSettingDescriptionLabel(i18n.T("Clear the blocked images list. Blocked images may be downloaded next time wallpapers are refreshed.")),
		ButtonText:     i18n.T("Reset"),
		ConfirmTitle:   i18n.T("Please Confirm"),
		ConfirmMessage: i18n.T("This cannot be undone. Are you sure?"),
		OnPressed:      b.plugin.cfg.ResetAvoidSet,
	}
	b.sm.CreateButtonWithConfirmationSetting(&resetConfig, c)
}

// BuildProviderTabs creates the provider accordions (Online, Local).
func (b *PrefsPanelBuilder) BuildProviderTabs() (fyne.CanvasObject, fyne.CanvasObject, int) {
	var onlineItems []accordionItem
	var localItems []accordionItem
	targetTabIndex := 0

	names := b.getSortedProviderIDs()

	for _, name := range names {
		p := b.plugin.providers[name]
		if p == nil {
			continue
		}

		item, tabIdx := b.createProviderAccordionItem(p)
		if tabIdx > 0 {
			targetTabIndex = tabIdx
		}
		if item == nil {
			continue
		}

		if p.Type() == provider.TypeLocal {
			localItems = append(localItems, *item)
		} else if p.Type() == provider.TypeOnline {
			onlineItems = append(onlineItems, *item)
		}
	}

	onlineTab, refreshOnline := createAccordion(onlineItems)
	localTab, refreshLocal := createAccordion(localItems)

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

func (b *PrefsPanelBuilder) getSortedProviderIDs() []string {
	var names []string
	for n := range b.plugin.providers {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func (b *PrefsPanelBuilder) createProviderAccordionItem(p provider.ImageProvider) (*accordionItem, int) {
	tabIndex := 0
	isPending := false
	pendingURL := ""

	// Check pending state
	if b.plugin.pendingAddUrl != "" {
		if _, err := p.ParseURL(b.plugin.pendingAddUrl); err == nil {
			pendingURL = b.plugin.pendingAddUrl
			isPending = true
			b.plugin.pendingAddUrl = ""
		}
	}
	if b.plugin.focusProviderName == p.ID() {
		isPending = true
		b.plugin.focusProviderName = ""
	}

	if isPending {
		switch p.Type() {
		case provider.TypeLocal:
			tabIndex = 2
		case provider.TypeAI:
			tabIndex = 3
		default:
			tabIndex = 1
		}
	}

	content := b.buildProviderContent(p, pendingURL)
	if content == nil {
		return nil, tabIndex
	}

	titleFunc := b.createTitleFunc(p)

	return &accordionItem{
		Title:     titleFunc(),
		TitleFunc: titleFunc,
		Content:   content,
		Open:      isPending,
		Icon:      p.GetProviderIcon(),
	}, tabIndex
}

func (b *PrefsPanelBuilder) buildProviderContent(p provider.ImageProvider, pendingURL string) fyne.CanvasObject {
	settingsPanel := p.CreateSettingsPanel(b.sm)
	queryPanel := p.CreateQueryPanel(b.sm, pendingURL)

	if settingsPanel != nil && queryPanel != nil {
		return container.NewBorder(settingsPanel, nil, nil, nil, queryPanel)
	} else if settingsPanel != nil {
		return settingsPanel
	} else if queryPanel != nil {
		return queryPanel
	}
	return nil
}

func (b *PrefsPanelBuilder) createTitleFunc(p provider.ImageProvider) func() string {
	return func() string {
		title := p.Title()
		if title == "" {
			title = i18n.Tf("Image Sources ({{.Name}})", map[string]any{"Name": p.Name()}) + "..."
		}
		activeCount := 0
		for _, q := range b.plugin.cfg.GetQueries() {
			if q.Provider == p.ID() && q.Active {
				activeCount++
			}
		}
		if activeCount > 0 {
			if activeCount == 1 {
				return i18n.Tf("{{.Title}} (1 active)", map[string]any{"Title": title})
			}
			return i18n.Tf("{{.Title}} ({{.Count}} active)", map[string]any{"Title": title, "Count": activeCount})
		}
		return title
	}
}
