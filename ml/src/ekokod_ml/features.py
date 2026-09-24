"""R363: calendar, day type, vacation, weather, lags and rolling means on the history's timeline."""

import warnings
from datetime import datetime, timedelta
from zoneinfo import ZoneInfo

import numpy as np

from .schemas import Covariates, Point

TZ = ZoneInfo("Europe/Istanbul")
DAY_TYPES = {"workday": 0, "weekend": 1, "holiday": 2, "half_day": 3}
LAGS = {"hour": (24, 168), "day": (1, 7)}
SEASON = {"hour": 168, "day": 7}


def timeline(points: list[Point], step: timedelta) -> tuple[list[datetime], np.ndarray]:
    """Every step from the first to the last point; NaN where absent or null."""
    if not points:
        return [], np.array([])
    by_ts = {p.ts: p.value for p in points}
    ts, t = [], points[0].ts
    while t <= points[-1].ts:
        ts.append(t)
        t += step
    return ts, np.array([np.nan if by_ts.get(x) is None else by_ts[x] for x in ts], dtype=float)


class Covariate:
    """Covariate lookups plus the names of those actually supplied."""

    def __init__(self, cov: Covariates):
        self.day_type = {d.date: DAY_TYPES[d.type] for d in cov.day_type}
        self.vacation = {v.date: 1.0 if v.vacation else 0.0 for v in cov.vacation}
        self.temp = {w.ts: w.temperature for w in cov.weather if w.temperature is not None}
        self.used = [n for n, present in (("weather", self.temp), ("day_type", self.day_type), ("vacation", self.vacation)) if present]

    def row(self, t: datetime) -> list[float]:
        local = t.astimezone(TZ)
        d = local.date()
        weekend = 1.0 if local.weekday() >= 5 else 0.0
        out = [local.hour, local.weekday(), local.month, weekend]
        if self.day_type:
            out.append(self.day_type.get(d, weekend))
        if self.vacation:
            out.append(self.vacation.get(d, 0.0))
        if self.temp:
            out.append(self.temp.get(t, np.nan))
        return out


def lag_features(y: np.ndarray, i: int, lags: tuple[int, int]) -> list[float]:
    a, b = lags
    with warnings.catch_warnings():
        warnings.simplefilter("ignore", RuntimeWarning)
        return [
            y[i - a] if i >= a else np.nan,
            y[i - b] if i >= b else np.nan,
            float(np.nanmean(y[max(0, i - a):i])) if i > 0 else np.nan,
            float(np.nanmean(y[max(0, i - b):i])) if i > 0 else np.nan,
        ]
