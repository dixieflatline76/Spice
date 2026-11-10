package ui

import (
	"sync/atomic"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
	"github.com/dixieflatline76/Spice/v2/asset"
	"github.com/dixieflatline76/Spice/v2/pkg/ui"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/schema"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type spyDriver struct {
	fyne.Driver
	doCalls atomic.Int32
}

func (d *spyDriver) DoFromGoroutine(f func(), wait bool) {
	d.doCalls.Add(1)
	d.Driver.DoFromGoroutine(f, wait)
}

type spyDesktopApp struct {
	fyne.App
	drv       *spyDriver
	menus     []*fyne.Menu
	trayIcons []fyne.Resource
	appIcons  []fyne.Resource
}

var _ desktop.App = (*spyDesktopApp)(nil)

func newSpyDesktopApp(t *testing.T) *spyDesktopApp {
	t.Helper()
	base := test.NewApp()
	drv := &spyDriver{Driver: base.Driver()}
	spy := &spyDesktopApp{App: base, drv: drv}
	fyne.SetCurrentApp(spy)
	t.Cleanup(func() {
		base.Quit()
		test.NewApp()
	})
	return spy
}

func (a *spyDesktopApp) Driver() fyne.Driver {
	return a.drv
}

func (a *spyDesktopApp) SetIcon(icon fyne.Resource) {
	a.appIcons = append(a.appIcons, icon)
	a.App.SetIcon(icon)
}

func (a *spyDesktopApp) SetSystemTrayMenu(menu *fyne.Menu) {
	a.menus = append(a.menus, menu)
}

func (a *spyDesktopApp) SetSystemTrayIcon(icon fyne.Resource) {
	a.trayIcons = append(a.trayIcons, icon)
}

func (a *spyDesktopApp) SetSystemTrayWindow(_ fyne.Window) {}

type spyOS struct {
	tooltips []string
}

var _ OS = (*spyOS)(nil)

func (s *spyOS) TransformToForeground()                 {}
func (s *spyOS) TransformToBackground()                 {}
func (s *spyOS) SetupLifecycle(_ fyne.App, _ *SpiceApp) {}
func (s *spyOS) SetTrayTooltip(tooltip string) {
	s.tooltips = append(s.tooltips, tooltip)
}

type mockTrayPlugin struct {
	artistLabel string
	favLabel    string
	favIcon     string
}

func (p *mockTrayPlugin) ID() string              { return "wallpaper" }
func (p *mockTrayPlugin) Name() string            { return "Wallpaper" }
func (p *mockTrayPlugin) Activate()               {}
func (p *mockTrayPlugin) Deactivate()             {}
func (p *mockTrayPlugin) Init(_ ui.PluginManager) {}
func (p *mockTrayPlugin) CreatePrefsSchema(_ setting.SettingsManager) *schema.TabsSchema {
	return nil
}

func (p *mockTrayPlugin) CreateTrayMenu() *schema.MenuSchema {
	return &schema.MenuSchema{
		Items: []schema.MenuItemSchema{
			{Label: "Next Wallpaper", IconName: "next.png", Action: func() {}},
			{Label: p.artistLabel, IconName: "view.png", Action: func() {}},
			{Label: p.favLabel, IconName: p.favIcon, Action: func() {}},
		},
	}
}

func TestCreateMenuItem_DoesNotRequeueViaFyneDo(t *testing.T) {
	spyApp := newSpyDesktopApp(t)
	sa := &SpiceApp{
		App:      spyApp,
		assetMgr: asset.NewManager(),
	}

	var actionCalls int
	mi := sa.CreateMenuItem("Next Wallpaper", func() {
		actionCalls++
	}, "next.png")

	require.NotNil(t, mi.Action)
	require.NotNil(t, mi.Icon)

	// Fyne's desktop driver already dispatches MenuItem.Action via runOnMain.
	// Invoking mi.Action must run the callback directly without re-queueing via fyne.Do.
	mi.Action()

	assert.Equal(t, 1, actionCalls, "Menu item action should execute immediately")
	assert.Equal(t, int32(0), spyApp.drv.doCalls.Load(), "CreateMenuItem must not re-queue action through fyne.Do")
}

func TestCreateMenuItem_NilActionStaysNil(t *testing.T) {
	spyApp := newSpyDesktopApp(t)
	sa := &SpiceApp{
		App:      spyApp,
		assetMgr: asset.NewManager(),
	}

	mi := sa.CreateMenuItem("Source: Initializing...", nil, "")
	assert.Nil(t, mi.Action, "Nil action must stay nil rather than wrapping nil in a closure")
}

