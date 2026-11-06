package ghalint

import (
	"encoding/json"
	"fmt"
	"strings"
)

type report struct {
	Findings []Finding     `json:"findings"`
	Cost     *CostEstimate `json:"cost,omitempty"`
}

// Format renders findings and an optional cost estimate as "text" or "json".
// Text findings look like file:line: RULE severity message. Any other format
// fails with ErrUnknownFormat.
func Format(findings []Finding, est *CostEstimate, format string) (string, error) {
	switch format {
	case "text":
		var b strings.Builder
		for _, f := range findings {
			fmt.Fprintf(&b, "%s:%d: %s %s %s\n", f.File, f.Line, f.Rule, f.Severity, f.Message)
		}
		if est != nil {
			for _, j := range est.Jobs {
				fmt.Fprintf(&b, "cost: %s (%s): %d combinations x %d min = %d min, weighted %d min\n", j.ID, j.OS, j.Combinations, j.TimeoutMin, j.Minutes, j.Weighted)
			}
			fmt.Fprintf(&b, "cost total: max %d min, weighted %d min\n", est.MaxMinutes, est.WeightedMinutes)
			if len(est.Unknown) > 0 {
				fmt.Fprintf(&b, "cost unknown: %s\n", strings.Join(est.Unknown, ", "))
			}
		}
		return b.String(), nil
	case "json":
		rep := report{Findings: findings}
		if rep.Findings == nil {
			rep.Findings = []Finding{}
		}
		if est != nil {
			c := *est
			if c.Jobs == nil {
				c.Jobs = []JobCost{}
			}
			if c.Unknown == nil {
				c.Unknown = []string{}
			}
			rep.Cost = &c
		}
		b, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			return "", err
		}
		return string(b) + "\n", nil
	}
	return "", &LintError{Code: "unknown_format", Msg: short("unknown format " + format)}
}
