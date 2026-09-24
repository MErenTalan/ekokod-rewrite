"""R369: configuration from the environment."""

import os
from dataclasses import dataclass, field


@dataclass(frozen=True)
class Settings:
    api_key: str
    model_dir: str = "/var/lib/ekokod-ml"
    default_model: str = "random_forest"
    disabled_models: frozenset[str] = field(default_factory=frozenset)
    min_history_hourly: int = 336
    min_history_daily: int = 28
    chronos_url: str = ""
    chronos_api_key: str = ""
    chronos_timeout: float = 30.0

    @classmethod
    def from_env(cls) -> "Settings":
        key = os.environ.get("EKOKOD_ML_API_KEY", "")
        if len(key) < 16:
            raise RuntimeError("EKOKOD_ML_API_KEY must be set (at least 16 characters)")
        disabled = frozenset(m.strip() for m in os.environ.get("ML_DISABLED_MODELS", "").split(",") if m.strip())
        return cls(
            api_key=key,
            model_dir=os.environ.get("ML_MODEL_DIR", cls.model_dir),
            default_model=os.environ.get("ML_DEFAULT_MODEL", cls.default_model),
            disabled_models=disabled,
            min_history_hourly=int(os.environ.get("ML_MIN_HISTORY_HOURS", cls.min_history_hourly)),
            chronos_url=os.environ.get("ML_CHRONOS_URL", ""),
            chronos_api_key=os.environ.get("ML_CHRONOS_API_KEY", ""),
        )
