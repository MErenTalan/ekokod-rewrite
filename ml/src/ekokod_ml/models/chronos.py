"""External foundation-model forecaster over the legacy sidecar contract (Q-I2):
POST {url}/predict {series_id, prediction_length, data: [{timestamp, value}]}, header X-API-Key
→ {timestamps, median, high_90, low_10}."""

import httpx

from ..schemas import ForecastRequest, Point
from .base import Prediction


class Chronos:
    id = "chronos"
    version = "sidecar-1"

    def __init__(self, url: str, api_key: str, timeout: float = 30.0, transport: httpx.BaseTransport | None = None):
        self.url, self.api_key, self.timeout, self.transport = url.rstrip("/"), api_key, timeout, transport

    def available(self) -> bool:
        return bool(self.url)

    def forecast(self, req: ForecastRequest, points: list[Point]) -> Prediction:
        body = {
            "series_id": req.series_id,
            "prediction_length": req.horizon,
            "data": [{"timestamp": p.ts.isoformat(), "value": p.value} for p in points if p.value is not None],
        }
        with httpx.Client(timeout=self.timeout, transport=self.transport) as client:
            res = client.post(f"{self.url}/predict", json=body, headers={"X-API-Key": self.api_key})
            res.raise_for_status()
            out = res.json()
        median, p10, p90 = out["median"], out["low_10"], out["high_90"]
        if not (len(median) == len(p10) == len(p90) == req.horizon):
            raise ValueError("chronos returned a different horizon")
        return Prediction(median=median, p10=p10, p90=p90)
