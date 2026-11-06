# gha-lint

[![Go Reference](https://pkg.go.dev/badge/github.com/Trysac/u084-gha-lint.svg)](https://pkg.go.dev/github.com/Trysac/u084-gha-lint)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![CI](https://github.com/Trysac/u084-gha-lint/actions/workflows/ci.yml/badge.svg)](https://github.com/Trysac/u084-gha-lint/actions/workflows/ci.yml)

Estimate the billable minutes of GitHub Actions workflows and lint them for token permissions, unpinned actions and script injection

## Problem

GitHub Actions cost is invisible until the bill arrives, especially with large matrices on macOS runners. Workflows also often run with broad token permissions, mutable action tags and untrusted event fields interpolated into scripts. gha-lint is a small Go CLI and library that reports an upper-bound minutes estimate and these findings with file and line, so they can be reviewed in pull requests.

## Install

```sh
go get github.com/Trysac/u084-gha-lint
```

Requires Go 1.23 or newer. The only dependency outside the standard library is `go.yaml.in/yaml/v3` (v3.0.4).

## Usage

```go
package main

import ghalint "github.com/Trysac/u084-gha-lint"

func main() {
	data := []byte("on: push\njobs:\n  b:\n    runs-on: macos-latest\n    timeout-minutes: 30\n")
	wf, err := ghalint.ParseWorkflow("ci.yml", data)
	if err != nil { panic(err) }
	cfg, _ := ghalint.LoadConfig([]byte("{}"))
	est := ghalint.EstimateCost(wf, cfg)
	println(est.MaxMinutes, est.WeightedMinutes, len(ghalint.Lint(wf, cfg)))
}
```

## Public API

- `ParseWorkflow(file string, data []byte) (*Workflow, error)`: parses YAML with line numbers. Input over 1 MiB is rejected.
- `Lint(wf *Workflow, cfg Config) []Finding`: rules GHA001 (no effective permissions, warning), GHA002 (write-all at any level or `contents: write` at workflow level, warning), GHA003 (action or reusable workflow not pinned to a 40-hex SHA, or docker image without `@sha256:` digest, error; local `./` actions ignored; owners listed in `allowTagOwners` exempt), GHA004 (`pull_request_target` checking out pull request code, error), GHA005 (untrusted context in `run:` or `actions/github-script` `script:`, error; `env:` values are not flagged), GHA007 (job without `timeout-minutes`, warning). Findings are sorted by file, line and rule.
- `EstimateCost(wf *Workflow, cfg Config) CostEstimate`: per job, matrix combinations are computed (product, then excludes, then includes) and capped at 256. Minutes equal combinations times `timeout-minutes` (or `defaultTimeout`). Weighted minutes are `ceil(minutes * multiplier)` for runners whose `runs-on` label starts with ubuntu, windows or macos. Other runners and reusable workflow jobs are listed in `Unknown`.
- `LoadConfig(data []byte) (Config, error)`: reads JSON with `disabled`, `allowTagOwners`, `defaultTimeout` (360) and `multipliers` (linux 1, windows 2, macos 10). Multipliers must be greater than 0.
- `Format(findings []Finding, est *CostEstimate, format string) (string, error)`: renders `text` or `json`.
- `Run(args []string, stdout, stderr io.Writer) int`: CLI entry point used by `cmd/gha-lint`. Exit code 0 means no findings, 1 means findings, 2 means usage or parse error.

### Errors

`*LintError` carries `Code`, `File`, `Line` and `Msg`, and unwraps to the sentinels `ErrParse`, `ErrTooLarge`, `ErrConfig` and `ErrUnknownFormat`. Use `errors.Is` and `errors.As` to branch on them.

### Limits

- Files over 1 MiB are rejected with `ErrTooLarge`.
- Matrix combinations are capped at 256 per job.
- Composite actions are skipped.
- go-yaml's alias expansion guard applies.

### CLI

`gha-lint [PATH ...] [--config FILE] [--format text|json] [--cost]`. Without paths it scans `.github/workflows` for `*.yml` and `*.yaml` files and does not follow symlinks.

## Why not actionlint?

actionlint and zizmor lint workflows but do not estimate cost. gha-lint adds an upper-bound minutes estimate next to its lint rules.

## Documentation

- [Architecture](docs/ARCHITECTURE.md) and [decisions](docs/decisions/)
- [Testing](docs/TESTING.md) and [security](docs/SECURITY.md)
- [Project record](docs/PROJECT.md) and [stack](docs/STACK.md)
- [Changelog](CHANGELOG.md) and [contributing guide](CONTRIBUTING.md)

## License

[MIT](LICENSE) (c) 2026 Miguel Gomez
