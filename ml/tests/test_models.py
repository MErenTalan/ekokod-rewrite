import math
import random
from datetime import timedelta

import pytest

from conftest import T0, hourly
from ekokod_ml.models.base import Prediction
from ekokod_ml.registry import Registry
from ekokod_ml.schemas import ForecastRequest
from ekokod_ml.settings import Settings

H = timedelta(hours=1)


def weekly(weeks, weekend_factor=0.4):
    """A clean weekly shape: day/night swing, low weekends (Jan 10–11 2026 is a weekend)."""
    out = []
    for i in range(weeks * 168):
        t = T0 + i * H
        base = 10 + 8 * math.sin((t.hour - 6) / 24 * 2 * math.pi)
        out.append(round(base * (weekend_factor if t.weekday() >= 5 else 1.0), 4))
    return out


def request(values, horizon=48, **kw):
    return ForecastRequest.model_validate({"series_id": "a", "granularity": "hour", "horizon": horizon, "history": hourly(values), **kw})


def settings(**kw):
    return Settings(api_key="x" * 16, **kw)


def test_baseline_repeats_a_clean_weekly_pattern_and_continues_the_timeline():
    values = weekly(4)
    res = Registry(settings()).forecast(request(values, model="seasonal_naive"))
    assert res.status == "ok" and res.model_id == "seasonal_naive" and res.model_version
    assert res.timestamps[0] == T0 + len(values) * H and len(res.median) == 48
    truth = weekly(5)[len(values):len(values) + 48]
    assert max(abs(a - b) for a, b in zip(res.median, truth)) < 1e-6


def test_baseline_works_with_every_heavier_model_disabled():
    reg = Registry(settings(disabled_models=frozenset({"random_forest", "lstm", "chronos"})))
    res = reg.forecast(request(weekly(3)))  # default model is random_forest
    assert res.status == "ok" and res.model_id == "seasonal_naive" and res.fallback_from == "random_forest"


def test_random_forest_learns_the_weekend_from_day_types():
    values = weekly(4)
    days = sorted({(T0 + i * H).date() for i in range(5 * 168)})
    day_type = [{"date": d.isoformat(), "type": "weekend" if d.weekday() >= 5 else "workday"} for d in days]
    res = Registry(settings()).forecast(request(values, horizon=168, model="random_forest", covariates={"day_type": day_type}))
    assert res.status == "ok" and res.model_id == "random_forest"
    assert "day_type" in res.used_covariates
    truth = weekly(5)[len(values):]
    mae = sum(abs(a - b) for a, b in zip(res.median, truth)) / len(truth)
    assert mae < 1.0, mae


@pytest.mark.parametrize("model", ["seasonal_naive", "random_forest"])
def test_quantiles_are_ordered_non_negative_and_sized(model):
    rng = random.Random(7)
    values = [max(0.0, v + rng.gauss(0, 2)) for v in weekly(3)]
    res = Registry(settings()).forecast(request(values, horizon=72, model=model))
    assert len(res.timestamps) == len(res.median) == len(res.p10) == len(res.p90) == 72
    for lo, mid, hi in zip(res.p10, res.median, res.p90):
        assert 0 <= lo <= mid <= hi


def test_too_little_history_is_insufficient_not_a_poor_forecast():
    res = Registry(settings()).forecast(request(weekly(3)[:335]))
    assert res.status == "insufficient_data" and res.median == [] and res.model_id


def test_empty_or_all_null_history_is_no_data():
    reg = Registry(settings())
    assert reg.forecast(request([])).status == "no_data"
    assert reg.forecast(request([None] * 400)).status == "no_data"


def test_gaps_ride_along_with_every_status():
    values = weekly(3)
    values[100:110] = [None] * 10
    res = Registry(settings()).forecast(request(values, model="seasonal_naive"))
    assert res.status == "ok"
    assert [(g.start, g.missing_hours) for g in res.gaps] == [(T0 + 100 * H, 10)]


class Boom:
    id = "random_forest"
    version = "boom"

    def available(self):
        return True

    def forecast(self, req, points):
        raise RuntimeError("model exploded")


def test_a_failing_model_falls_back_to_the_baseline_and_says_so():
    reg = Registry(settings())
    reg.models["random_forest"] = Boom()
    res = reg.forecast(request(weekly(3)))
    assert res.status == "ok" and res.model_id == "seasonal_naive" and res.fallback_from == "random_forest"


def test_a_failing_baseline_is_a_model_error():
    reg = Registry(settings())
    boom = Boom()
    boom.id = "seasonal_naive"
    reg.models["seasonal_naive"] = boom
    res = reg.forecast(request(weekly(3), model="seasonal_naive"))
    assert res.status == "model_error" and res.median == [] and res.model_id == "seasonal_naive"


def test_unknown_model_is_refused():
    with pytest.raises(KeyError):
        Registry(settings()).forecast(request(weekly(3), model="nope"))


def test_daily_granularity_forecasts_days():
    days = [{"ts": (T0 + timedelta(days=d)).isoformat(), "value": 100.0 + (0 if (T0 + timedelta(days=d)).weekday() < 5 else -60)} for d in range(56)]
    req = ForecastRequest.model_validate({"series_id": "a", "granularity": "day", "horizon": 14, "history": days})
    res = Registry(settings()).forecast(req)
    assert res.status == "ok" and len(res.median) == 14
    assert res.timestamps[1] - res.timestamps[0] == timedelta(days=1)
    assert isinstance(Prediction, type)
