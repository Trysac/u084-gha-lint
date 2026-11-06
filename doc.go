// Package ghalint is a small, dependency-free Go library: Estimate the billable minutes of GitHub Actions workflows and lint them for token permissions, unpinned actions and script injection
//
// The public API is every exported identifier of this root package. Functions are pure: they do
// no I/O, keep no global mutable state and report invalid input through returned errors.
package ghalint
