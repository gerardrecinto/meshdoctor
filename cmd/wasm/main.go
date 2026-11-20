//go:build js && wasm

// Browser build of the linter: lint(yamlText) returns a JSON string of findings,
// or {"error": "..."} when the YAML does not parse. Nothing leaves the page.
package main

import (
	"encoding/json"
	"syscall/js"

	"github.com/gerardrecinto/meshdoctor/internal/lint"
)

func main() {
	js.Global().Set("meshdoctorLint", js.FuncOf(func(_ js.Value, args []js.Value) any {
		res, err := lint.Parse("input.yaml", []byte(args[0].String()))
		if err != nil {
			b, _ := json.Marshal(map[string]string{"error": err.Error()})
			return string(b)
		}
		fs := lint.Check(res)
		if fs == nil {
			fs = []lint.Finding{}
		}
		b, _ := json.Marshal(map[string]any{"resources": len(res), "findings": fs})
		return string(b)
	}))
	select {}
}
