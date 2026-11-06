# Use yaml.Node for line-accurate linting and an upper-bound cost model

Status: accepted (generated, pending human review)

## Context

The linter must report file and line for each finding and estimate GitHub Actions minutes without running workflows or fetching action metadata. Decoding into plain maps loses line numbers.

## Decision

Parse with go.yaml.in/yaml/v3 into yaml.Node and keep job key nodes for line numbers. Rules are small pure functions over nodes. GHA005 scans only ${{ }} expressions, parses property paths (dot, bracket, wildcard) and matches 22 known untrusted contexts, ignoring quoted string literals and env values. Cost multiplies matrix combinations (exclude first, then include, capped at 256) by timeout-minutes (default 360) and a per-OS multiplier from the GitHub Docs billing table (2025), overridable by JSON config. Jobs with undeterminable runner or matrix are reported as Unknown, not guessed.

## Consequences

Findings carry exact lines and the estimate is a conservative upper bound that ignores job dependencies, fail-fast and free-tier minutes. Dynamic matrices and self-hosted runners are not costed. The yaml dependency must be added to go.mod by a human. Expression typing, shellcheck and auto-pinning are out of scope.
