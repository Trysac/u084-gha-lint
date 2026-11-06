package ghalint

import (
	"fmt"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

const sevWarning = "warning"
const sevError = "error"

type adder func(rule, severity string, line int, msg string)

var taintPatterns [][]string

func init() {
	list := strings.Fields("event.issue.title event.issue.body event.pull_request.title event.pull_request.body event.comment.body event.review.body event.review_comment.body event.pages.*.page_name event.commits.*.message event.head_commit.message event.head_commit.author.email event.head_commit.author.name event.commits.*.author.email event.commits.*.author.name event.pull_request.head.ref event.pull_request.head.label event.pull_request.head.repo.default_branch event.discussion.title event.discussion.body event.workflow_run.head_branch event.workflow_run.head_commit.message head_ref")
	for _, p := range list {
		taintPatterns = append(taintPatterns, strings.Split(p, "."))
	}
}

// Lint runs the rules GHA001, GHA002, GHA003, GHA004, GHA005 and GHA007 on
// wf. Findings are sorted by file, line and rule.
func Lint(wf *Workflow, cfg Config) []Finding {
	if wf == nil || wf.root == nil {
		return nil
	}
	cfg = cfg.normalized()
	var out []Finding
	add := func(rule, sev string, line int, msg string) {
		for _, d := range cfg.Disabled {
			if strings.EqualFold(d, rule) {
				return
			}
		}
		out = append(out, Finding{Rule: rule, Severity: sev, File: wf.File, Line: line, Message: short(msg)})
	}
	wpk, wpv := get(wf.root, "permissions")
	checkPerms(wpv, true, add)
	events := eventsOf(wf.root)
	for _, job := range wf.Jobs {
		jpk, jpv := get(job.Node, "permissions")
		if wpk == nil && jpk == nil {
			add("GHA001", sevWarning, job.Line, "no permissions declared at workflow or job level; the token gets default scopes")
		}
		checkPerms(jpv, false, add)
		if _, u := get(job.Node, "uses"); u != nil {
			checkUses(u, cfg, add)
		} else if tk, _ := get(job.Node, "timeout-minutes"); tk == nil {
			add("GHA007", sevWarning, job.Line, "job has no timeout-minutes")
		}
		checkSteps(job.Node, events, cfg, add)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Rule < b.Rule
	})
	return out
}

