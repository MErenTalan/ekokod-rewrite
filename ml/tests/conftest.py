from datetime import UTC, datetime, timedelta

import pytest

T0 = datetime(2026, 1, 5, tzinfo=UTC)  # a Monday


def hourly(values, start=T0):
    return [{"ts": (start + timedelta(hours=i)).isoformat(), "value": v} for i, v in enumerate(values)]


@pytest.fixture
def t0():
    return T0
