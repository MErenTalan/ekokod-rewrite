"""Q-I7 / R368: robust z-score against the same hour-of-week (Istanbul time)."""

import numpy as np

from .features import TZ
from .schemas import AnomalyRequest, AnomalyResponse

METHOD = "robust_zscore_same_hour_of_week"
VERSION = "1.0.0"
THRESHOLD = 3.5
MIN_POINTS = 8


def check(req: AnomalyRequest) -> AnomalyResponse:
    target = req.ts.astimezone(TZ)
    key = (target.weekday(), target.hour)
    same = [p.value for p in req.history
            if p.value is not None and p.ts < req.ts and (lambda t: (t.weekday(), t.hour))(p.ts.astimezone(TZ)) == key]
    if len(same) < MIN_POINTS:
        return AnomalyResponse(is_anomaly=False, score=None, expected=None, lower=None, upper=None,
                               method="insufficient_history", model_id=METHOD, model_version=VERSION)
    values = np.array(same, dtype=float)
    expected = float(np.median(values))
    scale = 1.4826 * float(np.median(np.abs(values - expected)))
    if scale == 0:  # a perfectly flat history: 5 % of the level, so tiny noise is not an anomaly
        scale = max(0.05 * abs(expected), 1e-6)
    score = abs(req.actual - expected) / scale
    return AnomalyResponse(
        is_anomaly=score > THRESHOLD,
        score=round(score, 4),
        expected=round(expected, 4),
        lower=round(max(0.0, expected - THRESHOLD * scale), 4),
        upper=round(expected + THRESHOLD * scale, 4),
        method=METHOD,
        model_id=METHOD,
        model_version=VERSION,
    )
