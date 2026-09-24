"""Random forest over engineered features (R363), forecast recursively; quantiles from the per-tree spread."""

import numpy as np
from sklearn.ensemble import RandomForestRegressor

from ..features import LAGS, Covariate, lag_features, timeline
from ..schemas import ForecastRequest, Point
from .base import Prediction


def design(y: np.ndarray, ts, cov: Covariate, lags) -> tuple[np.ndarray, np.ndarray]:
    rows, target = [], []
    for i in range(lags[0], len(y)):
        if np.isnan(y[i]):
            continue
        rows.append(cov.row(ts[i]) + lag_features(y, i, lags))
        target.append(y[i])
    return np.array(rows, dtype=float), np.array(target, dtype=float)


class RandomForest:
    id = "random_forest"
    version = "1.0.0"

    def __init__(self, trees: int = 60):
        self.trees = trees

    def available(self) -> bool:
        return True

    def fit(self, X: np.ndarray, target: np.ndarray) -> RandomForestRegressor:
        model = RandomForestRegressor(n_estimators=self.trees, min_samples_leaf=2, random_state=0, n_jobs=1)
        return model.fit(X, target)

    def forecast(self, req: ForecastRequest, points: list[Point], model: RandomForestRegressor | None = None) -> Prediction:
        ts, y = timeline(points, req.step)
        cov = Covariate(req.covariates)
        lags = LAGS[req.granularity]
        if model is None:
            X, target = design(y, ts, cov, lags)
            if len(target) < 24:
                raise ValueError("too few complete rows to fit")
            model = self.fit(X, target)
        ext = list(y)
        t = ts[-1]
        median, p10, p90 = [], [], []
        for _ in range(req.horizon):
            t = t + req.step
            x = np.array([cov.row(t) + lag_features(np.array(ext), len(ext), lags)], dtype=float)
            per_tree = np.array([tree.predict(x)[0] for tree in model.estimators_])
            m = float(np.median(per_tree))
            median.append(m)
            p10.append(float(np.quantile(per_tree, 0.1)))
            p90.append(float(np.quantile(per_tree, 0.9)))
            ext.append(m)
        return Prediction(median=median, p10=p10, p90=p90, used_covariates=cov.used)
