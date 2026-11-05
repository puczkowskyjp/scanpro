package scanpro

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type SymlinkPolicy int

const (
	SymlinkSkipDirs SymlinkPolicy = iota
	SymlinkFollowDirs
)

type BoundaryMode int

const (
	BoundaryNone BoundaryMode = iota
	BoundaryGitRoot
)

type ScanOptions struct {
	Roots         []string
	MaxDepth      int
	ExcludeNames  []string
	ExcludePaths  []string
	SymlinkPolicy SymlinkPolicy
	BoundaryMode  BoundaryMode
	// BoundaryGitIncludeNested applies only when BoundaryMode is BoundaryGitRoot.
	// Nested project types are always aggregated onto the git-root project.
	// When true, nested projects are also returned independently; when false,
	// only the git-root project row is returned.
	BoundaryGitIncludeNested bool

	Detectors     []Detector
	EnabledTypes  []string
	DisabledTypes []string
}

type Detector struct {
	TypeID  string
	Markers []string
}

type DiscoveredDir struct {
	Path  string
	Depth int
	Root  string
}

type TypeMatch struct {
	TypeID  string
	Markers []string
}

type Classification struct {
	Path    string
	Depth   int
	Root    string
	Matches []TypeMatch
}

type DiscoverResult struct {
	Directories []DiscoveredDir
	Issues      []Issue
}

type ScanResult struct {
	Projects []Classification
	Issues   []Issue
}

func Discover(ctx context.Context, opts ScanOptions) (DiscoverResult, error) {
	cfg, err := validateAndNormalizeOptions(opts)
	if err != nil {
		return DiscoverResult{}, err
	}
	return discover(ctx, cfg)
}

func ClassifyDir(ctx context.Context, dir string, opts ScanOptions) (Classification, error) {
	if len(opts.Roots) == 0 {
		opts.Roots = []string{"."}
	}
	cfg, err := validateAndNormalizeOptions(opts)
	if err != nil {
		return Classification{}, err
	}
	if err := ctx.Err(); err != nil {
		return Classification{}, err
	}
	return classifyDir(ctx, cfg, dir)
}

func Scan(ctx context.Context, opts ScanOptions) (ScanResult, error) {
	cfg, err := validateAndNormalizeOptions(opts)
	if err != nil {
		return ScanResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return ScanResult{}, err
	}

	discovery, err := discover(ctx, cfg)
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return ScanResult{}, err
	}

	result := ScanResult{
		Projects: make([]Classification, 0, len(discovery.Directories)),
		Issues:   append([]Issue(nil), discovery.Issues...),
	}
	gitRoots := make([]string, 0)
	if cfg.boundaryMode == BoundaryGitRoot {
		for _, dir := range discovery.Directories {
			isGitRoot, gerr := hasGitBoundary(dir.Path)
			if gerr != nil {
				result.Issues = append(result.Issues, Issue{
					Path:        dir.Path,
					Operation:   "git_boundary_check",
					Err:         gerr,
					Recoverable: true,
				})
				continue
			}
			if isGitRoot {
				gitRoots = append(gitRoots, dir.Path)
			}
		}
	}

	for _, dir := range discovery.Directories {
		if cerr := ctx.Err(); cerr != nil {
			sortIssues(result.Issues)
			sortClassifications(result.Projects)
			return result, cerr
		}

		classification, cerr := classifyDir(ctx, cfg, dir.Path)
		if cerr != nil {
			result.Issues = append(result.Issues, Issue{
				Path:        dir.Path,
				Operation:   "classify",
				Err:         cerr,
				Recoverable: true,
			})
			continue
		}
		if len(classification.Matches) == 0 && cfg.boundaryMode == BoundaryGitRoot {
			isGitRoot, gerr := hasGitBoundary(dir.Path)
			if gerr != nil {
				result.Issues = append(result.Issues, Issue{
					Path:        dir.Path,
					Operation:   "git_boundary_check",
					Err:         gerr,
					Recoverable: true,
				})
				continue
			}
			if isGitRoot {
				classification.Matches = []TypeMatch{
					{
						TypeID:  "git",
						Markers: []string{".git"},
					},
				}
			}
		}
		if cfg.boundaryMode == BoundaryGitRoot {
			isGitRoot, gerr := hasGitBoundary(dir.Path)
			if gerr != nil {
				result.Issues = append(result.Issues, Issue{
					Path:        dir.Path,
					Operation:   "git_boundary_check",
					Err:         gerr,
					Recoverable: true,
				})
				continue
			}
			if isGitRoot && !hasTypeMatch(classification.Matches, "git") {
				classification.Matches = append(classification.Matches, TypeMatch{
					TypeID:  "git",
					Markers: []string{".git"},
				})
				sort.Slice(classification.Matches, func(i, j int) bool {
					return classification.Matches[i].TypeID < classification.Matches[j].TypeID
				})
			}
		}
		if len(classification.Matches) > 0 {
			classification.Depth = dir.Depth
			classification.Root = dir.Root
			result.Projects = append(result.Projects, classification)
		}
	}

	if cfg.boundaryMode == BoundaryGitRoot {
		aggregateNestedMatchesIntoGitRoots(result.Projects, gitRoots)
		if !cfg.boundaryGitIncludeNested {
			filterNestedProjectsUnderGitRoots(&result.Projects, gitRoots)
		}
	}

	sortClassifications(result.Projects)
	sortIssues(result.Issues)
	if err != nil {
		return result, err
	}
	return result, nil
}

