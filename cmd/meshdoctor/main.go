// meshdoctor lints Istio and Kubernetes manifests for mesh mistakes.
package main

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/gerardrecinto/meshdoctor/internal/lint"
)

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("meshdoctor", flag.ContinueOnError)
	fl.SetOutput(stderr)
	format := fl.String("format", "text", "text, json or github")
	min := fl.String("min-severity", "info", "hide findings below info, warn or error")
	strict := fl.Bool("strict", false, "exit 1 on warnings too, not only errors")
	fl.Usage = func() {
		fmt.Fprintln(stderr, "usage: meshdoctor [flags] FILE|DIR|- ...\n\nExit codes: 0 clean, 1 findings at the failing level, 2 bad input.")
		fl.PrintDefaults()
	}
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if fl.NArg() == 0 {
		fl.Usage()
		return 2
	}
	if *format != "text" && *format != "json" && *format != "github" {
		fmt.Fprintf(stderr, "meshdoctor: unknown format %q\n", *format)
		return 2
	}
	var all []*lint.Resource
	for _, p := range fl.Args() {
		files, err := collect(p)
		if err != nil {
			fmt.Fprintln(stderr, "meshdoctor:", err)
			return 2
		}
		for _, f := range files {
			var data []byte
			if f == "-" {
				data, err = io.ReadAll(stdin)
			} else {
				data, err = os.ReadFile(f)
			}
			if err != nil {
				fmt.Fprintln(stderr, "meshdoctor:", err)
				return 2
			}
			res, err := lint.Parse(f, data)
			if err != nil {
				fmt.Fprintln(stderr, "meshdoctor:", err)
				return 2
			}
			all = append(all, res...)
		}
	}
	var shown []lint.Finding
	for _, f := range lint.Check(all) {
		if f.Severity.AtLeast(lint.Severity(*min)) {
			shown = append(shown, f)
		}
	}
	switch *format {
	case "json":
		if err := lint.WriteJSON(stdout, shown); err != nil {
			return 2
		}
	case "github":
		lint.WriteGitHub(stdout, shown)
	default:
		lint.WriteText(stdout, shown)
		e, w, i := lint.Summary(shown)
		fmt.Fprintf(stdout, "%d resources checked: %d errors, %d warnings, %d info\n", len(all), e, w, i)
	}
	e, w, _ := lint.Summary(shown)
	if e > 0 || (*strict && w > 0) {
		return 1
	}
	return 0
}

func collect(p string) ([]string, error) {
	if p == "-" {
		return []string{"-"}, nil
	}
	st, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return []string{p}, nil
	}
	var out []string
	err = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && (strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")) {
			out = append(out, path)
		}
		return err
	})
	return out, err
}
