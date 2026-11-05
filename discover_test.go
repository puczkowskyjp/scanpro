package scanpro 

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDiscover_OverlappingRootsAndDeterministicOrder(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	a := filepath.Join(root, "a")
	b := filepath.Join(root, "b")
	c := filepath.Join(a, "c")
	for _, dir := range []string{a, b, c} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	opts := ScanOptions{
		Roots: []string{root, a},
	}
	res1, err := Discover(context.Background(), opts)
	if err != nil {
		t.Fatalf("Discover #1 error: %v", err)
	}
	res2, err := Discover(context.Background(), opts)
	if err != nil {
		t.Fatalf("Discover #2 error: %v", err)
	}

	if !reflect.DeepEqual(res1.Directories, res2.Directories) {
		t.Fatalf("discoveries are not deterministic")
	}

	seen := map[string]struct{}{}
	for _, d := range res1.Directories {
		if _, ok := seen[d.Path]; ok {
			t.Fatalf("duplicate discovered path: %s", d.Path)
		}
		seen[d.Path] = struct{}{}
	}
}

func TestDiscover_MaxDepthAndExclusions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	level1 := filepath.Join(root, "level 1")
	level2 := filepath.Join(level1, "level 2")
	skipName := filepath.Join(root, "vendor")
	skipByPath := filepath.Join(root, "tmp", "skip")
	keep := filepath.Join(root, "tmp", "keep")
	keepDepthOne := filepath.Join(root, "tmp")
	for _, dir := range []string{level2, skipName, skipByPath, keep} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	res, err := Discover(context.Background(), ScanOptions{
		Roots:        []string{root},
		MaxDepth:     1,
		ExcludeNames: []string{"vendor"},
		ExcludePaths: []string{skipByPath},
	})
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}

	paths := discoveredPaths(res)
	if containsPath(paths, level2) {
		t.Fatalf("found level2 despite MaxDepth=1: %s", level2)
	}
	if containsPath(paths, skipName) {
		t.Fatalf("found excluded directory by name: %s", skipName)
	}
	if containsPath(paths, skipByPath) {
		t.Fatalf("found excluded directory by path: %s", skipByPath)
	}
	if !containsPath(paths, keepDepthOne) {
		t.Fatalf("did not find expected depth-1 directory: %s", keepDepthOne)
	}
}

func TestDiscover_ContextCancellation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	deep := root
	for i := 0; i < 120; i++ {
		deep = filepath.Join(deep, "d")
		if err := os.MkdirAll(deep, 0o755); err != nil {
			t.Fatalf("mkdir deep: %v", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Discover(ctx, ScanOptions{Roots: []string{root}})
	if err == nil {
		t.Fatalf("expected cancellation error, got nil")
	}
}

func TestDiscover_SymlinkPolicy(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatalf("mkdir real: %v", err)
	}
	linkDir := filepath.Join(root, "link")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Skipf("symlink unsupported on this environment: %v", err)
	}

	skipRes, err := Discover(context.Background(), ScanOptions{
		Roots:         []string{root},
		SymlinkPolicy: SymlinkSkipDirs,
	})
	if err != nil {
		t.Fatalf("Discover skip error: %v", err)
	}
	followRes, err := Discover(context.Background(), ScanOptions{
		Roots:         []string{root},
		SymlinkPolicy: SymlinkFollowDirs,
	})
	if err != nil {
		t.Fatalf("Discover follow error: %v", err)
	}

	if containsPath(discoveredPaths(skipRes), linkDir) {
		t.Fatalf("symlink path unexpectedly present with SymlinkSkipDirs")
	}
	if !containsPath(discoveredPaths(followRes), linkDir) {
		t.Fatalf("symlink path missing with SymlinkFollowDirs")
	}
}

func TestDiscover_PermissionErrorsRecoverable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission mode checks are not portable on Windows")
	}
	t.Parallel()

	root := t.TempDir()
	denied := filepath.Join(root, "denied")
	if err := os.MkdirAll(denied, 0o755); err != nil {
		t.Fatalf("mkdir denied: %v", err)
	}
	if err := os.Chmod(denied, 0); err != nil {
		t.Fatalf("chmod denied: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(denied, 0o755)
	})

	res, err := Discover(context.Background(), ScanOptions{Roots: []string{root}})
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}

	if len(res.Issues) == 0 {
		t.Fatalf("expected recoverable issues for inaccessible directory")
	}
}

func TestDiscover_HandlesFilesystemChangesDuringScan(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "changing")
	for i := 0; i < 250; i++ {
		if err := os.MkdirAll(filepath.Join(target, strings.Repeat("x", i%8+1), "child"), 0o755); err != nil {
			t.Fatalf("mkdir fixture: %v", err)
		}
	}

	go func() {
		time.Sleep(1 * time.Millisecond)
		_ = os.RemoveAll(target)
	}()

	_, err := Discover(context.Background(), ScanOptions{Roots: []string{root}})
	if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
		t.Fatalf("Discover should tolerate filesystem mutation, got: %v", err)
	}
}

func discoveredPaths(res DiscoverResult) []string {
	out := make([]string, 0, len(res.Directories))
	for _, d := range res.Directories {
		out = append(out, d.Path)
	}
	return out
}

func containsPath(paths []string, path string) bool {
	norm, err := normalizePath(path)
	if err != nil {
		return false
	}
	for _, p := range paths {
		if canonicalPathKey(p) == canonicalPathKey(norm) {
			return true
		}
	}
	return false
}
