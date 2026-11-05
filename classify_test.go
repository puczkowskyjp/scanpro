package powerwalk

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBuiltinDetectors_ClassifyDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "go.mod"))
	mustWriteFile(t, filepath.Join(dir, "package.json"))
	mustWriteFile(t, filepath.Join(dir, "Cargo.toml"))
	mustWriteFile(t, filepath.Join(dir, "app.sln"))
	mustWriteFile(t, filepath.Join(dir, "service.csproj"))
	mustWriteFile(t, filepath.Join(dir, "pyproject.toml"))

	got, err := ClassifyDir(context.Background(), dir, ScanOptions{Roots: []string{dir}})
	if err != nil {
		t.Fatalf("ClassifyDir returned error: %v", err)
	}

	typeIDs := make([]string, 0, len(got.Matches))
	for _, m := range got.Matches {
		typeIDs = append(typeIDs, m.TypeID)
	}
	want := []string{"dotnet", "go", "node", "python", "rust"}
	if !reflect.DeepEqual(typeIDs, want) {
		t.Fatalf("typeIDs mismatch: got %v want %v", typeIDs, want)
	}
}

func TestClassifyDir_CustomDetectorAndDisabledBuiltIn(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "package.json"))
	mustWriteFile(t, filepath.Join(dir, "myproj.custom"))

	opts := ScanOptions{
		Roots: []string{dir},
		Detectors: append(BuiltinDetectors(), Detector{
			TypeID:  "custom-x",
			Markers: []string{"*.custom"},
		}),
		DisabledTypes: []string{"node"},
	}

	got, err := ClassifyDir(context.Background(), dir, opts)
	if err != nil {
		t.Fatalf("ClassifyDir returned error: %v", err)
	}

	if len(got.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(got.Matches))
	}
	if got.Matches[0].TypeID != "custom-x" {
		t.Fatalf("unexpected type: %q", got.Matches[0].TypeID)
	}
	if !reflect.DeepEqual(got.Matches[0].Markers, []string{"*.custom"}) {
		t.Fatalf("unexpected markers: %v", got.Matches[0].Markers)
	}
}

func TestClassifyDir_MultipleMarkersSameTypeAndAllowlist(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "a.sln"))
	mustWriteFile(t, filepath.Join(dir, "b.csproj"))
	mustWriteFile(t, filepath.Join(dir, "go.mod"))

	got, err := ClassifyDir(context.Background(), dir, ScanOptions{
		Roots:        []string{dir},
		EnabledTypes: []string{"dotnet"},
	})
	if err != nil {
		t.Fatalf("ClassifyDir returned error: %v", err)
	}
	if len(got.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(got.Matches))
	}
	if got.Matches[0].TypeID != "dotnet" {
		t.Fatalf("unexpected type %q", got.Matches[0].TypeID)
	}
	if !reflect.DeepEqual(got.Matches[0].Markers, []string{"*.csproj", "*.sln"}) {
		t.Fatalf("unexpected markers: %v", got.Matches[0].Markers)
	}
}

func TestScan_UnknownProjectType(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	project := filepath.Join(root, "sample")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mustWriteFile(t, filepath.Join(project, "project.abc"))

	res, err := Scan(context.Background(), ScanOptions{
		Roots: []string{root},
		Detectors: []Detector{
			{TypeID: "unknown-abc", Markers: []string{"*.abc"}},
		},
	})
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(res.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(res.Projects))
	}
	if res.Projects[0].Matches[0].TypeID != "unknown-abc" {
		t.Fatalf("unexpected type id: %q", res.Projects[0].Matches[0].TypeID)
	}
}
