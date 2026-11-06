package ghalint_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Trysac/u084-gha-lint"
)

var sha = strings.Repeat("a", 40)

func parse(t *testing.T, src string) *ghalint.Workflow {
	t.Helper()
	wf, err := ghalint.ParseWorkflow("ci.yml", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return wf
}

func job(runs, extra string) string {
	return "on: push\npermissions: {}\njobs:\n  j:\n    runs-on: " + runs + "\n" + extra
}

func steps(s string) string {
	return "    timeout-minutes: 1\n    steps:\n" + s
}

func has(fs []ghalint.Finding, rule string, line int) bool {
	for _, f := range fs {
		if f.Rule == rule && (line == 0 || f.Line == line) {
			return true
		}
	}
	return false
}

func lintSrc(t *testing.T, src string, cfg ghalint.Config) []ghalint.Finding {
	t.Helper()
	return ghalint.Lint(parse(t, src), cfg)
}

func TestCostMacos(t *testing.T) {
	src := job("macos-latest", "    timeout-minutes: 30\n    strategy:\n      matrix:\n        os: [a, b, c]\n        v: [1, 2]\n")
	est := ghalint.EstimateCost(parse(t, src), ghalint.DefaultConfig())
	if est.MaxMinutes != 180 || est.WeightedMinutes != 1800 {
		t.Fatalf("got %+v", est)
	}
	cfg, err := ghalint.LoadConfig([]byte(`{"multipliers": {"macos": 6}}`))
	if err != nil {
		t.Fatal(err)
	}
	est = ghalint.EstimateCost(parse(t, src), cfg)
	if est.WeightedMinutes != 1080 {
		t.Fatalf("got %+v", est)
	}
}

func TestCostOtherOS(t *testing.T) {
	est := ghalint.EstimateCost(parse(t, job("windows-2022", "    timeout-minutes: 10\n")), ghalint.Config{})
	if est.WeightedMinutes != 20 || est.MaxMinutes != 10 {
		t.Fatalf("got %+v", est)
	}
	est = ghalint.EstimateCost(parse(t, job("[ubuntu-latest]", "    timeout-minutes: ${{ inputs.t }}\n")), ghalint.DefaultConfig())
	if est.MaxMinutes != 360 {
		t.Fatalf("got %+v", est)
	}
	est = ghalint.EstimateCost(nil, ghalint.DefaultConfig())
	if len(est.Jobs) != 0 {
		t.Fatal("nil workflow")
	}
}

func list(n int) string {
	return "[" + strings.TrimSuffix(strings.Repeat("1, ", n), ", ") + "]"
}

func mx(body string) string {
	return job("ubuntu-latest", "    timeout-minutes: 1\n    strategy:\n      matrix:\n"+body)
}

func TestMatrixCounts(t *testing.T) {
	cases := map[string]int{
		"        a: [1, 2]\n        b: [x, y]\n        exclude:\n          - {a: 1, b: x}\n        include:\n          - {a: 1, b: x}\n          - {a: 2, c: z}\n": 4,
		"        include:\n          - {a: 1}\n          - {a: 2}\n": 2,
		"        a: []\n": 0,
		"        a: [1, 2]\n        exclude: [{a: 1}, {a: 2}]\n":                                  0,
		"        a: [1, 2]\n        exclude: [{}, x]\n":                                           2,
		"        a: " + list(10) + "\n        b: " + list(10) + "\n        c: " + list(10) + "\n": 256,
		"        a: " + list(30) + "\n        b: " + list(30) + "\n        c: " + list(30) + "\n": 256,
	}
	for body, want := range cases {
		est := ghalint.EstimateCost(parse(t, mx(body)), ghalint.DefaultConfig())
		if est.MaxMinutes != want {
			t.Errorf("%q: got %d want %d", body, est.MaxMinutes, want)
		}
	}
	est := ghalint.EstimateCost(parse(t, job("ubuntu-latest", "    timeout-minutes: 1\n    strategy:\n      matrix: {}\n")), ghalint.DefaultConfig())
	if est.MaxMinutes != 1 {
		t.Fatalf("empty matrix: %+v", est)
	}
}

func TestUnknownJobs(t *testing.T) {
	srcs := []string{
		job("self-hosted", ""),
		job("[self-hosted, linux]", ""),
		job("{group: g}", ""),
		job("${{ matrix.os }}", ""),
		job("ubuntu-latest", "    strategy:\n      matrix: ${{ fromJSON(needs.a.outputs.m) }}\n"),
		job("ubuntu-latest", "    strategy:\n      matrix:\n        a: ${{ fromJSON(x) }}\n"),
		job("ubuntu-latest", "    strategy:\n      matrix:\n        include: ${{ fromJSON(x) }}\n"),
		job("custom-label", ""),
		"on: push\njobs:\n  j:\n    steps: []\n",
		"on: push\njobs:\n  j:\n    uses: o/r/.github/workflows/x.yml@v1\n",
	}
	for _, s := range srcs {
		est := ghalint.EstimateCost(parse(t, s), ghalint.DefaultConfig())
		if len(est.Unknown) != 1 || est.Unknown[0] != "j" || est.MaxMinutes != 0 || est.WeightedMinutes != 0 || len(est.Jobs) != 0 {
			t.Errorf("%q: got %+v", s, est)
		}
	}
}

func TestGHA001(t *testing.T) {
	src := "on: push\njobs:\n  a:\n    runs-on: x\n  b:\n    runs-on: x\n    permissions: read-all\n"
	n := 0
	for _, f := range lintSrc(t, src, ghalint.DefaultConfig()) {
		if f.Rule == "GHA001" {
			n++
			if f.Line != 3 || f.Severity != "warning" {
				t.Fatalf("bad finding %+v", f)
			}
		}
	}
	if n != 1 {
		t.Fatalf("want 1 GHA001, got %d", n)
	}
	if has(lintSrc(t, "on: push\npermissions: {}\njobs:\n  a:\n    runs-on: x\n", ghalint.DefaultConfig()), "GHA001", 0) {
		t.Fatal("permissions: {} must silence GHA001")
	}
}

func TestGHA002(t *testing.T) {
	cfg := ghalint.DefaultConfig()
	if !has(lintSrc(t, "on: push\npermissions: write-all\njobs: {}\n", cfg), "GHA002", 2) {
		t.Fatal("workflow write-all")
	}
	if !has(lintSrc(t, "on: push\njobs:\n  a:\n    runs-on: x\n    permissions: write-all\n", cfg), "GHA002", 5) {
		t.Fatal("job write-all")
	}
	if !has(lintSrc(t, "on: push\npermissions:\n  contents: write\njobs: {}\n", cfg), "GHA002", 3) {
		t.Fatal("workflow contents write")
	}
	if has(lintSrc(t, "on: push\npermissions: {}\njobs:\n  a:\n    runs-on: x\n    permissions:\n      contents: write\n", cfg), "GHA002", 0) {
		t.Fatal("job contents write is allowed")
	}
}

type usesCase struct {
	uses   string
	owners []string
	want   bool
}

func TestGHA003(t *testing.T) {
	cases := []usesCase{
		{"actions/checkout@v4", []string{}, true},
		{"actions/checkout@" + sha, []string{}, false},
		{"actions/checkout@v4", []string{"actions"}, false},
		{"./local-action", nil, false},
		{"docker://alpine:3", nil, true},
		{"docker://alpine@sha256:abc", nil, false},
		{"owner/repo", nil, true},
		{"actions/checkout@" + sha[:39], nil, true},
		{"actions/checkout@" + strings.Repeat("g", 40), nil, true},
	}
	for _, c := range cases {
		cfg := ghalint.DefaultConfig()
		cfg.AllowTagOwners = c.owners
		fs := lintSrc(t, job("x", steps("      - uses: "+c.uses+"\n")), cfg)
		if has(fs, "GHA003", 8) != c.want {
			t.Errorf("%s: want %v, got %+v", c.uses, c.want, fs)
		}
	}
	fs := lintSrc(t, "on: push\npermissions: {}\njobs:\n  r:\n    uses: o/r/.github/workflows/x.yml@v1\n", ghalint.DefaultConfig())
	if !has(fs, "GHA003", 5) || fs[0].Severity != "error" {
		t.Fatalf("reusable workflow: %+v", fs)
	}
}

func prt(event, ref string) string {
	return "on: " + event + "\npermissions: {}\njobs:\n  j:\n    runs-on: x\n" + steps("      - uses: actions/checkout@"+sha+"\n        with:\n          ref: "+ref+"\n")
}

func TestGHA004(t *testing.T) {
	refs := []string{"${{ github.event.pull_request.head.sha }}", "${{ github.event.pull_request.head.ref }}", "refs/pull/${{ github.event.number }}/merge"}
	for _, r := range refs {
		if !has(lintSrc(t, prt("pull_request_target", r), ghalint.DefaultConfig()), "GHA004", 10) {
			t.Errorf("want GHA004 for %s", r)
		}
		if has(lintSrc(t, prt("pull_request", r), ghalint.DefaultConfig()), "GHA004", 0) {
			t.Errorf("pull_request must not flag %s", r)
		}
	}
	if has(lintSrc(t, prt("[push, pull_request_target]", "main"), ghalint.DefaultConfig()), "GHA004", 0) {
		t.Fatal("safe ref flagged")
	}
	if !has(lintSrc(t, prt("{pull_request_target: {}}", "${{ github.head_ref }}"), ghalint.DefaultConfig()), "GHA004", 10) {
		t.Fatal("mapping event form")
	}
}

func runStep(cmd string) string {
	return job("x", steps("      - run: "+cmd+"\n"))
}

func TestGHA005(t *testing.T) {
	cfg := ghalint.DefaultConfig()
	ctxs := strings.Fields("issue.title issue.body pull_request.title pull_request.body comment.body review.body review_comment.body pages.*.page_name commits.*.message head_commit.message head_commit.author.email head_commit.author.name commits.*.author.email commits.*.author.name pull_request.head.ref pull_request.head.label pull_request.head.repo.default_branch discussion.title discussion.body workflow_run.head_branch workflow_run.head_commit.message")
	if len(ctxs) != 21 {
		t.Fatalf("expected 21 event contexts, got %d", len(ctxs))
	}
	for _, c := range ctxs {
		if !has(lintSrc(t, runStep("echo ${{ github.event."+c+" }}"), cfg), "GHA005", 8) {
			t.Errorf("missing GHA005 for %s", c)
		}
	}
	bad := []string{
		"echo ${{ github.head_ref }}",
		"echo ${{ github.event.pull_request.head.ref }}",
		"echo ${{ github['event']['issue']['title'] }}",
		"echo ${{ toJSON(github.event.issue.body) }}",
		"echo ${{ github.event.commits[0].message }}",
		"echo ${{ github.event.issue.body.x }}",
		"echo ${{ 1 }} ${{ github[\"event\"][ 'issue' ].title }}",
	}
	for _, b := range bad {
		if !has(lintSrc(t, runStep(b), cfg), "GHA005", 8) {
			t.Errorf("missing GHA005 for %s", b)
		}
	}
	good := []string{
		"echo ${{ github.event.pull_request.number }}",
		"echo ${{ github.event.issue.title_x }}",
		"echo ${{ 'github.event.issue.title' }}",
		"echo ${{ mygithub.event.issue.title }}",
		"echo ${{ githubx.head_ref }}",
		"echo ${{ x.github.head_ref }}",
		"echo ${{ github.event }}",
		"echo ${{ github[ }}",
		"echo ${{ github['a }}",
		"echo ${{ github['a' }}",
		"echo ${{ github.event.issue.title",
	}
	for _, g := range good[:7] {
		if has(lintSrc(t, runStep(g), cfg), "GHA005", 0) {
			t.Errorf("unexpected GHA005 for %s", g)
		}
	}
	for _, g := range good[7:10] {
		if has(lintSrc(t, runStep(g), cfg), "GHA005", 0) {
			t.Errorf("unexpected GHA005 for %s", g)
		}
	}
	if !has(lintSrc(t, runStep(good[10]), cfg), "GHA005", 8) {
		t.Error("unterminated expression should still be scanned")
	}
	env := job("x", steps("      - run: echo \"$T\"\n        env:\n          T: ${{ github.head_ref }}\n"))
	if has(lintSrc(t, env, cfg), "GHA005", 0) {
		t.Fatal("env values must not be flagged")
	}
	multi := job("x", steps("      - run: |\n          echo hi\n          echo ${{ github.head_ref }}\n"))
	if !has(lintSrc(t, multi, cfg), "GHA005", 10) {
		t.Fatal("block scalar line")
	}
	script := job("x", steps("      - uses: actions/github-script@"+sha+"\n        with:\n          script: |\n            core.info('${{ github.event.issue.title }}')\n"))
	if !has(lintSrc(t, script, cfg), "GHA005", 11) {
		t.Fatal("github-script")
	}
}

func TestGHA007AndConfig(t *testing.T) {
	fs := lintSrc(t, job("x", ""), ghalint.DefaultConfig())
	if !has(fs, "GHA007", 4) || !has(fs, "GHA007", 0) {
		t.Fatalf("got %+v", fs)
	}
	cfg, err := ghalint.LoadConfig([]byte(`{"disabled": ["gha007"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if has(lintSrc(t, job("x", ""), cfg), "GHA007", 0) {
		t.Fatal("disabled rule reported")
	}
	if ghalint.Lint(nil, cfg) != nil {
		t.Fatal("nil workflow")
	}
}

func TestSorting(t *testing.T) {
	src := "on: push\njobs:\n  b:\n    runs-on: x\n  a:\n    runs-on: x\n"
	fs := lintSrc(t, src, ghalint.DefaultConfig())
	for i := 1; i < len(fs); i++ {
		if fs[i-1].Line > fs[i].Line || (fs[i-1].Line == fs[i].Line && fs[i-1].Rule > fs[i].Rule) {
			t.Fatalf("unsorted: %+v", fs)
		}
	}
}

func TestParseErrors(t *testing.T) {
	_, err := ghalint.ParseWorkflow("ci.yml", []byte("a: [\n"))
	var le *ghalint.LintError
	if !errors.Is(err, ghalint.ErrParse) || !errors.As(err, &le) || le.File != "ci.yml" || le.Line < 1 {
		t.Fatalf("got %v", err)
	}
	for _, s := range []string{"", "- a\n"} {
		if _, err := ghalint.ParseWorkflow("x.yml", []byte(s)); !errors.Is(err, ghalint.ErrParse) {
			t.Fatalf("%q: got %v", s, err)
		}
	}
	_, err = ghalint.ParseWorkflow("big.yml", make([]byte, 1_048_577))
	if !errors.Is(err, ghalint.ErrTooLarge) {
		t.Fatalf("got %v", err)
	}
	_, err = ghalint.ParseWorkflow("edge.yml", bytes.Repeat([]byte("#"), 1<<20))
	if errors.Is(err, ghalint.ErrTooLarge) {
		t.Fatal("exactly 1 MiB must not be too large")
	}
}

func TestLintErrorMethods(t *testing.T) {
	if (&ghalint.LintError{Code: "x"}).Unwrap() != nil {
		t.Fatal("unknown code")
	}
	if got := (&ghalint.LintError{Code: "parse", File: "a.yml", Line: 2, Msg: "m"}).Error(); got != "a.yml:2: parse: m" {
		t.Fatal(got)
	}
	if got := (&ghalint.LintError{Code: "config", Msg: "m"}).Error(); got != "config: m" {
		t.Fatal(got)
	}
	long := &ghalint.LintError{Code: "unknown_format"}
	if !errors.Is(long, ghalint.ErrUnknownFormat) {
		t.Fatal("unwrap")
	}
}

func TestLoadConfig(t *testing.T) {
	cfg, err := ghalint.LoadConfig([]byte(`{"allowTagOwners": ["actions"], "defaultTimeout": 10, "multipliers": {"windows": 3}}`))
	if err != nil || cfg.DefaultTimeout != 10 || cfg.Multipliers["windows"] != 3 || cfg.Multipliers["macos"] != 10 || len(cfg.AllowTagOwners) != 1 {
		t.Fatalf("got %+v, %v", cfg, err)
	}
	bad := []string{"", "{", `{"x": 1}`, `{"multipliers": {"macos": 0}}`, `{"multipliers": {"macos": -1}}`, `{"multipliers": {"bsd": 1}}`, `{"defaultTimeout": 0}`, `{} {}`}
	for _, b := range bad {
		if _, err := ghalint.LoadConfig([]byte(b)); !errors.Is(err, ghalint.ErrConfig) {
			t.Errorf("%q: got %v", b, err)
		}
	}
	if _, err := ghalint.LoadConfig(make([]byte, 1_048_577)); !errors.Is(err, ghalint.ErrTooLarge) {
		t.Fatalf("got %v", err)
	}
}

func TestFormat(t *testing.T) {
	wf := parse(t, "on: push\njobs:\n  a:\n    runs-on: macos-latest\n    timeout-minutes: 2\n  b:\n    runs-on: self-hosted\n")
	est := ghalint.EstimateCost(wf, ghalint.DefaultConfig())
	fs := []ghalint.Finding{{Rule: "GHA001", Severity: "warning", File: "ci.yml", Line: 3, Message: "m"}}
	text, err := ghalint.Format(fs, &est, "text")
	if err != nil || !strings.Contains(text, "ci.yml:3: GHA001 warning m\n") || !strings.Contains(text, "cost total: max 2 min, weighted 20 min") || !strings.Contains(text, "cost unknown: b") {
		t.Fatalf("got %q, %v", text, err)
	}
	out, err := ghalint.Format(nil, nil, "json")
	if err != nil || !strings.Contains(out, `"findings": []`) {
		t.Fatalf("got %q, %v", out, err)
	}
	out, err = ghalint.Format(fs, &ghalint.CostEstimate{}, "json")
	if err != nil || !json.Valid([]byte(out)) || !strings.Contains(out, `"jobs": []`) {
		t.Fatalf("got %q, %v", out, err)
	}
	out, err = ghalint.Format(fs, &est, "json")
	if err != nil || !strings.Contains(out, `"weightedMinutes": 20`) {
		t.Fatalf("got %q, %v", out, err)
	}
	if _, err := ghalint.Format(nil, nil, "xml"); !errors.Is(err, ghalint.ErrUnknownFormat) {
		t.Fatalf("got %v", err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func run(args ...string) (int, string, string) {
	var o, e bytes.Buffer
	c := ghalint.Run(args, &o, &e)
	return c, o.String(), e.String()
}

func chdir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	return dir
}

const badWf = "on: push\njobs:\n  build:\n    runs-on: ubuntu-latest\n    timeout-minutes: 5\n    steps:\n      - run: echo\n"
const cleanWf = "on: push\npermissions: {}\njobs:\n  j:\n    runs-on: ubuntu-latest\n    timeout-minutes: 5\n    steps:\n      - run: echo hi\n"

func TestRun(t *testing.T) {
	chdir(t)
	if c, _, e := run(); c != 2 || e == "" {
		t.Fatalf("missing default dir: %d %q", c, e)
	}
	writeFile(t, "ci.yml", badWf)
	c, out, _ := run("ci.yml")
	if c != 1 || !strings.HasPrefix(out, "ci.yml:3: GHA001 warning ") {
		t.Fatalf("got %d %q", c, out)
	}
	writeFile(t, ".github/workflows/ci.yml", cleanWf)
	writeFile(t, ".github/workflows/a.yaml", cleanWf)
	writeFile(t, ".github/workflows/n.txt", badWf)
	if err := os.Symlink("../../ci.yml", ".github/workflows/link.yml"); err != nil {
		t.Log("symlinks unavailable:", err)
	}
	if c, out, _ := run(); c != 0 || out != "" {
		t.Fatalf("default dir: %d %q", c, out)
	}
	if c, out, _ := run("--cost", ".github/workflows"); c != 0 || !strings.Contains(out, "cost total: max 10 min, weighted 10 min") {
		t.Fatalf("cost: %d %q", c, out)
	}
	if c, out, _ := run("--format", "json", "ci.yml"); c != 1 || !json.Valid([]byte(out)) {
		t.Fatalf("json: %d %q", c, out)
	}
	if c, _, _ := run("--format=json", "--cost", "ci.yml"); c != 1 {
		t.Fatalf("format=: %d", c)
	}
	writeFile(t, "cfg.json", `{"disabled": ["GHA001"]}`)
	if c, _, _ := run("ci.yml", "--config", "cfg.json"); c != 0 {
		t.Fatalf("config: %d", c)
	}
	if c, _, _ := run("--config=cfg.json", "ci.yml"); c != 0 {
		t.Fatalf("config=: %d", c)
	}
	writeFile(t, "badcfg.json", "{")
	writeFile(t, "bad.yml", "a: [\n")
	writeFile(t, "big.yml", strings.Repeat("#", 1_048_577))
	failing := [][]string{
		{"--bogus"},
		{"--format"},
		{"--format", "xml", "ci.yml"},
		{"--config", "missing.json", "ci.yml"},
		{"--config=badcfg.json", "ci.yml"},
		{"missing.yml"},
		{"bad.yml"},
		{"big.yml"},
	}
	for _, a := range failing {
		if c, _, e := run(a...); c != 2 || e == "" {
			t.Errorf("%v: got %d %q", a, c, e)
		}
	}
}
