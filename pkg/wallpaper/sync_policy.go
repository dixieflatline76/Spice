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

	// Always check all monitors for resolution changes, even when
	// monitor count is unchanged. The count-only check was a legacy
	// optimization that accidentally skipped resolution change detection.

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
