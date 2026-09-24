import importlib.util
import json
from datetime import timedelta

import httpx
import pytest

from conftest import T0, hourly
from test_models import request, settings, weekly
from ekokod_ml.models.chronos import Chronos
from ekokod_ml.models.lstm import Lstm
from ekokod_ml.registry import Registry
from ekokod_ml.schemas import TrainRequest

H = timedelta(hours=1)
HAS_TORCH = importlib.util.find_spec("torch") is not None


@pytest.mark.skipif(HAS_TORCH, reason="checks the image without the lstm extra")
def test_lstm_without_torch_is_listed_unavailable_and_falls_back():
    reg = Registry(settings())
    assert {"id": "lstm", "available": False}.items() <= next(m for m in reg.catalogue() if m["id"] == "lstm").items()
    res = reg.forecast(request(weekly(3), model="lstm"))
    assert res.status == "ok" and res.model_id == "seasonal_naive" and res.fallback_from == "lstm"


@pytest.mark.skipif(not HAS_TORCH, reason="needs the lstm extra (Q-I1)")
def test_lstm_forecasts_with_torch():
    res = Registry(settings()).forecast(request(weekly(3), model="lstm"))
    assert res.status == "ok" and res.model_id == "lstm" and len(res.median) == 48


def chronos_transport(seen):
    def handle(req: httpx.Request) -> httpx.Response:
        body = json.loads(req.content)
        seen.append((req.url.path, req.headers.get("x-api-key"), body))
        n = body["prediction_length"]
        return httpx.Response(200, json={"timestamps": [], "median": [5.0] * n, "high_90": [7.0] * n, "low_10": [3.0] * n})
    return httpx.MockTransport(handle)


def test_chronos_speaks_the_legacy_sidecar_contract():
    seen = []
    chronos = Chronos("http://chronos:8000", "k" * 20, transport=chronos_transport(seen))
    reg = Registry(settings(chronos_url="http://chronos:8000"), extra={"chronos": chronos})
    values = weekly(3)
    values[5] = None
    res = reg.forecast(request(values, horizon=24, model="chronos"))
    assert res.status == "ok" and res.model_id == "chronos"
    assert res.median == [5.0] * 24 and res.p10 == [3.0] * 24 and res.p90 == [7.0] * 24
    path, key, body = seen[0]
    assert path == "/predict" and key == "k" * 20
    assert body["series_id"] == "a" and body["prediction_length"] == 24
    assert len(body["data"]) == len(values) - 1 and set(body["data"][0]) == {"timestamp", "value"}


def test_chronos_unconfigured_is_unavailable():
    assert Chronos("", "").available() is False
    reg = Registry(settings())
    assert next(m for m in reg.catalogue() if m["id"] == "chronos")["available"] is False


def test_catalogue_names_versions_and_the_default():
    cat = {m["id"]: m for m in Registry(settings()).catalogue()}
    assert set(cat) >= {"seasonal_naive", "random_forest", "lstm", "chronos"}
    assert cat["random_forest"]["default"] is True and cat["seasonal_naive"]["available"] is True
    assert all(m["version"] for m in cat.values())


def test_train_saves_a_pooled_forest_that_forecasts_then_use(tmp_path):
    reg = Registry(settings(model_dir=str(tmp_path)))
    series = [{"series_id": s, "history": hourly([v * f for v in weekly(3)])} for s, f in (("a", 1.0), ("b", 1.5))]
    out = reg.train(TrainRequest.model_validate({"model": "random_forest", "granularity": "hour", "series": series}))
    assert out["model_id"] == "random_forest" and out["samples"] > 500 and out["trained_at"]
    assert list(tmp_path.glob("rf-hour-*.joblib"))
    res = reg.forecast(request(weekly(3), model="random_forest"))
    assert res.status == "ok" and res.model_version == out["model_version"]
    fresh = Registry(settings(model_dir=str(tmp_path)))  # a restart finds the artifact
    assert next(m for m in fresh.catalogue() if m["id"] == "random_forest")["trained_at"] == out["trained_at"]


def test_train_refuses_a_dataset_without_usable_rows(tmp_path):
    reg = Registry(settings(model_dir=str(tmp_path)))
    with pytest.raises(ValueError):
        reg.train(TrainRequest.model_validate({"model": "random_forest", "granularity": "hour", "series": [{"series_id": "a", "history": hourly([1.0] * 10)}]}))


def test_a_pooled_forest_only_serves_its_own_covariate_set(tmp_path):
    reg = Registry(settings(model_dir=str(tmp_path)))
    reg.train(TrainRequest.model_validate({"model": "random_forest", "granularity": "hour", "series": [{"series_id": "a", "history": hourly(weekly(3))}]}))
    days = sorted({(T0 + i * H).date() for i in range(4 * 168)})
    cov = {"day_type": [{"date": d.isoformat(), "type": "weekend" if d.weekday() >= 5 else "workday"} for d in days]}
    res = reg.forecast(request(weekly(3), model="random_forest", covariates=cov))
    # Fitted on the request, not the pooled artifact (which would fail and fall back to the baseline).
    assert res.status == "ok" and res.model_id == "random_forest" and res.fallback_from is None and res.model_version == "1.0.0"
