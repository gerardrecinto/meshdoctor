#!/usr/bin/env bash
# Lint the manifests, build the Java service, run it on kind with Istio, and check what the mesh does.
set -euo pipefail
cd "$(dirname "$0")/.."
ISTIO=1.24.2
CLUSTER=meshdoctor
case "$(uname -s)-$(uname -m)" in
  Darwin-arm64) PLATFORM=osx-arm64 ;; Darwin-x86_64) PLATFORM=osx-amd64 ;; Linux-x86_64) PLATFORM=linux-amd64 ;; Linux-aarch64) PLATFORM=linux-arm64 ;;
  *) echo "unsupported platform"; exit 1 ;;
esac
pass() { echo "PASS  $*"; }
fail() { echo "FAIL  $*"; exit 1; }
cleanup() { [ "${KEEP:-0}" = 1 ] || kind delete cluster --name "$CLUSTER" >/dev/null 2>&1 || true; }
trap cleanup EXIT

mkdir -p .cache bin
go build -o bin/meshdoctor ./cmd/meshdoctor
bin/meshdoctor --strict --min-severity warn deploy && pass "meshdoctor: deploy/ has no errors or warnings"
if bin/meshdoctor testdata/bad.yaml >/dev/null; then fail "meshdoctor should reject testdata/bad.yaml"; fi
pass "meshdoctor: testdata/bad.yaml is rejected"

if [ ! -x ".cache/istio-$ISTIO/bin/istioctl" ]; then
  f="istioctl-$ISTIO-$PLATFORM.tar.gz"
  curl -fsSL -o ".cache/$f" "https://github.com/istio/istio/releases/download/$ISTIO/$f"
  curl -fsSL -o ".cache/$f.sha256" "https://github.com/istio/istio/releases/download/$ISTIO/$f.sha256"
  (cd .cache && echo "$(cut -d' ' -f1 "$f.sha256")  $f" | shasum -a 256 -c -)
  mkdir -p ".cache/istio-$ISTIO/bin" && tar -xzf ".cache/$f" -C ".cache/istio-$ISTIO/bin"
fi
export PATH="$PWD/.cache/istio-$ISTIO/bin:$PATH"

docker build -q -t meshdoctor/checkout:dev java/checkout >/dev/null
kind delete cluster --name "$CLUSTER" >/dev/null 2>&1 || true
kind create cluster --name "$CLUSTER" --wait 120s >/dev/null
kind load docker-image meshdoctor/checkout:dev --name "$CLUSTER" >/dev/null
istioctl install --set profile=minimal -y >/dev/null
kubectl -n istio-system rollout status deploy/istiod --timeout=180s >/dev/null
kubectl apply -f deploy/app.yaml >/dev/null
for d in checkout-v1 checkout-v2 client intruder; do kubectl -n shop rollout status deploy/$d --timeout=240s >/dev/null; done
kubectl -n legacy rollout status deploy/legacy-client --timeout=120s >/dev/null
kubectl apply -f deploy/mesh.yaml >/dev/null
sleep 8
istioctl analyze -n shop && pass "istioctl analyze: no issues"

calls() { # calls DEPLOY NAMESPACE N -> prints one body or http code per request
  kubectl -n "$2" exec "deploy/$1" -c curl -- sh -c \
    "for i in \$(seq 1 $3); do curl -s -m 5 -w ' %{http_code}\n' http://checkout.shop:8080/checkout; done"
}

out=$(calls client shop 200)
v2=$(echo "$out" | grep -c '^v2 200' || true); ok=$(echo "$out" | grep -c ' 200$' || true)
[ "$ok" -eq 200 ] && [ "$v2" -ge 6 ] && [ "$v2" -le 40 ] && pass "canary: $v2 of 200 requests reached v2 (weight 10)" || fail "canary: v2=$v2 ok=$ok"

code=$(kubectl -n legacy exec deploy/legacy-client -c curl -- curl -s -m 5 -o /dev/null -w '%{http_code}' http://checkout.shop:8080/checkout || true)
[ "$code" != 200 ] && pass "mTLS: pod without a sidecar is refused (curl status $code)" || fail "mTLS: legacy pod got 200"

code=$(kubectl -n shop exec deploy/intruder -c curl -- curl -s -m 5 -o /dev/null -w '%{http_code}' http://checkout.shop:8080/checkout)
[ "$code" = 403 ] && pass "authz: another service account gets 403" || fail "authz: intruder got $code"

# Envoy will not eject the only host of a subset when maxEjectionPercent is 50, so v2 gets a second, failing pod.
kubectl -n shop run checkout-bad --image=meshdoctor/checkout:dev --image-pull-policy=IfNotPresent \
  --labels=app=checkout,version=v2 --env=VERSION=v2 --env=FAIL_RATE=1 --port=8080 >/dev/null
kubectl -n shop wait --for=condition=Ready pod/checkout-bad --timeout=180s >/dev/null
sed 's/weight: 90/weight: 50/; s/weight: 10/weight: 50/' deploy/mesh.yaml | kubectl apply -f - >/dev/null
sleep 5
calls client shop 120 >/dev/null
eject=$(kubectl -n shop exec deploy/client -c istio-proxy -- pilot-agent request GET stats | { grep 'outlier_detection.ejections_total' || true; } | { grep 'checkout.shop' || true; } | awk -F': ' '{s+=$2} END {print s+0}')
[ "$eject" -ge 1 ] && pass "outlier detection: v2 ejected $eject time(s) after returning 5xx" || fail "outlier detection: no ejection recorded"
tail=$(calls client shop 30 | tail -n 20 | grep -c ' 200$' || true)
[ "$tail" -eq 20 ] && pass "retries and ejection: the last 20 of 20 requests succeeded" || fail "last 20 requests: $tail ok"
echo "ALL PASS"
