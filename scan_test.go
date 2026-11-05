package powerwalk

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestScan_NestedProjectsAndMetadata(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outer := filepath.Join(root, "outer")
	inner := filepath.Join(outer, "inner")
	for _, dir := range []string{outer, inner} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	mustWriteFile(t, filepath.Join(outer, "go.mod"))
	mustWriteFile(t, filepath.Join(inner, "package.json"))

	res, err := Scan(context.Background(), ScanOptions{
		Roots: []string{root},
	})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(res.Projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(res.Projects))
	}
	for _, p := range res.Projects {
		if p.Root == "" {
			t.Fatalf("expected Root metadata on project %+v", p)
		}
		if p.Path == p.Root && p.Depth != 0 {
			t.Fatalf("unexpected depth for root project: %+v", p)
		}
	}
}

func TestScan_PathsWithSpacesAndSpecialCharacters(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	project := filepath.Join(root, "proj [space] #1")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	mustWriteFile(t, filepath.Join(project, "Cargo.toml"))

	res, err := Scan(context.Background(), ScanOptions{Roots: []string{root}})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(res.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(res.Projects))
	}
}

func TestValidateAndNormalizeOptions_Errors(t *testing.T) {
	t.Parallel()

	cases := []ScanOptions{
		{},
		{Roots: []string{"."}, MaxDepth: -2},
		{Roots: []string{"."}, Detectors: []Detector{{TypeID: "", Markers: []string{"a"}}}},
		{Roots: []string{"."}, Detectors: []Detector{{TypeID: "a", Markers: nil}}},
		{Roots: []string{"."}, Detectors: []Detector{{TypeID: "a", Markers: []string{""}}}},
		{Roots: []string{"."}, Detectors: []Detector{
			{TypeID: "a", Markers: []string{"x"}},
			{TypeID: "a", Markers: []string{"y"}},
		}},
	}
	for i, tc := range cases {
		if _, err := validateAndNormalizeOptions(tc); err == nil {
			t.Fatalf("case %d: expected validation error", i)
		}
	}
}

func TestWindowsPathKeyCaseBehavior(t *testing.T) {
	t.Parallel()

	pathA := `C:\Work\Project`
	pathB := `c:\work\project`
	if runtime.GOOS == "windows" {
		if canonicalPathKey(pathA) != canonicalPathKey(pathB) {
			t.Fatalf("expected case-insensitive keys on windows")
		}
		return
	}
	if canonicalPathKey(pathA) == canonicalPathKey(pathB) {
		t.Fatalf("expected case-sensitive keys on non-windows")
	}
}