func hasTypeMatch(matches []TypeMatch, typeID string) bool {
	for _, m := range matches {
		if m.TypeID == typeID {
			return true
		}
	}
	return false
}

type normalizedOptions struct {
	roots                    []string
	maxDepth                 int
	excludeNames             map[string]struct{}
	excludePaths             []string
	symlinkPolicy            SymlinkPolicy
	boundaryMode             BoundaryMode
	boundaryGitIncludeNested bool
	detectors                []Detector
	enabledTypes             map[string]struct{}
	disabledTypes            map[string]struct{}
}

func validateAndNormalizeOptions(opts ScanOptions) (normalizedOptions, error) {
	if len(opts.Roots) == 0 {
		return normalizedOptions{}, errors.New("scanpro: at least one root is required")
	}
	if opts.MaxDepth < -1 {
		return normalizedOptions{}, fmt.Errorf("scanpro: max depth must be -1 or >= 0, got %d", opts.MaxDepth)
	}
	if opts.SymlinkPolicy != SymlinkSkipDirs && opts.SymlinkPolicy != SymlinkFollowDirs {
		return normalizedOptions{}, fmt.Errorf("scanpro: unsupported symlink policy value %d", opts.SymlinkPolicy)
	}
	if opts.BoundaryMode != BoundaryNone && opts.BoundaryMode != BoundaryGitRoot {
		return normalizedOptions{}, fmt.Errorf("scanpro: unsupported boundary mode value %d", opts.BoundaryMode)
	}

	roots, err := normalizeAndDedupeRoots(opts.Roots)
	if err != nil {
		return normalizedOptions{}, err
	}
	excludePaths, err := normalizeExcludePaths(opts.ExcludePaths)
	if err != nil {
		return normalizedOptions{}, err
	}

	detectors := opts.Detectors
	if len(detectors) == 0 {
		detectors = BuiltinDetectors()
	}
	detectors, err = normalizeDetectors(detectors)
	if err != nil {
		return normalizedOptions{}, err
	}

	enabledTypes := normalizeTypeSet(opts.EnabledTypes)
	disabledTypes := normalizeTypeSet(opts.DisabledTypes)

	maxDepth := opts.MaxDepth
	if maxDepth == 0 {
		maxDepth = -1
	}

	return normalizedOptions{
		roots:                    roots,
		maxDepth:                 maxDepth,
		excludeNames:             normalizeNameSet(opts.ExcludeNames),
		excludePaths:             excludePaths,
		symlinkPolicy:            opts.SymlinkPolicy,
		boundaryMode:             opts.BoundaryMode,
		boundaryGitIncludeNested: opts.BoundaryGitIncludeNested,
		detectors:                detectors,
		enabledTypes:             enabledTypes,
		disabledTypes:            disabledTypes,
	}, nil
}

