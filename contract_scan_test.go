package scanpro

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestScan_ProjectMetadataAndMarkersAcrossLevels(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	rootProject := filepath.Join(root, "root-proj")
	nestedContainer := filepath.Join(root, "apps", "service")
	deepNested := filepath.Join(nestedContainer, "worker")
	plainDir := filepath.Join(root, "docs")
	for _, d := range []string{rootProject, nestedContainer, deepNested, plainDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}

	// root-level project under scan root
	mustWriteFile(t, filepath.Join(rootProject, "go.mod"))
	// multiple types in one directory
	mustWriteFile(t, filepath.Join(nestedContainer, "package.json"))
	mustWriteFile(t, filepath.Join(nestedContainer, "pyproject.toml"))
	// nested distinct type
	mustWriteFile(t, filepath.Join(deepNested, "Cargo.toml"))

	res, err := Scan(context.Background(), ScanOptions{Roots: []string{root}})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(res.Issues) != 0 {
		t.Fatalf("unexpected issues: %+v", res.Issues)
	}

	got := projectsByPath(t, res.Projects)
	wantPaths := []string{
		mustNorm(t, rootProject),
		mustNorm(t, nestedContainer),
		mustNorm(t, deepNested),
	}
	if !reflect.DeepEqual(sortedKeys(got), sortedCopy(wantPaths)) {
		t.Fatalf("project paths mismatch: got=%v want=%v", sortedKeys(got), sortedCopy(wantPaths))
	}

	assertProject(t, got[mustNorm(t, rootProject)], mustNorm(t, root), 1, map[string][]string{
		"go": {"go.mod"},
	})
	assertProject(t, got[mustNorm(t, nestedContainer)], mustNorm(t, root), 2, map[string][]string{
		"node":   {"package.json"},
		"python": {"pyproject.toml"},
	})
	assertProject(t, got[mustNorm(t, deepNested)], mustNorm(t, root), 3, map[string][]string{
		"rust": {"Cargo.toml"},
	})
}

func TestScan_DeterministicOrderingAndNoMarkerDirsExcludedFromProjects(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := []string{
		filepath.Join(root, "z-no-marker"),
		filepath.Join(root, "a-proj"),
		filepath.Join(root, "m-proj"),
	}
	for _, p := range paths {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", p, err)
		}
	}
	mustWriteFile(t, filepath.Join(root, "a-proj", "go.mod"))
	mustWriteFile(t, filepath.Join(root, "m-proj", "package.json"))

	res1, err := Scan(context.Background(), ScanOptions{Roots: []string{root}})
	if err != nil {
		t.Fatalf("Scan #1 error: %v", err)
	}
	res2, err := Scan(context.Background(), ScanOptions{Roots: []string{root}})
	if err != nil {
		t.Fatalf("Scan #2 error: %v", err)
	}
	if !reflect.DeepEqual(res1, res2) {
		t.Fatalf("scan results are not deterministic")
	}
	if len(res1.Projects) != 2 {
		t.Fatalf("expected only marker directories as projects, got %d", len(res1.Projects))
	}
	if res1.Projects[0].Path > res1.Projects[1].Path {
		t.Fatalf("projects not sorted by path: %v then %v", res1.Projects[0].Path, res1.Projects[1].Path)
	}
}

