# Canary looks bad: roll back

1. Set the weights back: edit the VirtualService so the stable subset is 100 and the canary is 0 (or remove the canary route), then `kubectl apply`.
2. Confirm: `istioctl x describe pod <client-pod>` shows 100 to the stable subset.
3. Leave the canary pods running for debugging. Weight 0 sends them no traffic.
4. Run `meshdoctor` on the change before applying it. MD001 catches weights that do not add up to 100.
5. Write down the error ratio you saw. If it was over the burn-rate threshold in `slo/web.yaml`, the alert should have fired first. If it did not, fix the alert.
