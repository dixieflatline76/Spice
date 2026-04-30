# Creating New Plugins for Spice

Spice is designed to be extensible through a modular Plugin architecture. This guide walks you through creating a new plugin from scratch.

## 1. The Plugin Interface

All plugins must implement the `ui.Plugin` interface defined in `pkg/ui/plugin.go`:

```go
type Plugin interface {
    ID() string                                               // Stable identifier (e.g., "my-plugin")
    Name() string                                             // Display name (can be localized)
    Activate()                                                // Called when enabled
    Deactivate()                                              // Called when disabled or app shuts down
    CreateTrayMenuItems() []*fyne.MenuItem                    // Injects items into the Tray/Menu Bar
    CreatePrefsPanel(setting.SettingsManager) *fyne.Container // Injects a tab into Preferences
    Init(PluginManager)                                       // Dependency Injection boundary
}
```

## 2. Schema-Based Settings

Plugins should follow the **Hexagonal Architecture** used throughout Spice. While `CreatePrefsPanel` returns a `*fyne.Container` (for maximum flexibility), the recommended approach is to build your settings using the schema-driven pattern:

1. **Define your settings** as `*schema.PanelSchema` structs (pure Go, no Fyne imports).
2. **Render them** via `sm.RenderSchema()` inside `CreatePrefsPanel`.

This ensures consistent styling, automatic dirty tracking, and Apply button integration.

```go
func (p *MyPlugin) CreatePrefsPanel(sm setting.SettingsManager) *fyne.Container {
    panel := &schema.PanelSchema{
        Sections: []schema.SectionSchema{
            {
                Title: i18n.T("My Plugin Settings"),
                Items: []schema.ItemSchema{
                    schema.BoolItem{
                        Name:         "enableGreetings",
                        Label:        i18n.T("Enable Greetings"),
                        InitialValue: p.cfg.GetGreetingsEnabled(),
                        ApplyFunc: func(on bool) {
                            p.cfg.SetGreetingsEnabled(on)
                        },
                    },
                    schema.SelectItem{
                        Name:    "greetingStyle",
                        Label:   i18n.T("Greeting Style"),
                        Options: setting.StringOptions("Formal", "Casual", "Enthusiastic"),
                        InitialValue: p.cfg.GetGreetingStyle(),
                        ApplyFunc: func(idx int) {
                            p.cfg.SetGreetingStyle(idx)
                        },
                    },
                },
            },
        },
    }

    rendered := sm.RenderSchema(*panel)
    return container.NewVBox(rendered)
}
```

> **Key Rule**: Providers and plugins should never import `fyne.io/fyne/v2/widget` directly for settings UI. Use `schema.*` types and let the rendering engine handle widget creation. The only Fyne import needed is `fyne.io/fyne/v2/container` for the return type.

## 3. "Hello World" Example

Below is a complete implementation of a simple "Hello World" plugin:

```go
package hello

import (
    "fyne.io/fyne/v2"
    "fyne.io/fyne/v2/container"
    "github.com/dixieflatline76/Spice/v2/pkg/i18n"
    "github.com/dixieflatline76/Spice/v2/pkg/ui"
    "github.com/dixieflatline76/Spice/v2/pkg/ui/schema"
    "github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
)

type HelloPlugin struct {
    manager ui.PluginManager
    enabled bool
}

func (p *HelloPlugin) ID() string   { return "hello-world" }
func (p *HelloPlugin) Name() string { return i18n.T("Hello World") }

func (p *HelloPlugin) Init(mgr ui.PluginManager) {
    p.manager = mgr
}

func (p *HelloPlugin) Activate() {
    p.enabled = true
    p.manager.NotifyUser("Hello", "Plugin Activated!")
}

func (p *HelloPlugin) Deactivate() {
    p.enabled = false
}

func (p *HelloPlugin) CreateTrayMenuItems() []*fyne.MenuItem {
    return []*fyne.MenuItem{
        fyne.NewMenuItem(i18n.T("Say Hello"), func() {
            p.manager.NotifyUser("Spice", "Hello from the tray!")
        }),
    }
}

func (p *HelloPlugin) CreatePrefsPanel(sm setting.SettingsManager) *fyne.Container {
    panel := &schema.PanelSchema{
        Sections: []schema.SectionSchema{
            {
                Title: i18n.T("Hello World Settings"),
                Items: []schema.ItemSchema{
                    schema.BoolItem{
                        Name:         "enableGreetings",
                        Label:        i18n.T("Enable Greetings"),
                        InitialValue: p.enabled,
                        ApplyFunc: func(on bool) {
                            p.enabled = on
                        },
                    },
                },
            },
        },
    }

    rendered := sm.RenderSchema(*panel)
    return container.NewVBox(rendered)
}
```

## 4. Registering Your Plugin

To enable your plugin, register it in the main application entry point:

```go
func main() {
    // ... setup code ...
    myPlugin := &hello.HelloPlugin{}
    pluginManager.Register(myPlugin)
    // ... run app ...
}
```

## 5. Best Practices

*   **Schema First**: Always use `schema.*` types for settings UI. This ensures consistent styling, automatic dirty detection, and Apply button integration across all plugins.
*   **Non-Blocking**: Ensure `Activate()` and `Deactivate()` return quickly. Spawn goroutines for background work.
*   **UI Safety**: All UI updates from background goroutines must be wrapped in `fyne.DoMain` if using raw Fyne calls. The `PluginManager` handles its own thread safety for notifications.
*   **Internationalization**: All user-facing strings must use `i18n.T()` or `i18n.Tf()`. Run `make gen-i18n` after adding new strings.
*   **Settings Persistence**: Use `fyne.Preferences` for storage and declare your UI via schema types in `CreatePrefsPanel`.
