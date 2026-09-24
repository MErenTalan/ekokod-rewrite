from datetime import timedelta

import pytest
from pydantic import ValidationError

from conftest import T0, hourly
from ekokod_ml.gaps import detect_gaps
from ekokod_ml.schemas import ForecastRequest

H = timedelta(hours=1)


def req(history, **kw):
    return ForecastRequest.model_validate({"series_id": "a", "granularity": "hour", "horizon": 24, "history": history, **kw})


def test_absent_and_null_steps_form_gaps_with_their_hours():
    history = [p for i, p in enumerate(hourly([1.0] * 10)) if i not in (3, 4, 5)]  # 03:00–05:00 absent
    history[4]["value"] = None  # 07:00 present but null
    gaps = detect_gaps(req(history).points(), H)
    assert [(g.start, g.end, g.missing_hours) for g in gaps] == [(T0 + 3 * H, T0 + 5 * H, 3), (T0 + 7 * H, T0 + 7 * H, 1)]


def test_null_run_is_reported_exactly():
    history = hourly([1.0, None, None, 1.0])
    gaps = detect_gaps(req(history).points(), H)
    assert [(g.start, g.end, g.missing_hours) for g in gaps] == [(T0 + H, T0 + 2 * H, 2)]


def test_daily_steps_count_24_hours():
    days = [{"ts": (T0 + timedelta(days=d)).isoformat(), "value": 5.0} for d in (0, 1, 4)]
    gaps = detect_gaps(req(days, granularity="day").points(), timedelta(days=1))
    assert [(g.start, g.end, g.missing_hours) for g in gaps] == [(T0 + timedelta(days=2), T0 + timedelta(days=3), 48)]


def test_no_gaps_in_a_complete_series():
    assert detect_gaps(req(hourly([1.0] * 48)).points(), H) == []


def test_request_refuses_naive_timestamps_duplicates_and_limits():
    with pytest.raises(ValidationError):
        req([{"ts": "2026-01-05T00:00:00", "value": 1.0}])
    with pytest.raises(ValidationError):
        req(hourly([1.0]) + hourly([2.0]))
    with pytest.raises(ValidationError):
        req(hourly([1.0]), horizon=745)
    with pytest.raises(ValidationError):
        req(hourly([1.0]), horizon=63, granularity="day")
    with pytest.raises(ValidationError):
        req(hourly([1.0]), horizon=0)


def test_points_are_sorted_and_off_step_timestamps_refused():
    shuffled = list(reversed(hourly([1.0, 2.0, 3.0])))
    assert [p.value for p in req(shuffled).points()] == [1.0, 2.0, 3.0]
    off = hourly([1.0, 2.0])
    off[1]["ts"] = (T0 + timedelta(minutes=90)).isoformat()
    with pytest.raises(ValidationError):
        req(off)
