package lint

import (
	"os"
	"testing"
)

func run(t *testing.T, doc string) []Finding {
	t.Helper()
	res, err := Parse("t.yaml", []byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	return Check(res)
}

func has(fs []Finding, rule string, sev Severity) bool {
	for _, f := range fs {
		if f.Rule == rule && f.Severity == sev {
			return true
		}
	}
	return false
}

func TestRules(t *testing.T) {
	cases := []struct {
		name, doc, rule string
		sev             Severity
		want            bool
	}{
		{"weights 70+70", `
kind: VirtualService
metadata: {name: web, namespace: shop}
spec:
  hosts: [web]
  http:
  - route:
    - {destination: {host: web, subset: v1}, weight: 70}
    - {destination: {host: web, subset: v2}, weight: 70}
`, "MD001", Error, true},
		{"weights 90+10", `
kind: VirtualService
metadata: {name: web, namespace: shop}
spec:
  hosts: [web]
  http:
  - route:
    - {destination: {host: web, subset: v1}, weight: 90}
    - {destination: {host: web, subset: v2}, weight: 10}
`, "MD001", Error, false},
		{"undefined subset", `
kind: VirtualService
metadata: {name: web, namespace: shop}
spec:
  hosts: [web]
  http: [{route: [{destination: {host: web, subset: v3}}]}]
---
kind: DestinationRule
metadata: {name: web, namespace: shop}
spec: {host: web, subsets: [{name: v1}, {name: v2}]}
`, "MD002", Error, true},
		{"subset found via fqdn", `
kind: VirtualService
metadata: {name: web, namespace: shop}
spec:
  hosts: [web]
  http: [{route: [{destination: {host: web.shop.svc.cluster.local, subset: v1}}]}]
---
kind: DestinationRule
metadata: {name: web, namespace: shop}
spec: {host: web, subsets: [{name: v1}]}
`, "MD002", Error, false},
		{"no outlier", `
kind: DestinationRule
metadata: {name: web, namespace: shop}
spec: {host: web}
`, "MD003", Warn, true},
		{"outlier present", `
kind: DestinationRule
metadata: {name: web, namespace: shop}
spec: {host: web, trafficPolicy: {outlierDetection: {consecutive5xxErrors: 3}}}
`, "MD003", Warn, false},
		{"permissive", `
kind: PeerAuthentication
metadata: {name: default, namespace: shop}
spec: {mtls: {mode: PERMISSIVE}}
`, "MD004", Warn, true},
		{"strict is fine", `
kind: PeerAuthentication
metadata: {name: default, namespace: shop}
spec: {mtls: {mode: STRICT}}
`, "MD004", Warn, false},
		{"tls disable vs strict", `
kind: PeerAuthentication
metadata: {name: default, namespace: shop}
spec: {mtls: {mode: STRICT}}
---
kind: DestinationRule
metadata: {name: web, namespace: shop}
spec: {host: web, trafficPolicy: {tls: {mode: DISABLE}}}
`, "MD005", Error, true},
		{"tls disable in permissive ns", `
kind: PeerAuthentication
metadata: {name: default, namespace: shop}
spec: {mtls: {mode: PERMISSIVE}}
---
kind: DestinationRule
metadata: {name: web, namespace: shop}
spec: {host: web, trafficPolicy: {tls: {mode: DISABLE}}}
`, "MD005", Error, false},
		{"mesh strict applies to any ns", `
kind: PeerAuthentication
metadata: {name: default, namespace: istio-system}
spec: {mtls: {mode: STRICT}}
---
kind: DestinationRule
metadata: {name: web, namespace: shop}
spec: {host: web, subsets: [{name: v1, trafficPolicy: {tls: {mode: DISABLE}}}]}
`, "MD005", Error, true},
		{"retries 5 no per try", `
kind: VirtualService
metadata: {name: web, namespace: shop}
spec:
  hosts: [web]
  http: [{retries: {attempts: 5}, route: [{destination: {host: web}}]}]
`, "MD006", Warn, true},
		{"retries ok", `
kind: VirtualService
metadata: {name: web, namespace: shop}
spec:
  hosts: [web]
  http: [{retries: {attempts: 2, perTryTimeout: 2s}, timeout: 5s, route: [{destination: {host: web}}]}]
`, "MD006", Warn, false},
		{"no timeout", `
kind: VirtualService
metadata: {name: web, namespace: shop}
spec:
  hosts: [web]
  http: [{route: [{destination: {host: web}}]}]
`, "MD007", Info, true},
		{"https without cert", `
kind: Gateway
metadata: {name: gw, namespace: shop}
spec:
  servers:
  - {port: {number: 443, name: https, protocol: HTTPS}, hosts: ["*"], tls: {mode: SIMPLE}}
`, "MD008", Error, true},
		{"http no redirect", `
kind: Gateway
metadata: {name: gw, namespace: shop}
spec:
  servers:
  - {port: {number: 80, name: http, protocol: HTTP}, hosts: ["*"]}
`, "MD008", Warn, true},
		{"gateway ok", `
kind: Gateway
metadata: {name: gw, namespace: shop}
spec:
  servers:
  - {port: {number: 80, name: http, protocol: HTTP}, hosts: ["*"], tls: {httpsRedirect: true}}
  - {port: {number: 443, name: https, protocol: HTTPS}, hosts: ["*"], tls: {mode: SIMPLE, credentialName: cert}}
`, "MD008", Error, false},
		{"allow all", `
kind: AuthorizationPolicy
metadata: {name: open, namespace: shop}
spec: {rules: [{}]}
`, "MD009", Warn, true},
		{"deny with empty rule is not flagged", `
kind: AuthorizationPolicy
metadata: {name: d, namespace: shop}
spec: {action: DENY, rules: [{}]}
`, "MD009", Warn, false},
		{"not injected", `
kind: Namespace
metadata: {name: shop}
---
kind: Deployment
metadata: {name: web, namespace: shop}
spec: {template: {metadata: {labels: {app: web}}}}
`, "MD010", Warn, true},
		{"injected", `
kind: Namespace
metadata: {name: shop, labels: {istio-injection: enabled}}
---
kind: Deployment
metadata: {name: web, namespace: shop}
spec: {template: {metadata: {labels: {app: web}}}}
`, "MD010", Warn, false},
		{"opt out under strict", `
kind: PeerAuthentication
metadata: {name: default, namespace: shop}
spec: {mtls: {mode: STRICT}}
---
kind: Deployment
metadata: {name: web, namespace: shop}
spec: {template: {metadata: {labels: {app: web}, annotations: {sidecar.istio.io/inject: "false"}}}}
`, "MD010", Warn, true},
		{"duplicate vs", `
kind: VirtualService
metadata: {name: a, namespace: shop}
spec: {hosts: [web], http: [{route: [{destination: {host: web}}]}]}
---
kind: VirtualService
metadata: {name: b, namespace: shop}
spec: {hosts: [web], http: [{route: [{destination: {host: web}}]}]}
`, "MD011", Warn, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := has(run(t, c.doc), c.rule, c.sev); got != c.want {
				t.Fatalf("%s %s present=%v, want %v: %+v", c.rule, c.sev, got, c.want, run(t, c.doc))
			}
		})
	}
}

