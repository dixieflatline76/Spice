package activity

import "testing"

func TestBatteryStateString_ACNoBattery(t *testing.T) {
	s := BatteryState{OnAC: true, HasBattery: false}
	if got := s.String(); got != "ac" {
		t.Errorf("String() = %q, want 'ac'", got)
	}
}

func TestParseDecimal_NonDigit(t *testing.T) {
	if got := parseDecimal("123abc"); got != 123 {
		t.Errorf("parseDecimal('123abc') = %d, want 123", got)
	}
	if got := parseDecimal("abc"); got != 0 {
		t.Errorf("parseDecimal('abc') = %d, want 0", got)
	}
}

func TestRectArea_Zero(t *testing.T) {
	if got := (Rect{W: 0, H: 100}).area(); got != 0 {
		t.Errorf("area = %d, want 0", got)
	}
	if got := (Rect{W: 100, H: 0}).area(); got != 0 {
		t.Errorf("area = %d, want 0", got)
	}
}

func TestIsFullscreenGeometry_EdgeCases(t *testing.T) {
	// Zero area window
	if IsFullscreenGeometry(Rect{}, []Rect{{W: 1920, H: 1080}}, 0) {
		t.Error("zero win area should not be fullscreen")
	}
	// Zero area monitor in slice
	if IsFullscreenGeometry(Rect{W: 1920, H: 1080}, []Rect{{}}, 0) {
		t.Error("zero monitor area should not match")
	}
}

func TestClassifyWindow_EdgeCases(t *testing.T) {
	display := Rect{X: 0, Y: 0, W: 2560, H: 1440}
	full := display

	// Empty display area
	if got := ClassifyWindow("Steam", 0, full, Rect{}, "Spice"); got != BusyNone {
		t.Errorf("zero display area should be BusyNone, got %v", got)
	}
	// Empty frame area
	if got := ClassifyWindow("Steam", 0, Rect{}, display, "Spice"); got != BusyNone {
		t.Errorf("zero frame area should be BusyNone, got %v", got)
	}
}

func TestParseGpuEngineInstances_MissingPid(t *testing.T) {
	rows := []GpuEngineRow{
		{InstanceName: "luid_0x0_engtype_3D", Value: 50.0},
	}
	if got := ParseGpuEngineInstances(rows); len(got) != 0 {
		t.Errorf("expected empty map when pid_ is missing, got %+v", got)
	}
}

func TestNormalizeProcessName_EdgeCases(t *testing.T) {
	if got := normalizeProcessName("   "); got != "" {
		t.Errorf("whitespace only should normalize to empty, got %q", got)
	}
	if got := normalizeProcessName(`C:\Games\steam\`); got != "" {
		t.Errorf("trailing backslash should normalize to empty, got %q", got)
	}
}
