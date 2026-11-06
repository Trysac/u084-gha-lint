// Package ghalint estimates the billable minutes of GitHub Actions workflows
// and lints them for token permissions, unpinned actions and script injection.
//
// Use [ParseWorkflow] to load a workflow, [Lint] to get findings,
// [EstimateCost] for an upper-bound minutes estimate, [LoadConfig] to read a
// JSON configuration, [Format] to render results and [Run] for the CLI.
package ghalint
