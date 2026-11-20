"""Turn an availability SLO into Prometheus multi-window burn-rate alerts.

Reads Istio's istio_requests_total, so no application code has to export anything.
The four windows are the ones from the Google SRE Workbook: a fast burn pages,
a slow burn opens a ticket, and each needs both a long and a short window to be
over its threshold, which stops an alert that fires long after the problem ended.
"""
import argparse
import sys

import yaml

# (severity, burn rate, long window, short window)
WINDOWS = [
    ("page", 14.4, "1h", "5m"),
    ("page", 6.0, "6h", "30m"),
    ("ticket", 3.0, "1d", "2h"),
    ("ticket", 1.0, "3d", "6h"),
]


def validate(spec):
    errors = []
    for key in ("service", "namespace", "objective"):
        if key not in spec:
            errors.append(f"missing {key}")
    if errors:
        return errors
    obj = spec["objective"]
    if not isinstance(obj, (int, float)) or isinstance(obj, bool) or not 50 <= obj < 100:
        errors.append("objective is a percent from 50 up to, not including, 100")
    days = spec.get("window_days", 30)
    if not isinstance(days, int) or isinstance(days, bool) or days < 1:
        errors.append("window_days must be a whole number of days")
    return errors


def budget(spec):
    """Error budget as a ratio, and as minutes over the SLO window."""
    ratio = round(1 - spec["objective"] / 100, 10)
    return ratio, ratio * spec.get("window_days", 30) * 24 * 60


def hours_to_exhaust(spec, burn):
    """Hours until the whole budget is gone if errors keep running at `burn` times the allowed rate."""
    return spec.get("window_days", 30) * 24 / burn


def error_ratio(spec, window):
    sel = (f'destination_service_name="{spec["service"]}",destination_service_namespace="{spec["namespace"]}",'
           'reporter="destination"')
    return (f'sum(rate(istio_requests_total{{{sel},response_code=~"5.."}}[{window}]))'
            f' / sum(rate(istio_requests_total{{{sel}}}[{window}]))')


def render(spec):
    ratio, _ = budget(spec)
    svc = spec["service"]
    rules = []
    for sev, burn, long_w, short_w in WINDOWS:
        threshold = round(burn * ratio, 10)
        hours = hours_to_exhaust(spec, burn)
        rules.append({
            "alert": f"{svc.title().replace('-', '')}ErrorBudgetBurn",
            "expr": f"({error_ratio(spec, long_w)}) > {threshold} and ({error_ratio(spec, short_w)}) > {threshold}",
            "for": "2m",
            "labels": {"severity": sev, "service": svc, "window": long_w},
            "annotations": {
                "summary": f"{svc} is burning its {spec['objective']}% error budget {burn}x too fast",
                "description": (f"5xx ratio is over {threshold} for {long_w} and {short_w}. "
                                f"At this rate the {spec.get('window_days', 30)} day budget is gone in about {hours:.0f} hours."),
            },
        })
    return {
        "apiVersion": "monitoring.coreos.com/v1",
        "kind": "PrometheusRule",
        "metadata": {"name": f"{svc}-slo", "namespace": spec["namespace"]},
        "spec": {"groups": [{"name": f"{svc}-slo-burn", "rules": rules}]},
    }


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("file", help="SLO spec, see slo/web.yaml")
    ap.add_argument("--budget", action="store_true", help="print the error budget instead of rules")
    args = ap.parse_args(argv)
    try:
        with open(args.file) as f:
            spec = yaml.safe_load(f)
    except OSError as e:
        print(f"burnrate: {e}", file=sys.stderr)
        return 2
    errors = validate(spec) if isinstance(spec, dict) else ["spec must be a mapping"]
    if errors:
        print("\n".join(f"burnrate: {e}" for e in errors), file=sys.stderr)
        return 2
    if args.budget:
        ratio, minutes = budget(spec)
        print(f"{spec['objective']}% over {spec.get('window_days', 30)} days allows {ratio:.4%} errors, about {minutes:.0f} minutes of full outage")
        for sev, burn, long_w, _ in WINDOWS:
            print(f"  {sev:6} {burn:>4}x over {long_w:>2}: budget gone in {hours_to_exhaust(spec, burn):.0f} hours")
        return 0
    print(yaml.safe_dump(render(spec), sort_keys=False, width=1000), end="")
    return 0


if __name__ == "__main__":
    sys.exit(main())
