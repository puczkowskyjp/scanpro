package powerwalk

import (
	"context"
	"os"
	"path/filepath"
	"sort"
)

func classifyDir(ctx context.Context, cfg normalizedOptions, dir string) (Classification, error) {
	if err := ctx.Err(); err != nil {
		return Classification{}, err
	}
	normDir, err := normalizePath(dir)
	if err != nil {
		return Classification{}, err
	}

	entries, err := os.ReadDir(normDir)
	if err != nil {
		return Classification{}, err
	}

	fileNames := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fileNames = append(fileNames, entry.Name())
	}

	matches := make([]TypeMatch, 0, len(cfg.detectors))
	for _, detector := range cfg.detectors {
		if !detectorEnabled(cfg, detector.TypeID) {
			continue
		}
		matched := matchedMarkers(fileNames, detector.Markers)
		if len(matched) == 0 {
			continue
		}
		matches = append(matches, TypeMatch{
			TypeID:  detector.TypeID,
			Markers: matched,
		})
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].TypeID < matches[j].TypeID
	})
	return Classification{
		Path:    normDir,
		Matches: matches,
	}, nil
}

func matchedMarkers(fileNames []string, markers []string) []string {
	hit := make(map[string]struct{}, len(markers))
	for _, marker := range markers {
		for _, fileName := range fileNames {
			ok, err := filepath.Match(marker, fileName)
			if err != nil {
				continue
			}
			if ok {
				hit[marker] = struct{}{}
				break
			}
		}
	}
	out := make([]string, 0, len(hit))
	for marker := range hit {
		out = append(out, marker)
	}
	sort.Strings(out)
	return out
}

func sortClassifications(projects []Classification) {
	sort.Slice(projects, func(i, j int) bool {
		return projects[i].Path < projects[j].Path
	})
}
