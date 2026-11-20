package lint

import (
	"fmt"
	"sort"
	"strings"
)

type rule func(all []*Resource) []Finding

var rules = []rule{weightsSum, subsetDefined, outlierMissing, permissiveMTLS, mtlsMismatch,
	retryBudget, noTimeout, gatewayTLS, allowAllRule, sidecarMissing, duplicateVS}

// Check runs every rule over the resources and returns findings sorted by file and line.
func Check(all []*Resource) []Finding {
	var out []Finding
	for _, r := range rules {
		out = append(out, r(all)...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Rule < out[j].Rule
	})
	return out
}

func of(all []*Resource, kind string) []*Resource {
	var out []*Resource
	for _, r := range all {
		if r.Kind == kind {
			out = append(out, r)
		}
	}
	return out
}

// routes returns every http, tcp and tls route entry of a VirtualService.
func routes(vs *Resource, protos ...string) []map[string]any {
	var out []map[string]any
	for _, p := range protos {
		for _, e := range asList(vs.spec()[p]) {
			out = append(out, asMap(e))
		}
	}
	return out
}

// MD001: destination weights in one route must add up to 100.
func weightsSum(all []*Resource) []Finding {
	var out []Finding
	for _, vs := range of(all, "VirtualService") {
		for _, rt := range routes(vs, "http", "tcp", "tls") {
			dests := asList(rt["route"])
			if len(dests) < 2 {
				continue
			}
			sum := 0.0
			for _, d := range dests {
				w, _ := num(asMap(d)["weight"])
				sum += w
			}
			if sum != 100 {
				out = append(out, vs.finding("MD001", Error,
					fmt.Sprintf("route weights add up to %g, not 100", sum),
					"Make the weights of the destinations in this route add up to exactly 100."))
			}
		}
	}
	return out
}

// MD002: a subset named in a VirtualService must exist in a DestinationRule for that host.
func subsetDefined(all []*Resource) []Finding {
	have := map[string]map[string]bool{}
	for _, dr := range of(all, "DestinationRule") {
		h := fqdn(str(dr.spec()["host"]), dr.Namespace)
		if have[h] == nil {
			have[h] = map[string]bool{}
		}
		for _, s := range asList(dr.spec()["subsets"]) {
			have[h][str(asMap(s)["name"])] = true
		}
	}
	var out []Finding
	for _, vs := range of(all, "VirtualService") {
		for _, rt := range routes(vs, "http", "tcp", "tls") {
			for _, d := range asList(rt["route"]) {
				dest := asMap(asMap(d)["destination"])
				sub := str(dest["subset"])
				if sub == "" {
					continue
				}
				h := fqdn(str(dest["host"]), vs.Namespace)
				switch {
				case have[h] == nil:
					out = append(out, vs.finding("MD002", Warn,
						fmt.Sprintf("subset %q of %s has no DestinationRule in these files", sub, str(dest["host"])),
						"Add a DestinationRule for the host that defines the subset. Traffic to an undefined subset fails with 503 NR."))
				case !have[h][sub]:
					out = append(out, vs.finding("MD002", Error,
						fmt.Sprintf("subset %q is not defined for %s", sub, str(dest["host"])),
						"Add the subset to the DestinationRule, or fix the name. Traffic to it fails with 503."))
				}
			}
		}
	}
	return out
}

// MD003: a DestinationRule for a service should eject failing replicas.
func outlierMissing(all []*Resource) []Finding {
	var out []Finding
	for _, dr := range of(all, "DestinationRule") {
		host := str(dr.spec()["host"])
		if host == "" || strings.HasPrefix(host, "*") {
			continue
		}
		if _, ok := asMap(dr.spec()["trafficPolicy"])["outlierDetection"]; !ok {
			out = append(out, dr.finding("MD003", Warn, "no outlierDetection: a replica that returns 5xx keeps getting traffic",
				"Set trafficPolicy.outlierDetection with consecutive5xxErrors, interval, baseEjectionTime and maxEjectionPercent."))
		}
	}
	return out
}

func peerMode(pa *Resource) string {
	return str(asMap(pa.spec()["mtls"])["mode"])
}

