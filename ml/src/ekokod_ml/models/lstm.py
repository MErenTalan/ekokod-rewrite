"""LSTM forecaster (03 §6.4). Needs the optional `lstm` extra (torch); without it the model is unavailable (Q-I1)."""

import importlib.util

import numpy as np

from ..features import SEASON, timeline
from ..schemas import ForecastRequest, Point
from .base import Prediction


class Lstm:
    id = "lstm"
    version = "1.0.0"

    def __init__(self, epochs: int = 30, hidden: int = 32):
        self.epochs, self.hidden = epochs, hidden

    def available(self) -> bool:
        return importlib.util.find_spec("torch") is not None

    def forecast(self, req: ForecastRequest, points: list[Point]) -> Prediction:
        import torch  # noqa: PLC0415 — optional dependency

        torch.manual_seed(0)
        _, y = timeline(points, req.step)
        y = np.where(np.isnan(y), np.nanmedian(y), y)
        scale = float(np.max(np.abs(y))) or 1.0
        s = y / scale
        window = SEASON[req.granularity]
        if len(s) <= window + 8:
            raise ValueError("history shorter than one season plus a margin")
        xs = np.stack([s[i:i + window] for i in range(len(s) - window)])
        ts = s[window:]

        class Net(torch.nn.Module):
            def __init__(self, hidden: int):
                super().__init__()
                self.lstm = torch.nn.LSTM(1, hidden, batch_first=True)
                self.head = torch.nn.Linear(hidden, 1)

            def forward(self, x):
                out, _ = self.lstm(x)
                return self.head(out[:, -1, :]).squeeze(-1)

        net = Net(self.hidden)
        opt = torch.optim.Adam(net.parameters(), lr=0.01)
        X = torch.tensor(xs, dtype=torch.float32).unsqueeze(-1)
        T = torch.tensor(ts, dtype=torch.float32)
        for _ in range(self.epochs):
            opt.zero_grad()
            loss = torch.nn.functional.mse_loss(net(X), T)
            loss.backward()
            opt.step()
        with torch.no_grad():
            resid = (net(X) - T).numpy()
            buf = list(s[-window:])
            median = []
            for _ in range(req.horizon):
                v = float(net(torch.tensor([buf[-window:]], dtype=torch.float32).unsqueeze(-1))[0])
                median.append(v)
                buf.append(v)
        lo, hi = float(np.quantile(resid, 0.1)), float(np.quantile(resid, 0.9))
        med = [m * scale for m in median]
        return Prediction(median=med, p10=[m + lo * scale for m in med], p90=[m + hi * scale for m in med])
