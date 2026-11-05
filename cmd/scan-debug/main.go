package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/puczkowskyjp/scanpro"
)

type multiFlag []string

func (m *multiFlag) String() string {
	return strings.Join(*m, ",")
}

func (m *multiFlag) Set(v string) error {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return fmt.Errorf("empty value is not allowed")
	}
	*m = append(*m, trimmed)
	return nil
}

func main() {
	var rootFlags multiFlag
	var excludeNames multiFlag
	var excludePaths multiFlag
	var maxDepth int
	var followSymlinks bool
	var boundaryGit bool
	var boundaryGitIncludeNested bool
	var jsonOutput bool

	flag.Var(&rootFlags, "root", "scan root path (repeatable)")
	flag.Var(&excludeNames, "exclude-name", "directory base name to exclude (repeatable)")
	flag.Var(&excludePaths, "exclude-path", "directory path to exclude (repeatable)")
	flag.IntVar(&maxDepth, "max-depth", 0, "maximum traversal depth (0 and -1 mean unlimited)")
	flag.BoolVar(&followSymlinks, "follow-symlinks", false, "follow directory symlinks")
	flag.BoolVar(&boundaryGit, "boundary-git", false, "stop descending below directories containing .git")
	flag.BoolVar(&boundaryGitIncludeNested, "boundary-git-include-nested", false, "when boundary-git is set, also return nested projects as separate rows")
	flag.BoolVar(&jsonOutput, "json", false, "print machine-readable JSON output")
	flag.Parse()

	roots := make([]string, 0, len(rootFlags)+len(flag.Args()))
	roots = append(roots, rootFlags...)
	roots = append(roots, flag.Args()...)
	if len(roots) == 0 {
		printUsageAndExit("at least one root is required via --root or positional args")
	}

	symlinkPolicy := scanpro.SymlinkSkipDirs
	if followSymlinks {
		symlinkPolicy = scanpro.SymlinkFollowDirs
	}

	opts := scanpro.ScanOptions{
		Roots:         roots,
		MaxDepth:      maxDepth,
		ExcludeNames:  excludeNames,
		ExcludePaths:  excludePaths,
		SymlinkPolicy: symlinkPolicy,
	}
	if boundaryGit {
		opts.BoundaryMode = scanpro.BoundaryGitRoot
		opts.BoundaryGitIncludeNested = boundaryGitIncludeNested
	}

	result, err := scanpro.Scan(context.Background(), opts)
	if err != nil {
		if jsonOutput {
			_ = writeJSON(debugOutput{
				Options: optionsView{
					Roots:                    roots,
					MaxDepth:                 maxDepth,
					ExcludeNames:             excludeNames,
					ExcludePaths:             excludePaths,
					FollowSymlinks:           followSymlinks,
					BoundaryGit:              boundaryGit,
					BoundaryGitIncludeNested: boundaryGitIncludeNested,
				},
				FatalError: err.Error(),
			})
		} else {
			fmt.Fprintf(os.Stderr, "scan failed: %v\n", err)
		}
		os.Exit(1)
	}

	if jsonOutput {
		if jerr := writeJSON(debugOutput{
			Options: optionsView{
				Roots:                    roots,
				MaxDepth:                 maxDepth,
				ExcludeNames:             excludeNames,
				ExcludePaths:             excludePaths,
				FollowSymlinks:           followSymlinks,
				BoundaryGit:              boundaryGit,
				BoundaryGitIncludeNested: boundaryGitIncludeNested,
			},
			Result:  result,
			Summary: buildSummary(result),
		}); jerr != nil {
			fmt.Fprintf(os.Stderr, "json output error: %v\n", jerr)
			os.Exit(1)
		}
		return
	}

	printTable(result)
	printSummary(result)
}

type optionsView struct {
	Roots                    []string `json:"roots"`
	MaxDepth                 int      `json:"max_depth"`
	ExcludeNames             []string `json:"exclude_names,omitempty"`
	ExcludePaths             []string `json:"exclude_paths,omitempty"`
	FollowSymlinks           bool     `json:"follow_symlinks"`
	BoundaryGit              bool     `json:"boundary_git"`
	BoundaryGitIncludeNested bool     `json:"boundary_git_include_nested"`
}

type summaryView struct {
	ProjectCount int            `json:"project_count"`
	ByType       map[string]int `json:"by_type"`
	IssueCount   int            `json:"issue_count"`
}

type debugOutput struct {
	Options    optionsView          `json:"options"`
	Result     scanpro.ScanResult `json:"result,omitempty"`
	Summary    summaryView          `json:"summary,omitempty"`
	FatalError string               `json:"fatal_error,omitempty"`
}

func writeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printUsageAndExit(msg string) {
	fmt.Fprintf(os.Stderr, "error: %s\n\n", msg)
	fmt.Fprintf(os.Stderr, "Usage: scan-debug [flags] [root ...]\n\nFlags:\n")
	flag.PrintDefaults()
	os.Exit(2)
}

func printTable(result scanpro.ScanResult) {
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "PATH\tTYPES\tMATCHED_MARKERS\tROOT\tDEPTH")
	for _, p := range result.Projects {
		types := make([]string, 0, len(p.Matches))
		markers := make([]string, 0, len(p.Matches))
		for _, m := range p.Matches {
			types = append(types, m.TypeID)
			markers = append(markers, fmt.Sprintf("%s:[%s]", m.TypeID, strings.Join(m.Markers, ",")))
		}
		_, _ = fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\t%d\n",
			p.Path,
			strings.Join(types, ","),
			strings.Join(markers, " "),
			p.Root,
			p.Depth,
		)
	}
	_ = w.Flush()
}

func buildSummary(result scanpro.ScanResult) summaryView {
	byType := map[string]int{}
	for _, p := range result.Projects {
		for _, m := range p.Matches {
			byType[m.TypeID]++
		}
	}
	return summaryView{
		ProjectCount: len(result.Projects),
		ByType:       byType,
		IssueCount:   len(result.Issues),
	}
}

func printSummary(result scanpro.ScanResult) {
	s := buildSummary(result)
	fmt.Println()
	fmt.Printf("Projects discovered: %d\n", s.ProjectCount)
	fmt.Println("Counts by project type:")
	if len(s.ByType) == 0 {
		fmt.Println("  (none)")
	} else {
		keys := make([]string, 0, len(s.ByType))
		for k := range s.ByType {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("  %s: %d\n", k, s.ByType[k])
		}
	}

	fmt.Printf("Issues: %d\n", s.IssueCount)
	for _, issue := range result.Issues {
		fmt.Printf("  - path=%q op=%s recoverable=%t err=%v\n", issue.Path, issue.Operation, issue.Recoverable, issue.Err)
	}
}
