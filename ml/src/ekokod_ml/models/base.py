"""One interface for every forecaster (03 §6.4)."""

from dataclasses import dataclass, field
from typing import Protocol

from ..schemas import ForecastRequest, Point


@dataclass
class Prediction:
    median: list[float]
    p10: list[float]
    p90: list[float]
    used_covariates: list[str] = field(default_factory=list)


class Forecaster(Protocol):
    id: str
    version: str

    def available(self) -> bool: ...

    def forecast(self, req: ForecastRequest, points: list[Point]) -> Prediction: ...