// MD004: PERMISSIVE or DISABLE peer authentication lets plaintext in.
func permissiveMTLS(all []*Resource) []Finding {
	var out []Finding
	for _, pa := range of(all, "PeerAuthentication") {
		m := peerMode(pa)
		if m != "PERMISSIVE" && m != "DISABLE" {
			continue
		}
		scope := "workloads it selects"
		if len(asMap(pa.spec()["selector"])) == 0 {
			scope = "every workload in namespace " + orDefault(pa.Namespace)
		}
		out = append(out, pa.finding("MD004", Warn, fmt.Sprintf("mTLS mode %s accepts plaintext for %s", m, scope),
			"Use STRICT once every client has a sidecar. PERMISSIVE is for a migration, not a steady state."))
	}
	return out
}

func orDefault(ns string) string {
	if ns == "" {
		return "default"
	}
	return ns
}

// strictNamespaces returns namespaces under a namespace-wide STRICT policy, and whether the mesh is STRICT.
func strictNamespaces(all []*Resource) (map[string]bool, bool) {
	ns, mesh := map[string]bool{}, false
	for _, pa := range of(all, "PeerAuthentication") {
		if peerMode(pa) != "STRICT" || len(asMap(pa.spec()["selector"])) != 0 {
			continue
		}
		if pa.Namespace == "istio-system" {
			mesh = true
		} else {
			ns[orDefault(pa.Namespace)] = true
		}
	}
	return ns, mesh
}

func tlsModes(dr *Resource) []string {
	modes := []string{str(asMap(asMap(dr.spec()["trafficPolicy"])["tls"])["mode"])}
	for _, s := range asList(dr.spec()["subsets"]) {
		modes = append(modes, str(asMap(asMap(asMap(s)["trafficPolicy"])["tls"])["mode"]))
	}
	return modes
}

// MD005: a DestinationRule that turns client TLS off for a host that requires mTLS.
func mtlsMismatch(all []*Resource) []Finding {
	strict, mesh := strictNamespaces(all)
	var out []Finding
	for _, dr := range of(all, "DestinationRule") {
		host := fqdn(str(dr.spec()["host"]), dr.Namespace)
		parts := strings.Split(host, ".")
		if len(parts) < 2 {
			continue
		}
		if !strict[parts[1]] && !mesh {
			continue
		}
		for _, m := range tlsModes(dr) {
			if m == "DISABLE" {
				out = append(out, dr.finding("MD005", Error,
					"client TLS is DISABLE but the destination requires mTLS (STRICT PeerAuthentication)",
					"Use tls.mode ISTIO_MUTUAL, or remove the tls block. This mismatch shows up as 503 UC."))
				break
			}
		}
	}
	return out
}

// MD006: retries that can multiply load or never fit the timeout.
func retryBudget(all []*Resource) []Finding {
	var out []Finding
	for _, vs := range of(all, "VirtualService") {
		for _, rt := range routes(vs, "http") {
			r, ok := rt["retries"].(map[string]any)
			if !ok {
				continue
			}
			if a, _ := num(r["attempts"]); a > 3 {
				out = append(out, vs.finding("MD006", Warn, fmt.Sprintf("%g retry attempts can multiply load on a struggling service", a),
					"Keep attempts at 3 or fewer, and set a perTryTimeout."))
			}
			if str(r["perTryTimeout"]) == "" {
				out = append(out, vs.finding("MD006", Warn, "retries have no perTryTimeout, so one hung try can use the whole route timeout",
					"Set retries.perTryTimeout below the route timeout."))
			}
		}
	}
	return out
}

// MD007: an HTTP route without a timeout waits as long as the client does.
func noTimeout(all []*Resource) []Finding {
	var out []Finding
	for _, vs := range of(all, "VirtualService") {
		for _, rt := range routes(vs, "http") {
			if _, ok := rt["timeout"]; !ok {
				out = append(out, vs.finding("MD007", Info, "HTTP route has no timeout",
					"Set timeout on the route so a slow upstream cannot hold connections open."))
				break
			}
		}
	}
	return out
}