func TestScan_MaxDepthBoundaries(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	d0 := root
	d1 := filepath.Join(root, "d1")
	d2 := filepath.Join(d1, "d2")
	for _, d := range []string{d1, d2} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	mustWriteFile(t, filepath.Join(d0, "go.mod"))
	mustWriteFile(t, filepath.Join(d1, "package.json"))
	mustWriteFile(t, filepath.Join(d2, "Cargo.toml"))

	cases := []struct {
		name     string
		maxDepth int
		want     []string
	}{
		{name: "zero means unlimited", maxDepth: 0, want: []string{d0, d1, d2}},
		{name: "minus one unlimited", maxDepth: -1, want: []string{d0, d1, d2}},
		{name: "depth one", maxDepth: 1, want: []string{d0, d1}},
		{name: "depth two", maxDepth: 2, want: []string{d0, d1, d2}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			res, err := Scan(context.Background(), ScanOptions{
				Roots:    []string{root},
				MaxDepth: tc.maxDepth,
			})
			if err != nil {
				t.Fatalf("Scan error: %v", err)
			}
			got := make([]string, 0, len(res.Projects))
			for _, p := range res.Projects {
				got = append(got, p.Path)
			}
			want := make([]string, 0, len(tc.want))
			for _, p := range tc.want {
				want = append(want, mustNorm(t, p))
			}
			if !reflect.DeepEqual(sortedCopy(got), sortedCopy(want)) {
				t.Fatalf("paths mismatch: got=%v want=%v", sortedCopy(got), sortedCopy(want))
			}
		})
	}
}

func TestScan_ExcludeNamesAndPaths(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	keep := filepath.Join(root, "keep")
	skipName := filepath.Join(root, "vendor")
	skipPath := filepath.Join(root, "apps", "legacy")
	for _, d := range []string{keep, skipName, skipPath} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	mustWriteFile(t, filepath.Join(keep, "go.mod"))
	mustWriteFile(t, filepath.Join(skipName, "go.mod"))
	mustWriteFile(t, filepath.Join(skipPath, "Cargo.toml"))

	res, err := Scan(context.Background(), ScanOptions{
		Roots:        []string{root},
		ExcludeNames: []string{"vendor"},
		ExcludePaths: []string{skipPath},
	})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	got := projectPaths(res.Projects)
	if !containsNormPath(got, keep) {
		t.Fatalf("expected kept path: %s", keep)
	}
	if containsNormPath(got, skipName) {
		t.Fatalf("unexpected excluded-by-name path: %s", skipName)
	}
	if containsNormPath(got, skipPath) {
		t.Fatalf("unexpected excluded-by-path path: %s", skipPath)
	}
}

func TestScan_RootNormalizationAndDedupViaPublicAPI(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "proj"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mustWriteFile(t, filepath.Join(root, "proj", "go.mod"))

	res, err := Scan(context.Background(), ScanOptions{
		Roots: []string{
			root,
			filepath.Join(root, "."),
			filepath.Join(root, "proj", ".."),
		},
	})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(res.Projects) != 1 {
		t.Fatalf("expected 1 deduped project, got %d", len(res.Projects))
	}
	if res.Projects[0].Root != mustNorm(t, root) {
		t.Fatalf("unexpected normalized root: %q", res.Projects[0].Root)
	}
}

func TestScan_OverlappingRootsDedupAndRootMetadata(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sub := filepath.Join(root, "apps")
	project := filepath.Join(sub, "svc")
	for _, d := range []string{sub, project} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	mustWriteFile(t, filepath.Join(project, "package.json"))

	res, err := Scan(context.Background(), ScanOptions{
		Roots: []string{sub, root},
	})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(res.Projects) != 1 {
		t.Fatalf("expected deduped project once, got %d", len(res.Projects))
	}

	// roots are normalized + sorted; metadata root is deterministic first discover root
	wantRoot := mustNorm(t, root)
	if mustNorm(t, sub) < wantRoot {
		wantRoot = mustNorm(t, sub)
	}
	if res.Projects[0].Root != wantRoot {
		t.Fatalf("unexpected project root metadata: got %q want %q", res.Projects[0].Root, wantRoot)
	}
}

