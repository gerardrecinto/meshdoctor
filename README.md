# meshdoctor

Lint Istio and Kubernetes manifests for the mistakes that become 503s, in CI or in your browser. Go, with a Java workload, Python SLO alerts, a Jenkins pipeline and Terraform for EKS around it.

Site and in-browser demo: https://gerardrecinto.github.io/meshdoctor/

## Try it

Needs Go 1.23 or later.

```bash
git clone https://github.com/gerardrecinto/meshdoctor.git && cd meshdoctor
go run ./cmd/meshdoctor testdata/bad.yaml     # 4 errors, 9 warnings, exit 1
go run ./cmd/meshdoctor testdata/good.yaml    # clean, exit 0
```

`meshdoctor [--format text|json|github] [--min-severity info|warn|error] [--strict] FILE|DIR|-`. Exit 0 is clean, 1 means findings at the failing level (errors, or warnings with `--strict`), 2 is bad input. `--format github` prints annotations that show on a PR diff.

## Rules

| Rule | Catches |
|---|---|
| MD001 | route weights that do not add up to 100 |
| MD002 | subset used in a VirtualService but not defined in a DestinationRule |
| MD003 | DestinationRule with no outlier detection |
| MD004 | PERMISSIVE or DISABLE peer authentication |
| MD005 | client TLS DISABLE against a STRICT namespace (503 UC) |
| MD006 | more than 3 retries, or no perTryTimeout |
| MD007 | HTTP route with no timeout |
| MD008 | HTTPS gateway with no certificate, HTTP with no redirect |
| MD009 | ALLOW policy with an empty rule |
| MD010 | namespace not injected, or a pod that opts out under STRICT |
| MD011 | two VirtualServices for the same host |

## The rest

```
cmd/, internal/lint   Go linter and its WebAssembly build
java/checkout         Java 21 service: version from env, FAIL_RATE for outlier tests, drains on SIGTERM
python/burnrate.py    SLO to four multi-window burn-rate Prometheus alerts on istio_requests_total
slo/web.yaml          example SLO
deploy/               the manifests the e2e applies (they pass meshdoctor --strict)
Jenkinsfile           parallel tests, mesh gate, terraform validate, e2e on main
jenkins/vars          shared library step meshGate
terraform/eks         VPC, EKS, Istio and an NLB ingress through Helm
docs/                 GitHub Pages site and runbooks (503 flags, canary rollback, no sidecar)
```

```bash
go test ./...                                              # linter
(cd java/checkout && mvn -q -B test)                       # Java
pip install -r python/requirements.txt && (cd python && python -m pytest -q)
python python/burnrate.py slo/web.yaml --budget
bash scripts/e2e.sh                                        # needs Docker, kind, kubectl; about 6 minutes
```

## Limits

- The linter checks the files you give it. A rule that needs another resource warns when that resource is not there.
- Terraform is checked with `terraform validate` and `fmt`, and has not been applied to an AWS account.
- The Jenkinsfile has not been run on a build agent.
- The e2e uses one namespace and one replica per version. It shows behavior, not capacity.

MIT license.
