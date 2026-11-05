package ui

import (
	"fmt"
	"net/http"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/schema"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper/providers/favorites"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper/providers/googlephotos"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper/providers/localfolder"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper/providers/pexels"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper/providers/wallhaven"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper/providers/wikimedia"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

// renderProviderPanel mirrors PrefsPanelBuilder.buildProviderContent: settings + query sections in ONE panel.
func renderProviderPanel(sm *SettingsManager, p provider.ImageProvider) fyne.CanvasObject {
	var sections []schema.SectionSchema
	if s := p.CreateSettingsPanel(sm); s != nil {
		sections = append(sections, s.Sections...)
	}
	if q := p.CreateQueryPanel(sm, ""); q != nil {
		sections = append(sections, q.Sections...)
	}
	return sm.RenderSchema(schema.PanelSchema{Sections: sections})
}

// inspectTree walks the rendered tree, collecting query list widgets and any nested scrollable
// containers (which would trap trackpad scrolling and/or collapse their content to 1 row).
func inspectTree(obj fyne.CanvasObject, lists *[]*queryListWidget, nestedScrollers *[]string) {
	switch o := obj.(type) {
	case *queryListWidget:
		*lists = append(*lists, o)
		return
	case *widget.Entry:
		// Every Fyne text entry owns an internal text scroller for its own input text; that is
		// not a layout container and cannot crush sibling content, so don't descend into it.
		return
	case *container.Scroll:
		*nestedScrollers = append(*nestedScrollers, "container.Scroll")
	case *widget.List:
		*nestedScrollers = append(*nestedScrollers, "widget.List")
	case *widget.Table:
		*nestedScrollers = append(*nestedScrollers, "widget.Table")
	case *widget.Tree:
		*nestedScrollers = append(*nestedScrollers, "widget.Tree")
	case *widget.GridWrap:
		*nestedScrollers = append(*nestedScrollers, "widget.GridWrap")
	}

	var children []fyne.CanvasObject
	switch o := obj.(type) {
	case *fyne.Container:
		children = o.Objects
	case fyne.Widget:
		children = test.WidgetRenderer(o).Objects()
	}
	for _, c := range children {
		inspectTree(c, lists, nestedScrollers)
	}
}

// TestProviderQueryPanels_AllRowsVisibleInSingleScroll renders every REAL provider that has a
// query/collection table and verifies the MacBook Neo bug (list crushed to 1 row inside a
// nested scroller) cannot occur for any of them.
func TestProviderQueryPanels_AllRowsVisibleInSingleScroll(t *testing.T) {
	keyring.MockInit() // Never touch the real OS keychain (API keys / OAuth tokens) from tests.

	app := test.NewApp()
	defer app.Quit()
	w := app.NewWindow("ProviderPanels")
	cfg := wallpaper.GetConfig(app.Preferences())
	client := &http.Client{}
	tmp := t.TempDir()

	cases := []struct {
		name     string
		p        provider.ImageProvider
		addQuery func(i int) error // nil => provider has a single fixed system collection (no growth check)
	}{
		{"Wallhaven", wallhaven.NewProvider(cfg, client), func(i int) error {
			_, err := cfg.AddImageQuery(fmt.Sprintf("WH probe %d", i), fmt.Sprintf("https://wallhaven.cc/search?q=probe%d", i), false)
			return err
		}},
		{"Pexels", pexels.NewProvider(cfg, client), func(i int) error {
			_, err := cfg.AddPexelsQuery(fmt.Sprintf("Pexels probe %d", i), fmt.Sprintf("https://www.pexels.com/search/probe%d/", i), false)
			return err
		}},
		{"Wikimedia", wikimedia.NewProvider(cfg, client), func(i int) error {
			_, err := cfg.AddWikimediaQuery(fmt.Sprintf("Wiki probe %d", i), fmt.Sprintf("https://commons.wikimedia.org/wiki/Category:Probe%d", i), false)
			return err
		}},
		{"LocalFolder", localfolder.NewProvider(cfg), func(i int) error {
			_, err := cfg.AddLocalFolderQuery(fmt.Sprintf("Folder probe %d", i), filepath.Join(tmp, fmt.Sprintf("probe%d", i)), false)
			return err
		}},
		{"GooglePhotos", googlephotos.NewProvider(cfg, client), func(i int) error {
			_, err := cfg.AddGooglePhotosQuery(fmt.Sprintf("GP probe %d", i), fmt.Sprintf("googlephotos://probe%d", i), false)
			return err
		}},
		{"Favorites", favorites.NewProvider(cfg), nil},
	}

	const added = 5
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			check := func() (rows int, height float32, rowHeight float32) {
				rendered := renderProviderPanel(NewSettingsManager(w).(*SettingsManager), tc.p)
				scroll, ok := rendered.(*container.Scroll)
				require.True(t, ok, "%s panel must be wrapped in exactly one outer VScroll", tc.name)

				var lists []*queryListWidget
				var nested []string
				inspectTree(scroll.Content, &lists, &nested)
				assert.Empty(t, nested, "%s panel must not contain nested scrollable containers", tc.name)
				require.Len(t, lists, 1, "%s panel must render exactly one query list", tc.name)

				ql := lists[0]
				if len(ql.rows) > 0 {
					rowHeight = ql.rows[0].row.MinSize().Height
				}
				return len(ql.rows), scroll.Content.MinSize().Height, rowHeight
			}

			rowsBefore, heightBefore, _ := check()
			if tc.addQuery == nil {
				assert.GreaterOrEqual(t, rowsBefore, 1, "%s must show its system collection row", tc.name)
				return
			}

			for i := 0; i < added; i++ {
				require.NoError(t, tc.addQuery(i))
			}
			rowsAfter, heightAfter, rowHeight := check()

			assert.Equal(t, rowsBefore+added, rowsAfter, "%s must render a row for every query", tc.name)
			require.Greater(t, rowHeight, float32(0))
			assert.GreaterOrEqual(t, heightAfter-heightBefore, float32(added)*rowHeight*0.9,
				"%s scroll content must grow by every added row (not collapse to a single row)", tc.name)
		})
	}
}
