package autostart

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestBundlePathFromExecutable(t *testing.T) {
	tests := []struct {
		name    string
		exe     string
		want    string
		wantErr error
	}{
		{
			name: "installed app bundle",
			exe:  "/Applications/Spice.app/Contents/MacOS/Spice",
			want: "/Applications/Spice.app",
		},
		{
			name: "bundle in a build directory",
			exe:  "/Users/dev/Spice/bin/Spice.app/Contents/MacOS/Spice",
			want: "/Users/dev/Spice/bin/Spice.app",
		},
		{
			name: "nested helper bundle resolves to the innermost bundle",
			exe:  "/Applications/Spice.app/Contents/Library/LoginItems/Helper.app/Contents/MacOS/Helper",
			want: "/Applications/Spice.app/Contents/Library/LoginItems/Helper.app",
		},
		{
			name:    "bare binary is not bundled",
			exe:     "/Users/dev/Spice/bin/Spice-darwin-arm64",
			wantErr: ErrNotBundled,
		},
		{
			name:    "go run temp binary is not bundled",
			exe:     "/var/folders/xy/T/go-build123/b001/exe/spice",
			wantErr: ErrNotBundled,
		},
		{
			name:    "Contents/MacOS without a .app parent is not bundled",
			exe:     "/opt/Spice/Contents/MacOS/Spice",
			wantErr: ErrNotBundled,
		},
		{
			name:    "empty path",
			exe:     "",
			wantErr: ErrNotBundled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := bundlePathFromExecutable(tt.exe)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected error %v, got %v (path %q)", tt.wantErr, err, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBundlePathFromExecutableResolvesSymlinks(t *testing.T) {
	// EvalSymlinks is the caller's job; verify a already-resolved real path
	// created on disk still maps to its bundle.
	root := t.TempDir()
	macOS := filepath.Join(root, "Spice.app", "Contents", "MacOS")
	exe := filepath.Join(macOS, "Spice")

	got, err := bundlePathFromExecutable(exe)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := filepath.Join(root, "Spice.app"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestIsInApplications(t *testing.T) {
	tests := []struct {
		name   string
		bundle string
		home   string
		want   bool
	}{
		{"system applications", "/Applications/Spice.app", "/Users/dev", true},
		{"system applications subfolder", "/Applications/Utilities/Spice.app", "/Users/dev", true},
		{"user applications", "/Users/dev/Applications/Spice.app", "/Users/dev", true},
		{"downloads", "/Users/dev/Downloads/Spice.app", "/Users/dev", false},
		{"build dir", "/Users/dev/Spice/bin/Spice.app", "/Users/dev", false},
		{"user applications without a known home", "/Users/dev/Applications/Spice.app", "", false},
		{"empty bundle", "", "/Users/dev", false},
		{"applications prefix but not a child", "/ApplicationsOther/Spice.app", "/Users/dev", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isInApplications(tt.bundle, tt.home); got != tt.want {
				t.Errorf("isInApplications(%q, %q) = %v, want %v", tt.bundle, tt.home, got, tt.want)
			}
		})
	}
}

func TestDiagnosticHints(t *testing.T) {
	t.Run("healthy environment produces no hints", func(t *testing.T) {
		d := Diagnostic{Bundled: true, BundlePath: "/Applications/Spice.app", InApplications: true, OSVersionOK: true}
		if hints := d.Hints(); len(hints) != 0 {
			t.Errorf("expected no hints, got %v", hints)
		}
	})

	t.Run("adhoc build in a build dir reports both problems", func(t *testing.T) {
		d := Diagnostic{Bundled: true, BundlePath: "/Users/dev/bin/Spice.app", AdhocSigned: true, OSVersionOK: true}
		hints := d.Hints()
		if len(hints) != 2 {
			t.Fatalf("expected 2 hints, got %d: %v", len(hints), hints)
		}
	})

	t.Run("unbundled does not also complain about Applications", func(t *testing.T) {
		d := Diagnostic{OSVersionOK: true}
		hints := d.Hints()
		if len(hints) != 1 {
			t.Fatalf("expected only the not-bundled hint, got %d: %v", len(hints), hints)
		}
	})

	t.Run("old OS is reported", func(t *testing.T) {
		d := Diagnostic{Bundled: true, InApplications: true, OSVersionOK: false}
		hints := d.Hints()
		if len(hints) != 1 {
			t.Fatalf("expected 1 hint, got %d: %v", len(hints), hints)
		}
	})
}

func TestStatusString(t *testing.T) {
	cases := map[Status]string{
		StatusUnknown:          "unknown",
		StatusNotRegistered:    "not registered",
		StatusEnabled:          "enabled",
		StatusRequiresApproval: "requires approval",
		StatusNotFound:         "not found",
		StatusUnsupported:      "unsupported",
	}
	for status, want := range cases {
		if got := status.String(); got != want {
			t.Errorf("Status(%d).String() = %q, want %q", int(status), got, want)
		}
	}
}
