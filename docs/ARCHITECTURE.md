# Architecture

## Purpose

`github.com/Trysac/u084-gha-lint` is a small Go library and CLI. It estimates the billable minutes of GitHub Actions workflows and lints them for token permissions, unpinned actions and script injection. Its only non-standard dependency is `go.yaml.in/yaml/v3`, used through `yaml.Node` to obtain line numbers.

## Shape

```text
caller code --> package ghalint (repository root)
                    |
                    +-- ParseWorkflow, Lint, EstimateCost, LoadConfig, Format
                    +-- Run (used by cmd/gha-lint/main.go)
```

- **Public API**: the six exported entry points of the root package (`ParseWorkflow`, `Lint`, `EstimateCost`, `LoadConfig`, `Format`, `Run`) and their exported types. Unexported identifiers may change without a major version.
- **Errors**: invalid input returns an error. `*LintError` wraps the sentinels `ErrParse`, `ErrTooLarge`, `ErrConfig` and `ErrUnknownFormat`, so callers branch with `errors.Is`. The library does not panic on input.
- **Side effects**: the library functions take data as arguments and do not read files. `Run` writes to the `io.Writer` values it receives and reads the paths given on the command line.

## Quality contract

`scripts/check` is the single definition of the gates; `fabrica.json` tells the factory and CI how to run it. See [TESTING.md](TESTING.md).

## Decisions

Architecture decisions are recorded in [decisions/](decisions/) as numbered ADRs.