func TestLineNumbersPointAtTheDocument(t *testing.T) {
	fs := run(t, "kind: Namespace\nmetadata: {name: a}\n---\nkind: DestinationRule\nmetadata: {name: web}\nspec: {host: web}\n")
	if len(fs) == 0 || fs[0].Line != 4 {
		t.Fatalf("want first finding at line 4, got %+v", fs)
	}
}

func TestParseErrorIsReported(t *testing.T) {
	if _, err := Parse("bad.yaml", []byte("kind: [unclosed\n")); err == nil {
		t.Fatal("want a YAML error")
	}
}

func TestSampleFiles(t *testing.T) {
	good, _ := os.ReadFile("../../testdata/good.yaml")
	res, err := Parse("good.yaml", good)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range Check(res) {
		if f.Severity != Info {
			t.Errorf("good.yaml should have no error or warn findings: %+v", f)
		}
	}
	bad, _ := os.ReadFile("../../testdata/bad.yaml")
	res, _ = Parse("bad.yaml", bad)
	seen := map[string]bool{}
	for _, f := range Check(res) {
		seen[f.Rule] = true
	}
	for _, id := range []string{"MD001", "MD002", "MD003", "MD004", "MD005", "MD006", "MD007", "MD008", "MD009", "MD010", "MD011"} {
		if !seen[id] {
			t.Errorf("bad.yaml should trigger %s", id)
		}
	}
}
