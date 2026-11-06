# Project record

## Objective

Estimate the billable minutes of GitHub Actions workflows and lint them for token permissions, unpinned actions and script injection. The cost estimate is an upper bound computed from matrix size, timeout and runner multipliers from GitHub Docs "About billing for GitHub Actions" (2025). The lint rules report findings with file and line for review in pull requests.

## Scope

- In scope: the public API documented in [README.md](../README.md): `ParseWorkflow`, `Lint`, `EstimateCost`, `LoadConfig`, `Format` and `Run`, with rules GHA001, GHA002, GHA003, GHA004, GHA005 and GHA007.
- Out of scope: expression typing, shellcheck, auto-pinning of actions, fetching action metadata, and anything that needs dependencies or I/O without an accepted ADR.

## Timeline

| Date | Event |
|---|---|
| 2026 | Repository created from the factory utility template. |

## Effort metrics

Values that have not been measured are shown as pending measurement.

| Metric | Value |
|---|---|
| Tokens (input / output) | 19801 / 323665 |
| Hours (human) | pending measurement |
| Cost (USD) | 1.6080 |

## Factory run u084 (measured from API usage)

| Phase | Model | Batch | Input | Output | Cache write | Cache read | Cost (USD) |
|---|---|---|---|---|---|---|---|
| generation | claude-sonnet-5-5 | yes | 2797 | 128000 | 4285 | 0 | 0.6514 |
| generation | claude-sonnet-5-5 | yes | 2797 | 128000 | 4285 | 0 | 0.6514 |
| generation | claude-sonnet-5-5 | yes | 2797 | 59433 | 0 | 4285 | 0.3004 |
| audit | claude-sonnet-5-5 | no | 0 | 0 | 0 | 0 | 0.0000 |
| docs | claude-haiku-5-5 | no | 7125 | 8232 | 0 | 0 | 0.0048 |

Total: $1.6080. Prices: Anthropic list prices.
