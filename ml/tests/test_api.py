from datetime import timedelta

import pytest
from fastapi.testclient import TestClient

from conftest import T0, hourly
from test_models import settings, weekly
from ekokod_ml.api import create_app

KEY = "s3cret-key-0123456789"
AUTH = {"Authorization": f"Bearer {KEY}"}
H = timedelta(hours=1)


@pytest.fixture
def client(tmp_path):
    return TestClient(create_app(settings(api_key=KEY, model_dir=str(tmp_path))))


def forecast_body(values, **kw):
    return {"series_id": "a", "granularity": "hour", "horizon": 24, "history": hourly(values), **kw}


def test_health_is_open_and_everything_else_needs_the_secret(client):
    assert client.get("/health").json() == {"status": "ok"}
    assert client.get("/v1/models").status_code == 401
    assert client.get("/v1/models", headers={"Authorization": "Bearer wrong"}).status_code == 401
    assert client.post("/v1/forecast", json=forecast_body(weekly(3))).status_code == 401
    assert client.get("/v1/models", headers=AUTH).status_code == 200


def test_forecast_contract(client):
    res = client.post("/v1/forecast", json=forecast_body(weekly(3), model="seasonal_naive"), headers=AUTH)
    assert res.status_code == 200
    body = res.json()
    assert set(body) >= {"status", "model_id", "model_version", "timestamps", "median", "p10", "p90", "used_covariates", "gaps"}
    assert body["status"] == "ok" and len(body["median"]) == 24


def test_statuses_are_explicit_over_http(client):
    assert client.post("/v1/forecast", json=forecast_body([]), headers=AUTH).json()["status"] == "no_data"
    short = client.post("/v1/forecast", json=forecast_body([1.0] * 50), headers=AUTH).json()
    assert short["status"] == "insufficient_data" and short["model_id"] and short["model_version"]


def test_bad_requests_are_422(client):
    assert client.post("/v1/forecast", json=forecast_body(weekly(3), model="nope"), headers=AUTH).status_code == 422
    assert client.post("/v1/forecast", json=forecast_body(weekly(3), horizon=10_000), headers=AUTH).status_code == 422
    assert client.post("/v1/train", json={"model": "random_forest", "granularity": "hour", "series": [{"series_id": "a", "history": hourly([1.0] * 5)}]}, headers=AUTH).status_code == 422


def test_oversized_body_is_413(client):
    res = client.post("/v1/forecast", content=b"{" + b" " * (33 * 1024 * 1024) + b"}", headers={**AUTH, "content-type": "application/json"})
    assert res.status_code == 413


def test_models_and_train_over_http(client):
    items = client.get("/v1/models", headers=AUTH).json()["items"]
    assert {"seasonal_naive", "random_forest", "lstm", "chronos"} <= {m["id"] for m in items}
    out = client.post("/v1/train", json={"model": "random_forest", "granularity": "hour", "series": [{"series_id": "a", "history": hourly(weekly(3))}]}, headers=AUTH).json()
    assert out["model_version"].startswith("pooled-")


def anomaly_body(actual, weeks=10, ts=None):
    values = weekly(weeks)
    return {"series_id": "a", "ts": (ts or T0 + len(values) * H).isoformat(), "actual": actual, "history": hourly(values)}


def test_anomaly_flags_a_spike_and_accepts_a_normal_value(client):
    normal = client.post("/v1/anomaly", json=anomaly_body(2.0), headers=AUTH).json()
    spike = client.post("/v1/anomaly", json=anomaly_body(80.0), headers=AUTH).json()
    assert normal["is_anomaly"] is False and spike["is_anomaly"] is True
    assert spike["method"] == "robust_zscore_same_hour_of_week" and spike["score"] > normal["score"]
    assert spike["lower"] <= spike["expected"] <= spike["upper"]
    assert spike["model_id"] and spike["model_version"]


def test_anomaly_needs_enough_same_hour_history(client):
    out = client.post("/v1/anomaly", json=anomaly_body(80.0, weeks=3), headers=AUTH).json()
    assert out == {**out, "is_anomaly": False, "method": "insufficient_history", "score": None}


def test_openapi_documents_the_v1_contract(client):
    paths = client.get("/openapi.json").json()["paths"]
    assert {"/v1/forecast", "/v1/anomaly", "/v1/models", "/v1/train", "/health"} <= set(paths)


def test_anomaly_boundary_is_3_5_robust_sigmas():
    # Same hour-of-week values 10±: median 10, MAD 1 → scale 1.4826; 15.5 scores 3.7097, 15.0 scores 3.3725.
    same = [10, 11, 9, 10, 12, 8, 10, 11, 9, 10]
    history = [{"ts": (T0 + w * 168 * H).isoformat(), "value": v} for w, v in enumerate(same)]
    ts = (T0 + 10 * 168 * H).isoformat()
    from ekokod_ml.anomaly import check
    from ekokod_ml.schemas import AnomalyRequest
    hot = check(AnomalyRequest.model_validate({"series_id": "a", "ts": ts, "actual": 15.5, "history": history}))
    warm = check(AnomalyRequest.model_validate({"series_id": "a", "ts": ts, "actual": 15.0, "history": history}))
    assert hot.is_anomaly and not warm.is_anomaly
    assert (hot.expected, hot.lower, hot.upper, hot.score) == (10.0, 4.8109, 15.1891, 3.7097)
