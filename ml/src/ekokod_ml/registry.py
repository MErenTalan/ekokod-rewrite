"""Model choice, statuses (R360) and the baseline fallback (Q-I6)."""

import logging
from datetime import UTC, datetime
from pathlib import Path

import joblib
import numpy as np

from .gaps import detect_gaps
from .models.base import Forecaster, Prediction
from .features import LAGS, Covariate, timeline
from .models.baseline import SeasonalNaive
from .models.chronos import Chronos
from .models.forest import RandomForest, design
from .models.lstm import Lstm
from .schemas import STEP, ForecastRequest, ForecastResponse, TrainRequest
from .settings import Settings

log = logging.getLogger(__name__)
BASELINE = "seasonal_naive"


class Registry:
    def __init__(self, settings: Settings, extra: dict[str, Forecaster] | None = None):
        self.settings = settings
        self.forest = RandomForest()
        self.models: dict[str, Forecaster] = {
            BASELINE: SeasonalNaive(),
            "random_forest": self.forest,
            "lstm": Lstm(),
            "chronos": Chronos(settings.chronos_url, settings.chronos_api_key, settings.chronos_timeout),
            **(extra or {}),
        }
        self._load_artifacts()

    def _load_artifacts(self) -> None:
        # Only files this service wrote itself (train) are ever loaded.
        folder = Path(self.settings.model_dir)
        for granularity in ("hour", "day"):
            files = sorted(folder.glob(f"rf-{granularity}-*.joblib")) if folder.is_dir() else []
            if files:
                self.forest.pooled[granularity] = joblib.load(files[-1])

    def catalogue(self) -> list[dict]:
        out = []
        for model_id, m in self.models.items():
            pooled = self.forest.pooled.get("hour") if model_id == "random_forest" else None
            out.append({
                "id": model_id,
                "version": pooled["version"] if pooled else m.version,
                "available": self.usable(model_id),
                "trained_at": pooled["trained_at"] if pooled else None,
                "default": model_id == self.settings.default_model,
            })
        return out

    def train(self, req: TrainRequest) -> dict:
        lags = LAGS[req.granularity]
        blocks, targets, covsets = [], [], set()
        for series, points in zip(req.series, req.points(), strict=True):
            cov = Covariate(series.covariates)
            covsets.add(tuple(cov.used))
            ts, y = timeline(points, STEP[req.granularity])
            X, t = design(y, ts, cov, lags)
            if len(t):
                blocks.append(X)
                targets.append(t)
        if len(covsets) > 1:
            raise ValueError("every series must supply the same covariates")
        if not targets:
            raise ValueError("no usable rows in the dataset")
        X, t = np.vstack(blocks), np.concatenate(targets)
        now = datetime.now(UTC)
        version = now.strftime("pooled-%Y%m%d%H%M%S")
        art = {"model": self.forest.fit(X, t), "version": version, "trained_at": now.isoformat(), "covariates": list(covsets.pop())}
        folder = Path(self.settings.model_dir)
        folder.mkdir(parents=True, exist_ok=True)
        joblib.dump(art, folder / f"rf-{req.granularity}-{version}.joblib")
        self.forest.pooled[req.granularity] = art
        return {"model_id": "random_forest", "model_version": version, "trained_at": art["trained_at"], "samples": int(len(t))}

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
            **{**base, "model_version": pred.version or base["model_version"]},
        )
