package main

import (
	"bytes"
	"strings"
	"testing"
)

func exec(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(args, strings.NewReader(stdin), &out, &errb)
	return code, out.String(), errb.String()
}

func TestBadFileFailsAndGoodFilePasses(t *testing.T) {
	if code, out, _ := exec(t, "", "../../testdata/bad.yaml"); code != 1 || !strings.Contains(out, "MD001") {
		t.Fatalf("bad.yaml: code %d\n%s", code, out)
	}
	if code, out, _ := exec(t, "", "--strict", "--min-severity", "warn", "../../testdata/good.yaml"); code != 0 {
		t.Fatalf("good.yaml: code %d\n%s", code, out)
	}
}

func TestStdinAndJSON(t *testing.T) {
	code, out, _ := exec(t, "kind: DestinationRule\nmetadata: {name: a}\nspec: {host: a}\n", "--format", "json", "-")
	if code != 0 || !strings.Contains(out, `"rule": "MD003"`) {
		t.Fatalf("code %d\n%s", code, out)
	}
}

func TestStrictTurnsWarningsIntoFailure(t *testing.T) {
	doc := "kind: DestinationRule\nmetadata: {name: a}\nspec: {host: a}\n"
	if code, _, _ := exec(t, doc, "--strict", "-"); code != 1 {
		t.Fatalf("strict code %d", code)
	}
}

func TestUsageErrors(t *testing.T) {
	if code, _, _ := exec(t, ""); code != 2 {
		t.Fatalf("no args code %d", code)
	}
	if code, _, e := exec(t, "", "--format", "xml", "x"); code != 2 || !strings.Contains(e, "unknown format") {
		t.Fatalf("code %d %s", code, e)
	}
	if code, _, _ := exec(t, "", "missing.yaml"); code != 2 {
		t.Fatalf("missing file code %d", code)
	}
	if code, _, _ := exec(t, "kind: [x\n", "-"); code != 2 {
		t.Fatalf("bad yaml code %d", code)
	}
}

func TestGitHubFormat(t *testing.T) {
	_, out, _ := exec(t, "", "--format", "github", "../../testdata/bad.yaml")
	if !strings.Contains(out, "::error file=../../testdata/bad.yaml,line=") {
		t.Fatalf("no annotation:\n%s", out)
	}
}
