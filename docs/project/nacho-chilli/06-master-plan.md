# Spice Unified Master Plan: Event Bus, MCP, & Nacho Chilli

## Goal
Build a cohesive architecture that enables both local AI companions (Nacho Chilli) and external AI assistants (via MCP) to interface with Spice. We will sequence the work starting from the core communication layer (Event Bus) up to the external APIs (MCP) and finally the frontend desktop companion (Nacho Chilli).

---

## Proposed Sequence of Work

### Phase 1: The Core Foundation (Event Bus)
Before any plugin can react to state changes, we need a bi-directional event system.

#### `pkg/ui/eventbus.go`
- Implement a thread-safe `EventBus` interface on the `PluginManager`.
- Expose `Publish(topic string, data interface{})` and `Subscribe(topic string) <-chan interface{}`.

#### `pkg/wallpaper/` (Integration)
- Wire `ImageStore` and `MonitorController` to publish events like `wallpaper.changed`, `store.download_started`, and `store.error`.

---

### Phase 2: The MCP Server Plugin
With the Event Bus active, we can expose Spice to external AI agents (like Claude Desktop, OpenClaw, or custom scripts).

#### `pkg/mcp/mcp.go`
- Scaffold a new `ui.Plugin` that registers the MCP server.
- Provide a `CreatePrefsPanel()` implementation that allows the user to configure the SSE listening port (e.g., default `13306`).

#### `pkg/mcp/server.go`
- We will use the `github.com/metoro-io/mcp-golang` SDK for the server implementation, as it provides automatic JSON schema generation from Go structs and native SSE/HTTP support.
- We will expose an SSE (Server-Sent Events) endpoint so long-running clients can connect and disconnect without restarting Spice.
- Subscribe to the internal `EventBus` and forward events (`wallpaper.changed`) to connected MCP clients.

#### MCP Tools to Expose
The LLM will have access to the following actions. Each action must support targeting a specific monitor ID or "all" monitors:
- `forward`: Skip to the next wallpaper.
- `back`: Revert to the previous wallpaper in history.
- `pause`: Pause or resume the automatic wallpaper scheduler.
- `favorite`: Mark the current image as a favorite.
- `block` (delete): Remove the current image and add it to the avoid list.
- `search`: A free-form search query dispatched across all active online providers.

#### `pkg/provider/provider.go` (Search Extension)
- Add a new optional interface following the existing extension pattern (`ResolutionAwareProvider`, `Syncer`, etc.):
  ```go
  type SearchableProvider interface {
      Search(ctx context.Context, query string) (string, error)
  }
  ```
- `Search` returns an **API URL** (not `[]Image`). This maximizes reuse: the returned URL is directly compatible with `FetchImages(apiURL, page)` and `cfg.AddImageQuery(desc, apiURL, active)`.
- Each provider delegates the query-to-URL translation internally (e.g., Wallhaven constructs a search URL and runs it through `ParseURL`; Pexels does the same via its `/search/{query}` path).
- The MCP `search` tool iterates loaded providers, checks `if sp, ok := p.(provider.SearchableProvider); ok`, calls `Search()`, and **persists the result as a new ImageQuery** via `cfg.AddImageQuery()`. This means the existing fetch loop, pipeline, and rotation machinery handle everything downstream — zero new logic paths.
- Initial implementations: Wallhaven, Pexels. Museums and Wikimedia are likely trivial follow-ups.

---

### Phase 3: Nacho Chilli - Visuals & Window
Building the frontend for the local desktop companion.

#### `pkg/nachochilli/ui/window.go`
- Create a persistent, frameless, always-on-top Fyne window for the sprite.
- Override close behavior to hide the window instead of quitting the app.
- Add tray menu toggles to show/hide the companion.

#### `pkg/nachochilli/ui/sprite.go`
- Implement a Fyne canvas loop for frame-by-frame sprite animation.
- Load the generated pixel art PNG sequences (IDLE, THINKING, SPEAKING).
- Build the `SpriteController` state machine to swap animation sets instantly.

---

### Phase 4: Nacho Chilli - Intelligence & Integration
Wiring the companion's brain to the Event Bus and the local LLM.

#### `pkg/nachochilli/ui/bridge.go`
- Subscribe to the `EventBus` for `wallpaper.changed`.
- Parse the event payload into an `ArtworkMetadata` struct and pass it to the inner ring.

#### `pkg/nachochilli/llm_client.go`
- Pure Go HTTP client (`POST /v1/chat/completions` with `"stream": true`).
- Connect to your local inference server (Lemon / Qwen-Coder on port 13305).
- Stream tokens incrementally into the Fyne speech bubble widget, triggering the `SPEAKING` animation state.

---

## Finalized Decisions

> [!NOTE]
> **Transport & SDK Research:** Based on research, we will use **SSE (Server-Sent Events) over local HTTP** using the **`metoro-io/mcp-golang`** SDK. 
> *Why:* Because Spice is a long-running desktop app, `stdio` transport would incorrectly spawn a second instance. SSE allows external assistants to attach/detach safely. The `metoro-io` library is the most modern Go implementation, offering automatic JSON schema generation and native SSE support, minimizing our boilerplate.

## Verification Plan

### Phase 1 & 2 (MCP)
- Use an external MCP Inspector client to connect to Spice.
- Verify calling `next_wallpaper` changes the background.
- Verify natural wallpaper changes stream correctly to the Inspector.

### Phase 3 & 4 (Nacho Chilli)
- Verify the always-on-top Fyne window behaves correctly without stealing focus.
- Verify the pixel art loops smoothly at 8-12 FPS without memory leaks.
- End-to-End Demo: Wallpaper changes -> Nacho Chilli subscribes via Event Bus -> Enters "Thinking" state -> Connects to Lemon/Qwen -> Streams commentary into speech bubble -> Enters "Speaking" state.
