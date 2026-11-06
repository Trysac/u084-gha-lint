package ghalint

import (
	"math"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

const maxCombos = 256
const maxMaterialize = 10000

// JobCost is the estimate of one job.
type JobCost struct {
	ID           string `json:"id"`
	Line         int    `json:"line"`
	OS           string `json:"os"`
	Combinations int    `json:"combinations"`
	TimeoutMin   int    `json:"timeoutMinutes"`
	Minutes      int    `json:"minutes"`
	Weighted     int    `json:"weightedMinutes"`
}

// CostEstimate is an upper-bound estimate of billable minutes.
type CostEstimate struct {
	Jobs            []JobCost `json:"jobs"`
	MaxMinutes      int       `json:"maxMinutes"`
	WeightedMinutes int       `json:"weightedMinutes"`
	Unknown         []string  `json:"unknown"`
}

// EstimateCost estimates billable minutes per job: matrix combinations
// (exclude first, then include, capped at 256, GitHub's matrix limit) times
// timeout-minutes (cfg.DefaultTimeout, 360 by default, when absent). Weighted
// minutes are ceil(minutes * multiplier) for the runner OS. The bundled
// multipliers linux 1, windows 2 and macos 10 are the minute multipliers table
// of the GitHub Docs page "About billing for GitHub Actions" as published in
// 2025; Config.Multipliers overrides them. Jobs whose runner or matrix cannot
// be determined (self-hosted, expressions, reusable workflows) are listed in
// Unknown and excluded from both totals.
func EstimateCost(wf *Workflow, cfg Config) CostEstimate {
	cfg = cfg.normalized()
	est := CostEstimate{Jobs: []JobCost{}, Unknown: []string{}}
	if wf == nil {
		return est
	}
	for _, job := range wf.Jobs {
		osName, ok := runnerOS(job.Node)
		combos, ok2 := matrixCombos(job.Node)
		if !ok || !ok2 {
			est.Unknown = append(est.Unknown, job.ID)
			continue
		}
		timeout := cfg.DefaultTimeout
		if _, t := get(job.Node, "timeout-minutes"); t != nil {
			if n, err := strconv.Atoi(t.Value); err == nil && n > 0 {
				timeout = n
			}
		}
		minutes := combos * timeout
		weighted := int(math.Ceil(float64(minutes) * cfg.Multipliers[osName]))
		est.Jobs = append(est.Jobs, JobCost{ID: job.ID, Line: job.Line, OS: osName, Combinations: combos, TimeoutMin: timeout, Minutes: minutes, Weighted: weighted})
		est.MaxMinutes += minutes
		est.WeightedMinutes += weighted
	}
	return est
}

func labelList(n *yaml.Node) []string {
	n = deref(n)
	if n == nil {
		return nil
	}
	switch n.Kind {
	case yaml.ScalarNode:
		return []string{n.Value}
	case yaml.SequenceNode:
		var out []string
		for _, c := range n.Content {
			if v, ok := scalarOf(c); ok {
				out = append(out, v)
			}
		}
		return out
	case yaml.MappingNode:
		_, l := get(n, "labels")
		if l != nil && l.Kind != yaml.MappingNode {
			return labelList(l)
		}
	}
	return nil
}

func runnerOS(job *yaml.Node) (string, bool) {
	_, ro := get(job, "runs-on")
	found := ""
	for _, l := range labelList(ro) {
		l = strings.ToLower(l)
		switch {
		case strings.Contains(l, "${{") || l == "self-hosted":
			return "", false
		case found == "" && strings.HasPrefix(l, "ubuntu"):
			found = "linux"
		case found == "" && strings.HasPrefix(l, "windows"):
			found = "windows"
		case found == "" && strings.HasPrefix(l, "macos"):
			found = "macos"
		}
	}
	return found, found != ""
}

func entries(seq *yaml.Node) []map[string]string {
	var out []map[string]string
	for _, item := range seq.Content {
		item = deref(item)
		if item == nil || item.Kind != yaml.MappingNode {
			continue
		}
		m := map[string]string{}
		for i := 0; i+1 < len(item.Content); i += 2 {
			m[item.Content[i].Value] = valStr(item.Content[i+1])
		}
		out = append(out, m)
	}
	return out
}

func clone(m map[string]string) map[string]string {
	n := make(map[string]string, len(m)+1)
	for k, v := range m {
		n[k] = v
	}
	return n
}

func excluded(c map[string]string, excludes []map[string]string) bool {
	for _, e := range excludes {
		if len(e) == 0 {
			continue
		}
		all := true
		for k, v := range e {
			if cv, ok := c[k]; !ok || cv != v {
				all = false
				break
			}
		}
		if all {
			return true
		}
	}
	return false
}

func matrixCombos(job *yaml.Node) (int, bool) {
	_, st := get(job, "strategy")
	_, m := get(st, "matrix")
	if m == nil {
		return 1, true
	}
	if m.Kind != yaml.MappingNode {
		return 0, false
	}
	var dimKeys []string
	dims := map[string][]string{}
	var includes, excludes []map[string]string
	for i := 0; i+1 < len(m.Content); i += 2 {
		key := m.Content[i].Value
		val := deref(m.Content[i+1])
		if val == nil || val.Kind != yaml.SequenceNode {
			return 0, false
		}
		switch key {
		case "include":
			includes = entries(val)
		case "exclude":
			excludes = entries(val)
		default:
			var vals []string
			for _, c := range val.Content {
				vals = append(vals, valStr(c))
			}
			dims[key] = vals
			dimKeys = append(dimKeys, key)
		}
	}
	prod := 1
	for _, k := range dimKeys {
		prod *= len(dims[k])
		if prod > maxMaterialize {
			return maxCombos, true
		}
	}
	if len(dimKeys) == 0 && len(includes) == 0 {
		return 1, true
	}
	var combos []map[string]string
	if len(dimKeys) > 0 {
		combos = []map[string]string{{}}
		for _, k := range dimKeys {
			var next []map[string]string
			for _, c := range combos {
				for _, v := range dims[k] {
					n := clone(c)
					n[k] = v
					next = append(next, n)
				}
			}
			combos = next
		}
	}
	var kept []map[string]string
	for _, c := range combos {
		if !excluded(c, excludes) {
			kept = append(kept, c)
		}
	}
	combos = kept
	nOrig := len(combos)
	for _, inc := range includes {
		matched := false
		for idx := 0; idx < nOrig; idx++ {
			c := combos[idx]
			ok := true
			for k, v := range inc {
				if _, orig := dims[k]; orig && c[k] != v {
					ok = false
					break
				}
			}
			if ok {
				for k, v := range inc {
					c[k] = v
				}
				matched = true
			}
		}
		if !matched {
			combos = append(combos, clone(inc))
		}
	}
	n := len(combos)
	if n > maxCombos {
		n = maxCombos
	}
	return n, true
}
