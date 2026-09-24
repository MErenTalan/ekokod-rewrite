"""The 03 §6.3 HTTP surface: /v1 behind a shared secret (Q-I3), /health open."""

import hmac

from fastapi import Depends, FastAPI, HTTPException, Request
from fastapi.responses import JSONResponse
from fastapi.security import HTTPAuthorizationCredentials, HTTPBearer

from . import anomaly
from .registry import Registry
from .schemas import AnomalyRequest, AnomalyResponse, ForecastRequest, ForecastResponse, TrainRequest
from .settings import Settings

MAX_BODY = 32 * 1024 * 1024  # two years of hourly points is ~1 MB; this only stops abuse


def create_app(settings: Settings | None = None) -> FastAPI:
    settings = settings or Settings.from_env()
    registry = Registry(settings)
    bearer = HTTPBearer(auto_error=False)

    def authorised(creds: HTTPAuthorizationCredentials | None = Depends(bearer)) -> None:
        if creds is None or not hmac.compare_digest(creds.credentials.encode(), settings.api_key.encode()):
            raise HTTPException(status_code=401, detail="unauthorised", headers={"WWW-Authenticate": "Bearer"})

    app = FastAPI(title="ekokod ML", version="1.0.0", docs_url=None, redoc_url=None)

    @app.middleware("http")
    async def body_limit(request: Request, call_next):
        length = request.headers.get("content-length")
        if length and length.isdigit() and int(length) > MAX_BODY:
            return JSONResponse({"detail": "request body too large"}, status_code=413)
        return await call_next(request)

    @app.get("/health")
    def health() -> dict:
        return {"status": "ok"}

    @app.post("/v1/forecast", response_model=ForecastResponse, dependencies=[Depends(authorised)])
    def forecast(req: ForecastRequest) -> ForecastResponse:
        try:
            return registry.forecast(req)
        except KeyError as err:
            raise HTTPException(status_code=422, detail=f"unknown model {err}") from err

    @app.post("/v1/anomaly", response_model=AnomalyResponse, dependencies=[Depends(authorised)])
    def check(req: AnomalyRequest) -> AnomalyResponse:
        return anomaly.check(req)

    @app.get("/v1/models", dependencies=[Depends(authorised)])
    def models() -> dict:
        return {"items": registry.catalogue()}

    @app.post("/v1/train", dependencies=[Depends(authorised)])
    def train(req: TrainRequest) -> dict:
        try:
            return registry.train(req)
        except ValueError as err:
            raise HTTPException(status_code=422, detail=str(err)) from err

    return app
