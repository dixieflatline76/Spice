package wallpaper

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/dixieflatline76/Spice/v2/util/log"
)

// monitorStateFileName holds the per-monitor wallpaper state between runs.
const monitorStateFileName = "monitor_state.json"

// monitorStateVersion is bumped when the on-disk shape changes incompatibly.
const monitorStateVersion = 1

// monitorStateDebounce coalesces the burst of writes that a multi-monitor
// wallpaper change produces.
const monitorStateDebounce = 500 * time.Millisecond

// PersistedMonitorState is the slice of a monitor's state that must survive a
// restart.
//
// macOS does not record wallpapers set through NSWorkspace in its own
// persistent store, so at the next login the system restores whatever it had
// and the desktop reverts to the default. Spice therefore has to remember what
// it applied and put it back itself.
type PersistedMonitorState struct {
	Key         string    `json:"key"`
	DevicePath  string    `json:"device_path"`
	Index       int       `json:"index"`
	CurrentID   string    `json:"current_id"`
	AppliedPath string    `json:"applied_path"`
	ResKey      string    `json:"res_key"`
	RandomPos   int       `json:"random_pos"`
	ShuffleIDs  []string  `json:"shuffle_ids,omitempty"`
	History     []string  `json:"history,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// monitorStateFile is the on-disk envelope.
type monitorStateFile struct {
	Version  int                              `json:"version"`
	Monitors map[string]PersistedMonitorState `json:"monitors"`
}

// monitorStateKey identifies a monitor across runs.
//
// DevicePath alone is not unique: on macOS it is NSScreen.localizedName, which
// two identical displays share. Pairing it with the index disambiguates the
// common case, and lookup falls back to the bare DevicePath so a single-display
// user whose index shifted still matches.
func monitorStateKey(devicePath string, index int) string {
	return devicePath + "#" + strconv.Itoa(index)
}

// monitorStateStore reads and writes the per-monitor state file.
//
// It is safe for concurrent use, debounces writes, and tolerates a nil
// receiver: a Plugin assembled without Init (as tests do) simply persists
// nothing rather than crashing.
type monitorStateStore struct {
	mu      sync.Mutex
	path    string
	entries map[string]PersistedMonitorState

	timer    *time.Timer
	debounce time.Duration
}

// newMonitorStateStore creates a store backed by the given file path.
func newMonitorStateStore(path string) *monitorStateStore {
	return &monitorStateStore{
		path:     path,
		entries:  make(map[string]PersistedMonitorState),
		debounce: monitorStateDebounce,
	}
}

// Load reads the state file into memory. A missing or unreadable file is not an
// error: it just means there is nothing to restore. A corrupt file is reported
// and discarded rather than allowed to abort startup.
func (s *monitorStateStore) Load() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[MonitorState] Could not read %s: %v", s.path, err)
		}
		return
	}

	var file monitorStateFile
	if err := json.Unmarshal(data, &file); err != nil {
		log.Printf("[MonitorState] Ignoring corrupt state file %s: %v", s.path, err)
		return
	}

	if file.Version != monitorStateVersion {
		log.Printf("[MonitorState] Ignoring state file with unsupported version %d (want %d).", file.Version, monitorStateVersion)
		return
	}

	if file.Monitors != nil {
		s.entries = file.Monitors
	}
	log.Debugf("[MonitorState] Loaded state for %d monitor(s).", len(s.entries))
}

// Lookup returns the stored state for a monitor, preferring an exact
// device+index match and falling back to any entry with the same device path.
func (s *monitorStateStore) Lookup(devicePath string, index int) (PersistedMonitorState, bool) {
	if s == nil {
		return PersistedMonitorState{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if ps, ok := s.entries[monitorStateKey(devicePath, index)]; ok {
		return ps, true
	}

	if devicePath != "" {
		for _, ps := range s.entries {
			if ps.DevicePath == devicePath {
				return ps, true
			}
		}
	}

	return PersistedMonitorState{}, false
}

// CurrentIDs returns every image ID recorded as being on screen. Cleanup uses
// this so a pass that runs before the first apply of the session cannot delete
// the image that is about to be restored.
func (s *monitorStateStore) CurrentIDs() map[string]bool {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	ids := make(map[string]bool, len(s.entries))
	for _, ps := range s.entries {
		if ps.CurrentID != "" {
			ids[ps.CurrentID] = true
		}
	}
	return ids
}

// Record stores a monitor snapshot and schedules a debounced write.
func (s *monitorStateStore) Record(ps PersistedMonitorState) {
	if s == nil {
		return
	}
	s.mu.Lock()
	ps.Key = monitorStateKey(ps.DevicePath, ps.Index)
	s.entries[ps.Key] = ps

	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(s.debounce, func() { s.Flush() })
	s.mu.Unlock()
}

// Flush writes the current state to disk immediately, cancelling any pending
// debounced write. Call it on shutdown so the last change is not lost.
func (s *monitorStateStore) Flush() {
	if s == nil {
		return
	}
	s.mu.Lock()

	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}

	if s.path == "" {
		s.mu.Unlock()
		return
	}

	snapshot := monitorStateFile{
		Version:  monitorStateVersion,
		Monitors: make(map[string]PersistedMonitorState, len(s.entries)),
	}
	for k, v := range s.entries {
		snapshot.Monitors[k] = v
	}
	path := s.path
	s.mu.Unlock()

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		log.Printf("[MonitorState] Could not encode state: %v", err)
		return
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		log.Printf("[MonitorState] Could not create state directory: %v", err)
		return
	}

	// Write-then-rename so a crash mid-write cannot leave a truncated file
	// that would silently discard the user's wallpaper on the next launch.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		log.Printf("[MonitorState] Could not write state: %v", err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		log.Printf("[MonitorState] Could not commit state: %v", err)
		_ = os.Remove(tmp)
	}
}
