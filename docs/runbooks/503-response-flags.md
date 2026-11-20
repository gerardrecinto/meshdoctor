# 503 from the mesh: read the response flag first

Get the flag from the sidecar access log (`%RESPONSE_FLAGS%`), or `istioctl proxy-config log` at debug for one pod.

| Flag | Meaning | First check |
|---|---|---|
| NR | No route | VirtualService host or subset does not match. Run `meshdoctor` (MD002) and `istioctl analyze`. |
| UH | No healthy upstream | All endpoints ejected or not ready. Check `outlier_detection.ejections_active` and pod readiness. |
| UF | Upstream connection failure | Pod not listening, or mTLS mismatch (MD005). |
| UC | Upstream connection terminated | Server closed the connection, often STRICT mTLS vs plaintext client, or idle timeout. |
| URX | Retry limit exceeded | Upstream keeps failing. Look at the upstream, not the retry settings. |
| UT | Upstream request timeout | Route timeout shorter than the upstream's p99. |

Rule of thumb: NR is config, UH and UF are endpoints, UC is TLS or connection handling, URX and UT are the service itself.

## Outlier detection did not eject the bad pod

Seen in this repo's e2e: the sidecar counted 64 consecutive-5xx detections but `ejections_enforced_total` stayed 0. A subset with one pod is 100% of its hosts, and `maxEjectionPercent: 50` will not eject past 50%. Run at least two pods per subset, or raise `maxEjectionPercent`. The stats are off by default; enable them with the `proxy.istio.io/config` annotation shown in `deploy/app.yaml`.