func TestScan_NonexistentAndFileRootsAreRecoverableIssues(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fileRoot := filepath.Join(root, "not-dir.txt")
	mustWriteFile(t, fileRoot)
	nonexistent := filepath.Join(root, "does-not-exist")
	validProj := filepath.Join(root, "ok")
	if err := os.MkdirAll(validProj, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mustWriteFile(t, filepath.Join(validProj, "go.mod"))

	res, err := Scan(context.Background(), ScanOptions{
		Roots: []string{fileRoot, nonexistent, root},
	})
	if err != nil {
		t.Fatalf("Scan should continue on recoverable root errors: %v", err)
	}
	if len(res.Projects) != 1 {
		t.Fatalf("expected valid root project still returned, got %d", len(res.Projects))
	}
	if len(res.Issues) < 2 {
		t.Fatalf("expected issues for file root and nonexistent root, got %+v", res.Issues)
	}

	var sawFileRootCheck bool
	var sawNonexistentLstat bool
	for _, is := range res.Issues {
		if canonicalPathKey(is.Path) == canonicalPathKey(fileRoot) && is.Operation == "root_check" {
			sawFileRootCheck = true
		}
		if canonicalPathKey(is.Path) == canonicalPathKey(nonexistent) && is.Operation == "lstat" {
			sawNonexistentLstat = true
		}
	}
	if !sawFileRootCheck || !sawNonexistentLstat {
		t.Fatalf("missing expected root issues: %+v", res.Issues)
	}
}

func TestScan_CancellationBeforeAndDuring(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	current := root
	for i := 0; i < 250; i++ {
		current = filepath.Join(current, "d")
		if err := os.MkdirAll(current, 0o755); err != nil {
			t.Fatalf("mkdir deep %d: %v", i, err)
		}
		mustWriteFile(t, filepath.Join(current, "go.mod"))
	}

	preCtx, preCancel := context.WithCancel(context.Background())
	preCancel()
	if _, err := Scan(preCtx, ScanOptions{Roots: []string{root}}); err == nil {
		t.Fatalf("expected pre-cancelled scan to error")
	}

	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()
	time.AfterFunc(2*time.Millisecond, runCancel)
	_, err := Scan(runCtx, ScanOptions{Roots: []string{root}})
	if err == nil {
		t.Fatalf("expected cancelled scan to return context error")
	}
	if err != context.Canceled && !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("expected cancellation error, got: %v", err)
	}
}

func TestScan_SymlinkFollowFindsProjectsThroughSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	real := filepath.Join(root, "real")
	linkedTarget := filepath.Join(real, "svc")
	if err := os.MkdirAll(linkedTarget, 0o755); err != nil {
		t.Fatalf("mkdir linked target: %v", err)
	}
	mustWriteFile(t, filepath.Join(linkedTarget, "go.mod"))

	link := filepath.Join(root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink unsupported in this environment: %v", err)
	}

	skipRes, err := Scan(context.Background(), ScanOptions{
		Roots:         []string{root},
		SymlinkPolicy: SymlinkSkipDirs,
	})
	if err != nil {
		t.Fatalf("scan skip error: %v", err)
	}
	followRes, err := Scan(context.Background(), ScanOptions{
		Roots:         []string{root},
		SymlinkPolicy: SymlinkFollowDirs,
	})
	if err != nil {
		t.Fatalf("scan follow error: %v", err)
	}

	linkedProjectPath := filepath.Join(link, "svc")
	if containsNormPath(projectPaths(skipRes.Projects), linkedProjectPath) {
		t.Fatalf("unexpected linked project found with SymlinkSkipDirs")
	}
	if !containsNormPath(projectPaths(followRes.Projects), linkedProjectPath) {
		t.Fatalf("expected linked project path with SymlinkFollowDirs, projects=%v", projectPaths(followRes.Projects))
	}
}

func TestScan_WindowsCaseRootDedupWhenAvailable(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-specific root-case behavior")
	}
	t.Parallel()

	root := t.TempDir()
	project := filepath.Join(root, "svc")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mustWriteFile(t, filepath.Join(project, "go.mod"))

	altCase := root
	if len(altCase) > 0 {
		if altCase[0] >= 'a' && altCase[0] <= 'z' {
			altCase = strings.ToUpper(altCase[:1]) + altCase[1:]
		} else {
			altCase = strings.ToLower(altCase[:1]) + altCase[1:]
		}
	}

	res, err := Scan(context.Background(), ScanOptions{
		Roots: []string{root, altCase},
	})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(res.Projects) != 1 {
		t.Fatalf("expected case-insensitive deduped root/project once, got %d", len(res.Projects))
	}
}

