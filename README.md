# powerwalk

`powerwalk` is a Go library for discovering directories and classifying project types using filesystem markers.

It is intentionally application-agnostic: no CLI/TUI, no Git-specific coupling, no config-file parsing, and no tool execution.

## Features

- Multi-root directory discovery
- Max depth, path/name exclusions, and symlink policy
- Optional Git-root boundary mode
- Deterministic results across runs
- Duplicate-path deduplication across overlapping roots
- Marker-based project classification (built-in + custom detectors)
- Recoverable per-directory scan issues + fatal config/cancellation errors

## Built-in project types (v1)

- `go`: `go.mod`
- `node`: `package.json`
- `dotnet`: `*.sln`, `*.slnx`, `*.csproj`, `*.fsproj`, `*.vbproj`
- `python`: `pyproject.toml`, `setup.py`, `requirements.txt`
- `rust`: `Cargo.toml`

## Public API

```go
result, err := powerwalk.Scan(ctx, powerwalk.ScanOptions{
    Roots: []string{`C:\dev`, `/home/me/src`},
    MaxDepth: 4, // 0 means unlimited (default), -1 also means unlimited
    ExcludeNames: []string{"node_modules", ".git"},
    ExcludePaths: []string{`C:\dev\archive`},
    SymlinkPolicy: powerwalk.SymlinkSkipDirs,
    BoundaryMode: powerwalk.BoundaryGitRoot, // optional: stop descending below dirs containing .git
    BoundaryGitIncludeNested: true, // optional with BoundaryGitRoot: also return nested project rows
    Detectors: append(powerwalk.BuiltinDetectors(), powerwalk.Detector{
        TypeID:  "custom-tool",
        Markers: []string{"tool.yaml", "*.tool"},
    }),
    DisabledTypes: []string{"python"},
})
```

Use:

- `Discover(ctx, opts)` to traverse only.
- `ClassifyDir(ctx, dir, opts)` to classify one directory.
- `Scan(ctx, opts)` to do both and return classified results.

## Path and traversal semantics

- Paths are normalized to absolute, cleaned, OS-native paths.
- Deduplication uses normalized absolute paths (case-insensitive on Windows).
- `MaxDepth`: `0` and `-1` mean unlimited; positive values are relative depth from each root.
- `ExcludeNames` applies to directory basenames; case-insensitive on Windows.
- `ExcludePaths` applies to exact path or descendants after normalization.
- Default symlink policy is `SymlinkSkipDirs`.
- Default boundary mode is `BoundaryNone`.
- `BoundaryGitRoot` treats any directory containing `.git` (directory or file) as a git-root project boundary.
- In `BoundaryGitRoot`, git-root directories include type `git` with marker `.git`; nested type matches are aggregated onto the git-root project.
- With `BoundaryGitIncludeNested=true`, nested projects are also returned as separate rows; otherwise only git-root rows are returned.
