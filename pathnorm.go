package scanpro

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

func normalizePath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("scanpro: empty path")
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("scanpro: normalize path %q: %w", path, err)
	}
	return filepath.Clean(abs), nil
}

func normalizeAndDedupeRoots(roots []string) ([]string, error) {
	out := make([]string, 0, len(roots))
	seen := make(map[string]struct{}, len(roots))

	for _, root := range roots {
		norm, err := normalizePath(root)
		if err != nil {
			return nil, err
		}
		key := canonicalPathKey(norm)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, norm)
	}

	sort.Strings(out)
	return out, nil
}

func normalizeExcludePaths(paths []string) ([]string, error) {
	out := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))

	for _, p := range paths {
		norm, err := normalizePath(p)
		if err != nil {
			return nil, err
		}
		key := canonicalPathKey(norm)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, norm)
	}

	sort.Strings(out)
	return out, nil
}

func normalizeNameSet(names []string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		if runtime.GOOS == "windows" {
			trimmed = strings.ToLower(trimmed)
		}
		out[trimmed] = struct{}{}
	}
	return out
}

func canonicalPathKey(path string) string {
	clean := filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(clean)
	}
	return clean
}

func shouldExcludeByName(exclude map[string]struct{}, base string) bool {
	if runtime.GOOS == "windows" {
		base = strings.ToLower(base)
	}
	_, ok := exclude[base]
	return ok
}

func shouldExcludeByPath(exclude []string, dir string) bool {
	for _, ex := range exclude {
		if isSameOrDescendant(dir, ex) {
			return true
		}
	}
	return false
}

func isSameOrDescendant(path, parent string) bool {
	rel, err := filepath.Rel(parent, path)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == "" {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
