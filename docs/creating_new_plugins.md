# Creating New Plugins for Spice

Spice is designed to be extensible through a flexible Plugin architecture. This guide will walk you through creating a new plugin from scratch.

## 1. The Plugin Interface

All plugins must implement the `ui.Plugin` interface defined in `pkg/ui/plugin.go`:

```go
type Plugin interface {
 Name() string                                             // Unique identifier
 Init(PluginManager)                                       // Dependency Injection
 Activate()                                                // Start background work
 Deactivate()                                              // Cleanup and stop
 CreateTrayMenuItems() []*fyne.MenuItem                    // Add items to system tray
 CreatePrefsPanel(setting.SettingsManager) *fyne.Container // Add tab to Settings UI
}
```

## 2. Step-by-Step Implementation

### Step 1: Define Your Structure

Create a new package or file (e.g., `pkg/myplugin/myplugin.go`) and define your struct.

```go
package myplugin

import (
    "fyne.io/fyne/v2"
    "github.com/dixieflatline76/Spice/pkg/ui"
    "github.com/dixieflatline76/Spice/pkg/ui/setting"
)

type MyPlugin struct {
    manager ui.PluginManager
}
```

### Step 2: Implement Metadata

```go
func (p *MyPlugin) Name() string {
    return "My Awesome Plugin"
}
```

### Step 3: Initialization

Use `Init` to store the manager reference, which gives you access to core services like Preferences, Asset Manager, and Notifications.

```go
func (p *MyPlugin) Init(mgr ui.PluginManager) {
    p.manager = mgr
}
```

### Step 4: Lifecycle Management

`Activate` is called when Spice starts or when your plugin is enabled. `Deactivate` is called on shutdown or disable.

```go
func (p *MyPlugin) Activate() {
    p.manager.NotifyUser("MyPlugin", "I am active!")
    // Start background goroutines here
}

func (p *MyPlugin) Deactivate() {
    // Stop goroutines, close channels
}
```

### Step 5: UI Integration

Add menu items to the tray menu. Return `nil` if you don't need any.

```go
func (p *MyPlugin) CreateTrayMenuItems() []*fyne.MenuItem {
    return []*fyne.MenuItem{
        fyne.NewMenuItem("Do Something", func() {
            p.manager.NotifyUser("Action", "Did something!")
        }),
    }
}
```

Create a settings panel if your plugin needs configuration.

```go
func (p *MyPlugin) CreatePrefsPanel(sm setting.SettingsManager) *fyne.Container {
    return container.NewVBox(
        widget.NewLabel("My Plugin Settings"),
        // Add widgets here
    )
}
```

## 3. Registering Your Plugin

To enable your plugin, you must register it in the main application entry point (usually `main.go` or `cmd/spice/main.go`).

```go
func main() {
    // ... setup code ...
    myParams := &myplugin.MyPlugin{}
    pluginManager.Register(myParams)
    // ... run app ...
}
```