func normalizeTypeSet(in []string) map[string]struct{} {
	out := make(map[string]struct{}, len(in))
	for _, v := range in {
		key := strings.TrimSpace(v)
		if key == "" {
			continue
		}
		out[key] = struct{}{}
	}
	return out
}

func aggregateNestedMatchesIntoGitRoots(projects []Classification, gitRoots []string) {
	if len(projects) == 0 || len(gitRoots) == 0 {
		return
	}

	sortClassifications(projects)
	sortedRoots := append([]string(nil), gitRoots...)
	sort.Slice(sortedRoots, func(i, j int) bool {
		return len(sortedRoots[i]) > len(sortedRoots[j])
	})

	projectIndex := make(map[string]int, len(projects))
	for i := range projects {
		projectIndex[projects[i].Path] = i
	}

	for i := range projects {
		child := projects[i]
		targetRoot := ""
		for _, candidate := range sortedRoots {
			if canonicalPathKey(child.Path) == canonicalPathKey(candidate) {
				continue
			}
			if isSameOrDescendant(child.Path, candidate) {
				targetRoot = candidate
				break
			}
		}
		if targetRoot == "" {
			continue
		}
		rootIdx, ok := projectIndex[targetRoot]
		if !ok {
			continue
		}

		rootProject := &projects[rootIdx]
		typeToMarkers := make(map[string]map[string]struct{}, len(rootProject.Matches))
		for _, m := range rootProject.Matches {
			if _, exists := typeToMarkers[m.TypeID]; !exists {
				typeToMarkers[m.TypeID] = map[string]struct{}{}
			}
			for _, marker := range m.Markers {
				typeToMarkers[m.TypeID][marker] = struct{}{}
			}
		}

		for _, m := range child.Matches {
			if m.TypeID == "git" {
				continue
			}
			if _, exists := typeToMarkers[m.TypeID]; !exists {
				typeToMarkers[m.TypeID] = map[string]struct{}{}
			}
			for _, marker := range m.Markers {
				typeToMarkers[m.TypeID][marker] = struct{}{}
			}
		}

		merged := make([]TypeMatch, 0, len(typeToMarkers))
		for typeID, markerSet := range typeToMarkers {
			markers := make([]string, 0, len(markerSet))
			for marker := range markerSet {
				markers = append(markers, marker)
			}
			sort.Strings(markers)
			merged = append(merged, TypeMatch{
				TypeID:  typeID,
				Markers: markers,
			})
		}
		sort.Slice(merged, func(a, b int) bool { return merged[a].TypeID < merged[b].TypeID })
		rootProject.Matches = merged
	}
}

func filterNestedProjectsUnderGitRoots(projects *[]Classification, gitRoots []string) {
	if projects == nil || len(*projects) == 0 || len(gitRoots) == 0 {
		return
	}
	roots := append([]string(nil), gitRoots...)
	sort.Slice(roots, func(i, j int) bool {
		return len(roots[i]) > len(roots[j])
	})

	filtered := make([]Classification, 0, len(*projects))
	for _, p := range *projects {
		drop := false
		for _, root := range roots {
			if canonicalPathKey(p.Path) == canonicalPathKey(root) {
				continue
			}
			if isSameOrDescendant(p.Path, root) {
				drop = true
				break
			}
		}
		if !drop {
			filtered = append(filtered, p)
		}
	}
	*projects = filtered
}