func TestScan_BoundaryGitRoot_StopsAtGitDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	projectA := filepath.Join(root, "projectA")
	library := filepath.Join(projectA, "library")
	testsDir := filepath.Join(projectA, "tests")
	for _, d := range []string{library, testsDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	if err := os.Mkdir(filepath.Join(projectA, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	mustWriteFile(t, filepath.Join(projectA, "go.mod"))
	mustWriteFile(t, filepath.Join(library, "package.json"))
	mustWriteFile(t, filepath.Join(testsDir, "Cargo.toml"))

	defaultRes, err := Scan(context.Background(), ScanOptions{
		Roots: []string{root},
	})
	if err != nil {
		t.Fatalf("default scan error: %v", err)
	}
	if !containsNormPath(projectPaths(defaultRes.Projects), library) || !containsNormPath(projectPaths(defaultRes.Projects), testsDir) {
		t.Fatalf("default mode should include nested projects under projectA")
	}

	boundaryRes, err := Scan(context.Background(), ScanOptions{
		Roots:        []string{root},
		BoundaryMode: BoundaryGitRoot,
	})
	if err != nil {
		t.Fatalf("boundary scan error: %v", err)
	}

	if !containsNormPath(projectPaths(boundaryRes.Projects), projectA) {
		t.Fatalf("expected git-root directory project to be kept")
	}
	projectMap := projectsByPath(t, boundaryRes.Projects)
	p := projectMap[mustNorm(t, projectA)]
	gotTypes := make([]string, 0, len(p.Matches))
	for _, m := range p.Matches {
		gotTypes = append(gotTypes, m.TypeID)
	}
	sort.Strings(gotTypes)
	if !reflect.DeepEqual(gotTypes, []string{"git", "go", "node", "rust"}) {
		t.Fatalf("expected aggregated root types, got %+v", p.Matches)
	}
	if containsNormPath(projectPaths(boundaryRes.Projects), library) {
		t.Fatalf("expected library to be excluded by git boundary")
	}
	if containsNormPath(projectPaths(boundaryRes.Projects), testsDir) {
		t.Fatalf("expected tests to be excluded by git boundary")
	}
}

func TestScan_BoundaryGitRoot_StopsAtGitFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	projectA := filepath.Join(root, "projectA")
	child := filepath.Join(projectA, "child")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatalf("mkdir child: %v", err)
	}
	mustWriteFile(t, filepath.Join(projectA, ".git"))
	mustWriteFile(t, filepath.Join(projectA, "go.mod"))
	mustWriteFile(t, filepath.Join(child, "package.json"))

	res, err := Scan(context.Background(), ScanOptions{
		Roots:        []string{root},
		BoundaryMode: BoundaryGitRoot,
	})
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	if !containsNormPath(projectPaths(res.Projects), projectA) {
		t.Fatalf("expected projectA in results")
	}
	projectMap := projectsByPath(t, res.Projects)
	p := projectMap[mustNorm(t, projectA)]
	gotTypes := make([]string, 0, len(p.Matches))
	for _, m := range p.Matches {
		gotTypes = append(gotTypes, m.TypeID)
	}
	sort.Strings(gotTypes)
	if !reflect.DeepEqual(gotTypes, []string{"git", "go", "node"}) {
		t.Fatalf("expected aggregated root types, got %+v", p.Matches)
	}
	if containsNormPath(projectPaths(res.Projects), child) {
		t.Fatalf("expected child to be excluded when .git file is present")
	}
}

