import pytest
import yaml

import burnrate

SPEC = {"service": "web", "namespace": "shop", "objective": 99.9, "window_days": 30}


def test_budget_for_three_nines_is_43_minutes():
    ratio, minutes = burnrate.budget(SPEC)
    assert ratio == pytest.approx(0.001)
    assert minutes == pytest.approx(43.2)


def test_fast_burn_exhausts_in_50_hours():
    assert burnrate.hours_to_exhaust(SPEC, 14.4) == pytest.approx(50)
    assert burnrate.hours_to_exhaust(SPEC, 1) == pytest.approx(720)


def test_four_alerts_with_workbook_thresholds():
    rules = burnrate.render(SPEC)["spec"]["groups"][0]["rules"]
    assert [r["labels"]["severity"] for r in rules] == ["page", "page", "ticket", "ticket"]
    assert "> 0.0144 and" in rules[0]["expr"]
    assert "> 0.006 and" in rules[1]["expr"]
    assert "> 0.003 and" in rules[2]["expr"]
    assert "> 0.001 and" in rules[3]["expr"]


def test_every_alert_needs_long_and_short_window():
    for r in burnrate.render(SPEC)["spec"]["groups"][0]["rules"]:
        assert r["expr"].count("rate(istio_requests_total") == 4  # two windows, numerator and denominator each
        assert r["expr"].count(" and ") == 1


def test_query_reads_istio_metric_for_the_destination():
    expr = burnrate.render(SPEC)["spec"]["groups"][0]["rules"][0]["expr"]
    assert 'destination_service_name="web"' in expr
    assert 'destination_service_namespace="shop"' in expr
    assert 'reporter="destination"' in expr


def test_output_is_valid_yaml_prometheusrule():
    doc = yaml.safe_load(yaml.safe_dump(burnrate.render(SPEC)))
    assert doc["kind"] == "PrometheusRule"
    assert doc["metadata"] == {"name": "web-slo", "namespace": "shop"}


@pytest.mark.parametrize("bad,msg", [
    ({"service": "w", "namespace": "n"}, "missing objective"),
    ({"service": "w", "namespace": "n", "objective": 100}, "objective"),
    ({"service": "w", "namespace": "n", "objective": 99.9, "window_days": 0}, "window_days"),
    ({"service": "w", "namespace": "n", "objective": True}, "objective"),
])
def test_validate_rejects(bad, msg):
    assert any(msg in e for e in burnrate.validate(bad))


def test_cli_prints_rules_and_budget(tmp_path, capsys):
    f = tmp_path / "s.yaml"
    f.write_text(yaml.safe_dump(SPEC))
    assert burnrate.main([str(f)]) == 0
    assert "PrometheusRule" in capsys.readouterr().out
    assert burnrate.main([str(f), "--budget"]) == 0
    out = capsys.readouterr().out
    assert "about 43 minutes" in out and "50 hours" in out


def test_cli_missing_file_is_exit_2(capsys):
    assert burnrate.main(["nope.yaml"]) == 2
