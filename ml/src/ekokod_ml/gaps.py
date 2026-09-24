"""R361: runs of absent or null steps between the first and last history point."""

from datetime import timedelta

from .schemas import Gap, Point


def detect_gaps(points: list[Point], step: timedelta) -> list[Gap]:
    present = {p.ts for p in points if p.value is not None}
    if not points:
        return []
    gaps: list[Gap] = []
    run_start = None
    t, last = points[0].ts, points[-1].ts
    hours = step / timedelta(hours=1)
    while t <= last + step:
        missing = t <= last and t not in present
        if missing and run_start is None:
            run_start = t
        if not missing and run_start is not None:
            end = t - step
            gaps.append(Gap(start=run_start, end=end, missing_hours=int(((end - run_start) / step + 1) * hours)))
            run_start = None
        t += step
    return gaps
