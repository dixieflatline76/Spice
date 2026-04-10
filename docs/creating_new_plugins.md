# Creating New Plugins for Spice

Spice is designed to be extensible through a modular Plugin architecture. This guide will walk you through creating a new plugin from scratch.

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

## 2. "Hello World" Example

Below is a complete implementation of a simple "Hello World" plugin that adds a toggle to the tray and a welcome message to the settings.

```go
package hello

import (
    "fyne.io/fyne/v2"
    "fyne.io/fyne/v2/container"
    "fyne.io/fyne/v2/widget"
    "github.com/dixieflatline76/Spice/v2/pkg/ui"
    "github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
)

type HelloPlugin struct {
    manager ui.PluginManager
    enabled bool
}

func (p *HelloPlugin) ID() string   { return "hello-world" }
func (p *HelloPlugin) Name() string { return "Hello World" }

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
        fyne.NewMenuItem("Say Hello", func() {
            p.manager.NotifyUser("Spice", "Hello from the tray!")
        }),
    }
}

func (p *HelloPlugin) CreatePrefsPanel(sm setting.SettingsManager) *fyne.Container {
    return container.NewVBox(
        widget.NewLabel("Hello World Plugin Settings"),
        widget.NewCheck("Enable Greetings", func(val bool) {
            if val {
                p.Activate()
            } else {
                p.Deactivate()
            }
        }),
    )
}
```

## 3. Registering Your Plugin

To enable your plugin, you must register it in the main application entry point.

```go
func main() {
    // ... setup code ...
    myPlugin := &hello.HelloPlugin{}
    pluginManager.Register(myPlugin)
    // ... run app ...
}
```

## 4. Best Practices

*   **Non-Blocking**: Ensure `Activate()` and `Deactivate()` return quickly. Spawn goroutines if you need to do background work.
*   **UI Safety**: All UI updates from background goroutines must be wrapped in `fyne.DoMain` if using raw Fyne calls, but the `PluginManager` handles its own thread safety for notifications.
*   **Settings Persistence**: Use the provided `setting.SettingsManager` in `CreatePrefsPanel` to register keys for persistent configuration.
