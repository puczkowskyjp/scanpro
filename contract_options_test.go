package scanpro

import (
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestValidateAndNormalizeOptions_ValidNormalizationAndDefaults(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	other := t.TempDir()

	cfg, err := validateAndNormalizeOptions(ScanOptions{
		Roots: []string{
			filepath.Join(root, "."),
			filepath.Join(root, "a", ".."),
			other,
		},
		MaxDepth:     0, // default/unlimited normalization
		ExcludeNames: []string{"  Vendor  ", "", "tmp"},
		ExcludePaths: []string{
			filepath.Join(root, "x", "..", "skip"),
			filepath.Join(root, "skip"),
		},
	})
	if err != nil {
		t.Fatalf("validateAndNormalizeOptions error: %v", err)
	}

	if cfg.maxDepth != -1 {
		t.Fatalf("expected MaxDepth 0 normalized to -1, got %d", cfg.maxDepth)
	}
	if cfg.symlinkPolicy != SymlinkSkipDirs {
		t.Fatalf("zero-value symlink policy should be SymlinkSkipDirs, got %d", cfg.symlinkPolicy)
	}
	if len(cfg.detectors) != len(BuiltinDetectors()) {
		t.Fatalf("expected builtin detectors by default, got %d", len(cfg.detectors))
	}

	wantRoots := []string{mustNorm(t, other), mustNorm(t, root)}
	if !reflect.DeepEqual(cfg.roots, sortedCopy(wantRoots)) {
		t.Fatalf("roots normalization mismatch: got=%v want=%v", cfg.roots, sortedCopy(wantRoots))
	}

	skipNorm := mustNorm(t, filepath.Join(root, "skip"))
	if len(cfg.excludePaths) != 1 || cfg.excludePaths[0] != skipNorm {
		t.Fatalf("exclude paths not normalized/deduped: %v", cfg.excludePaths)
	}

	if runtime.GOOS == "windows" {
		if _, ok := cfg.excludeNames["vendor"]; !ok {
			t.Fatalf("expected lowercased vendor on windows: %v", cfg.excludeNames)
		}
	} else {
		if _, ok := cfg.excludeNames["Vendor"]; !ok {
			t.Fatalf("expected case preserved on non-windows: %v", cfg.excludeNames)
		}
	}
	if _, ok := cfg.excludeNames["tmp"]; !ok {
		t.Fatalf("missing tmp exclude name")
	}
}

func TestValidateAndNormalizeOptions_InvalidBoundariesAndDetectorConfigs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	absMarker := filepath.Join(root, "abs.marker")

	cases := []struct {
		name string
		opts ScanOptions
	}{
		{
			name: "unsupported symlink policy",
			opts: ScanOptions{Roots: []string{root}, SymlinkPolicy: SymlinkPolicy(99)},
		},
		{
			name: "unsupported boundary mode",
			opts: ScanOptions{Roots: []string{root}, BoundaryMode: BoundaryMode(99)},
		},
		{
			name: "max depth too small",
			opts: ScanOptions{Roots: []string{root}, MaxDepth: -2},
		},
		{
			name: "absolute marker path invalid",
			opts: ScanOptions{
				Roots:     []string{root},
				Detectors: []Detector{{TypeID: "x", Markers: []string{absMarker}}},
			},
		},
		{
			name: "marker with separator invalid",
			opts: ScanOptions{
				Roots:     []string{root},
				Detectors: []Detector{{TypeID: "x", Markers: []string{"a" + string(filepath.Separator) + "b"}}},
			},
		},
		{
			name: "empty type id invalid",
			opts: ScanOptions{
				Roots:     []string{root},
				Detectors: []Detector{{TypeID: "  ", Markers: []string{"x"}}},
			},
		},
		{
			name: "duplicate type ids invalid",
			opts: ScanOptions{
				Roots: []string{root},
				Detectors: []Detector{
					{TypeID: "x", Markers: []string{"a"}},
					{TypeID: "x", Markers: []string{"b"}},
				},
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := validateAndNormalizeOptions(tc.opts); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}

func TestValidateAndNormalizeOptions_DetectorMarkerDedupAndSort(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg, err := validateAndNormalizeOptions(ScanOptions{
		Roots: []string{root},
		Detectors: []Detector{
			{
				TypeID:  "custom",
				Markers: []string{"z.txt", "a.txt", "z.txt", "  a.txt  "},
			},
		},
		EnabledTypes:  []string{" custom ", "go", ""},
		DisabledTypes: []string{"go", " "},
	})
	if err != nil {
		t.Fatalf("validateAndNormalizeOptions error: %v", err)
	}
	if len(cfg.detectors) != 1 {
		t.Fatalf("expected single detector, got %d", len(cfg.detectors))
	}
	if !reflect.DeepEqual(cfg.detectors[0].Markers, []string{"a.txt", "z.txt"}) {
		t.Fatalf("unexpected marker normalization: %v", cfg.detectors[0].Markers)
	}
	if _, ok := cfg.enabledTypes["custom"]; !ok {
		t.Fatalf("enabled type was not trimmed: %v", cfg.enabledTypes)
	}
	if _, ok := cfg.disabledTypes["go"]; !ok {
		t.Fatalf("disabled type missing: %v", cfg.disabledTypes)
	}
}

func TestValidateAndNormalizeOptions_RootPathTrimmed(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	withSpace := "  " + root + "  "
	cfg, err := validateAndNormalizeOptions(ScanOptions{
		Roots: []string{withSpace},
	})
	if err != nil {
		t.Fatalf("validateAndNormalizeOptions error: %v", err)
	}
	if len(cfg.roots) != 1 {
		t.Fatalf("expected one root, got %d", len(cfg.roots))
	}
	if strings.TrimSpace(cfg.roots[0]) != cfg.roots[0] {
		t.Fatalf("normalized root still has spaces: %q", cfg.roots[0])
	}
	if cfg.roots[0] != mustNorm(t, root) {
		t.Fatalf("normalized root mismatch: got=%q want=%q", cfg.roots[0], mustNorm(t, root))
	}
}
