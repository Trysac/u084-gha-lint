package ghalint

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// MaxFileBytes is the largest workflow or configuration file accepted (1 MiB).
const MaxFileBytes = 1 << 20

// ErrParse marks YAML that cannot be parsed into a workflow.
var ErrParse = errors.New("ghalint: parse error")

// ErrTooLarge marks input larger than MaxFileBytes.
var ErrTooLarge = errors.New("ghalint: input too large")

// ErrConfig marks an invalid configuration.
var ErrConfig = errors.New("ghalint: invalid configuration")

// ErrUnknownFormat marks an unsupported output format.
var ErrUnknownFormat = errors.New("ghalint: unknown format")

var lineRe = regexp.MustCompile(`line (\d+)`)

// LintError is the typed error of the library. Code is stable and machine
// readable: parse, too_large, config or unknown_format.
type LintError struct {
	Code string
	File string
	Line int
	Msg  string
}

// Error implements error.
func (e *LintError) Error() string {
	if e.File != "" {
		return fmt.Sprintf("%s:%d: %s: %s", e.File, e.Line, e.Code, e.Msg)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Msg)
}

// Unwrap returns the sentinel error matching Code.
func (e *LintError) Unwrap() error {
	switch e.Code {
	case "parse":
		return ErrParse
	case "too_large":
		return ErrTooLarge
	case "config":
		return ErrConfig
	case "unknown_format":
		return ErrUnknownFormat
	}
	return nil
}

func short(s string) string {
	r := []rune(s)
	if len(r) > 200 {
		return string(r[:200])
	}
	return s
}

// Finding is one lint result.
type Finding struct {
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Message  string `json:"message"`
}

// Job is one job of a workflow.
type Job struct {
	ID   string
	Line int
	Node *yaml.Node
}

// Workflow is a parsed workflow file.
type Workflow struct {
	File string
	Jobs []Job
	root *yaml.Node
}

// ParseWorkflow parses a workflow. Files over MaxFileBytes fail with
// ErrTooLarge and invalid YAML with ErrParse, both as *LintError.
func ParseWorkflow(file string, data []byte) (*Workflow, error) {
	if len(data) > MaxFileBytes {
		return nil, &LintError{Code: "too_large", File: file, Msg: "file exceeds 1 MiB"}
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		line := 1
		if m := lineRe.FindStringSubmatch(err.Error()); m != nil {
			line, _ = strconv.Atoi(m[1])
		}
		return nil, &LintError{Code: "parse", File: file, Line: line, Msg: short(err.Error())}
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || deref(doc.Content[0]).Kind != yaml.MappingNode {
		return nil, &LintError{Code: "parse", File: file, Line: 1, Msg: "workflow must be a YAML mapping"}
	}
	root := deref(doc.Content[0])
	wf := &Workflow{File: file, root: root}
	_, jobs := get(root, "jobs")
	if jobs != nil && jobs.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(jobs.Content); i += 2 {
			k := jobs.Content[i]
			v := deref(jobs.Content[i+1])
			if v != nil && v.Kind == yaml.MappingNode {
				wf.Jobs = append(wf.Jobs, Job{ID: k.Value, Line: k.Line, Node: v})
			}
		}
	}
	return wf, nil
}

func deref(n *yaml.Node) *yaml.Node {
	for n != nil && n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	return n
}

func get(n *yaml.Node, key string) (*yaml.Node, *yaml.Node) {
	n = deref(n)
	if n == nil || n.Kind != yaml.MappingNode {
		return nil, nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i], deref(n.Content[i+1])
		}
	}
	return nil, nil
}

func scalarOf(n *yaml.Node) (string, bool) {
	n = deref(n)
	if n != nil && n.Kind == yaml.ScalarNode {
		return n.Value, true
	}
	return "", false
}

func valStr(n *yaml.Node) string {
	n = deref(n)
	if n == nil {
		return ""
	}
	if n.Kind == yaml.ScalarNode {
		return n.Value
	}
	b, err := yaml.Marshal(n)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
