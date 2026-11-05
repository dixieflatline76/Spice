package wallpaper

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestConfig_ActivityGatePrefs covers defaults, round-trips (TDD step 2.18),
// and the battery level -> percentage mapping for the Activity Gate prefs.
func TestConfig_ActivityGatePrefs(t *testing.T) {
	ResetConfig()
	cfg := GetConfig(NewMockPreferences())

	// --- Defaults (ship-on decision: gate on, game mode off) ---
	assert.True(t, cfg.GetPauseWhenBusy(), "pause-when-busy must default to on")
	assert.True(t, cfg.GetBatterySaver(), "battery saver must default to on")
	assert.Equal(t, 2, cfg.GetBatterySaverLevel(), "battery threshold default index 2 (20%)")
	assert.Equal(t, 20, cfg.BatterySaverThresholdPct(), "default threshold must be 20%")
	assert.False(t, cfg.GetGameMode(), "game mode must default to off")
	assert.Equal(t, "", cfg.GetGateGameProcesses(), "game allowlist must default to empty")
	assert.Equal(t, 2, cfg.GetGateGraceMinutes(), "grace period default 2 minutes")

	// --- Boolean round-trips ---
	cfg.SetPauseWhenBusy(false)
	assert.False(t, cfg.GetPauseWhenBusy())
	cfg.SetPauseWhenBusy(true)
	assert.True(t, cfg.GetPauseWhenBusy())

	cfg.SetBatterySaver(false)
	assert.False(t, cfg.GetBatterySaver())
	cfg.SetBatterySaver(true)
	assert.True(t, cfg.GetBatterySaver())

	cfg.SetGameMode(true)
	assert.True(t, cfg.GetGameMode())
	cfg.SetGameMode(false)
	assert.False(t, cfg.GetGameMode())

	// --- Battery level round-trip + percentage mapping ---
	for idx, wantPct := range map[int]int{0: 10, 1: 15, 2: 20, 3: 25} {
		cfg.SetBatterySaverLevel(idx)
		assert.Equal(t, idx, cfg.GetBatterySaverLevel())
		assert.Equal(t, wantPct, cfg.BatterySaverThresholdPct())
	}
	// Out-of-range index falls back to 20%.
	cfg.SetBatterySaverLevel(9)
	assert.Equal(t, 9, cfg.GetBatterySaverLevel())
	assert.Equal(t, 20, cfg.BatterySaverThresholdPct())

	// --- Allowlist round-trip ---
	cfg.SetGateGameProcesses("cs2.exe, eldenring.exe")
	assert.Equal(t, "cs2.exe, eldenring.exe", cfg.GetGateGameProcesses())
	assert.Equal(t, []string{"cs2.exe", "eldenring.exe"}, cfg.GetGameProcesses())

	// --- Structured GameProcesses domain methods & persistence ---
	assert.NoError(t, cfg.AddGameProcess(`C:\Program Files\Hades\Hades.exe`))
	assert.Equal(t, []string{"cs2.exe", "eldenring.exe", "Hades.exe"}, cfg.GetGameProcesses())
	// Duplicate (case-insensitive) should be ignored cleanly.
	assert.NoError(t, cfg.AddGameProcess("CS2.EXE"))
	assert.Equal(t, []string{"cs2.exe", "eldenring.exe", "Hades.exe"}, cfg.GetGameProcesses())

	assert.NoError(t, cfg.RemoveGameProcess("ELDENRING.EXE"))
	assert.Equal(t, []string{"cs2.exe", "Hades.exe"}, cfg.GetGameProcesses())
	assert.Equal(t, "cs2.exe, Hades.exe", cfg.GetGateGameProcesses())

	// Reload from the same backing Preferences to verify JSON + pref persistence.
	prefs := cfg.Preferences
	ResetConfig()
	reloaded := GetConfig(prefs)
	assert.Equal(t, []string{"cs2.exe", "Hades.exe"}, reloaded.GetGameProcesses())
	assert.Equal(t, "cs2.exe, Hades.exe", reloaded.GetGateGameProcesses())

	// --- Grace period round-trip ---
	reloaded.SetGateGraceMinutes(5)
	assert.Equal(t, 5, reloaded.GetGateGraceMinutes())
}

// TestConfig_ActivityGatePrefKeysUnique guards against key collisions between
// new Activity Gate preference keys and existing preference keys (2.19).
func TestConfig_ActivityGatePrefKeysUnique(t *testing.T) {
	keys := []string{
		SmartFitPrefKey,
		SmartFitModePrefKey,
		CacheSizePrefKey,
		WallpaperChgFreqPrefKey,
		WallpaperChgFreqMinsPrefKey,
		ImgShufflePrefKey,
		ChgImgOnStartPrefKey,
		NightlyRefreshPrefKey,
		FaceBoostPrefKey,
		FaceCropPrefKey,
		StaggerMonitorChangesPrefKey,
		ShortcutsDisabledPrefKey,
		TargetedShortcutsDisabledPrefKey,
		LogLevelPrefKey,
		MaxConcurrentProcessorsPrefKey,
		PauseWhenBusyPrefKey,
		BatterySaverPrefKey,
		BatterySaverLevelPrefKey,
		GameModePrefKey,
		GateGameProcessesPrefKey,
		GateGraceMinutesPrefKey,
	}
	seen := make(map[string]bool, len(keys))
	for _, k := range keys {
		assert.False(t, seen[k], "duplicate pref key: %s", k)
		assert.NotEmpty(t, k, "pref key must not be empty")
		seen[k] = true
	}
}
