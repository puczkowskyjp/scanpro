package scanpro

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
)

func TestClassifyDir_BuiltinDetectors_AllMarkers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		files      []string
		wantType   string
		wantMarker []string
	}{
		{name: "go marker", files: []string{"go.mod"}, wantType: "go", wantMarker: []string{"go.mod"}},
		{name: "node marker", files: []string{"package.json"}, wantType: "node", wantMarker: []string{"package.json"}},
		{name: "dotnet sln", files: []string{"app.sln"}, wantType: "dotnet", wantMarker: []string{"*.sln"}},
		{name: "dotnet slnx", files: []string{"app.slnx"}, wantType: "dotnet", wantMarker: []string{"*.slnx"}},
		{name: "dotnet csproj", files: []string{"app.csproj"}, wantType: "dotnet", wantMarker: []string{"*.csproj"}},
		{name: "dotnet fsproj", files: []string{"app.fsproj"}, wantType: "dotnet", wantMarker: []string{"*.fsproj"}},
		{name: "dotnet vbproj", files: []string{"app.vbproj"}, wantType: "dotnet", wantMarker: []string{"*.vbproj"}},
		{name: "python pyproject", files: []string{"pyproject.toml"}, wantType: "python", wantMarker: []string{"pyproject.toml"}},
		{name: "python setup", files: []string{"setup.py"}, wantType: "python", wantMarker: []string{"setup.py"}},
		{name: "python requirements", files: []string{"requirements.txt"}, wantType: "python", wantMarker: []string{"requirements.txt"}},
		{name: "rust marker", files: []string{"Cargo.toml"}, wantType: "rust", wantMarker: []string{"Cargo.toml"}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			for _, f := range tc.files {
				mustWriteFile(t, filepath.Join(dir, f))
			}
			got, err := ClassifyDir(context.Background(), dir, ScanOptions{Roots: []string{dir}})
			if err != nil {
				t.Fatalf("ClassifyDir error: %v", err)
			}
			if len(got.Matches) != 1 {
				t.Fatalf("expected 1 match, got %d (%+v)", len(got.Matches), got.Matches)
			}
			if got.Matches[0].TypeID != tc.wantType {
				t.Fatalf("type mismatch: got=%q want=%q", got.Matches[0].TypeID, tc.wantType)
			}
			if !reflect.DeepEqual(got.Matches[0].Markers, tc.wantMarker) {
				t.Fatalf("marker mismatch: got=%v want=%v", got.Matches[0].Markers, tc.wantMarker)
			}
		})
	}
}

func TestClassifyDir_DisabledAndEnabledTypeFiltering(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "go.mod"))
	mustWriteFile(t, filepath.Join(dir, "package.json"))

	t.Run("allowlist only", func(t *testing.T) {
		got, err := ClassifyDir(context.Background(), dir, ScanOptions{
			Roots:        []string{dir},
			EnabledTypes: []string{"go"},
		})
		if err != nil {
			t.Fatalf("ClassifyDir error: %v", err)
		}
		if len(got.Matches) != 1 || got.Matches[0].TypeID != "go" {
			t.Fatalf("unexpected matches: %+v", got.Matches)
		}
	})

	t.Run("denylist wins", func(t *testing.T) {
		got, err := ClassifyDir(context.Background(), dir, ScanOptions{
			Roots:         []string{dir},
			EnabledTypes:  []string{"go", "node"},
			DisabledTypes: []string{"node"},
		})
		if err != nil {
			t.Fatalf("ClassifyDir error: %v", err)
		}
		if len(got.Matches) != 1 || got.Matches[0].TypeID != "go" {
			t.Fatalf("unexpected matches: %+v", got.Matches)
		}
	})
}

func TestClassifyDir_CustomDetectorFilenameAndGlob(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "tool.lock"))
	mustWriteFile(t, filepath.Join(dir, "service.custom"))

	got, err := ClassifyDir(context.Background(), dir, ScanOptions{
		Roots: []string{dir},
		Detectors: []Detector{
			{TypeID: "custom-a", Markers: []string{"tool.lock", "*.custom"}},
		},
	})
	if err != nil {
		t.Fatalf("ClassifyDir error: %v", err)
	}
	if len(got.Matches) != 1 {
		t.Fatalf("expected custom match, got %+v", got.Matches)
	}
	if !reflect.DeepEqual(got.Matches[0].Markers, []string{"*.custom", "tool.lock"}) {
		t.Fatalf("unexpected markers: %v", got.Matches[0].Markers)
	}
}

func TestClassifyDir_DoesNotRequireRoots(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "go.mod"))

	got, err := ClassifyDir(context.Background(), dir, ScanOptions{})
	if err != nil {
		t.Fatalf("ClassifyDir should not require Roots, got error: %v", err)
	}
	if len(got.Matches) != 1 || got.Matches[0].TypeID != "go" {
		t.Fatalf("unexpected matches: %+v", got.Matches)
	}
}
