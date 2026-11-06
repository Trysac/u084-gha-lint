package ghalint

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const defaultDir = ".github/workflows"

func fail(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "gha-lint: %v\n", err)
	return 2
}

func readLimited(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
}

func collect(paths []string) ([]string, error) {
	var files []string
	for _, p := range paths {
		info, err := os.Lstat(p)
		if err != nil {
			return nil, err
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			continue
		case info.IsDir():
			entries, err := os.ReadDir(p)
			if err != nil {
				return nil, err
			}
			for _, e := range entries {
				ext := strings.ToLower(filepath.Ext(e.Name()))
				if e.Type().IsRegular() && (ext == ".yml" || ext == ".yaml") {
					files = append(files, filepath.Join(p, e.Name()))
				}
			}
		default:
			files = append(files, p)
		}
	}
	return files, nil
}

// Run is the gha-lint command: gha-lint [PATH ...] [--config FILE]
// [--format text|json] [--cost]. It returns 0 without findings, 1 with
// findings and 2 on usage, I/O or parse errors.
func Run(args []string, stdout, stderr io.Writer) int {
	cfgPath, format, cost := "", "text", false
	var paths []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--cost" || a == "-cost":
			cost = true
		case a == "--config" || a == "-config" || a == "--format" || a == "-format":
			if i+1 >= len(args) {
				return fail(stderr, fmt.Errorf("%s needs a value", a))
			}
			i++
			if strings.HasSuffix(a, "config") {
				cfgPath = args[i]
			} else {
				format = args[i]
			}
		case strings.HasPrefix(a, "--config="):
			cfgPath = strings.TrimPrefix(a, "--config=")
		case strings.HasPrefix(a, "--format="):
			format = strings.TrimPrefix(a, "--format=")
		case strings.HasPrefix(a, "-") && a != "-":
			return fail(stderr, fmt.Errorf("unknown flag %s; usage: gha-lint [PATH ...] [--config FILE] [--format text|json] [--cost]", short(a)))
		default:
			paths = append(paths, a)
		}
	}
	if format != "text" && format != "json" {
		return fail(stderr, &LintError{Code: "unknown_format", Msg: short("unknown format " + format)})
	}
	cfg := DefaultConfig()
	if cfgPath != "" {
		data, err := readLimited(cfgPath)
		if err != nil {
			return fail(stderr, err)
		}
		if cfg, err = LoadConfig(data); err != nil {
			return fail(stderr, err)
		}
	}
	if len(paths) == 0 {
		paths = []string{defaultDir}
	}
	files, err := collect(paths)
	if err != nil {
		return fail(stderr, err)
	}
	var all []Finding
	var est *CostEstimate
	if cost {
		est = &CostEstimate{Jobs: []JobCost{}, Unknown: []string{}}
	}
	for _, f := range files {
		data, err := readLimited(f)
		if err != nil {
			return fail(stderr, err)
		}
		wf, err := ParseWorkflow(f, data)
		if err != nil {
			return fail(stderr, err)
		}
		all = append(all, Lint(wf, cfg)...)
		if est != nil {
			e := EstimateCost(wf, cfg)
			est.Jobs = append(est.Jobs, e.Jobs...)
			est.MaxMinutes += e.MaxMinutes
			est.WeightedMinutes += e.WeightedMinutes
			for _, u := range e.Unknown {
				if len(files) > 1 {
					u = f + ":" + u
				}
				est.Unknown = append(est.Unknown, u)
			}
		}
	}
	out, err := Format(all, est, format)
	if err != nil {
		return fail(stderr, err)
	}
	fmt.Fprint(stdout, out)
	if len(all) > 0 {
		return 1
	}
	return 0
}
