package powerwalk

import "sort"

type Issue struct {
	Path        string
	Operation   string
	Err         error
	Recoverable bool
}

func sortIssues(issues []Issue) {
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Path != issues[j].Path {
			return issues[i].Path < issues[j].Path
		}
		if issues[i].Operation != issues[j].Operation {
			return issues[i].Operation < issues[j].Operation
		}
		return issues[i].Recoverable && !issues[j].Recoverable
	})
}
