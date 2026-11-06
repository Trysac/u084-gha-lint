// Command gha-lint lints GitHub Actions workflows and estimates their cost.
package main

import (
	"os"

	"github.com/Trysac/u084-gha-lint"
)

func main() {
	os.Exit(ghalint.Run(os.Args[1:], os.Stdout, os.Stderr))
}
