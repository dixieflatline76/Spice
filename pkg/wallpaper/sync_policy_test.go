package wallpaper

import (
	"image"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSyncPolicy_DetectsResolutionChange is a regression test for a bug where
// the sync policy returned nil when the monitor count was unchanged, even if a
// monitor's resolution changed. The 90-second watcher calls SyncMonitors(false),
// and the old policy had:
//
//	if !force && len(current) == existingCount { return nil }
//
// This caused resolution changes to be invisible until the user manually clicked
// "Refresh Display".
func TestSyncPolicy_DetectsResolutionChange(t *testing.T) {
	policy := NewDefaultSyncPolicy()

	existing := map[int]*MonitorController{
		0: {Monitor: Monitor{ID: 0, Name: "Primary", Rect: image.Rect(0, 0, 1920, 1080)}},
		1: {Monitor: Monitor{ID: 1, Name: "Secondary", Rect: image.Rect(1920, 0, 3520, 1080)}},
	}

	// Same monitors, but monitor 0 changed resolution to 2560x1440
	current := []Monitor{
		{ID: 0, Name: "Primary", Rect: image.Rect(0, 0, 2560, 1440)},
		{ID: 1, Name: "Secondary", Rect: image.Rect(2560, 0, 4160, 1080)},
	}

	// force=false (as the watcher calls it)
	actions := policy.Evaluate(current, existing, false)

	// Should detect resolution changes despite same monitor count
	require.NotEmpty(t, actions, "policy should detect resolution changes even when monitor count is unchanged")

	// Both monitors changed (primary resolution, secondary position)
	updateCount := 0
	for _, a := range actions {
		if a.Type == SyncActionUpdate {
			updateCount++
		}
	}
	assert.GreaterOrEqual(t, updateCount, 1, "at least one SyncActionUpdate expected for resolution change")
}

// TestSyncPolicy_NoActionWhenUnchanged verifies the policy returns no actions
// when monitors haven't changed at all (performance: avoid needless work every 90s).
func TestSyncPolicy_NoActionWhenUnchanged(t *testing.T) {
	policy := NewDefaultSyncPolicy()

	existing := map[int]*MonitorController{
		0: {Monitor: Monitor{ID: 0, Name: "Primary", Rect: image.Rect(0, 0, 1920, 1080)}},
	}

	current := []Monitor{
		{ID: 0, Name: "Primary", Rect: image.Rect(0, 0, 1920, 1080)},
	}

	actions := policy.Evaluate(current, existing, false)
	assert.Empty(t, actions, "no actions expected when monitors are completely unchanged")
}

// TestSyncPolicy_DetectsNewMonitor verifies the policy detects a newly plugged-in monitor.
func TestSyncPolicy_DetectsNewMonitor(t *testing.T) {
	policy := NewDefaultSyncPolicy()

	existing := map[int]*MonitorController{
		0: {Monitor: Monitor{ID: 0, Name: "Primary", Rect: image.Rect(0, 0, 1920, 1080)}},
	}

	current := []Monitor{
		{ID: 0, Name: "Primary", Rect: image.Rect(0, 0, 1920, 1080)},
		{ID: 1, Name: "New Monitor", Rect: image.Rect(1920, 0, 3840, 1080)},
	}

	actions := policy.Evaluate(current, existing, false)
	require.Len(t, actions, 1)
	assert.Equal(t, SyncActionCreate, actions[0].Type)
	assert.Equal(t, 1, actions[0].MonitorID)
}

// TestSyncPolicy_DetectsRemovedMonitor verifies the policy detects an unplugged monitor.
func TestSyncPolicy_DetectsRemovedMonitor(t *testing.T) {
	policy := NewDefaultSyncPolicy()

	existing := map[int]*MonitorController{
		0: {Monitor: Monitor{ID: 0, Name: "Primary", Rect: image.Rect(0, 0, 1920, 1080)}},
		1: {Monitor: Monitor{ID: 1, Name: "Secondary", Rect: image.Rect(1920, 0, 3840, 1080)}},
	}

	current := []Monitor{
		{ID: 0, Name: "Primary", Rect: image.Rect(0, 0, 1920, 1080)},
	}

	actions := policy.Evaluate(current, existing, false)
	require.Len(t, actions, 1)
	assert.Equal(t, SyncActionRemove, actions[0].Type)
	assert.Equal(t, 1, actions[0].MonitorID)
}

// Rearranging displays on macOS/Linux changes Bounds but not Rect.
// Must refresh metadata WITHOUT triggering a resolution-change Update.
func TestSyncPolicy_BoundsOnlyChange_EmitsRefresh(t *testing.T) {
	policy := NewDefaultSyncPolicy()
	existing := map[int]*MonitorController{
		0: {Monitor: Monitor{ID: 0, Name: "Built-in", Rect: image.Rect(0, 0, 3456, 2234), Bounds: image.Rect(0, 0, 1728, 1117), Primary: true}},
		1: {Monitor: Monitor{ID: 1, Name: "Studio", Rect: image.Rect(0, 0, 5120, 2880), Bounds: image.Rect(1728, 0, 4288, 1440)}},
	}
	current := []Monitor{
		{ID: 0, Name: "Built-in", Rect: image.Rect(0, 0, 3456, 2234), Bounds: image.Rect(0, 0, 1728, 1117), Primary: true},
		{ID: 1, Name: "Studio", Rect: image.Rect(0, 0, 5120, 2880), Bounds: image.Rect(-2560, 0, 0, 1440)},
	}

	actions := policy.Evaluate(current, existing, false)
	require.Len(t, actions, 1)
	assert.Equal(t, SyncActionRefresh, actions[0].Type)
	assert.Equal(t, 1, actions[0].MonitorID)
	assert.Equal(t, image.Rect(-2560, 0, 0, 1440), actions[0].Monitor.Bounds)
}

func TestSyncPolicy_NoChange_EmitsNothing(t *testing.T) {
	policy := NewDefaultSyncPolicy()
	m := Monitor{ID: 0, Name: "A", DevicePath: "p0", Rect: image.Rect(0, 0, 1920, 1080), Bounds: image.Rect(0, 0, 1920, 1080), Primary: true}
	existing := map[int]*MonitorController{0: {Monitor: m}}
	assert.Empty(t, policy.Evaluate([]Monitor{m}, existing, false))
}

// When the physical device behind an ID changes (e.g. Windows primary switched),
// the controller must be replaced so State/pause follow the physical screen.
func TestSyncPolicy_DevicePathChange_EmitsReplace(t *testing.T) {
	policy := NewDefaultSyncPolicy()
	existing := map[int]*MonitorController{
		0: {Monitor: Monitor{ID: 0, DevicePath: "path-A", Rect: image.Rect(0, 0, 2560, 1440), Bounds: image.Rect(0, 0, 2560, 1440), Primary: true}},
	}
	current := []Monitor{
		{ID: 0, DevicePath: "path-B", Rect: image.Rect(0, 0, 1920, 1080), Bounds: image.Rect(0, 0, 1920, 1080), Primary: true},
	}
	actions := policy.Evaluate(current, existing, false)
	require.Len(t, actions, 1)
	assert.Equal(t, SyncActionReplace, actions[0].Type)
	assert.Equal(t, "path-B", actions[0].Monitor.DevicePath)
}

// Empty DevicePaths (fallback monitors, legacy tests) must never trigger Replace.
func TestSyncPolicy_EmptyDevicePath_NoReplace(t *testing.T) {
	policy := NewDefaultSyncPolicy()
	existing := map[int]*MonitorController{
		0: {Monitor: Monitor{ID: 0, DevicePath: "", Rect: image.Rect(0, 0, 1920, 1080)}},
	}
	current := []Monitor{{ID: 0, DevicePath: "path-B", Rect: image.Rect(0, 0, 1920, 1080)}}
	for _, a := range policy.Evaluate(current, existing, false) {
		assert.NotEqual(t, SyncActionReplace, a.Type)
	}
}
