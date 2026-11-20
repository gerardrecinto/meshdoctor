# Pod has no sidecar

- `kubectl get ns <ns> --show-labels`: needs `istio-injection=enabled` or an `istio.io/rev` label (meshdoctor MD010).
- The label only affects new pods. Run `kubectl rollout restart deploy/<name>`.
- `kubectl get pod <pod> -o jsonpath='{.spec.containers[*].name}'` should list `istio-proxy`.
- Check the pod for `sidecar.istio.io/inject: "false"`. Under STRICT mTLS a pod that opts out cannot be called by mesh clients.
- If the pod is stuck in `Init`, look at `istio-init` logs. It needs NET_ADMIN, which a restricted Pod Security level blocks. Use the Istio CNI plugin there.