func TestCreateToggleMenuItem_DoesNotRequeueViaFyneDo(t *testing.T) {
	spyApp := newSpyDesktopApp(t)
	sa := &SpiceApp{
		App:      spyApp,
		assetMgr: asset.NewManager(),
		trayMenu: fyne.NewMenu(""),
	}

	var toggledVal bool
	mi := sa.CreateToggleMenuItem("Shuffle", func(v bool) {
		toggledVal = v
	}, "", false)

	require.NotNil(t, mi.Action)
	mi.Action()

	assert.True(t, toggledVal)
	assert.True(t, mi.Checked)
	assert.Equal(t, int32(0), spyApp.drv.doCalls.Load(), "CreateToggleMenuItem must not re-queue action through fyne.Do")
}

func TestCreateTrayMenu_RebuildsDynamicItemsEveryTime_SetsTrayIconOnce(t *testing.T) {
	spyApp := newSpyDesktopApp(t)
	plugin := &mockTrayPlugin{
		artistLabel: "By: Monet",
		favLabel:    "Add to Favorites",
		favIcon:     "favorite.png",
	}
	sa := &SpiceApp{
		App:      spyApp,
		assetMgr: asset.NewManager(),
		plugins:  []ui.Plugin{plugin},
	}

	// 1. Initial tray menu build at startup
	sa.CreateTrayMenu()
	require.Len(t, spyApp.menus, 1)
	assert.Equal(t, "By: Monet", spyApp.menus[0].Items[1].Label)
	require.NotNil(t, spyApp.menus[0].Items[2].Icon)
	assert.Equal(t, "favorite.png", spyApp.menus[0].Items[2].Icon.Name())
	assert.Len(t, spyApp.trayIcons, 1, "Initial CreateTrayMenu should set the static system tray icon")
	assert.Len(t, spyApp.appIcons, 1, "Initial CreateTrayMenu should set the app icon")

	// 2. Wallpaper changes to a favorited Hokusai piece -> menu must update, static tray icon must not be re-uploaded
	plugin.artistLabel = "By: Hokusai"
	plugin.favLabel = "Remove from Favorites"
	plugin.favIcon = "unfavorite.png"
	sa.CreateTrayMenu()

	require.Len(t, spyApp.menus, 2, "SetSystemTrayMenu must run on every rebuild")
	assert.Equal(t, "By: Hokusai", spyApp.menus[1].Items[1].Label)
	assert.Equal(t, "Remove from Favorites", spyApp.menus[1].Items[2].Label)
	require.NotNil(t, spyApp.menus[1].Items[2].Icon)
	assert.Equal(t, "unfavorite.png", spyApp.menus[1].Items[2].Icon.Name())
	assert.Len(t, spyApp.trayIcons, 1, "Static tray.png icon must not be re-sent on menu rebuilds")
	assert.Len(t, spyApp.appIcons, 1, "Static app icon must not be re-sent on menu rebuilds")

	// 3. LockTrayMenu during tuning + subsequent CreateTrayMenu restore
	sa.LockTrayMenu(nil)
	require.Len(t, spyApp.menus, 3)

	plugin.artistLabel = "By: Vermeer"
	plugin.favLabel = "Add to Favorites"
	plugin.favIcon = "favorite.png"
	sa.CreateTrayMenu()

	require.Len(t, spyApp.menus, 4)
	assert.Equal(t, "By: Vermeer", spyApp.menus[3].Items[1].Label)
	assert.Equal(t, "favorite.png", spyApp.menus[3].Items[2].Icon.Name())
	assert.Len(t, spyApp.trayIcons, 1, "Static tray.png icon must still only have been set once")
}

func TestCreateTrayMenu_SetsTrayTooltipOnceWhenOSPresent(t *testing.T) {
	spyApp := newSpyDesktopApp(t)
	osSpy := &spyOS{}
	plugin := &mockTrayPlugin{
		artistLabel: "By: Monet",
		favLabel:    "Add to Favorites",
		favIcon:     "favorite.png",
	}
	sa := &SpiceApp{
		App:      spyApp,
		os:       osSpy,
		assetMgr: asset.NewManager(),
		plugins:  []ui.Plugin{plugin},
	}

	// 1. Initial CreateTrayMenu sets tooltip once alongside the tray icon
	sa.CreateTrayMenu()
	assert.Equal(t, []string{"Spice Wallpaper Manager"}, osSpy.tooltips)

	// 2. Subsequent menu rebuilds do not re-set the static tray tooltip
	plugin.artistLabel = "By: Hokusai"
	sa.CreateTrayMenu()
	assert.Equal(t, []string{"Spice Wallpaper Manager"}, osSpy.tooltips)
}
