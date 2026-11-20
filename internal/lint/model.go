// Package lint checks Istio and Kubernetes manifests for mesh mistakes that
// istioctl analyze does not flag, and for ones that only show up as 503s.
package lint

import (
	"bytes"
	"errors"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

type Severity string

const (
	Error Severity = "error"
	Warn  Severity = "warn"
	Info  Severity = "info"
)

func (s Severity) rank() int {
	switch s {
	case Error:
		return 3
	case Warn:
		return 2
	}
	return 1
}

// AtLeast reports whether s is as severe as min.
func (s Severity) AtLeast(min Severity) bool { return s.rank() >= min.rank() }

type Finding struct {
	Rule      string   `json:"rule"`
	Severity  Severity `json:"severity"`
	Kind      string   `json:"kind"`
	Name      string   `json:"name"`
	Namespace string   `json:"namespace,omitempty"`
	File      string   `json:"file"`
	Line      int      `json:"line"`
	Message   string   `json:"message"`
	Fix       string   `json:"fix,omitempty"`
}

// Resource is one YAML document with a Kubernetes shape.
type Resource struct {
	File      string
	Line      int
	APIVer    string
	Kind      string
	Name      string
	Namespace string
	Labels    map[string]any
	Obj       map[string]any
}

func (r *Resource) spec() map[string]any { return asMap(r.Obj["spec"]) }

func (r *Resource) finding(rule string, sev Severity, msg, fix string) Finding {
	return Finding{Rule: rule, Severity: sev, Kind: r.Kind, Name: r.Name, Namespace: r.Namespace,
		File: r.File, Line: r.Line, Message: msg, Fix: fix}
}

// Parse reads every document in data. A document that is not a mapping with a
// kind is skipped; a YAML syntax error is returned.
func Parse(file string, data []byte) ([]*Resource, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var out []*Resource
	for {
		var node yaml.Node
		err := dec.Decode(&node)
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return out, errors.New(file + ": " + err.Error())
		}
		if node.Kind != yaml.DocumentNode || len(node.Content) == 0 {
			continue
		}
		var obj map[string]any
		if err := node.Content[0].Decode(&obj); err != nil || obj == nil {
			continue
		}
		kind, _ := obj["kind"].(string)
		if kind == "" {
			continue
		}
		meta := asMap(obj["metadata"])
		name, _ := meta["name"].(string)
		ns, _ := meta["namespace"].(string)
		out = append(out, &Resource{File: file, Line: node.Content[0].Line, APIVer: str(obj["apiVersion"]),
			Kind: kind, Name: name, Namespace: ns, Labels: asMap(meta["labels"]), Obj: obj})
	}
}

func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func num(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}

// fqdn turns a short service host into name.namespace.svc.cluster.local.
func fqdn(host, ns string) string {
	if ns == "" {
		ns = "default"
	}
	switch strings.Count(host, ".") {
	case 0:
		return host + "." + ns + ".svc.cluster.local"
	case 1:
		return host + ".svc.cluster.local"
	}
	return host
}
