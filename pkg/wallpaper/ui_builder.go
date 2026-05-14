package wallpaper

import (
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/schema"
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

// BuildGeneralTabSchema creates the General settings tab schema.
func (b *PrefsPanelBuilder) BuildGeneralTabSchema() *schema.PanelSchema {
	return &schema.PanelSchema{
		Sections: []schema.SectionSchema{
			{
				Title:       i18n.T("Wallpaper Cycle & Cache"),
				Description: i18n.T("Configure how often wallpapers change and how many images are kept locally."),
				Items: []schema.ItemSchema{
					schema.SelectItem{
						Name:         "changeFrequency",
						Label:        i18n.T("Wallpaper Change Frequency:"),
						Help:         i18n.T("Set how often the wallpaper changes. Set to \"Never\" to disable wallpaper changes."),
						Options:      setting.StringOptions(GetFrequencies()),
						InitialValue: int(b.plugin.cfg.GetWallpaperChangeFrequency()),
						ApplyFunc: func(val interface{}) {
							freq := Frequency(val.(int))
							b.plugin.ChangeWallpaperFrequency(freq, true)
						},
					},
					schema.SelectItem{
						Name:         "cacheSize",
						Label:        i18n.T("Cache Size:"),
						Help:         i18n.T("Set how many images to cache for faster startup and less network usage. Set to \"None\" to disable caching."),
						Options:      setting.StringOptions(GetCacheSizes()),
						InitialValue: int(b.plugin.cfg.GetCacheSize()),
						ApplyFunc: func(val interface{}) {
							size := CacheSize(val.(int))
							b.plugin.cfg.SetCacheSize(size)
						},
					},
				},
			},
			{
				Title:       i18n.T("Smart Fit & Face Detection"),
				Description: i18n.T("Control how images are fitted to your screen and optimized for faces."),
				Items: []schema.ItemSchema{
					schema.SelectItem{
						Name:         "smartFitMode",
						Label:        i18n.T("Smart Fit Mode:"),
						Help:         i18n.T("Control how images are fitted to your screen:\n- Disabled: Original image.\n- Quality: Rejects images with mismatched aspect ratio.\n- Flexibility: Allows high-res images to crop aggressively."),
						Options:      GetSmartFitModes(),
						InitialValue: int(b.plugin.cfg.GetSmartFitMode()),
						ApplyFunc: func(val interface{}) {
							mode := SmartFitMode(val.(int))
							b.plugin.cfg.SetSmartFitMode(mode)
						},
						NeedsRefresh: true,
					},
					schema.BoolItem{
						Name:         "faceCrop",
						Label:        i18n.T("Enable Face Crop:"),
						Help:         i18n.T("Aggressively crops the image to center on the largest face found. Good for portraits."),
						InitialValue: b.plugin.cfg.GetFaceCropEnabled(),
						NeedsRefresh: true,
						EnabledIf: func() bool {
							val := b.sm.GetValue("smartFitMode")
							if val == nil {
								return true
							}
							return SmartFitMode(val.(int)) != SmartFitOff
						},
						OnChanged: func(val bool) {
							if val {
								b.sm.SetValue("faceBoost", false)
							}
						},
						ApplyFunc: func(val bool) {
							b.plugin.cfg.SetFaceCropEnabled(val)
							if val {
								b.plugin.cfg.SetFaceBoostEnabled(false)
							}
						},
					},
					schema.BoolItem{
						Name:         "faceBoost",
						Label:        i18n.T("Enable Face Boost:"),
						Help:         i18n.T("Uses face detection to hint the smart cropper. Keeps faces in frame but balances with other image details."),
						InitialValue: b.plugin.cfg.GetFaceBoostEnabled(),
						NeedsRefresh: true,
						EnabledIf: func() bool {
							val := b.sm.GetValue("smartFitMode")
							if val == nil {
								return true
							}
							return SmartFitMode(val.(int)) != SmartFitOff
						},
						OnChanged: func(val bool) {
							if val {
								b.sm.SetValue("faceCrop", false)
							}
						},
						ApplyFunc: func(val bool) {
							b.plugin.cfg.SetFaceBoostEnabled(val)
							if val {
								b.plugin.cfg.SetFaceCropEnabled(false)
							}
						},
					},
				},
			},
			{
				Title:       i18n.T("Toggles"),
				Description: i18n.T("Miscellaneous behavioral settings."),
				Items: []schema.ItemSchema{
					schema.BoolItem{
						Name:         "staggerChanges",
						Label:        i18n.T("Stagger monitor changes:"),
						Help:         i18n.T("Introduces a random delay when changing wallpapers across multiple screens to prevent a jarring simultaneous flash."),
						InitialValue: b.plugin.cfg.GetStaggerMonitorChanges(),
						ApplyFunc: func(val bool) {
							b.plugin.cfg.SetStaggerMonitorChanges(val)
						},
					},
					schema.BoolItem{
						Name:         "chgImgOnStart",
						Label:        i18n.T("Change wallpaper on start:"),
						Help:         i18n.T("Disable if you prefer the wallpaper to change only based on its timer or a manual refresh."),
						InitialValue: b.plugin.cfg.GetChgImgOnStart(),
						ApplyFunc: func(val bool) {
							b.plugin.cfg.SetChgImgOnStart(val)
						},
					},
					schema.BoolItem{
						Name:         "nightlyRefresh",
						Label:        i18n.T("Refresh wallpapers nightly:"),
						Help:         i18n.T("Controls whether Spice automatically downloads new wallpapers each night. Background maintenance tasks (cache cleanup, metadata sync) always run regardless of this setting."),
						InitialValue: b.plugin.cfg.GetNightlyRefresh(),
						ApplyFunc: func(val bool) {
							b.plugin.cfg.SetNightlyRefresh(val)
						},
					},
				},
			},
			{
				Title:       i18n.T("Actions"),
				Description: i18n.T("Manual maintenance and display synchronization."),
				Items: []schema.ItemSchema{
					schema.ButtonItem{
						Name:       "refreshDisplays",
						Label:      i18n.T("Display Configuration:"),
						Help:       i18n.T("Synchronize Spice with currently connected monitors. Use this if you plugged or unplugged a monitor while Spice was running."),
						ButtonText: i18n.T("Refresh Displays"),
						OnPressed: func() {
							b.plugin.SyncMonitors(true)
						},
					},
					schema.ConfirmButtonItem{
						Name:           "clearCache",
						Label:          i18n.T("Clear Wallpaper Cache:"),
						Help:           i18n.T("Delete all downloaded wallpapers (Source and Derivatives). This is a safety feature."),
						ButtonText:     i18n.T("Clear Cache"),
						ConfirmTitle:   i18n.T("Clear Cache?"),
						ConfirmMessage: i18n.T("Are you sure? This will delete ALL downloaded images from disk. You will need internet to see new wallpapers."),
						Importance:     schema.ImportanceDanger,
						OnPressed:      b.plugin.ClearCache,
					},
					schema.ConfirmButtonItem{
						Name:           "resetAvoidSet",
						Label:          i18n.T("Blocked Images:"),
						Help:           i18n.T("Clear the blocked images list. Blocked images may be downloaded next time wallpapers are refreshed."),
						ButtonText:     i18n.T("Reset"),
						ConfirmTitle:   i18n.T("Please Confirm"),
						ConfirmMessage: i18n.T("This cannot be undone. Are you sure?"),
						Importance:     schema.ImportanceMedium,
						OnPressed:      b.plugin.cfg.ResetAvoidSet,
					},
				},
			},
		},
	}
}

// BuildGeneralTabAccordion splits the general settings schema into accordion items,
// one per section. The first section is open by default.
func (b *PrefsPanelBuilder) BuildGeneralTabAccordion(sm setting.SettingsManager) []accordionItem {
	generalSchema := b.BuildGeneralTabSchema()
	var items []accordionItem

	// Icons for each General section, in order:
	// Wallpaper Cycle & Cache, Smart Fit & Face Detection, Toggles, Actions
	sectionIcons := []fyne.Resource{
		theme.HistoryIcon(),
		theme.ViewFullScreenIcon(),
		theme.CheckButtonCheckedIcon(),
		theme.ComputerIcon(),
	}

	for i, section := range generalSchema.Sections {
		// Strip title/description — the accordion header already shows these.
		title := section.Title
		section.Title = ""
		section.Description = ""

		sectionPanel := schema.PanelSchema{Sections: []schema.SectionSchema{section}}
		sectionContent := sm.RenderSchema(sectionPanel)

		var icon fyne.Resource
		if i < len(sectionIcons) {
			icon = sectionIcons[i]
		}

		items = append(items, accordionItem{
			Title:   title,
			Content: sectionContent,
			Open:    i == 0,
			Icon:    icon,
		})
	}

	return items
}

// BuildProviderTabs creates the provider accordions (Community, Personal, Museums).
func (b *PrefsPanelBuilder) BuildProviderTabs() (fyne.CanvasObject, fyne.CanvasObject, fyne.CanvasObject, int) {
	var onlineItems []accordionItem
	var localItems []accordionItem
	var museumItems []accordionItem
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

		switch p.Type() {
		case provider.TypePersonal:
			localItems = append(localItems, *item)
		case provider.TypeMuseum:
			museumItems = append(museumItems, *item)
		default:
			onlineItems = append(onlineItems, *item)
		}
	}

	onlineTab, refreshOnline := createAccordion(onlineItems)
	localTab, refreshLocal := createAccordion(localItems)
	museumTab, refreshMuseum := createAccordion(museumItems)

	b.sm.RegisterOnSettingsSaved(func() {
		if refreshOnline != nil {
			refreshOnline()
		}
		if refreshLocal != nil {
			refreshLocal()
		}
		if refreshMuseum != nil {
			refreshMuseum()
		}
	})

	// Register with SM to refresh on ANY settings change (Reactive Titles)
	b.sm.RegisterRefreshFunc(func() {
		if refreshOnline != nil {
			refreshOnline()
		}
		if refreshLocal != nil {
			refreshLocal()
		}
		if refreshMuseum != nil {
			refreshMuseum()
		}
	})

	return onlineTab, localTab, museumTab, targetTabIndex
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
		case provider.TypePersonal:
			tabIndex = 2
		case provider.TypeMuseum:
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
		Icon:      asResource(p.GetProviderIcon(), p.ID()),
	}, tabIndex
}

func (b *PrefsPanelBuilder) buildProviderContent(p provider.ImageProvider, pendingURL string) fyne.CanvasObject {
	var settingsPanel fyne.CanvasObject
	if panelSchema := p.CreateSettingsPanel(b.sm); panelSchema != nil {
		settingsPanel = b.sm.RenderSchema(*panelSchema)
	}

	var queryPanel fyne.CanvasObject
	if querySchema := p.CreateQueryPanel(b.sm, pendingURL); querySchema != nil {
		queryPanel = b.sm.RenderSchema(*querySchema)
	}

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
			if q.Provider == p.ID() {
				// Reactive: Check pending state in SM first, then fallback to persisted config
				isActive := q.Active
				if b.sm.HasPendingChange(q.ID) {
					isActive = !q.Active // If it has a pending change, the state is flipped from baseline
				}

				if isActive {
					activeCount++
				}
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