func eventsOf(root *yaml.Node) map[string]bool {
	ev := map[string]bool{}
	_, on := get(root, "on")
	if on == nil {
		return ev
	}
	switch on.Kind {
	case yaml.ScalarNode:
		ev[on.Value] = true
	case yaml.SequenceNode:
		for _, n := range on.Content {
			if s, ok := scalarOf(n); ok {
				ev[s] = true
			}
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(on.Content); i += 2 {
			ev[on.Content[i].Value] = true
		}
	}
	return ev
}

func checkPerms(v *yaml.Node, workflow bool, add adder) {
	if v == nil {
		return
	}
	if s, ok := scalarOf(v); ok && s == "write-all" {
		add("GHA002", sevWarning, v.Line, "permissions: write-all grants write access to every scope")
		return
	}
	if workflow && v.Kind == yaml.MappingNode {
		_, c := get(v, "contents")
		if s, ok := scalarOf(c); ok && s == "write" {
			add("GHA002", sevWarning, c.Line, "contents: write at workflow level; scope it to the job that needs it")
		}
	}
}

func isHex40(s string) bool {
	if len(s) != 40 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9') && !(c >= 'a' && c <= 'f') && !(c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

func checkUses(n *yaml.Node, cfg Config, add adder) {
	val, ok := scalarOf(n)
	if !ok {
		return
	}
	val = strings.TrimSpace(val)
	if strings.HasPrefix(val, "./") {
		return
	}
	if strings.HasPrefix(val, "docker://") {
		if !strings.Contains(val, "@sha256:") {
			add("GHA003", sevError, n.Line, fmt.Sprintf("docker image %s is not pinned by @sha256: digest", val))
		}
		return
	}
	ref := ""
	if i := strings.LastIndex(val, "@"); i >= 0 {
		ref = val[i+1:]
	}
	if isHex40(ref) {
		return
	}
	owner := strings.ToLower(strings.SplitN(val, "/", 2)[0])
	for _, o := range cfg.AllowTagOwners {
		if strings.ToLower(o) == owner {
			return
		}
	}
	add("GHA003", sevError, n.Line, fmt.Sprintf("%s is not pinned to a 40-character commit SHA", val))
}

func badRef(s string) bool {
	l := strings.ToLower(s)
	return strings.Contains(l, "pull_request.head") || strings.Contains(l, "refs/pull/") || strings.Contains(l, "head_ref")
}

func checkSteps(job *yaml.Node, events map[string]bool, cfg Config, add adder) {
	_, st := get(job, "steps")
	if st == nil || st.Kind != yaml.SequenceNode {
		return
	}
	for _, raw := range st.Content {
		step := deref(raw)
		_, uv := get(step, "uses")
		_, with := get(step, "with")
		name := ""
		if uv != nil {
			checkUses(uv, cfg, add)
			s, _ := scalarOf(uv)
			name = strings.ToLower(strings.SplitN(s, "@", 2)[0])
		}
		if name == "actions/checkout" && events["pull_request_target"] {
			_, ref := get(with, "ref")
			if s, ok := scalarOf(ref); ok && badRef(s) {
				add("GHA004", sevError, ref.Line, "pull_request_target checks out untrusted pull request code")
			}
		}
		if _, run := get(step, "run"); run != nil {
			scanScript(run, add)
		}
		if name == "actions/github-script" {
			_, sc := get(with, "script")
			scanScript(sc, add)
		}
	}
}

func scanScript(n *yaml.Node, add adder) {
	s, ok := scalarOf(n)
	if !ok {
		return
	}
	n = deref(n)
	literal := n.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0
	base := n.Line
	if literal {
		base++
	}
	pos := 0
	for {
		i := strings.Index(s[pos:], "${{")
		if i < 0 {
			return
		}
		start := pos + i + 3
		end, next := len(s), len(s)
		if j := strings.Index(s[start:], "}}"); j >= 0 {
			end = start + j
			next = end + 2
		}
		line := n.Line
		if literal {
			line = base + strings.Count(s[:pos+i], "\n")
		}
		for _, p := range taintedPaths(s[start:end]) {
			add("GHA005", sevError, line, fmt.Sprintf("untrusted context %s used in script; pass it through env: instead", p))
		}
		pos = next
	}
}

func isIdent(c byte) bool {
	return c == '_' || c == '-' || c == '*' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func taintedPaths(expr string) []string {
	var out []string
	i := 0
	for i < len(expr) {
		if expr[i] == '\'' {
			j := i + 1
			for j < len(expr) && expr[j] != '\'' {
				j++
			}
			i = j + 1
			continue
		}
		if strings.HasPrefix(expr[i:], "github") && (i == 0 || (!isIdent(expr[i-1]) && expr[i-1] != '.')) && (i+6 >= len(expr) || !isIdent(expr[i+6])) {
			segs, end := parseSegs(expr, i+6)
			if isTainted(segs) {
				out = append(out, "github."+strings.Join(segs, "."))
			}
			i = end
			continue
		}
		i++
	}
	return out
}

func parseSegs(expr string, p int) ([]string, int) {
	var segs []string
	for p < len(expr) {
		switch expr[p] {
		case '.':
			q := p + 1
			for q < len(expr) && isIdent(expr[q]) {
				q++
			}
			if q == p+1 {
				return segs, p
			}
			segs = append(segs, strings.ToLower(expr[p+1:q]))
			p = q
		case '[':
			q := p + 1
			for q < len(expr) && expr[q] == ' ' {
				q++
			}
			if q < len(expr) && (expr[q] == '\'' || expr[q] == '"') {
				quote := expr[q]
				r := q + 1
				for r < len(expr) && expr[r] != quote {
					r++
				}
				if r >= len(expr) {
					return segs, len(expr)
				}
				seg := expr[q+1 : r]
				r++
				for r < len(expr) && expr[r] == ' ' {
					r++
				}
				if r >= len(expr) || expr[r] != ']' {
					return segs, r
				}
				segs = append(segs, strings.ToLower(seg))
				p = r + 1
			} else {
				r := strings.IndexByte(expr[q:], ']')
				if r < 0 {
					return segs, len(expr)
				}
				segs = append(segs, strings.ToLower(strings.TrimSpace(expr[q:q+r])))
				p = q + r + 1
			}
		default:
			return segs, p
		}
	}
	return segs, p
}

func isTainted(segs []string) bool {
	for _, p := range taintPatterns {
		if len(segs) < len(p) {
			continue
		}
		match := true
		for k := range p {
			if p[k] != "*" && p[k] != segs[k] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
