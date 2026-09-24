"""Model choice, statuses (R360) and the baseline fallback (Q-I6)."""

import logging

import numpy as np

from .gaps import detect_gaps
from .models.base import Forecaster, Prediction
from .models.baseline import SeasonalNaive
from .models.forest import RandomForest
from .schemas import ForecastRequest, ForecastResponse
from .settings import Settings

log = logging.getLogger(__name__)
BASELINE = "seasonal_naive"


class Registry:
    def __init__(self, settings: Settings, extra: dict[str, Forecaster] | None = None):
        self.settings = settings
        self.models: dict[str, Forecaster] = {BASELINE: SeasonalNaive(), "random_forest": RandomForest(), **(extra or {})}

    def usable(self, model_id: str) -> bool:
        m = self.models.get(model_id)
        return m is not None and model_id not in self.settings.disabled_models and m.available()

    def forecast(self, req: ForecastRequest) -> ForecastResponse:
        wanted = req.model or self.settings.default_model
        if wanted not in self.models:
            raise KeyError(wanted)
        points = req.points()
        gaps = detect_gaps(points, req.step)
        chosen = wanted if self.usable(wanted) else BASELINE
        model = self.models[chosen]
        base = {"model_id": model.id, "model_version": model.version, "gaps": gaps,
                "fallback_from": wanted if chosen != wanted else None}
        observed = sum(p.value is not None for p in points)
        if observed == 0:
            return ForecastResponse(status="no_data", **base)
        minimum = self.settings.min_history_hourly if req.granularity == "hour" else self.settings.min_history_daily
        if observed < minimum:
            return ForecastResponse(status="insufficient_data", message=f"{observed} of {minimum} required points", **base)
        try:
            pred = model.forecast(req, points)
        except Exception:  # noqa: BLE001 — any model failure degrades to the baseline (Q-I6)
            log.exception("model %s failed", chosen)
            if chosen == BASELINE:
                return ForecastResponse(status="model_error", **base)
            model = self.models[BASELINE]
            base.update(model_id=model.id, model_version=model.version, fallback_from=wanted)
            try:
                pred = model.forecast(req, points)
            except Exception:  # noqa: BLE001
                log.exception("baseline failed")
                return ForecastResponse(status="model_error", **base)
        return self._response(req, points, pred, base)

    @staticmethod
    def _response(req: ForecastRequest, points, pred: Prediction, base: dict) -> ForecastResponse:
        last = points[-1].ts
        med = np.maximum(np.nan_to_num(np.array(pred.median, dtype=float)), 0.0)
        lo = np.minimum(np.maximum(np.nan_to_num(np.array(pred.p10, dtype=float)), 0.0), med)
        hi = np.maximum(np.nan_to_num(np.array(pred.p90, dtype=float)), med)
        return ForecastResponse(
            status="ok",
            timestamps=[last + (k + 1) * req.step for k in range(req.horizon)],
            median=[round(float(v), 4) for v in med],
            p10=[round(float(v), 4) for v in lo],
            p90=[round(float(v), 4) for v in hi],
            used_covariates=pred.used_covariates,
            **base,
        )
