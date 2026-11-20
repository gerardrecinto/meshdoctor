package lint

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// WriteText prints one finding per line, then one line with the fix.
func WriteText(w io.Writer, fs []Finding) {
	for _, f := range fs {
		fmt.Fprintf(w, "%s:%d: %s %s %s: %s\n", f.File, f.Line, f.Severity, f.Rule, target(f), f.Message)
		if f.Fix != "" {
			fmt.Fprintf(w, "    fix: %s\n", f.Fix)
		}
	}
}

func target(f Finding) string {
	if f.Namespace != "" {
		return f.Kind + " " + f.Namespace + "/" + f.Name
	}
	return f.Kind + " " + f.Name
}

// WriteJSON prints the findings as a JSON array.
func WriteJSON(w io.Writer, fs []Finding) error {
	if fs == nil {
		fs = []Finding{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(fs)
}

// WriteGitHub prints GitHub Actions workflow commands so findings show up on the PR diff.
func WriteGitHub(w io.Writer, fs []Finding) {
	for _, f := range fs {
		level := map[Severity]string{Error: "error", Warn: "warning", Info: "notice"}[f.Severity]
		msg := strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A").Replace(target(f) + ": " + f.Message)
		fmt.Fprintf(w, "::%s file=%s,line=%d,title=%s::%s\n", level, f.File, f.Line, f.Rule, msg)
	}
}

// Summary returns counts by severity.
func Summary(fs []Finding) (errs, warns, infos int) {
	for _, f := range fs {
		switch f.Severity {
		case Error:
			errs++
		case Warn:
			warns++
		default:
			infos++
		}
	}
	return
}