// MD008: gateway servers that serve TLS without a certificate, or HTTP without a redirect.
func gatewayTLS(all []*Resource) []Finding {
	var out []Finding
	for _, gw := range of(all, "Gateway") {
		for _, s := range asList(gw.spec()["servers"]) {
			srv := asMap(s)
			proto := strings.ToUpper(str(asMap(srv["port"])["protocol"]))
			tls := asMap(srv["tls"])
			mode := str(tls["mode"])
			switch proto {
			case "HTTPS":
				if (mode == "" || mode == "SIMPLE" || mode == "MUTUAL") &&
					str(tls["credentialName"]) == "" && str(tls["serverCertificate"]) == "" {
					out = append(out, gw.finding("MD008", Error, "HTTPS server has no certificate (tls.credentialName)",
						"Set tls.mode SIMPLE and tls.credentialName to a secret in the gateway namespace."))
				}
			case "HTTP":
				if redirect, _ := tls["httpsRedirect"].(bool); !redirect {
					out = append(out, gw.finding("MD008", Warn, "HTTP server does not redirect to HTTPS",
						"Set tls.httpsRedirect: true on the port 80 server."))
				}
			}
		}
	}
	return out
}

// MD009: an ALLOW rule with nothing in it matches every request.
func allowAllRule(all []*Resource) []Finding {
	var out []Finding
	for _, ap := range of(all, "AuthorizationPolicy") {
		action := str(ap.spec()["action"])
		if action != "" && action != "ALLOW" {
			continue
		}
		for _, r := range asList(ap.spec()["rules"]) {
			if len(asMap(r)) == 0 {
				out = append(out, ap.finding("MD009", Warn, "ALLOW policy has an empty rule, which matches every request",
					"Add from, to or when to the rule, or delete the policy."))
				break
			}
		}
	}
	return out
}

func podLabels(r *Resource) (map[string]any, map[string]any) {
	meta := asMap(asMap(asMap(r.spec()["template"])["metadata"]))
	return asMap(meta["labels"]), asMap(meta["annotations"])
}

// MD010: workloads in a namespace that is not injected, or that opt out under STRICT.
func sidecarMissing(all []*Resource) []Finding {
	injected, known := map[string]bool{}, map[string]bool{}
	for _, ns := range of(all, "Namespace") {
		known[ns.Name] = true
		_, rev := ns.Labels["istio.io/rev"]
		if str(ns.Labels["istio-injection"]) == "enabled" || rev {
			injected[ns.Name] = true
		}
	}
	strict, mesh := strictNamespaces(all)
	var out []Finding
	for _, r := range all {
		if r.Kind != "Deployment" && r.Kind != "StatefulSet" && r.Kind != "DaemonSet" {
			continue
		}
		ns := orDefault(r.Namespace)
		labels, ann := podLabels(r)
		optOut := str(labels["sidecar.istio.io/inject"]) == "false" || str(ann["sidecar.istio.io/inject"]) == "false"
		switch {
		case optOut && (strict[ns] || mesh):
			out = append(out, r.finding("MD010", Warn, "pods opt out of the sidecar but the namespace requires mTLS",
				"Mesh clients will send mTLS to a pod that cannot terminate it. Remove the opt-out or add a PERMISSIVE PeerAuthentication for this workload."))
		case known[ns] && !injected[ns] && !optOut:
			out = append(out, r.finding("MD010", Warn, "namespace "+ns+" is not labeled istio-injection=enabled, so this workload gets no sidecar",
				"Label the namespace istio-injection=enabled, or an istio.io/rev label, and restart the pods."))
		}
	}
	return out
}

// MD011: two VirtualServices for the same host in one namespace.
func duplicateVS(all []*Resource) []Finding {
	seen := map[string]*Resource{}
	var out []Finding
	for _, vs := range of(all, "VirtualService") {
		for _, h := range asList(vs.spec()["hosts"]) {
			key := orDefault(vs.Namespace) + "/" + fmt.Sprint(h) + "/" + fmt.Sprint(vs.spec()["gateways"])
			if first, ok := seen[key]; ok && first != vs {
				out = append(out, vs.finding("MD011", Warn,
					fmt.Sprintf("host %v is already routed by VirtualService %s (line %d)", h, first.Name, first.Line),
					"Merge the routes into one VirtualService. Istio does not define which one wins."))
				continue
			}
			seen[key] = vs
		}
	}
	return out
}
