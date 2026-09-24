"""The 03 §6.3 contract. Timestamps are timezone-aware; history is one point per step."""

from datetime import date, datetime, timedelta
from typing import Literal, Self

from pydantic import AwareDatetime, BaseModel, Field, model_validator

Granularity = Literal["hour", "day"]
Status = Literal["ok", "insufficient_data", "no_data", "model_error"]
DayType = Literal["workday", "weekend", "holiday", "half_day"]

MAX_HISTORY = 17_544  # two years hourly (R367)
MAX_HORIZON = {"hour": 744, "day": 62}
STEP = {"hour": timedelta(hours=1), "day": timedelta(days=1)}


class Point(BaseModel):
    ts: AwareDatetime
    value: float | None


class WeatherPoint(BaseModel):
    ts: AwareDatetime
    temperature: float | None = None


class DayTypePoint(BaseModel):
    date: date
    type: DayType


class VacationPoint(BaseModel):
    date: date
    vacation: bool = True


class Covariates(BaseModel):
    weather: list[WeatherPoint] = Field(default_factory=list, max_length=MAX_HISTORY + 744)
    day_type: list[DayTypePoint] = Field(default_factory=list, max_length=800)
    vacation: list[VacationPoint] = Field(default_factory=list, max_length=800)


def _check_series(history: list[Point], step: timedelta) -> list[Point]:
    pts = sorted(history, key=lambda p: p.ts)
    for a, b in zip(pts, pts[1:], strict=False):
        if a.ts == b.ts:
            raise ValueError(f"duplicate timestamp {a.ts.isoformat()}")
        if (b.ts - a.ts) % step:
            raise ValueError(f"timestamp {b.ts.isoformat()} is off the {step} step")
    return pts


class ForecastRequest(BaseModel):
    series_id: str = Field(min_length=1, max_length=200)
    granularity: Granularity
    horizon: int = Field(ge=1)
    history: list[Point] = Field(max_length=MAX_HISTORY)
    covariates: Covariates = Field(default_factory=Covariates)
    model: str | None = Field(default=None, max_length=64)

    @model_validator(mode="after")
    def _limits(self) -> Self:
        if self.horizon > MAX_HORIZON[self.granularity]:
            raise ValueError(f"horizon must be at most {MAX_HORIZON[self.granularity]} for {self.granularity}")
        self._points = _check_series(self.history, STEP[self.granularity])
        return self

    @property
    def step(self) -> timedelta:
        return STEP[self.granularity]

    def points(self) -> list[Point]:
        """History sorted by time (validated: unique, on-step)."""
        return self._points


class Gap(BaseModel):
    start: datetime
    end: datetime
    missing_hours: int


class ForecastResponse(BaseModel):
    status: Status
    model_id: str
    model_version: str
    timestamps: list[datetime] = Field(default_factory=list)
    median: list[float] = Field(default_factory=list)
    p10: list[float] = Field(default_factory=list)
    p90: list[float] = Field(default_factory=list)
    used_covariates: list[str] = Field(default_factory=list)
    gaps: list[Gap] = Field(default_factory=list)
    fallback_from: str | None = None
    message: str | None = None


class AnomalyRequest(BaseModel):
    series_id: str = Field(min_length=1, max_length=200)
    ts: AwareDatetime
    actual: float
    history: list[Point] = Field(max_length=MAX_HISTORY)


class AnomalyResponse(BaseModel):
    is_anomaly: bool
    score: float | None
    expected: float | None
    lower: float | None
    upper: float | None
    method: str
    model_id: str
    model_version: str


class TrainSeries(BaseModel):
    series_id: str = Field(min_length=1, max_length=200)
    history: list[Point] = Field(max_length=MAX_HISTORY)
    covariates: Covariates = Field(default_factory=Covariates)


class TrainRequest(BaseModel):
    """R365: a pooled random forest over many series."""

    model: Literal["random_forest"]
    granularity: Granularity
    series: list[TrainSeries] = Field(min_length=1, max_length=200)

    @model_validator(mode="after")
    def _series(self) -> Self:
        self._points = [_check_series(s.history, STEP[self.granularity]) for s in self.series]
        return self

    def points(self) -> list[list[Point]]:
        return self._points
