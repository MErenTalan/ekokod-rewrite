"""Seasonal-naive baseline: the median of the same position in the last four seasons. Always available."""

import numpy as np

from ..features import SEASON, timeline
from ..schemas import ForecastRequest, Point
from .base import Prediction

K = 4


def _estimate(y: np.ndarray, i: int, season: int) -> float:
    past = [y[j] for j in (i - m * season for m in range(1, K + 1)) if 0 <= j < len(y) and not np.isnan(y[j])]
    return float(np.median(past)) if past else np.nan


class SeasonalNaive:
    id = "seasonal_naive"
    version = "1.0.0"

    def available(self) -> bool:
        return True

    def forecast(self, req: ForecastRequest, points: list[Point]) -> Prediction:
        _, y = timeline(points, req.step)
        season = SEASON[req.granularity]
        n = len(y)
        resid = [y[i] - e for i in range(season, n) if not np.isnan(y[i]) and not np.isnan(e := _estimate(y, i, season))]
        level = float(np.nanmedian(y))
        ext = np.concatenate([y, np.full(req.horizon, np.nan)])
        median = []
        for k in range(req.horizon):
            e = _estimate(ext[:n], n + k, season)
            median.append(level if np.isnan(e) else e)
        if len(resid) >= 10:
            lo, hi = float(np.quantile(resid, 0.1)), float(np.quantile(resid, 0.9))
        else:
            lo, hi = -0.1 * abs(level), 0.1 * abs(level)
        return Prediction(median=median, p10=[m + lo for m in median], p90=[m + hi for m in median])
