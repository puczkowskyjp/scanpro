package powerwalk

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func discover(ctx context.Context, cfg normalizedOptions) (DiscoverResult, error) {
	result := DiscoverResult{
		Directories: []DiscoveredDir{},
		Issues:      []Issue{},
	}
	seenDirs := make(map[string]struct{})
	visitedSymlinkTargets := make(map[string]struct{})

	for _, root := range cfg.roots {
		if err := ctx.Err(); err != nil {
			sortDiscoveredDirs(result.Directories)
			sortIssues(result.Issues)
			return result, err
		}

		info, err := os.Lstat(root)
		if err != nil {
			result.Issues = append(result.Issues, Issue{
				Path:        root,
				Operation:   "lstat",
				Err:         err,
				Recoverable: true,
			})
			continue
		}
		if !info.IsDir() {
			result.Issues = append(result.Issues, Issue{
				Path:        root,
				Operation:   "root_check",
				Err:         errors.New("root is not a directory"),
				Recoverable: true,
			})
			continue
		}

		walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if cerr := ctx.Err(); cerr != nil {
				return cerr
			}
			if walkErr != nil {
				result.Issues = append(result.Issues, Issue{
					Path:        path,
					Operation:   "walk",
					Err:         walkErr,
					Recoverable: true,
				})
				if d != nil && d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			if !d.IsDir() {
				return nil
			}

			normPath, nerr := normalizePath(path)
			if nerr != nil {
				result.Issues = append(result.Issues, Issue{
					Path:        path,
					Operation:   "normalize",
					Err:         nerr,
					Recoverable: true,
				})
				return filepath.SkipDir
			}

			depth, derr := relativeDepth(root, normPath)
			if derr != nil {
				result.Issues = append(result.Issues, Issue{
					Path:        normPath,
					Operation:   "depth",
					Err:         derr,
					Recoverable: true,
				})
				return filepath.SkipDir
			}

			if cfg.maxDepth >= 0 && depth > cfg.maxDepth {
				return filepath.SkipDir
			}

			base := filepath.Base(normPath)
			if shouldExcludeByName(cfg.excludeNames, base) {
				return filepath.SkipDir
			}
			if shouldExcludeByPath(cfg.excludePaths, normPath) {
				return filepath.SkipDir
			}

			isSymlink, slErr := isDirectorySymlink(d, normPath)
			if slErr != nil {
				result.Issues = append(result.Issues, Issue{
					Path:        normPath,
					Operation:   "symlink_check",
					Err:         slErr,
					Recoverable: true,
				})
				return filepath.SkipDir
			}
			if isSymlink {
				if cfg.symlinkPolicy == SymlinkSkipDirs {
					return filepath.SkipDir
				}
				resolved, rerr := filepath.EvalSymlinks(normPath)
				if rerr != nil {
					result.Issues = append(result.Issues, Issue{
						Path:        normPath,
						Operation:   "eval_symlink",
						Err:         rerr,
						Recoverable: true,
					})
					return filepath.SkipDir
				}
				resolved, rerr = normalizePath(resolved)
				if rerr != nil {
					result.Issues = append(result.Issues, Issue{
						Path:        normPath,
						Operation:   "normalize",
						Err:         rerr,
						Recoverable: true,
					})
					return filepath.SkipDir
				}
				key := canonicalPathKey(resolved)
				if _, ok := visitedSymlinkTargets[key]; ok {
					return filepath.SkipDir
				}
				visitedSymlinkTargets[key] = struct{}{}
			}

			key := canonicalPathKey(normPath)
			if _, ok := seenDirs[key]; ok {
				return nil
			}
			seenDirs[key] = struct{}{}
			result.Directories = append(result.Directories, DiscoveredDir{
				Path:  normPath,
				Depth: depth,
				Root:  root,
			})

			if cfg.boundaryMode == BoundaryGitRoot {
				_, gerr := hasGitBoundary(normPath)
				if gerr != nil {
					result.Issues = append(result.Issues, Issue{
						Path:        normPath,
						Operation:   "git_boundary_check",
						Err:         gerr,
						Recoverable: true,
					})
				}
			}
			return nil
		})

		if walkErr != nil {
			if errors.Is(walkErr, context.Canceled) || errors.Is(walkErr, context.DeadlineExceeded) {
				sortDiscoveredDirs(result.Directories)
				sortIssues(result.Issues)
				return result, walkErr
			}
			result.Issues = append(result.Issues, Issue{
				Path:        root,
				Operation:   "walk",
				Err:         walkErr,
				Recoverable: true,
			})
		}
	}

	sortDiscoveredDirs(result.Directories)
	sortIssues(result.Issues)
	return result, nil
}

func sortDiscoveredDirs(dirs []DiscoveredDir) {
	sort.Slice(dirs, func(i, j int) bool {
		if dirs[i].Path != dirs[j].Path {
			return dirs[i].Path < dirs[j].Path
		}
		if dirs[i].Depth != dirs[j].Depth {
			return dirs[i].Depth < dirs[j].Depth
		}
		return dirs[i].Root < dirs[j].Root
	})
}

func relativeDepth(root, path string) (int, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return 0, err
	}
	if rel == "." {
		return 0, nil
	}
	parts := strings.Split(rel, string(filepath.Separator))
	return len(parts), nil
}

func isDirectorySymlink(entry fs.DirEntry, path string) (bool, error) {
	if entry.Type()&os.ModeSymlink == 0 {
		return false, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}

func hasGitBoundary(dir string) (bool, error) {
	gitPath := filepath.Join(dir, ".git")
	_, err := os.Lstat(gitPath)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}
