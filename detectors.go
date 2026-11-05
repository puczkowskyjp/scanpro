package scanpro

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

func BuiltinDetectors() []Detector {
	return []Detector{
		{TypeID: "go", Markers: []string{"go.mod"}},
		{TypeID: "node", Markers: []string{"package.json"}},
		{TypeID: "dotnet", Markers: []string{"*.sln", "*.slnx", "*.csproj", "*.fsproj", "*.vbproj"}},
		{TypeID: "python", Markers: []string{"pyproject.toml", "setup.py", "requirements.txt"}},
		{TypeID: "rust", Markers: []string{"Cargo.toml"}},
	}
}

func normalizeDetectors(detectors []Detector) ([]Detector, error) {
	out := make([]Detector, 0, len(detectors))
	seenType := make(map[string]struct{}, len(detectors))

	for _, d := range detectors {
		typeID := strings.TrimSpace(d.TypeID)
		if typeID == "" {
			return nil, fmt.Errorf("scanpro: detector type id must not be empty")
		}
		if _, ok := seenType[typeID]; ok {
			return nil, fmt.Errorf("scanpro: duplicate detector type id %q", typeID)
		}
		seenType[typeID] = struct{}{}

		if len(d.Markers) == 0 {
			return nil, fmt.Errorf("scanpro: detector %q must define at least one marker", typeID)
		}

		seenMarker := make(map[string]struct{}, len(d.Markers))
		markers := make([]string, 0, len(d.Markers))
		for _, marker := range d.Markers {
			marker = strings.TrimSpace(marker)
			if marker == "" {
				return nil, fmt.Errorf("scanpro: detector %q has empty marker", typeID)
			}
			if filepath.IsAbs(marker) {
				return nil, fmt.Errorf("scanpro: detector %q marker must be a file name or glob, got absolute path %q", typeID, marker)
			}
			if strings.Contains(marker, string(filepath.Separator)) {
				return nil, fmt.Errorf("scanpro: detector %q marker must not include path separators: %q", typeID, marker)
			}
			if _, ok := seenMarker[marker]; ok {
				continue
			}
			seenMarker[marker] = struct{}{}
			markers = append(markers, marker)
		}
		sort.Strings(markers)

		out = append(out, Detector{TypeID: typeID, Markers: markers})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].TypeID < out[j].TypeID
	})
	return out, nil
}

func detectorEnabled(cfg normalizedOptions, typeID string) bool {
	if len(cfg.enabledTypes) > 0 {
		if _, ok := cfg.enabledTypes[typeID]; !ok {
			return false
		}
	}
	if _, ok := cfg.disabledTypes[typeID]; ok {
		return false
	}
	return true
}
