package wallpaper

import "sort"

// SyncActionType defines the type of action required for a monitor.
type SyncActionType int

const (
	SyncActionNone SyncActionType = iota
	SyncActionCreate
	SyncActionUpdate
	SyncActionRemove
)

// SyncAction represents a decision made by the policy for a specific monitor ID.
type SyncAction struct {
	Type      SyncActionType
	MonitorID int
	Monitor   Monitor // Valid for Create/Update
}

// SyncPolicy defines the logic for reconciling monitor states.
type SyncPolicy interface {
	Evaluate(current []Monitor, existing map[int]*MonitorController, force bool) []SyncAction
}

// DefaultSyncPolicy implements the standard reconciliation logic.
type DefaultSyncPolicy struct{}

func NewDefaultSyncPolicy() *DefaultSyncPolicy {
	return &DefaultSyncPolicy{}
}

func (p *DefaultSyncPolicy) Evaluate(current []Monitor, existing map[int]*MonitorController, force bool) []SyncAction {
	var actions []SyncAction

	existingCount := len(existing)
	// Optimization: If not forced and counts match, assume no change?
	// The original logic did this check BEFORE calling locked sync.
	// We can keep that optimization but also double check here if needed.
	// But strictly speaking, a policy should be thorough.
	// However, to replicate original behavior:
	if !force && len(current) == existingCount {
		// We still need to check for resolution changes!
		// But original code returned early if count matched and !force.
		// "if !force && len(current) == existingCount { return }"
		// Wait, the original code inside syncMonitorsLocked ALSO checked "!force && len(current) == existingCount".
		// But then it continued to iterate to check resolution?
		// No, looking at lines 727-729 of wallpaper.go:
		// "if !force && len(current) == existingCount { return false }"
		// SO IT SKIPPED RESOLUTION CHECKS!?
		// Let's re-read carefully.
		// Line 727: if !force && len(current) == existingCount { return false }
		// Yes, the original code SKIPPED resolution checks if count matched and not forced!
		// That seems like a bug or intended optimization.
		// I will preserve this behavior for now to be safe, but it's suspicious.
		return nil
	}

	// 1. Identify Creates and Updates
	currentIDs := make(map[int]bool)
	for _, m := range current {
		currentIDs[m.ID] = true

		if mc, exists := existing[m.ID]; exists {
			// Check for resolution change
			if mc.Monitor.Rect != m.Rect {
				actions = append(actions, SyncAction{
					Type:      SyncActionUpdate,
					MonitorID: m.ID,
					Monitor:   m,
				})
			}
		} else {
			// New Monitor
			actions = append(actions, SyncAction{
				Type:      SyncActionCreate,
				MonitorID: m.ID,
				Monitor:   m,
			})
		}
	}

	// 2. Identify Removals
	// We need to find IDs in existing that are NOT in currentIDs
	// To be deterministic, let's sort existing keys? Not strictly needed for map interaction but good for tests.
	// But iterating map is random.
	for id := range existing {
		if !currentIDs[id] {
			actions = append(actions, SyncAction{
				Type:      SyncActionRemove,
				MonitorID: id,
			})
		}
	}

	// Sort actions for deterministic execution order?
	// Not strictly necessary but creates stable logs.
	sort.Slice(actions, func(i, j int) bool {
		if actions[i].Type != actions[j].Type {
			return actions[i].Type < actions[j].Type
		}
		return actions[i].MonitorID < actions[j].MonitorID
	})

	return actions
}