func TestScan_BoundaryGitRoot_IncludesGitOnlyProjects(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	gitOnly := filepath.Join(root, "git-only")
	child := filepath.Join(gitOnly, "nested")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Mkdir(filepath.Join(gitOnly, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	res, err := Scan(context.Background(), ScanOptions{
		Roots:        []string{root},
		BoundaryMode: BoundaryGitRoot,
	})
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}
	if !containsNormPath(projectPaths(res.Projects), gitOnly) {
		t.Fatalf("expected git-only directory to be included")
	}
	if containsNormPath(projectPaths(res.Projects), child) {
		t.Fatalf("expected child to be excluded by git boundary")
	}
	projectMap := projectsByPath(t, res.Projects)
	p := projectMap[mustNorm(t, gitOnly)]
	if len(p.Matches) != 1 || p.Matches[0].TypeID != "git" {
		t.Fatalf("expected git-only classification, got %+v", p.Matches)
	}
	if !reflect.DeepEqual(p.Matches[0].Markers, []string{".git"}) {
		t.Fatalf("unexpected git markers: %v", p.Matches[0].Markers)
	}
}

func TestScan_BoundaryGitRoot_IncludeNested_AggregatesTypesAndKeepsChildren(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	projectA := filepath.Join(root, "projectA")
	api := filepath.Join(projectA, "api")
	frontend := filepath.Join(projectA, "frontend")
	for _, d := range []string{api, frontend} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	if err := os.Mkdir(filepath.Join(projectA, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	mustWriteFile(t, filepath.Join(api, "service.csproj"))
	mustWriteFile(t, filepath.Join(frontend, "package.json"))

	res, err := Scan(context.Background(), ScanOptions{
		Roots:                    []string{root},
		BoundaryMode:             BoundaryGitRoot,
		BoundaryGitIncludeNested: true,
	})
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	paths := projectPaths(res.Projects)
	if !containsNormPath(paths, projectA) {
		t.Fatalf("expected git root in results")
	}
	if !containsNormPath(paths, api) || !containsNormPath(paths, frontend) {
		t.Fatalf("expected nested projects to remain in results")
	}

	pm := projectsByPath(t, res.Projects)
	rootProj := pm[mustNorm(t, projectA)]
	wantTypes := []string{"dotnet", "git", "node"}
	gotTypes := make([]string, 0, len(rootProj.Matches))
	for _, m := range rootProj.Matches {
		gotTypes = append(gotTypes, m.TypeID)
	}
	sort.Strings(gotTypes)
	if !reflect.DeepEqual(gotTypes, wantTypes) {
		t.Fatalf("unexpected aggregated root types: got=%v want=%v", gotTypes, wantTypes)
	}
}

func projectsByPath(t *testing.T, in []Classification) map[string]Classification {
	t.Helper()
	out := make(map[string]Classification, len(in))
	for _, p := range in {
		if _, ok := out[p.Path]; ok {
			t.Fatalf("duplicate project path in results: %s", p.Path)
		}
		out[p.Path] = p
	}
	return out
}

func assertProject(t *testing.T, p Classification, wantRoot string, wantDepth int, want map[string][]string) {
	t.Helper()
	if p.Root != wantRoot {
		t.Fatalf("root mismatch for %s: got=%q want=%q", p.Path, p.Root, wantRoot)
	}
	if p.Depth != wantDepth {
		t.Fatalf("depth mismatch for %s: got=%d want=%d", p.Path, p.Depth, wantDepth)
	}
	got := map[string][]string{}
	for _, m := range p.Matches {
		got[m.TypeID] = append([]string(nil), m.Markers...)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("matches mismatch for %s: got=%v want=%v", p.Path, got, want)
	}
}

func projectPaths(projects []Classification) []string {
	out := make([]string, 0, len(projects))
	for _, p := range projects {
		out = append(out, p.Path)
	}
	return out
}

func containsNormPath(paths []string, p string) bool {
	norm, err := normalizePath(p)
	if err != nil {
		return false
	}
	for _, got := range paths {
		if canonicalPathKey(got) == canonicalPathKey(norm) {
			return true
		}
	}
	return false
}

func mustNorm(t *testing.T, p string) string {
	t.Helper()
	n, err := normalizePath(p)
	if err != nil {
		t.Fatalf("normalize path %q: %v", p, err)
	}
	return n
}

func sortedKeys(m map[string]Classification) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
